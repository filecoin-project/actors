package storage_miner

import (
	"bytes"

	addr "github.com/filecoin-project/go-address"
	cid "github.com/ipfs/go-cid"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	storage_market "github.com/filecoin-project/specs-actors/actors/builtin/storage_market"
	storage_power "github.com/filecoin-project/specs-actors/actors/builtin/storage_power"
	crypto "github.com/filecoin-project/specs-actors/actors/crypto"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	indices "github.com/filecoin-project/specs-actors/actors/runtime/indices"
	autil "github.com/filecoin-project/specs-actors/actors/util"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type Runtime = vmr.Runtime

var Assert = autil.Assert
var AssertNoError = autil.AssertNoError

const epochUndefined = abi.ChainEpoch(-1)

type StorageMinerActor struct{}

type CronEventType int64

const (
	CronEventType_Miner_SurpriseExpiration = CronEventType(1)
	CronEventType_Miner_WorkerKeyChange    = CronEventType(2)
	CronEventType_Miner_PreCommitExpiry    = CronEventType(3)
	CronEventType_Miner_SectorExpiry       = CronEventType(4)
	CronEventType_Miner_TempFault          = CronEventType(5)
)

type CronEventPayload struct {
	EventType CronEventType
	Sectors   []abi.SectorNumber // Empty for global events, such as SurprisePoSt expiration.
}

/////////////////
// Constructor //
/////////////////

// Storage miner actors are created exclusively by the storage power actor. In order to break a circular dependency
// between the two, the construction parameters are defined in the power actor.
type ConstructorParams = storage_power.MinerConstructorParams

func (a *StorageMinerActor) Constructor(rt Runtime, params *ConstructorParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)

	// TODO: fix this, check that the account actor at the other end of this address has a BLS key.
	if params.WorkerAddr.Protocol() != addr.BLS {
		rt.Abort(exitcode.ErrIllegalArgument, "Worker Key must be BLS.")
	}

	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		st.Sectors = SectorsAMT_Empty()
		st.FaultSet = abi.NewBitField()
		st.PoStState = MinerPoStState{
			LastSuccessfulPoSt:     epochUndefined,
			SurpriseChallengeEpoch: epochUndefined,
			ChallengedSectors:      nil,
			NumConsecutiveFailures: 0,
		}
		st.ProvingSet = st.GetCurrentProvingSet()
		st.Info = MinerInfo_New(params.OwnerAddr, params.WorkerAddr, params.SectorSize, params.PeerId)
		return nil
	})
	return &adt.EmptyValue{}
}

///////////////////////
// Worker Key Change //
///////////////////////

type StageWorkerKeyChangeParams struct {
	newKey addr.Address
}

func (a *StorageMinerActor) StageWorkerKeyChange(rt Runtime, params *StageWorkerKeyChangeParams) *adt.EmptyValue {
	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		rt.ValidateImmediateCallerIs(st.Info.Owner)

		// must be BLS since the worker key will be used alongside a BLS-VRF
		// Specifically, this check isn't quite right
		// TODO: check that the account actor at the other end of this address has a BLS key.
		if params.newKey.Protocol() != addr.BLS {
			rt.Abort(exitcode.ErrIllegalArgument, "Worker Key must be BLS.")
		}

		keyChange := WorkerKeyChange{
			NewWorker:   params.newKey,
			EffectiveAt: rt.CurrEpoch() + indices.StorageMining_WorkerKeyChangeFreeze(),
		}

		// note that this may replace another pending key change
		st.Info.PendingWorkerKey = keyChange

		cronPayload := CronEventPayload{
			EventType: CronEventType_Miner_WorkerKeyChange,
			Sectors:   nil,
		}
		a.enrollCronEvent(rt, rt.CurrEpoch()+keyChange.EffectiveAt, &cronPayload)

		return nil
	})
	return &adt.EmptyValue{}
}

func (a *StorageMinerActor) _rtCommitWorkerKeyChange(rt Runtime) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.CronActorAddr)

	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		if (st.Info.PendingWorkerKey == WorkerKeyChange{}) {
			rt.Abort(exitcode.ErrIllegalState, "No pending key change.")
		}

		if st.Info.PendingWorkerKey.EffectiveAt > rt.CurrEpoch() {
			rt.Abort(exitcode.ErrIllegalState, "Too early for key change. Current: %v, Change: %v)", rt.CurrEpoch(), st.Info.PendingWorkerKey.EffectiveAt)
		}

		st.Info.Worker = st.Info.PendingWorkerKey.NewWorker
		st.Info.PendingWorkerKey = WorkerKeyChange{}

		return nil
	})
	return &adt.EmptyValue{}
}

//////////////////
// SurprisePoSt //
//////////////////

// Called by StoragePowerActor to notify StorageMiner of SurprisePoSt Challenge.
func (a *StorageMinerActor) OnSurprisePoStChallenge(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)

	var st StorageMinerActorState
	challenged := rt.State().Transaction(&st, func() interface{} {
		// If already challenged, do not challenge again.
		// Failed PoSt will automatically reset the state to not-challenged.
		if st.PoStState.Is_Challenged() {
			return false
		}
		// Do not challenge if the last successful PoSt was recent enough.
		noChallengePeriod := indices.StorageMining_PoStNoChallengePeriod()
		if st.PoStState.LastSuccessfulPoSt >= rt.CurrEpoch()-noChallengePeriod {
			return false
		}

		var curRecBuf bytes.Buffer
		err := rt.CurrReceiver().MarshalCBOR(&curRecBuf)
		autil.AssertNoError(err)

		randomnessK := rt.GetRandomness(rt.CurrEpoch() - indices.StorageMining_SpcLookbackPoSt())
		challengedSectorsRandomness := crypto.DeriveRandWithMinerAddr(crypto.DomainSeparationTag_SurprisePoStSampleSectors, randomnessK, rt.CurrReceiver())

		st.ProvingSet = st.GetCurrentProvingSet()
		challengedSectors := _surprisePoStSampleChallengedSectors(
			challengedSectorsRandomness,
			st.ProvingSet,
		)

		st.PoStState = MinerPoStState{
			LastSuccessfulPoSt:     st.PoStState.LastSuccessfulPoSt,
			SurpriseChallengeEpoch: rt.CurrEpoch(),
			ChallengedSectors:      challengedSectors,
			NumConsecutiveFailures: st.PoStState.NumConsecutiveFailures,
		}
		return true
	}).(bool)

	if challenged {
		// Request deferred Cron check for SurprisePoSt challenge expiry.
		cronPayload := CronEventPayload{
			EventType: CronEventType_Miner_SurpriseExpiration,
			Sectors:   nil,
		}
		surpriseDuration := indices.StorageMining_SurprisePoStChallengeDuration()
		a.enrollCronEvent(rt, rt.CurrEpoch()+surpriseDuration, &cronPayload)
	}
	return &adt.EmptyValue{}
}

type SubmitSurprisePoStResponseParams struct {
	onChainInfo abi.OnChainSurprisePoStVerifyInfo
}

// Invoked by miner's worker address to submit a response to a pending SurprisePoSt challenge.
func (a *StorageMinerActor) SubmitSurprisePoStResponse(rt Runtime, params *SubmitSurprisePoStResponseParams) *adt.EmptyValue {
	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		rt.ValidateImmediateCallerIs(st.Info.Worker)
		if !st.PoStState.Is_Challenged() {
			rt.Abort(exitcode.ErrIllegalState, "Not currently challenged")
		}
		a.verifySurprisePost(rt, &st, &params.onChainInfo)

		st.PoStState = MinerPoStState{
			LastSuccessfulPoSt:     rt.CurrEpoch(),
			SurpriseChallengeEpoch: epochUndefined,
			ChallengedSectors:      nil,
			NumConsecutiveFailures: 0,
		}
		return nil
	})

	_, code := rt.Send(
		builtin.StoragePowerActorAddr,
		builtin.Method_StoragePowerActor_OnMinerSurprisePoStSuccess,
		&adt.EmptyValue{},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to notify storage power actor")
	return &adt.EmptyValue{}
}

// Called by StoragePowerActor.
func (a *StorageMinerActor) OnDeleteMiner(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)
	minerAddr := rt.CurrReceiver()
	rt.DeleteActor(minerAddr)
	return &adt.EmptyValue{}
}

//////////////////
// ElectionPoSt //
//////////////////

// Called by the VM interpreter once an ElectionPoSt has been verified.
func (a *StorageMinerActor) OnVerifiedElectionPoSt(rt Runtime, _ *adt.EmptyValue) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)
	if rt.ToplevelBlockWinner() != rt.CurrReceiver() {
		rt.Abort(exitcode.ErrForbidden, "receiver must be miner of this block")
	}

	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		updateSuccessEpoch := st.PoStState.Is_OK()

		// Advance the timestamp of the most recent PoSt success, provided the miner is currently
		// in normal state. (Cannot do this if SurprisePoSt mechanism already underway.)
		if updateSuccessEpoch {
			st.PoStState = MinerPoStState{
				LastSuccessfulPoSt:     rt.CurrEpoch(),
				SurpriseChallengeEpoch: st.PoStState.SurpriseChallengeEpoch, // expected to be undef because PoStState is OK
				ChallengedSectors:      st.PoStState.ChallengedSectors,      // expected to be empty
				NumConsecutiveFailures: st.PoStState.NumConsecutiveFailures, // expected to be 0
			}
		}
		return nil
	})
	return &adt.EmptyValue{}
}

///////////////////////
// Sector Commitment //
///////////////////////

type PreCommitSectorParams struct {
	info SectorPreCommitInfo
}

// Deals must be posted on chain via sma.PublishStorageDeals before PreCommitSector.
// Optimization: PreCommitSector could contain a list of deals that are not published yet.
func (a *StorageMinerActor) PreCommitSector(rt Runtime, params *PreCommitSectorParams) *adt.EmptyValue {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	rt.ValidateImmediateCallerIs(st.Info.Worker)
	if _, found := st.Sectors[params.info.SectorNumber]; found {
		rt.Abort(exitcode.ErrIllegalArgument, "Sector number already exists in table")
	}

	cidx := rt.CurrIndices()
	depositReq := cidx.StorageMining_PreCommitDeposit(st.Info.SectorSize, params.info.Expiration)
	builtin.RT_ConfirmFundsReceiptOrAbort_RefundRemainder(rt, depositReq)

	// TODO HS Check on valid SealEpoch

	rt.State().Transaction(&st, func() interface{} {
		st.PreCommittedSectors[params.info.SectorNumber] = SectorPreCommitOnChainInfo{
			Info:             params.info,
			PreCommitDeposit: depositReq,
			PreCommitEpoch:   rt.CurrEpoch(),
		}
		return nil
	})

	if params.info.Expiration <= rt.CurrEpoch() {
		rt.Abort(exitcode.ErrIllegalArgument, "PreCommit sector must expire (%v) after now (%v)", params.info.Expiration, rt.CurrEpoch())
	}

	// Request deferred Cron check for PreCommit expiry check.
	cronPayload := CronEventPayload{
		EventType: CronEventType_Miner_PreCommitExpiry,
		Sectors:   []abi.SectorNumber{params.info.SectorNumber},
	}
	expiryBound := rt.CurrEpoch() + indices.StorageMining_MaxProveCommitSectorEpoch() + 1
	a.enrollCronEvent(rt, expiryBound, &cronPayload)

	return &adt.EmptyValue{}
}

type ProveCommitSectorParams struct {
	info SectorProveCommitInfo
}

func (a *StorageMinerActor) ProveCommitSector(rt Runtime, params *ProveCommitSectorParams) *adt.EmptyValue {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	rt.ValidateImmediateCallerIs(st.Info.Worker)

	preCommitSector, found := st.PreCommittedSectors[params.info.SectorNumber]
	if !found {
		rt.Abort(exitcode.ErrNotFound, "no such sector %v precommitted", params.info.SectorNumber)
	}

	if rt.CurrEpoch() > preCommitSector.PreCommitEpoch+indices.StorageMining_MaxProveCommitSectorEpoch() || rt.CurrEpoch() < preCommitSector.PreCommitEpoch+indices.StorageMining_MinProveCommitSectorEpoch() {
		rt.Abort(exitcode.ErrIllegalArgument, "Invalid ProveCommitSector epoch")
	}

	autil.TODO()
	// TODO HS: How are SealEpoch, InteractiveEpoch determined (and intended to be used)?
	// Presumably they cannot be derived from the SectorProveCommitInfo provided by an untrusted party.

	a._rtVerifySealOrAbort(rt, &abi.OnChainSealVerifyInfo{
		SealedCID:    preCommitSector.Info.SealedCID,
		SealEpoch:    preCommitSector.Info.SealEpoch,
		Proof:        params.info.Proof,
		DealIDs:      preCommitSector.Info.DealIDs,
		SectorNumber: preCommitSector.Info.SectorNumber,
	})

	// Check (and activate) storage deals associated to sector. Abort if checks failed.
	// return DealWeight for the deal set in the sector
	var dealWeight abi.DealWeight
	ret, code := rt.Send(
		builtin.StorageMarketActorAddr,
		builtin.Method_StorageMarketActor_VerifyDealsOnSectorProveCommit,
		&storage_market.VerifyDealsOnSectorProveCommitParams{
			DealIDs:      preCommitSector.Info.DealIDs,
			SectorExpiry: preCommitSector.Info.Expiration,
		},
		abi.NewTokenAmount(0),
	)

	builtin.RequireSuccess(rt, code, "failed to verify deals and get deal weight")
	autil.AssertNoError(ret.Into(&dealWeight))

	rt.State().Transaction(&st, func() interface{} {
		st.Sectors[params.info.SectorNumber] = SectorOnChainInfo{
			Info:            preCommitSector.Info,
			ActivationEpoch: rt.CurrEpoch(),
			DealWeight:      dealWeight,
		}

		delete(st.PreCommittedSectors, params.info.SectorNumber)
		st.ProvingSet = st.GetCurrentProvingSet()
		return nil
	})

	// Request deferred Cron check for sector expiry.
	cronPayload := CronEventPayload{
		EventType: CronEventType_Miner_SectorExpiry,
		Sectors:   []abi.SectorNumber{params.info.SectorNumber},
	}
	a.enrollCronEvent(rt, preCommitSector.Info.Expiration, &cronPayload)

	// Notify SPA to update power associated to newly activated sector.
	storageWeightDesc := a._rtGetStorageWeightDescForSector(rt, params.info.SectorNumber)
	_, code = rt.Send(
		builtin.StoragePowerActorAddr,
		builtin.Method_StoragePowerActor_OnSectorProveCommit,
		&storage_power.OnSectorProveCommitParams{
			Weight: storageWeightDesc,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to notify power actor")

	// Return PreCommit deposit to worker upon successful ProveCommit.
	_, code = rt.Send(st.Info.Worker, builtin.MethodSend, nil, preCommitSector.PreCommitDeposit)
	builtin.RequireSuccess(rt, code, "failed to send funds")
	return &adt.EmptyValue{}
}

/////////////////////////
// Sector Modification //
/////////////////////////

type ExtendSectorExpirationParams struct {
	sectorNumber  abi.SectorNumber
	newExpiration abi.ChainEpoch
}

func (a *StorageMinerActor) ExtendSectorExpiration(rt Runtime, params *ExtendSectorExpirationParams) *adt.EmptyValue {
	storageWeightDescPrev := a._rtGetStorageWeightDescForSector(rt, params.sectorNumber)

	var extensionLength abi.ChainEpoch
	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		rt.ValidateImmediateCallerIs(st.Info.Worker)

		sectorInfo, found := st.Sectors[params.sectorNumber]
		if !found {
			rt.Abort(exitcode.ErrNotFound, "Sector not found")
		}

		extensionLength = params.newExpiration - sectorInfo.Info.Expiration
		if extensionLength < 0 {
			rt.Abort(exitcode.ErrIllegalArgument, "Cannot reduce sector expiration")
		}

		sectorInfo.Info.Expiration = params.newExpiration
		st.Sectors[params.sectorNumber] = sectorInfo
		return nil
	})

	storageWeightDescNew := storageWeightDescPrev
	storageWeightDescNew.Duration = storageWeightDescPrev.Duration + extensionLength

	_, code := rt.Send(
		builtin.StoragePowerActorAddr,
		builtin.Method_StoragePowerActor_OnSectorModifyWeightDesc,
		&storage_power.OnSectorModifyWeightDescParams{
			PrevWeight: storageWeightDescPrev,
			NewWeight:  storageWeightDescNew,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to modify sector weight")
	return &adt.EmptyValue{}
}

type TerminateSectorsParams struct {
	sectorNumbers []abi.SectorNumber
}

func (a *StorageMinerActor) TerminateSectors(rt Runtime, params *TerminateSectorsParams) *adt.EmptyValue {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	rt.ValidateImmediateCallerIs(st.Info.Worker)

	for _, sectorNumber := range params.sectorNumbers {
		a._rtTerminateSector(rt, sectorNumber, builtin.UserTermination)
	}

	return &adt.EmptyValue{}
}

////////////
// Faults //
////////////

type DeclareTemporaryFaultsParams struct {
	sectorNumbers []abi.SectorNumber
	duration      abi.ChainEpoch
}

func (a *StorageMinerActor) DeclareTemporaryFaults(rt Runtime, params DeclareTemporaryFaultsParams) *adt.EmptyValue {
	if params.duration <= abi.ChainEpoch(0) {
		rt.Abort(exitcode.ErrIllegalArgument, "non-positive fault duration %v", params.duration)
	}

	storageWeightDescs := a._rtGetStorageWeightDescsForSectors(rt, params.sectorNumbers)
	requiredFee := temporaryFaultFee(storageWeightDescs, params.duration)

	builtin.RT_ConfirmFundsReceiptOrAbort_RefundRemainder(rt, requiredFee)
	_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, requiredFee)
	builtin.RequireSuccess(rt, code, "failed to burn fee")

	effectiveBeginEpoch := rt.CurrEpoch() + indices.StorageMining_DeclaredFaultEffectiveDelay()
	effectiveEndEpoch := effectiveBeginEpoch + params.duration

	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		rt.ValidateImmediateCallerIs(st.Info.Worker)

		for _, sectorNumber := range params.sectorNumbers {
			sectorInfo, found := st.Sectors[sectorNumber]
			if !found || st.IsSectorInTemporaryFault(sectorNumber) {
				continue
			}

			sectorInfo.DeclaredFaultEpoch = rt.CurrEpoch()
			sectorInfo.DeclaredFaultDuration = params.duration
			st.Sectors[sectorNumber] = sectorInfo
		}
		return nil
	})

	// Request deferred Cron invocation to update temporary fault state.
	cronPayload := CronEventPayload{
		EventType: CronEventType_Miner_TempFault,
		Sectors:   params.sectorNumbers,
	}
	// schedule cron event to start marking temp fault at BeginEpoch
	a.enrollCronEvent(rt, effectiveBeginEpoch, &cronPayload)
	// schedule cron event to end marking temp fault at EndEpoch
	a.enrollCronEvent(rt, effectiveEndEpoch, &cronPayload)
	return &adt.EmptyValue{}
}

//////////
// Cron //
//////////

type OnDeferredCronEventParams struct {
	callbackPayload []byte
}

func (a *StorageMinerActor) OnDeferredCronEvent(rt Runtime, params *OnDeferredCronEventParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)

	var payload CronEventPayload
	err := payload.UnmarshalCBOR(bytes.NewReader(params.callbackPayload))
	if err != nil {
		rt.Abort(exitcode.ErrIllegalArgument, "failed to deserialize event payload")
	}

	if payload.EventType == CronEventType_Miner_TempFault {
		for _, sectorNumber := range payload.Sectors {
			a._rtCheckTemporaryFaultEvents(rt, sectorNumber)
		}
	}

	if payload.EventType == CronEventType_Miner_PreCommitExpiry {
		for _, sectorNumber := range payload.Sectors {
			a._rtCheckPreCommitExpiry(rt, sectorNumber)
		}
	}

	if payload.EventType == CronEventType_Miner_SectorExpiry {
		for _, sectorNumber := range payload.Sectors {
			a._rtCheckSectorExpiry(rt, sectorNumber)
		}
	}

	if payload.EventType == CronEventType_Miner_SurpriseExpiration {
		a._rtCheckSurprisePoStExpiry(rt)
	}

	if payload.EventType == CronEventType_Miner_WorkerKeyChange {
		a._rtCommitWorkerKeyChange(rt)
	}

	return &adt.EmptyValue{}
}

////////////////////////////////////////////////////////////////////////////////
// Method utility functions
////////////////////////////////////////////////////////////////////////////////

func (a *StorageMinerActor) _rtCheckTemporaryFaultEvents(rt Runtime, sectorNumber abi.SectorNumber) {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	checkSector, found := st.Sectors[sectorNumber]

	if !found {
		return
	}

	storageWeightDesc := a._rtGetStorageWeightDescForSector(rt, sectorNumber)

	if !st.IsSectorInTemporaryFault(sectorNumber) && rt.CurrEpoch() >= checkSector.EffectiveFaultBeginEpoch() {
		_, code := rt.Send(
			builtin.StoragePowerActorAddr,
			builtin.Method_StoragePowerActor_OnSectorTemporaryFaultEffectiveBegin,
			&storage_power.OnSectorTemporaryFaultEffectiveBeginParams{
				Weight: storageWeightDesc,
			},
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to begin fault")

		rt.State().Transaction(&st, func() interface{} {
			st.FaultSet.Set(uint64(sectorNumber))
			return nil
		})
	}

	if st.IsSectorInTemporaryFault(sectorNumber) && rt.CurrEpoch() >= checkSector.EffectiveFaultEndEpoch() {
		checkSector.DeclaredFaultEpoch = epochUndefined
		checkSector.DeclaredFaultDuration = epochUndefined

		_, code := rt.Send(
			builtin.StoragePowerActorAddr,
			builtin.Method_StoragePowerActor_OnSectorTemporaryFaultEffectiveEnd,
			&storage_power.OnSectorTemporaryFaultEffectiveEndParams{
				Weight: storageWeightDesc,
			},
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to end fault")

		rt.State().Transaction(&st, func() interface{} {
			st.FaultSet.Unset(uint64(sectorNumber))
			return nil
		})
	}

	rt.State().Transaction(&st, func() interface{} {
		st.Sectors[sectorNumber] = checkSector
		return nil
	})
}

func (a *StorageMinerActor) _rtCheckPreCommitExpiry(rt Runtime, sectorNumber abi.SectorNumber) {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	checkSector, found := st.PreCommittedSectors[sectorNumber]

	if !found {
		return
	}

	if rt.CurrEpoch()-checkSector.PreCommitEpoch > indices.StorageMining_MaxProveCommitSectorEpoch() {
		rt.State().Transaction(&st, func() interface{} {
			delete(st.PreCommittedSectors, sectorNumber)
			return nil
		})
		_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, checkSector.PreCommitDeposit)
		builtin.RequireSuccess(rt, code, "failed to burn funds")
	}
	return
}

func (a *StorageMinerActor) _rtCheckSectorExpiry(rt Runtime, sectorNumber abi.SectorNumber) {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	checkSector, found := st.Sectors[sectorNumber]

	if !found {
		return
	}

	// Note: the following test may be false, if sector expiration has been extended by the worker
	// in the interim after the Cron request was enrolled.
	if rt.CurrEpoch() >= checkSector.Info.Expiration {
		a._rtTerminateSector(rt, sectorNumber, builtin.NormalExpiration)
	}
	return
}

func (a *StorageMinerActor) _rtTerminateSector(rt Runtime, sectorNumber abi.SectorNumber, terminationType builtin.SectorTermination) {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	si, found := st.Sectors[sectorNumber]
	Assert(found)

	storageWeightDesc := a._rtGetStorageWeightDescForSector(rt, sectorNumber)

	if st.IsSectorInTemporaryFault(sectorNumber) {
		// To avoid boundary-case errors in power accounting, make sure we explicitly end
		// the temporary fault state first, before terminating the sector.
		_, code := rt.Send(
			builtin.StoragePowerActorAddr,
			builtin.Method_StoragePowerActor_OnSectorTemporaryFaultEffectiveEnd,
			&storage_power.OnSectorTemporaryFaultEffectiveEndParams{
				Weight: storageWeightDesc,
			},
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to end fault")
	}

	{
		_, code := rt.Send(
			builtin.StorageMarketActorAddr,
			builtin.Method_StorageMarketActor_OnMinerSectorsTerminate,
			&storage_market.OnMinerSectorsTerminateParams{
				DealIDs: si.Info.DealIDs,
			},
			abi.NewTokenAmount(0),
		)
		builtin.RequireSuccess(rt, code, "failed to terminate sector with market actor")
	}

	_, code := rt.Send(
		builtin.StoragePowerActorAddr,
		builtin.Method_StoragePowerActor_OnSectorTerminate,
		&storage_power.OnSectorTerminateParams{
			TerminationType: terminationType,
			Weight:          storageWeightDesc,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to terminate sector with power actor")

	a._rtDeleteSectorEntry(rt, sectorNumber)
}

func (a *StorageMinerActor) _rtCheckSurprisePoStExpiry(rt Runtime) {
	rt.ValidateImmediateCallerIs(builtin.StoragePowerActorAddr)

	var st StorageMinerActorState
	rt.State().Readonly(&st)

	if !st.PoStState.Is_Challenged() {
		// Already exited challenged state successfully prior to expiry.
		return
	}

	provingPeriod := indices.StorageMining_SurprisePoStProvingPeriod()
	if rt.CurrEpoch() < st.PoStState.SurpriseChallengeEpoch+provingPeriod {
		// Challenge not yet expired.
		return
	}

	numConsecutiveFailures := st.PoStState.NumConsecutiveFailures + 1

	if numConsecutiveFailures > indices.StoragePower_SurprisePoStMaxConsecutiveFailures() {
		// Terminate all sectors, notify power and market actors to terminate
		// associated storage deals, and reset miner's PoSt state to OK.
		var terminatedSectors []abi.SectorNumber
		for sectorNumber := range st.Sectors {
			terminatedSectors = append(terminatedSectors, sectorNumber)
		}
		a._rtNotifyMarketForTerminatedSectors(rt, terminatedSectors)
	} else {
		// Increment count of consecutive failures, and continue.
		rt.State().Transaction(&st, func() interface{} {
			st.PoStState = MinerPoStState{
				LastSuccessfulPoSt:     st.PoStState.LastSuccessfulPoSt,
				SurpriseChallengeEpoch: epochUndefined,
				ChallengedSectors:      nil,
				NumConsecutiveFailures: numConsecutiveFailures,
			}
			return nil
		})
	}

	_, code := rt.Send(
		builtin.StoragePowerActorAddr,
		builtin.Method_StoragePowerActor_OnMinerSurprisePoStFailure,
		&storage_power.OnMinerSurprisePoStFailureParams{
			NumConsecutiveFailures: numConsecutiveFailures,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to notify power actor")
}

func (a *StorageMinerActor) enrollCronEvent(rt Runtime, eventEpoch abi.ChainEpoch, callbackPayload *CronEventPayload) {
	var payload []byte
	err := callbackPayload.MarshalCBOR(bytes.NewBuffer(payload))
	if err != nil {
		rt.Abort(exitcode.ErrIllegalArgument, "failed to serialize payload: %v", err)
	}
	_, code := rt.Send(
		builtin.StoragePowerActorAddr,
		builtin.Method_StoragePowerActor_OnMinerEnrollCronEvent,
		&storage_power.EnrollCronEventParams{
			EventEpoch: eventEpoch,
			Payload:    payload,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to enroll cron event")
}

func (a *StorageMinerActor) _rtDeleteSectorEntry(rt Runtime, sectorNumber abi.SectorNumber) {
	var st StorageMinerActorState
	rt.State().Transaction(&st, func() interface{} {
		delete(st.Sectors, sectorNumber)
		return nil
	})
}

func (a *StorageMinerActor) _rtGetStorageWeightDescForSector(rt Runtime, sectorNumber abi.SectorNumber) storage_power.SectorStorageWeightDesc {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	return st._getStorageWeightDescForSector(sectorNumber)
}

func (a *StorageMinerActor) _rtGetStorageWeightDescsForSectors(rt Runtime, sectorNumbers []abi.SectorNumber) []storage_power.SectorStorageWeightDesc {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	return st._getStorageWeightDescsForSectors(sectorNumbers)
}

func (a *StorageMinerActor) _rtNotifyMarketForTerminatedSectors(rt Runtime, sectorNumbers []abi.SectorNumber) {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	var dealIds []abi.DealID
	for _, sectorNo := range sectorNumbers {
		dealIds = append(dealIds, st._getSectorDealIDsAssert(sectorNo)...)
	}

	_, code := rt.Send(
		builtin.StorageMarketActorAddr,
		builtin.Method_StorageMarketActor_OnMinerSectorsTerminate,
		&storage_market.OnMinerSectorsTerminateParams{
			DealIDs: dealIds,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to terminate sectors")
}

func (a *StorageMinerActor) verifySurprisePost(rt Runtime, st *StorageMinerActorState, onChainInfo *abi.OnChainSurprisePoStVerifyInfo) {
	Assert(st.PoStState.Is_Challenged())
	sectorSize := st.Info.SectorSize
	challengeEpoch := st.PoStState.SurpriseChallengeEpoch
	challengedSectors := st.PoStState.ChallengedSectors

	// verify no duplicate tickets
	challengeIndices := make(map[int64]bool)
	for _, tix := range onChainInfo.Candidates {
		if _, ok := challengeIndices[tix.ChallengeIndex]; ok {
			rt.Abort(exitcode.ErrIllegalArgument, "Invalid Surprise PoSt. Duplicate ticket included.")
		}
		challengeIndices[tix.ChallengeIndex] = true
	}

	autil.TODO(challengedSectors)
	// TODO HS: Determine what should be the acceptance criterion for sector numbers
	// proven in SurprisePoSt proofs.
	//
	// Previous note:
	// Verify the partialTicket values
	// if !a._rtVerifySurprisePoStMeetsTargetReq(rt) {
	// 	rt.AbortStateMsg("Invalid Surprise PoSt. Tickets do not meet target.")
	// }

	randomnessK := rt.GetRandomness(challengeEpoch - indices.StorageMining_SpcLookbackPoSt())
	// regenerate randomness used. The PoSt Verification below will fail if
	// the same was not used to generate the proof
	postRandomness := crypto.DeriveRandWithMinerAddr(crypto.DomainSeparationTag_SurprisePoStChallengeSeed, randomnessK, rt.CurrReceiver())

	// Get public inputs

	pvInfo := abi.PoStVerifyInfo{
		Candidates: onChainInfo.Candidates,
		Proofs:     onChainInfo.Proofs,
		Randomness: abi.PoStRandomness(postRandomness),
		// EligibleSectors_: FIXME: verification needs these.
	}

	// Verify the PoSt Proof
	isVerified := rt.Syscalls().VerifyPoSt(sectorSize, pvInfo)

	if !isVerified {
		rt.Abort(exitcode.ErrIllegalArgument, "Surprise PoSt failed to verify")
	}
}

func (a *StorageMinerActor) _rtVerifySealOrAbort(rt Runtime, onChainInfo *abi.OnChainSealVerifyInfo) {
	var st StorageMinerActorState
	rt.State().Readonly(&st)
	info := st.Info
	sectorSize := info.SectorSize

	// if IsValidAtCommitEpoch(onChainInfo.RegisteredProof, rt.CurrEpoch()) // Ensure proof type is valid at current epoch.
	// Check randomness.
	if onChainInfo.SealEpoch < (rt.CurrEpoch() - indices.StorageMining_Finality() - indices.StorageMining_MaxSealTime32GiBWinStackedSDR()) {
		rt.Abort(exitcode.ErrIllegalArgument, "Seal references ticket from invalid epoch")
	}

	var infos storage_market.GetPieceInfosForDealIDsReturn
	ret, code := rt.Send(
		builtin.StorageMarketActorAddr,
		builtin.Method_StorageMarketActor_GetPieceInfosForDealIDs,
		&storage_market.GetPieceInfosForDealIDsParams{
			DealIDs: onChainInfo.DealIDs,
		},
		abi.NewTokenAmount(0),
	)
	builtin.RequireSuccess(rt, code, "failed to fetch piece info")
	autil.AssertNoError(ret.Into(&infos))

	// Unless we enforce a minimum padding amount, this totalPieceSize calculation can be removed.
	// Leaving for now until that decision is entirely finalized.
	var totalPieceSize int64
	for _, pieceInfo := range infos.Pieces {
		pieceSize := pieceInfo.Size
		totalPieceSize += pieceSize
	}

	unsealedCID, err := rt.Syscalls().ComputeUnsealedSectorCID(sectorSize, infos.Pieces)
	if err != nil {
		rt.Abort(exitcode.ErrIllegalState, "invalid sector piece infos")
	}

	minerActorID, err := addr.IDFromAddress(rt.CurrReceiver())
	if err != nil {
		rt.Abort(exitcode.ErrIllegalState, "receiver must be ID address")
	}

	svInfoRandomness := rt.GetRandomness(onChainInfo.SealEpoch)
	svInfoInteractiveRandomness := rt.GetRandomness(onChainInfo.InteractiveEpoch)

	svInfo := abi.SealVerifyInfo{
		SectorID: abi.SectorID{
			Miner:  abi.ActorID(minerActorID),
			Number: onChainInfo.SectorNumber,
		},
		OnChain:               *onChainInfo,
		Randomness:            abi.SealRandomness(svInfoRandomness),
		InteractiveRandomness: abi.InteractiveSealRandomness(svInfoInteractiveRandomness),
		UnsealedCID:           unsealedCID,
	}

	isVerified := rt.Syscalls().VerifySeal(sectorSize, svInfo)

	if !isVerified {
		rt.Abort(exitcode.ErrIllegalState, "Sector seal failed to verify")
	}
}

func _surprisePoStSampleChallengedSectors(sampleRandomness abi.Randomness, provingSet cid.Cid) []abi.SectorNumber {
	// TODO: HS
	autil.TODO()
	panic("")
}
