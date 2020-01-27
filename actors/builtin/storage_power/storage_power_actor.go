package storage_power

import (
	"math"

	addr "github.com/filecoin-project/go-address"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	initact "github.com/filecoin-project/specs-actors/actors/builtin/init"
	crypto "github.com/filecoin-project/specs-actors/actors/crypto"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	indices "github.com/filecoin-project/specs-actors/actors/runtime/indices"
	serde "github.com/filecoin-project/specs-actors/actors/serde"
	autil "github.com/filecoin-project/specs-actors/actors/util"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type BalanceTableHAMT = autil.BalanceTableHAMT
type SectorStorageWeightDesc = autil.SectorStorageWeightDesc
type SectorTerminationType = autil.SectorTermination
type Runtime = vmr.Runtime

var Assert = autil.Assert
var IMPL_FINISH = autil.IMPL_FINISH
var TODO = autil.TODO

type ConsensusFaultType int

const (
	UncommittedPowerFault ConsensusFaultType = 0
	DoubleForkMiningFault ConsensusFaultType = 1
	ParentGrindingFault   ConsensusFaultType = 2
	TimeOffsetMiningFault ConsensusFaultType = 3
)

type StoragePowerActor struct{}

////////////////////////////////////////////////////////////////////////////////
// Actor methods
////////////////////////////////////////////////////////////////////////////////

func (a *StoragePowerActor) AddBalance(rt Runtime, minerAddr addr.Address) *vmr.EmptyReturn {
	builtin.RT_MinerEntry_ValidateCaller_DetermineFundsLocation(rt, minerAddr, builtin.MinerEntrySpec_MinerOnly)

	msgValue := rt.ValueReceived()

	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		newTable, ok := autil.BalanceTable_WithAdd(st.EscrowTable, minerAddr, msgValue)
		if !ok {
			rt.AbortStateMsg("Escrow operation failed")
		}
		st.EscrowTable = newTable
		return nil
	})
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) WithdrawBalance(rt Runtime, minerAddr addr.Address, amountRequested abi.TokenAmount) *vmr.EmptyReturn {
	if amountRequested.LessThan(big.Zero()) {
		rt.Abort(exitcode.ErrIllegalArgument, "negative withdrawal %v", amountRequested)
	}

	recipientAddr := builtin.RT_MinerEntry_ValidateCaller_DetermineFundsLocation(rt, minerAddr, builtin.MinerEntrySpec_MinerOnly)

	var amountExtracted abi.TokenAmount
	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		minBalanceMaintainRequired, err := a.getPledgeCollateralReqForMiner(rt, &st, minerAddr)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to get required pledge collateral required for miner: %v", err)
		}
		newTable, ex, ok := autil.BalanceTable_WithExtractPartial(
			st.EscrowTable, minerAddr, amountRequested, minBalanceMaintainRequired)
		if !ok {
			rt.AbortStateMsg("Escrow operation failed")
		}
		st.EscrowTable = newTable
		amountExtracted = ex
		return nil
	})

	_, code := rt.Send(recipientAddr, builtin.MethodSend, nil, amountExtracted)
	builtin.RequireSuccess(rt, code, "failed to send funds")
	return &vmr.EmptyReturn{}
}

type CreateMinerReturn struct {
	IDAddress     addr.Address // The canonical ID-based address for the actor.
	RobustAddress addr.Address // A mre expensive but re-org-safe address for the newly created actor.
}

func (a *StoragePowerActor) CreateMiner(rt Runtime, workerAddr addr.Address, sectorSize abi.SectorSize, peerId peer.ID) *CreateMinerReturn {
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	ownerAddr := rt.ImmediateCaller()

	ret, code := rt.Send(
		builtin.InitActorAddr,
		builtin.Method_InitActor_Exec,
		serde.MustSerializeParams(
			builtin.StorageMinerActorCodeID,
			ownerAddr,
			workerAddr,
			sectorSize,
			peerId,
		),
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to init new actor")
	var addresses initact.ExecReturn
	autil.AssertNoError(ret.Into(addresses))

	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		newTable, ok := autil.BalanceTable_WithNewAddressEntry(st.EscrowTable, addresses.IDAddress, rt.ValueReceived())
		Assert(ok)
		st.EscrowTable = newTable
		var err error
		st.PowerTable, err = putStoragePower(adt.AsStore(rt), st.PowerTable, addresses.IDAddress, abi.NewStoragePower(0))
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to put power in power table while creating miner: %v", err)
		}
		st.ClaimedPower, err = putStoragePower(adt.AsStore(rt), st.ClaimedPower, addresses.IDAddress, abi.NewStoragePower(0))
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to put power in claimed table while creating miner: %v", err)
		}
		st.NominalPower, err = putStoragePower(adt.AsStore(rt), st.NominalPower, addresses.IDAddress, abi.NewStoragePower(0))
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to put power in nominal power table while creating miner: %v", err)
		}
		st.MinerCount += 1
		return nil
	})
	return &CreateMinerReturn{
		IDAddress:     addresses.IDAddress,
		RobustAddress: addresses.RobustAddress,
	}
}

func (a *StoragePowerActor) DeleteMiner(rt Runtime, minerAddr addr.Address) *vmr.EmptyReturn {
	var st StoragePowerActorState
	rt.State().Readonly(&st)

	minerPledgeBalance, ok := autil.BalanceTable_GetEntry(st.EscrowTable, minerAddr)
	if !ok {
		rt.Abort(exitcode.ErrNotFound, "no such miner %v", minerAddr)
	}

	if minerPledgeBalance.GreaterThan(abi.NewTokenAmount(0)) {
		rt.AbortStateMsg("Deletion requested for miner with pledge balance still remaining")
	}

	minerPower, found, err := getStoragePower(adt.AsStore(rt), st.PowerTable, minerAddr)
	if err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to get miner power from power table for deletion request: %v", err)
	}
	if !found {
		rt.Abort(exitcode.ErrIllegalState, "Failed to find miner power in power table for deletion request")
	}
	if minerPower.GreaterThan(big.Zero()) {
		rt.AbortStateMsg("Deletion requested for miner with power still remaining")
	}

	ownerAddr, workerAddr := builtin.RT_GetMinerAccountsAssert(rt, minerAddr)
	rt.ValidateImmediateCallerIs(ownerAddr, workerAddr)

	if err := a.deleteMinerActor(rt, minerAddr); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to delete miner actor: %v", err)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnSectorProveCommit(rt Runtime, storageWeightDesc SectorStorageWeightDesc) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	if err := a.addPowerForSector(rt, rt.ImmediateCaller(), storageWeightDesc); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to add power for sector: %v", err)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnSectorTerminate(
	rt Runtime, storageWeightDesc SectorStorageWeightDesc, terminationType SectorTerminationType) *vmr.EmptyReturn {

	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()
	if err := a.deductClaimedPowerForSector(rt, minerAddr, storageWeightDesc); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to deduct claimed power for sector: %v", err)
	}

	if terminationType != autil.NormalExpiration {
		cidx := rt.CurrIndices()
		amountToSlash := cidx.StoragePower_PledgeSlashForSectorTermination(storageWeightDesc, terminationType)
		a.slashPledgeCollateral(rt, minerAddr, amountToSlash)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnSectorTemporaryFaultEffectiveBegin(rt Runtime, storageWeightDesc SectorStorageWeightDesc) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	if err := a.deductClaimedPowerForSector(rt, rt.ImmediateCaller(), storageWeightDesc); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to deduct claimed power for sector: %v", err)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnSectorTemporaryFaultEffectiveEnd(rt Runtime, storageWeightDesc SectorStorageWeightDesc) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	if err := a.addPowerForSector(rt, rt.ImmediateCaller(), storageWeightDesc); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to add power for sector: %v", err)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnSectorModifyWeightDesc(
	rt Runtime, storageWeightDescPrev SectorStorageWeightDesc, storageWeightDescNew SectorStorageWeightDesc) *vmr.EmptyReturn {

	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	if err := a.deductClaimedPowerForSector(rt, rt.ImmediateCaller(), storageWeightDescPrev); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to deduct claimed power for sector: %v", err)
	}
	if err := a.addPowerForSector(rt, rt.ImmediateCaller(), storageWeightDescNew); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to add power for sector: %v", err)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnMinerSurprisePoStSuccess(rt Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		delete(st.PoStDetectedFaultMiners, minerAddr)
		if err := st._updatePowerEntriesFromClaimedPower(adt.AsStore(rt), minerAddr); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to update miners claimed power table on surprise PoSt success: %v", err)
		}
		return nil
	})
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnMinerSurprisePoStFailure(rt Runtime, numConsecutiveFailures int64) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	var minerClaimedPower abi.StoragePower
	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		st.PoStDetectedFaultMiners[minerAddr] = true
		if err := st._updatePowerEntriesFromClaimedPower(adt.AsStore(rt), minerAddr); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to update power entries for claimed power: %v", err)
		}

		var found bool
		var err error
		minerClaimedPower, found, err = getStoragePower(adt.AsStore(rt), st.ClaimedPower, minerAddr)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to get miner power from claimed power table for surprise PoSt failure: %v", err)
		}
		if !found {
			rt.Abort(exitcode.ErrIllegalState, "Failed to find miner power in claimed power table for surprise PoSt failure")
		}
		return nil
	})

	if numConsecutiveFailures > indices.StoragePower_SurprisePoStMaxConsecutiveFailures() {
		if err := a.deleteMinerActor(rt, minerAddr); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "Failed to delete miner actor: %v", err)
		}
	} else {
		cidx := rt.CurrIndices()
		amountToSlash := cidx.StoragePower_PledgeSlashForSurprisePoStFailure(minerClaimedPower, numConsecutiveFailures)
		a.slashPledgeCollateral(rt, minerAddr, amountToSlash)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) OnMinerEnrollCronEvent(rt Runtime, eventEpoch abi.ChainEpoch, payload []byte) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()
	minerEvent := autil.MinerCallbackEvent{
		MinerAddr:       minerAddr,
		CallbackPayload: payload,
	}

	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		if _, found := st.CachedDeferredCronEvents[eventEpoch]; !found {
			st.CachedDeferredCronEvents[eventEpoch] = autil.MinerCallbackEventSetHAMT_Empty()
		}
		st.CachedDeferredCronEvents[eventEpoch] = append(st.CachedDeferredCronEvents[eventEpoch], minerEvent)
		return nil
	})
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) ReportConsensusFault(rt Runtime, blockHeader1, blockHeader2 []byte, slashee addr.Address, faultEpoch abi.ChainEpoch, faultType ConsensusFaultType) *vmr.EmptyReturn {
	TODO()
	// TODO: The semantics here are quite delicate:
	//
	// - (proof []block.Block) can't be validated in isolation; we must query the runtime to confirm
	//   that at least one of the blocks provided actually appeared in the current chain.
	// - We must prevent duplicate slashes on the same offense, taking into account that the blocks
	//   may appear in different orders.
	// - We must determine how to reward multiple reporters of the same fault within a single epoch.
	//
	// Deferring to followup after these security/mechanism design questions have been resolved.
	// Previous notes:

	// validation checks
	// - there should be exactly two block headers in proof
	// - both blocks are mined by the same miner
	// - two block headers are different
	// - first block is of the same or lower block height as the second block
	//
	// Use EC's IsValidConsensusFault method to validate the proof
	isValidConsensusFault := rt.Syscalls().VerifyConsensusFault(blockHeader1, blockHeader2)
	if !isValidConsensusFault {
		rt.Abort(exitcode.ErrIllegalArgument, "spa.ReportConsensusFault: unverified consensus fault")
	}

	slasherAddr := rt.ImmediateCaller()
	var amountToSlasher abi.TokenAmount
	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		claimedPower, powerOk, err := getStoragePower(adt.AsStore(rt), st.ClaimedPower, slashee)
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "spa.ReportConsensusFault failed to read claimed power for fault: %v", err)
		}
		if !powerOk {
			rt.Abort(exitcode.ErrIllegalArgument, "spa.ReportConsensusFault: miner already slashed")
		}
		Assert(claimedPower.GreaterThan(big.Zero()))

		currPledge, pledgeOk := st._getCurrPledgeForMiner(slashee)
		if !pledgeOk {
			rt.Abort(exitcode.ErrIllegalArgument, "spa.ReportConsensusFault: miner has no pledge")
		}
		Assert(currPledge.GreaterThan(big.Zero()))

		// elapsed epoch from the latter block which committed the fault
		elapsedEpoch := rt.CurrEpoch() - faultEpoch
		if elapsedEpoch <= 0 {
			rt.Abort(exitcode.ErrIllegalArgument, "spa.ReportConsensusFault: invalid block")
		}

		collateralToSlash := st._getPledgeSlashForConsensusFault(currPledge, faultType)
		slasherReward := _getConsensusFaultSlasherReward(elapsedEpoch, collateralToSlash)

		// request slasherReward to be deducted from EscrowTable
		amountToSlasher = st._slashPledgeCollateral(slasherAddr, slasherReward)
		Assert(slasherReward == amountToSlasher)
		return nil
	})

	// reward slasher
	_, code := rt.Send(slasherAddr, builtin.MethodSend, nil, amountToSlasher)
	builtin.RequireSuccess(rt, code, "failed to reward slasher")

	// burn the rest of pledge collateral
	// delete miner from power table
	if err := a.deleteMinerActor(rt, slashee); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to delete miner actor: %v", err)
	}
	return &vmr.EmptyReturn{}
}

// Called by Cron.
func (a *StoragePowerActor) OnEpochTickEnd(rt Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerIs(builtin.CronActorAddr)

	if err := a.initiateNewSurprisePoStChallenges(rt); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to initiate new surprise PoSt challenges: %v", err)
	}
	if err := a.processDeferredCronEvents(rt); err != nil {
		rt.Abort(exitcode.ErrIllegalState, "Failed to process deferred cron events: %v", err)
	}
	return &vmr.EmptyReturn{}
}

func (a *StoragePowerActor) Constructor(rt Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)

	rt.State().Construct(func() vmr.CBORMarshaler {
		emptyMap, err := adt.MakeEmptyMap(adt.AsStore(rt))
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to create empty map: %v", err)
		}

		var st StoragePowerActorState
		st.TotalNetworkPower = abi.NewStoragePower(0)
		st.PowerTable = emptyMap.Root()
		st.EscrowTable = autil.BalanceTableHAMT_Empty()
		st.CachedDeferredCronEvents = MinerCallbackEventsHAMT_Empty()
		st.PoStDetectedFaultMiners = autil.MinerSetHAMT_Empty()
		st.ClaimedPower = emptyMap.Root()
		st.NominalPower = emptyMap.Root()
		st.NumMinersMeetingMinPower = 0
		return &st
	})
	return &vmr.EmptyReturn{}
}

////////////////////////////////////////////////////////////////////////////////
// Method utility functions
////////////////////////////////////////////////////////////////////////////////

func (a *StoragePowerActor) addPowerForSector(rt Runtime, minerAddr addr.Address, storageWeightDesc SectorStorageWeightDesc) error {
	var st StoragePowerActorState
	var txErr error
	rt.State().Transaction(&st, func() interface{} {
		if err := st._addClaimedPowerForSector(adt.AsStore(rt), minerAddr, storageWeightDesc); err != nil {
			txErr = errors.Wrap(err, "failed to add power power for sector")
		}
		return nil
	})
	return txErr
}

func (a *StoragePowerActor) deductClaimedPowerForSector(rt Runtime, minerAddr addr.Address, storageWeightDesc SectorStorageWeightDesc) error {
	var st StoragePowerActorState
	var txErr error
	rt.State().Transaction(&st, func() interface{} {
		if err := st._deductClaimedPowerForSectorAssert(adt.AsStore(rt), minerAddr, storageWeightDesc); err != nil {
			txErr = errors.Wrap(err, "failed to deducted claimed power for sector")
		}
		return nil
	})
	return txErr
}

func (a *StoragePowerActor) initiateNewSurprisePoStChallenges(rt Runtime) error {
	provingPeriod := indices.StorageMining_SurprisePoStProvingPeriod()
	var surprisedMiners []addr.Address
	var st StoragePowerActorState
	var txErr error
	rt.State().Transaction(&st, func() interface{} {
		var err error
		// sample the actor addresses
		minerSelectionSeed := rt.GetRandomness(rt.CurrEpoch())
		randomness := crypto.DeriveRandWithEpoch(crypto.DomainSeparationTag_SurprisePoStSelectMiners, minerSelectionSeed, int(rt.CurrEpoch()))

		IMPL_FINISH() // BigInt arithmetic (not floating-point)
		challengeCount := math.Ceil(float64(st.MinerCount) / float64(provingPeriod))
		surprisedMiners, err = st._selectMinersToSurprise(adt.AsStore(rt), int(challengeCount), randomness)
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
			builtin.Method_StorageMinerActor_OnSurprisePoStChallenge,
			nil,
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to challenge miner")
	}
	return nil
}

func (a *StoragePowerActor) processDeferredCronEvents(rt Runtime) error {
	epoch := rt.CurrEpoch()

	var minerEvents []autil.MinerCallbackEvent
	var st StoragePowerActorState
	rt.State().Transaction(&st, func() interface{} {
		// TODO should we be checking the second return here?
		minerEvents, _ = st.CachedDeferredCronEvents[epoch]
		delete(st.CachedDeferredCronEvents, epoch)
		return nil
	})

	minerEventsRetain := []autil.MinerCallbackEvent{}
	for _, minerEvent := range minerEvents {
		if _, found, err := getStoragePower(adt.AsStore(rt), st.PowerTable, minerEvent.MinerAddr); err != nil {
			return errors.Wrap(err, "Failed to get miner power from power table while processing cron events")
		} else if found {
			minerEventsRetain = append(minerEventsRetain, minerEvent)
		}
	}

	for _, minerEvent := range minerEventsRetain {
		_, code := rt.Send(
			minerEvent.MinerAddr,
			builtin.Method_StorageMinerActor_OnDeferredCronEvent,
			serde.MustSerializeParams(
				minerEvent.CallbackPayload,
			),
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to defer cron event")
	}
	return nil
}

func (a *StoragePowerActor) getPledgeCollateralReqForMiner(rt Runtime, st *StoragePowerActorState, minerAddr addr.Address) (abi.TokenAmount, error) {
	minerNominalPower, found, err := getStoragePower(adt.AsStore(rt), st.NominalPower, minerAddr)
	if err != nil {
		return abi.NewTokenAmount(0), errors.Wrap(err, "Failed to get miner power from nominal power table")
	}
	if !found {
		return abi.NewTokenAmount(0), errors.Errorf("no miner %v", minerAddr)
	}
	return rt.CurrIndices().PledgeCollateralReq(minerNominalPower), nil
}

func (a *StoragePowerActor) slashPledgeCollateral(rt Runtime, minerAddr addr.Address, amountToSlash abi.TokenAmount) {
	var st StoragePowerActorState
	amountSlashed := rt.State().Transaction(&st, func() interface{} {
		return st._slashPledgeCollateral(minerAddr, amountToSlash)
	}).(abi.TokenAmount)

	_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashed)
	builtin.RequireSuccess(rt, code, "failed to burn funds")
}

func (a *StoragePowerActor) deleteMinerActor(rt Runtime, minerAddr addr.Address) error {
	var st StoragePowerActorState
	var txErr error
	amountSlashed := rt.State().Transaction(&st, func() interface{} {
		var err error
		st.PowerTable, err = deleteStoragePower(adt.AsStore(rt), st.PowerTable, minerAddr)
		if err != nil {
			txErr = errors.Wrap(err, "failed to delete storage power from storage power table")
			return abi.NewTokenAmount(0)
		}
		st.ClaimedPower, err = deleteStoragePower(adt.AsStore(rt), st.ClaimedPower, minerAddr)
		if err != nil {
			txErr = errors.Wrap(err, "failed to delete storage power from claimed power table")
			return abi.NewTokenAmount(0)
		}
		st.NominalPower, err = deleteStoragePower(adt.AsStore(rt), st.NominalPower, minerAddr)
		if err != nil {
			txErr = errors.Wrap(err, "failed to delete storage power from nominal power table")
			return abi.NewTokenAmount(0)
		}
		st.MinerCount -= 1
		delete(st.PoStDetectedFaultMiners, minerAddr)

		newTable, amountSlashed, ok := autil.BalanceTable_WithExtractAll(st.EscrowTable, minerAddr)
		Assert(ok)
		newTable, ok = autil.BalanceTable_WithDeletedAddressEntry(newTable, minerAddr)
		Assert(ok)
		st.EscrowTable = newTable
		return amountSlashed
	}).(abi.TokenAmount)

	if txErr != nil {
		return txErr
	}

	_, code := rt.Send(
		minerAddr,
		builtin.Method_StorageMinerActor_OnDeleteMiner,
		serde.MustSerializeParams(),
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to delete miner actor")

	_, code = rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashed)
	builtin.RequireSuccess(rt, code, "failed to burn funds")

	return nil
}
