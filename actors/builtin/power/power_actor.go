package power

import (
	"bytes"
	"fmt"
	"math"

	addr "github.com/filecoin-project/go-address"
	peer "github.com/libp2p/go-libp2p-core/peer"
	errors "github.com/pkg/errors"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	initact "github.com/filecoin-project/specs-actors/actors/builtin/init"
	crypto "github.com/filecoin-project/specs-actors/actors/crypto"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	. "github.com/filecoin-project/specs-actors/actors/util"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type Runtime = vmr.Runtime

type ConsensusFaultType int64

const (
	//ConsensusFaultUncommittedPower ConsensusFaultType = 0
	ConsensusFaultDoubleForkMining ConsensusFaultType = 1
	ConsensusFaultParentGrinding   ConsensusFaultType = 2
	ConsensusFaultTimeOffsetMining ConsensusFaultType = 3
)

type SectorTermination int64

const (
	SectorTerminationExpired SectorTermination = iota // Implicit termination after all deals expire
	SectorTerminationManual                           // Unscheduled explicit termination by the miner
)

type Actor struct{}

func (a Actor) Exports() []interface{} {
	return []interface{}{
		builtin.MethodConstructor: a.Constructor,
		2:                         a.AddBalance,
		3:                         a.WithdrawBalance,
		4:                         a.CreateMiner,
		5:                         a.DeleteMiner,
		6:                         a.OnSectorProveCommit,
		7:                         a.OnSectorTerminate,
		8:                         a.OnSectorTemporaryFaultEffectiveBegin,
		9:                         a.OnSectorTemporaryFaultEffectiveEnd,
		10:                        a.OnSectorModifyWeightDesc,
		11:                        a.OnMinerSurprisePoStSuccess,
		12:                        a.OnMinerSurprisePoStFailure,
		13:                        a.EnrollCronEvent,
		14:                        a.ReportConsensusFault,
		15:                        a.OnEpochTickEnd,
	}
}

var _ abi.Invokee = Actor{}

// Storage miner actor constructor params are defined here so the power actor can send them to the init actor
// to instantiate miners.
type MinerConstructorParams struct {
	OwnerAddr  addr.Address
	WorkerAddr addr.Address
	SectorSize abi.SectorSize
	PeerId     peer.ID
}

type SectorStorageWeightDesc struct {
	SectorSize abi.SectorSize
	Duration   abi.ChainEpoch
	DealWeight abi.DealWeight
}

////////////////////////////////////////////////////////////////////////////////
// Actor methods
////////////////////////////////////////////////////////////////////////////////

func (a Actor) Constructor(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)

	rt.State().Construct(func() vmr.CBORMarshaler {
		st, err := ConstructState(adt.AsStore(rt))
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to create storage power state: %v", err)
		}
		return st
	})
	return &adt.EmptyValue{}
}

type AddBalanceParams struct {
	Miner addr.Address
}

func (a Actor) AddBalance(rt Runtime, params *AddBalanceParams) *adt.EmptyValue {
	validatePledgeAccount(rt, params.Miner)

	ownerAddr, workerAddr := builtin.RequestMinerControlAddrs(rt, params.Miner)
	rt.ValidateImmediateCallerIs(ownerAddr, workerAddr)

	var err error
	var st State
	rt.State().Transaction(&st, func() interface{} {
		err = st.addMinerBalance(adt.AsStore(rt), params.Miner, rt.ValueReceived())
		abortIfError(rt, err, "failed to add pledge balance")
		return nil
	})
	return &adt.EmptyValue{}
}

type WithdrawBalanceParams struct {
	Miner     addr.Address
	Requested abi.TokenAmount
}

func (a Actor) WithdrawBalance(rt Runtime, params *WithdrawBalanceParams) *adt.EmptyValue {
	validatePledgeAccount(rt, params.Miner)
	ownerAddr, workerAddr := builtin.RequestMinerControlAddrs(rt, params.Miner)
	rt.ValidateImmediateCallerIs(ownerAddr, workerAddr)

	if params.Requested.LessThan(big.Zero()) {
		rt.Abort(exitcode.ErrIllegalArgument, "negative withdrawal %v", params.Requested)
	}

	var amountExtracted abi.TokenAmount
	var st State
	rt.State().Transaction(&st, func() interface{} {
		claim, found, err := st.getClaim(adt.AsStore(rt), params.Miner)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to load claim for miner %v", params.Miner)
			panic("can't get here") // Convince Go that claim will not be used while nil below
		}
		if !found {
			// This requirement prevents a terminated miner from withdrawing any posted collateral in excess of
			// their previous requirements. This is consistent with the slashing routine burning it all.
			// Alternatively, we could interpret a missing claim here as evidence of termination and allow
			// withdrawal of any residual balance.
			rt.Abort(exitcode.ErrIllegalArgument, "no claim for miner %v", params.Miner)
		}

		// Pledge for sectors in temporary fault has already been subtracted from the claim.
		// If the miner has failed a scheduled PoSt, collateral remains locked for further penalization.
		// Thus the current claimed pledge is the amount to keep locked.
		subtracted, err := st.subtractMinerBalance(adt.AsStore(rt), params.Miner, params.Requested, claim.Pledge)
		abortIfError(rt, err, "failed to subtract pledge balance")
		amountExtracted = subtracted
		return nil
	})

	// Balance is always withdrawn to the miner owner account.
	_, code := rt.Send(ownerAddr, builtin.MethodSend, nil, amountExtracted)
	builtin.RequireSuccess(rt, code, "failed to send funds")
	return &adt.EmptyValue{}
}

type CreateMinerParams struct {
	Worker     addr.Address
	SectorSize abi.SectorSize
	Peer       peer.ID
}

type CreateMinerReturn struct {
	IDAddress     addr.Address // The canonical ID-based address for the actor.
	RobustAddress addr.Address // A mre expensive but re-org-safe address for the newly created actor.
}

func (a Actor) CreateMiner(rt Runtime, params *CreateMinerParams) *CreateMinerReturn {
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	ownerAddr := rt.ImmediateCaller()

	ctorParams := MinerConstructorParams{
		OwnerAddr:  ownerAddr,
		WorkerAddr: params.Worker,
		SectorSize: params.SectorSize,
		PeerId:     params.Peer,
	}
	var ctorParamBytes []byte
	err := ctorParams.MarshalCBOR(bytes.NewBuffer(ctorParamBytes))
	if err != nil {
		rt.Abort(exitcode.ErrPlaceholder, "failed to serialize miner constructor params %v: %v", ctorParams, err)
	}
	ret, code := rt.Send(
		builtin.InitActorAddr,
		builtin.MethodsInit.Exec,
		&initact.ExecParams{
			CodeCID:           builtin.StorageMinerActorCodeID,
			ConstructorParams: ctorParamBytes,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to init new actor")
	var addresses initact.ExecReturn
	err = ret.Into(&addresses)
	if err != nil {
		rt.Abort(exitcode.ErrIllegalState, "unmarshaling exec return value: %v", err)
	}

	var st State
	rt.State().Transaction(&st, func() interface{} {
		store := adt.AsStore(rt)
		err = st.setMinerBalance(store, addresses.IDAddress, rt.ValueReceived())
		abortIfError(rt, err, "failed to set pledge balance")
		err = st.setClaim(store, addresses.IDAddress, &Claim{abi.NewStoragePower(0), abi.NewTokenAmount(0)})
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to put power in claimed table while creating miner: %v", err)
		}
		st.MinerCount += 1
		return nil
	})
	return &CreateMinerReturn{
		IDAddress:     addresses.IDAddress,
		RobustAddress: addresses.RobustAddress,
	}
}

type DeleteMinerParams struct {
	Miner addr.Address
}

func (a Actor) DeleteMiner(rt Runtime, params *DeleteMinerParams) *adt.EmptyValue {
	var st State
	rt.State().Readonly(&st)

	balance, err := st.getMinerBalance(adt.AsStore(rt), params.Miner)
	abortIfError(rt, err, "failed to get pledge balance for deletion")

	if balance.GreaterThan(abi.NewTokenAmount(0)) {
		rt.Abort(exitcode.ErrForbidden, "deletion requested for miner %v with pledge balance %v", params.Miner, balance)
	}

	claim, found, err := st.getClaim(adt.AsStore(rt), params.Miner)
	if err != nil {
		rt.Abort(exitcode.ErrIllegalState, "failed to load miner claim for deletion: %v", err)
	}
	if !found {
		rt.Abort(exitcode.ErrIllegalState, "failed to find miner %v claim for deletion", params.Miner)
	}
	if claim.Power.GreaterThan(big.Zero()) {
		rt.Abort(exitcode.ErrIllegalState, "deletion requested for miner %v with power %v", params.Miner, claim.Power)
	}

	ownerAddr, workerAddr := builtin.RequestMinerControlAddrs(rt, params.Miner)
	rt.ValidateImmediateCallerIs(ownerAddr, workerAddr)

	err = a.deleteMinerActor(rt, params.Miner)
	abortIfError(rt, err, "failed to delete miner %v", params.Miner)
	return &adt.EmptyValue{}
}

type OnSectorProveCommitParams struct {
	Weight SectorStorageWeightDesc
}

// Returns the computed pledge collateral requirement, which is now committed.
func (a Actor) OnSectorProveCommit(rt Runtime, params *OnSectorProveCommitParams) *big.Int {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	var pledge abi.TokenAmount
	var st State
	rt.State().Transaction(&st, func() interface{} {
		power := consensusPowerForWeight(&params.Weight)
		pledge = pledgeForWeight(&params.Weight, st.TotalNetworkPower)
		err := st.addToClaim(adt.AsStore(rt), rt.ImmediateCaller(), power, pledge)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to add power for sector: %v", err)
		}
		return nil
	})

	return &pledge
}

type OnSectorTerminateParams struct {
	TerminationType SectorTermination
	Weights         []SectorStorageWeightDesc // TODO: replace with power if it can be computed by miner
	Pledge          abi.TokenAmount
}

func (a Actor) OnSectorTerminate(rt Runtime, params *OnSectorTerminateParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	var st State
	rt.State().Transaction(&st, func() interface{} {
		power := consensusPowerForWeights(params.Weights)
		err := st.addToClaim(adt.AsStore(rt), minerAddr, power.Neg(), params.Pledge.Neg())
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to deduct claimed power for sector: %v", err)
		}
		return nil
	})

	if params.TerminationType != SectorTerminationExpired {
		amountToSlash := pledgePenaltyForSectorTermination(params.Pledge, params.TerminationType)
		a.slashPledgeCollateral(rt, minerAddr, amountToSlash) // state transactions could be combined.
	}
	return &adt.EmptyValue{}
}

type OnSectorTemporaryFaultEffectiveBeginParams struct {
	Weights []SectorStorageWeightDesc // TODO: replace with power if it can be computed by miner
	Pledge  abi.TokenAmount
}

func (a Actor) OnSectorTemporaryFaultEffectiveBegin(rt Runtime, params *OnSectorTemporaryFaultEffectiveBeginParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	var st State
	rt.State().Transaction(&st, func() interface{} {
		power := consensusPowerForWeights(params.Weights)
		err := st.addToClaim(adt.AsStore(rt), rt.ImmediateCaller(), power.Neg(), params.Pledge.Neg())
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to deduct claimed power for sector: %v", err)
		}
		return nil
	})

	return &adt.EmptyValue{}
}

type OnSectorTemporaryFaultEffectiveEndParams struct {
	Weights []SectorStorageWeightDesc // TODO: replace with power if it can be computed by miner
	Pledge  abi.TokenAmount
}

func (a Actor) OnSectorTemporaryFaultEffectiveEnd(rt Runtime, params *OnSectorTemporaryFaultEffectiveEndParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)

	var st State
	rt.State().Transaction(&st, func() interface{} {
		power := consensusPowerForWeights(params.Weights)
		err := st.addToClaim(adt.AsStore(rt), rt.ImmediateCaller(), power, params.Pledge)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to add claimed power for sector: %v", err)
		}
		return nil
	})

	return &adt.EmptyValue{}
}

type OnSectorModifyWeightDescParams struct {
	PrevWeight SectorStorageWeightDesc // TODO: replace with power if it can be computed by miner
	PrevPledge abi.TokenAmount
	NewWeight  SectorStorageWeightDesc
}

// Returns new pledge collateral requirement, now committed in place of the old.
func (a Actor) OnSectorModifyWeightDesc(rt Runtime, params *OnSectorModifyWeightDescParams) *abi.TokenAmount {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	var newPledge abi.TokenAmount
	var st State
	rt.State().Transaction(&st, func() interface{} {
		prevPower := consensusPowerForWeight(&params.PrevWeight)
		err := st.addToClaim(adt.AsStore(rt), rt.ImmediateCaller(), prevPower.Neg(), params.PrevPledge.Neg())
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to deduct claimed power for sector: %v", err)
		}

		newPower := consensusPowerForWeight(&params.NewWeight)
		newPledge = pledgeForWeight(&params.NewWeight, st.TotalNetworkPower)
		err = st.addToClaim(adt.AsStore(rt), rt.ImmediateCaller(), newPower, newPledge)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to add power for sector: %v", err)
		}
		return nil
	})

	return &newPledge
}

func (a Actor) OnMinerSurprisePoStSuccess(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	var st State
	rt.State().Transaction(&st, func() interface{} {
		if err := st.deleteFault(adt.AsStore(rt), minerAddr); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to delete miner fault: %v", err)
		}

		return nil
	})
	return &adt.EmptyValue{}
}

type OnMinerSurprisePoStFailureParams struct {
	NumConsecutiveFailures int64
}

func (a Actor) OnMinerSurprisePoStFailure(rt Runtime, params *OnMinerSurprisePoStFailureParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	var claim *Claim
	var st State
	rt.State().Transaction(&st, func() interface{} {
		if err := st.putFault(adt.AsStore(rt), minerAddr); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to put miner fault: %v", err)
		}

		var found bool
		var err error
		claim, found, err = st.getClaim(adt.AsStore(rt), minerAddr)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to get miner power from claimed power table for surprise PoSt failure: %v", err)
		}
		if !found {
			rt.Abort(exitcode.ErrIllegalState, "Failed to find miner power in claimed power table for surprise PoSt failure")
		}
		return nil
	})

	if params.NumConsecutiveFailures > SurprisePostFailureLimit {
		err := a.deleteMinerActor(rt, minerAddr)
		abortIfError(rt, err, "failed to delete failed miner %v", minerAddr)
	} else {
		// Penalise pledge collateral without reducing the claim.
		// The miner will have to deposit more when recovering the fault (unless already in sufficient surplus).
		amountToSlash := pledgePenaltyForSurprisePoStFailure(claim.Pledge, params.NumConsecutiveFailures)
		a.slashPledgeCollateral(rt, minerAddr, amountToSlash)
	}
	return &adt.EmptyValue{}
}

type EnrollCronEventParams struct {
	EventEpoch abi.ChainEpoch
	Payload    []byte
}

func (a Actor) EnrollCronEvent(rt Runtime, params *EnrollCronEventParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()
	minerEvent := CronEvent{
		MinerAddr:       minerAddr,
		CallbackPayload: params.Payload,
	}

	var st State
	rt.State().Transaction(&st, func() interface{} {
		err := st.appendCronEvent(adt.AsStore(rt), params.EventEpoch, &minerEvent)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to enroll cron event: %v", err)
		}
		return nil
	})
	return &adt.EmptyValue{}
}

type ReportConsensusFaultParams struct {
	BlockHeader1 []byte
	BlockHeader2 []byte
	Target       addr.Address
	FaultEpoch   abi.ChainEpoch
	FaultType    ConsensusFaultType
}

func (a Actor) ReportConsensusFault(rt Runtime, params *ReportConsensusFaultParams) *adt.EmptyValue {
	// TODO: jz, zx determine how to reward multiple reporters of the same fault within a single epoch.

	isValidConsensusFault := rt.Syscalls().VerifyConsensusFault(params.BlockHeader1, params.BlockHeader2)
	if !isValidConsensusFault {
		rt.Abort(exitcode.ErrIllegalArgument, "reported consensus fault failed verification")
	}

	reporter := rt.ImmediateCaller()
	var st State
	reward := rt.State().Transaction(&st, func() interface{} {
		store := adt.AsStore(rt)
		claim, powerOk, err := st.getClaim(store, params.Target)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to read claimed power for fault: %v", err)
		}
		if !powerOk {
			rt.Abort(exitcode.ErrIllegalArgument, "miner %v not registered (already slashed?)", params.Target)
		}
		Assert(claim.Power.GreaterThanEqual(big.Zero()))

		currBalance, err := st.getMinerBalance(store, params.Target)
		abortIfError(rt, err, "failed to get miner pledge balance")
		Assert(currBalance.GreaterThanEqual(big.Zero()))

		// elapsed epoch from the latter block which committed the fault
		elapsedEpoch := rt.CurrEpoch() - params.FaultEpoch
		if elapsedEpoch <= 0 {
			rt.Abort(exitcode.ErrIllegalArgument, "invalid fault epoch %v ahead of current %v", params.FaultEpoch, rt.CurrEpoch())
		}

		// Note: this slashes the miner's whole balance, including any excess over the required claim.Pledge.
		collateralToSlash := pledgePenaltyForConsensusFault(currBalance, params.FaultType)
		targetReward := rewardForConsensusSlashReport(elapsedEpoch, collateralToSlash)

		availableReward, err := st.subtractMinerBalance(store, params.Target, targetReward, big.Zero())
		abortIfError(rt, err, "failed to subtract pledge for reward")
		return availableReward
	}).(abi.TokenAmount)

	// reward reporter
	_, code := rt.Send(reporter, builtin.MethodSend, nil, reward)
	builtin.RequireSuccess(rt, code, "failed to reward reporter")

	// burn the rest of pledge collateral
	// delete miner from power table
	err := a.deleteMinerActor(rt, params.Target)
	abortIfError(rt, err, "failed to remove slashed miner %v", params.Target)
	return &adt.EmptyValue{}
}

// Called by Cron.
func (a Actor) OnEpochTickEnd(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.CronActorAddr)

	if err := a.initiateNewSurprisePoStChallenges(rt); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to initiate new surprise PoSt challenges: %v", err)
	}
	if err := a.processDeferredCronEvents(rt); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to process deferred cron events: %v", err)
	}
	return &adt.EmptyValue{}
}

////////////////////////////////////////////////////////////////////////////////
// Method utility functions
////////////////////////////////////////////////////////////////////////////////

func (a Actor) initiateNewSurprisePoStChallenges(rt Runtime) error {
	provingPeriod := SurprisePoStPeriod
	var surprisedMiners []addr.Address
	var st State
	var txErr error
	rt.State().Transaction(&st, func() interface{} {
		var err error
		// sample the actor addresses
		minerSelectionSeed := rt.GetRandomness(rt.CurrEpoch())
		randomness := crypto.DeriveRandWithEpoch(crypto.DomainSeparationTag_SurprisePoStSelectMiners, minerSelectionSeed, int64(rt.CurrEpoch()))

		TODO() // BigInt arithmetic (not floating-point)
		challengeCount := math.Ceil(float64(st.MinerCount) / float64(provingPeriod))
		surprisedMiners, err = st.selectMinersToSurprise(adt.AsStore(rt), int64(challengeCount), randomness)
		if err != nil {
			txErr = errors.Wrap(err, "failed to select miner to surprise")
		}
		return nil
	})

	if txErr != nil {
		return txErr
	}

	for _, address := range surprisedMiners {
		_, code := rt.Send(
			address,
			builtin.MethodsMiner.OnSurprisePoStChallenge,
			&adt.EmptyValue{},
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to challenge miner")
	}
	return nil
}

func (a Actor) processDeferredCronEvents(rt Runtime) error {
	epoch := rt.CurrEpoch()

	var epochEvents []CronEvent
	var st State
	rt.State().Transaction(&st, func() interface{} {
		store := adt.AsStore(rt)
		var err error
		epochEvents, err = st.loadCronEvents(store, epoch)
		if err != nil {
			return errors.Wrapf(err, "failed to load cron events at %v", epoch)
		}

		err = st.clearCronEvents(store, epoch)
		if err != nil {
			return errors.Wrapf(err, "failed to clear cron events at %v", epoch)
		}
		return nil
	})

	for _, event := range epochEvents {
		_, code := rt.Send(
			event.MinerAddr,
			builtin.MethodsMiner.OnDeferredCronEvent,
			vmr.CBORBytes(event.CallbackPayload),
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to defer cron event")
	}
	return nil
}

func (a Actor) slashPledgeCollateral(rt Runtime, minerAddr addr.Address, amountToSlash abi.TokenAmount) {
	var st State
	amountSlashed := rt.State().Transaction(&st, func() interface{} {
		subtracted, err := st.subtractMinerBalance(adt.AsStore(rt), minerAddr, amountToSlash, big.Zero())
		abortIfError(rt, err, "failed to subtract collateral for slash")
		return subtracted
	}).(abi.TokenAmount)

	_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashed)
	builtin.RequireSuccess(rt, code, "failed to burn funds")
}

func (a Actor) deleteMinerActor(rt Runtime, miner addr.Address) error {
	var st State
	var txErr error
	amountSlashed := rt.State().Transaction(&st, func() interface{} {
		var err error

		err = st.deleteClaim(adt.AsStore(rt), miner)
		if err != nil {
			txErr = errors.Wrapf(err, "failed to delete %v from claimed power table", miner)
			return big.Zero()
		}

		st.MinerCount -= 1
		if err = st.deleteFault(adt.AsStore(rt), miner); err != nil {
			return err
		}

		table := adt.AsBalanceTable(adt.AsStore(rt), st.EscrowTable)
		balance, err := table.Remove(miner)
		if err != nil {
			txErr = errors.Wrapf(err, "failed to delete pledge balance entry for %v", miner)
			return big.Zero()
		}
		st.EscrowTable = table.Root()
		return balance
	}).(abi.TokenAmount)

	if txErr != nil {
		return txErr
	}

	_, code := rt.Send(
		miner,
		builtin.MethodsMiner.OnDeleteMiner,
		&adt.EmptyValue{},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to delete miner actor")

	_, code = rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashed)
	builtin.RequireSuccess(rt, code, "failed to burn funds")

	return nil
}

func validatePledgeAccount(rt Runtime, addr addr.Address) {
	codeID, ok := rt.GetActorCodeCID(addr)
	if !ok {
		rt.Abort(exitcode.ErrIllegalArgument, "no code for address %v", addr)
	}
	if !codeID.Equals(builtin.StorageMinerActorCodeID) {
		rt.Abort(exitcode.ErrIllegalArgument, "pledge account %v must be address of miner actor, was %v", addr, codeID)
	}
}

func consensusPowerForWeights(weights []SectorStorageWeightDesc) abi.StoragePower {
	power := big.Zero()
	for i := range weights {
		power = big.Add(power, consensusPowerForWeight(&weights[i]))
	}
	return power
}

func abortIfError(rt Runtime, err error, msg string, args ...interface{}) {
	if err != nil {
		code := exitcode.ErrIllegalState
		if _, ok := err.(adt.ErrNotFound); ok {
			code = exitcode.ErrNotFound
		}
		fmtmst := fmt.Sprintf(msg, args...)
		rt.Abort(code, "%s: %v", fmtmst, err)
	}
}

func bigProduct(p big.Int, rest ...big.Int) big.Int {
	for _, r := range rest {
		p = big.Mul(p, r)
	}
	return p
}
