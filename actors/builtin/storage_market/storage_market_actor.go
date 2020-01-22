package storage_market

import (
	addr "github.com/filecoin-project/go-address"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	cid "github.com/ipfs/go-cid"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	actor_util "github.com/filecoin-project/specs-actors/actors/util"
)

type DealWeight int64

type StorageMarketActor struct{}

func (a *StorageMarketActor) State(rt Runtime) (vmr.ActorStateHandle, StorageMarketActorState) {
	h := rt.AcquireState()
	var state StorageMarketActorState
	stateCID := cid.Cid(h.Take())
	if !rt.IpldGet(stateCID, &state) {
		rt.Abort(exitcode.ErrPlaceholder, "state not found")
	}
	return h, state
}

////////////////////////////////////////////////////////////////////////////////
// Actor methods
////////////////////////////////////////////////////////////////////////////////

// Attempt to withdraw the specified amount from the balance held in escrow.
// If less than the specified amount is available, yields the entire available balance.
func (a *StorageMarketActor) WithdrawBalance(rt Runtime, entryAddr addr.Address, amountRequested abi.TokenAmount) *vmr.EmptyReturn {
	IMPL_FINISH() // BigInt arithmetic
	amountSlashedTotal := abi.NewTokenAmount(0)

	if amountRequested.LessThan(big.Zero()) {
		rt.Abort(exitcode.ErrIllegalArgument, "negative amount %v", amountRequested)
	}

	recipientAddr := vmr.RT_MinerEntry_ValidateCaller_DetermineFundsLocation(rt, entryAddr, vmr.MinerEntrySpec_MinerOrSignable)

	h, st := a.State(rt)
	st._rtAbortIfAddressEntryDoesNotExist(rt, entryAddr)

	// Before any operations that check the balance tables for funds, execute all deferred
	// deal state updates.
	//
	// Note: as an optimization, implementations may cache efficient data structures indicating
	// which of the following set of updates are redundant and can be skipped.
	amountSlashedTotal = big.Add(amountSlashedTotal, st._rtUpdatePendingDealStatesForParty(rt, entryAddr))

	minBalance := st._getLockedReqBalanceInternal(entryAddr)
	newTable, amountExtracted, ok := actor_util.BalanceTable_WithExtractPartial(
		st.EscrowTable, entryAddr, amountRequested, minBalance)
	Assert(ok)
	st.EscrowTable = newTable

	UpdateRelease(rt, h, st)

	_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashedTotal)
	vmr.RequireSuccess(rt, code, "failed to burn slashed funds")
	_, code = rt.Send(recipientAddr, builtin.MethodSend, nil, amountExtracted)
	vmr.RequireSuccess(rt, code, "failed to send funds")
	return &vmr.EmptyReturn{}
}

// Deposits the specified amount into the balance held in escrow.
// Note: the amount is included implicitly in the message.
func (a *StorageMarketActor) AddBalance(rt Runtime, entryAddr addr.Address) *vmr.EmptyReturn {
	vmr.RT_MinerEntry_ValidateCaller_DetermineFundsLocation(rt, entryAddr, vmr.MinerEntrySpec_MinerOrSignable)

	h, st := a.State(rt)
	st._rtAbortIfAddressEntryDoesNotExist(rt, entryAddr)

	msgValue := rt.ValueReceived()
	newTable, ok := actor_util.BalanceTable_WithAdd(st.EscrowTable, entryAddr, msgValue)
	if !ok {
		// Entry not found; create implicitly.
		newTable, ok = actor_util.BalanceTable_WithNewAddressEntry(st.EscrowTable, entryAddr, msgValue)
		Assert(ok)
	}
	st.EscrowTable = newTable

	UpdateRelease(rt, h, st)
	return &vmr.EmptyReturn{}
}

// Publish a new set of storage deals (not yet included in a sector).
func (a *StorageMarketActor) PublishStorageDeals(rt Runtime, newStorageDeals []StorageDeal) *vmr.EmptyReturn {
	IMPL_FINISH() // BigInt arithmetic
	amountSlashedTotal := abi.NewTokenAmount(0)

	// Deal message must have a From field identical to the provider of all the deals.
	// This allows us to retain and verify only the client's signature in each deal proposal itself.
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)

	h, st := a.State(rt)

	// All storage deals will be added in an atomic transaction; this operation will be unrolled if any of them fails.
	for _, newDeal := range newStorageDeals {
		p := newDeal.Proposal

		if p.Provider != rt.ImmediateCaller() {
			rt.Abort(exitcode.ErrForbidden, "caller is not provider %v", p.Provider)
		}

		_rtAbortIfNewDealInvalid(rt, newDeal)

		// Before any operations that check the balance tables for funds, execute all deferred
		// deal state updates.
		//
		// Note: as an optimization, implementations may cache efficient data structures indicating
		// which of the following set of updates are redundant and can be skipped.
		amountSlashedTotal = big.Add(amountSlashedTotal, st._rtUpdatePendingDealStatesForParty(rt, p.Client))
		amountSlashedTotal = big.Add(amountSlashedTotal, st._rtUpdatePendingDealStatesForParty(rt, p.Provider))

		st._rtLockBalanceOrAbort(rt, p.Client, p.ClientBalanceRequirement())
		st._rtLockBalanceOrAbort(rt, p.Provider, p.ProviderBalanceRequirement())

		id := st._generateStorageDealID(newDeal)

		onchainDeal := OnChainDeal{
			ID:               id,
			Deal:             newDeal,
			SectorStartEpoch: epochUndefined,
		}

		st.Deals[id] = onchainDeal

		if _, found := st.CachedExpirationsPending[p.EndEpoch]; !found {
			st.CachedExpirationsPending[p.EndEpoch] = actor_util.DealIDQueue_Empty()
		}
		cep := st.CachedExpirationsPending[p.EndEpoch]
		cep.Enqueue(id)
	}

	st.CurrEpochNumDealsPublished += len(newStorageDeals)

	UpdateRelease(rt, h, st)

	_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashedTotal)
	vmr.RequireSuccess(rt, code, "failed to burn funds")
	return &vmr.EmptyReturn{}
}

// Verify that a given set of storage deals is valid for a sector currently being PreCommitted.
// Note: in the case of a capacity-commitment sector (one with zero deals), this function should succeed vacuously.
func (a *StorageMarketActor) VerifyDealsOnSectorPreCommit(rt Runtime, dealIDs abi.DealIDs, sectorExpiry abi.ChainEpoch) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	h, st := a.State(rt)

	for _, dealID := range dealIDs.Items {
		deal, _ := st._rtGetOnChainDealOrAbort(rt, dealID)
		_rtAbortIfDealInvalidForNewSectorSeal(rt, minerAddr, sectorExpiry, deal)
	}

	Release(rt, h, st)
	return &vmr.EmptyReturn{}
}

// Verify that a given set of storage deals is valid for a sector currently being ProveCommitted,
// and update the market's internal state accordingly.
func (a *StorageMarketActor) UpdateDealsOnSectorProveCommit(rt Runtime, dealIDs abi.DealIDs, sectorExpiry abi.ChainEpoch) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	h, st := a.State(rt)

	for _, dealID := range dealIDs.Items {
		deal, _ := st._rtGetOnChainDealOrAbort(rt, dealID)
		_rtAbortIfDealInvalidForNewSectorSeal(rt, minerAddr, sectorExpiry, deal)
		ocd := st.Deals[dealID]
		ocd.SectorStartEpoch = rt.CurrEpoch()
		st.Deals[dealID] = ocd
	}

	UpdateRelease(rt, h, st)
	return &vmr.EmptyReturn{}
}

type GetPieceInfosForDealIDsReturn struct {
	Pieces []abi.PieceInfo
}

func (a *StorageMarketActor) GetPieceInfosForDealIDs(rt Runtime, dealIDs abi.DealIDs) *GetPieceInfosForDealIDsReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)

	ret := []abi.PieceInfo{}

	h, st := a.State(rt)

	for _, dealID := range dealIDs.Items {
		_, dealP := st._rtGetOnChainDealOrAbort(rt, dealID)
		ret = append(ret, abi.PieceInfo{
			PieceCID: dealP.PieceCID,
			Size:     dealP.PieceSize.Total(),
		})
	}

	Release(rt, h, st)

	return &GetPieceInfosForDealIDsReturn{Pieces: ret}
}

type GetWeightForDealSetReturn struct {
	Weight abi.DealWeight
}

// Get the weight for a given set of storage deals.
// The weight is defined as the sum, over all deals in the set, of the product of its size
// with its duration. This quantity may be an input into the functions specifying block reward,
// sector power, collateral, and/or other parameters.
func (a *StorageMarketActor) GetWeightForDealSet(rt Runtime, dealIDs abi.DealIDs) *GetWeightForDealSetReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()
	totalWeight := big.Zero()

	h, st := a.State(rt)
	for _, dealID := range dealIDs.Items {
		_, dealP := st._getOnChainDealAssert(dealID)
		Assert(dealP.Provider == minerAddr)

		dur := big.NewInt(int64(dealP.Duration()))
		siz := big.NewInt(dealP.PieceSize.Total())
		weight := big.Mul(dur, siz)
		totalWeight = big.Add(totalWeight, weight)
	}
	UpdateRelease(rt, h, st)

	return &GetWeightForDealSetReturn{abi.DealWeight(totalWeight)}
}

// Terminate a set of deals in response to their containing sector being terminated.
// Slash provider collateral, refund client collateral, and refund partial unpaid escrow
// amount to client.
func (a *StorageMarketActor) OnMinerSectorsTerminate(rt Runtime, dealIDs abi.DealIDs) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.StorageMinerActorCodeID)
	minerAddr := rt.ImmediateCaller()

	h, st := a.State(rt)

	for _, dealID := range dealIDs.Items {
		_, dealP := st._rtGetOnChainDealOrAbort(rt, dealID)
		Assert(dealP.Provider == minerAddr)

		// Note: we do not perform the balance transfers here, but rather simply record the flag
		// to indicate that _processDealSlashed should be called when the deferred state computation
		// is performed.
		ocd := st.Deals[dealID]
		ocd.SlashEpoch = rt.CurrEpoch()
		st.Deals[dealID] = ocd
	}

	UpdateRelease(rt, h, st)
	return &vmr.EmptyReturn{}
}

func (a *StorageMarketActor) OnEpochTickEnd(rt Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerIs(builtin.CronActorAddr)

	h, st := a.State(rt)

	// Some deals may never be affected by the normal calls to _rtUpdatePendingDealStatesForParty
	// (notably, if the relevant party never checks its balance).
	// Without some cleanup mechanism, these deals may gradually accumulate and cause
	// the StorageMarketActor state to grow without bound.
	// To prevent this, we amortize the cost of this cleanup by processing a relatively
	// small number of deals every epoch, independent of the calls above.
	//
	// More specifically, we process deals:
	//   (a) In priority order of expiration epoch, up until the current epoch
	//   (b) Within a given expiration epoch, in order of original publishing.
	//
	// We stop once we have exhausted this valid set, or when we have hit a certain target
	// (DEAL_PROC_AMORTIZED_SCALE_FACTOR times the number of deals freshly published in the
	// current epoch) of deals dequeued, whichever comes first.

	const DEAL_PROC_AMORTIZED_SCALE_FACTOR = 2
	numDequeuedTarget := st.CurrEpochNumDealsPublished * DEAL_PROC_AMORTIZED_SCALE_FACTOR

	numDequeued := 0
	extractedDealIDs := []abi.DealID{}

	for {
		if st.CachedExpirationsNextProcEpoch > rt.CurrEpoch() {
			break
		}

		if numDequeued >= numDequeuedTarget {
			break
		}

		queue, found := st.CachedExpirationsPending[st.CachedExpirationsNextProcEpoch]
		if !found {
			st.CachedExpirationsNextProcEpoch += 1
			continue
		}

		queueDepleted := false
		for {
			dealID, ok := queue.Dequeue()
			if !ok {
				queueDepleted = true
				break
			}
			numDequeued += 1
			if _, found := st.Deals[dealID]; found {
				// May have already processed expiration, independently, via _rtUpdatePendingDealStatesForParty.
				// If not, add it to the list to be processed.
				extractedDealIDs = append(extractedDealIDs, dealID)
			}
		}

		if !queueDepleted {
			Assert(numDequeued >= numDequeuedTarget)
			break
		}

		delete(st.CachedExpirationsPending, st.CachedExpirationsNextProcEpoch)
		st.CachedExpirationsNextProcEpoch += 1
	}

	amountSlashedTotal := st._updatePendingDealStates(extractedDealIDs, rt.CurrEpoch())

	// Reset for next epoch.
	st.CurrEpochNumDealsPublished = 0

	UpdateRelease(rt, h, st)

	_, code := rt.Send(builtin.BurntFundsActorAddr, builtin.MethodSend, nil, amountSlashedTotal)
	vmr.RequireSuccess(rt, code, "failed to burn funds")
	return &vmr.EmptyReturn{}
}

func (a *StorageMarketActor) Constructor(rt Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)
	h := rt.AcquireState()

	st := StorageMarketActorState{
		Deals:                          DealsAMT_Empty(),
		EscrowTable:                    actor_util.BalanceTableHAMT_Empty(),
		LockedReqTable:                 actor_util.BalanceTableHAMT_Empty(),
		NextID:                         abi.DealID(0),
		CachedDealIDsByParty:           CachedDealIDsByPartyHAMT_Empty(),
		CachedExpirationsPending:       CachedExpirationsPendingHAMT_Empty(),
		CachedExpirationsNextProcEpoch: abi.ChainEpoch(0),
	}

	UpdateRelease(rt, h, st)
	return &vmr.EmptyReturn{}
}

////////////////////////////////////////////////////////////////////////////////
// Method utility functions
////////////////////////////////////////////////////////////////////////////////

func _rtAbortIfDealAlreadyProven(rt Runtime, deal OnChainDeal) {
	if deal.SectorStartEpoch != epochUndefined {
		rt.AbortStateMsg("Deal has already appeared in proven sector.")
	}
}

func _rtAbortIfDealNotFromProvider(rt Runtime, dealP StorageDealProposal, minerAddr addr.Address) {
	if dealP.Provider != minerAddr {
		rt.AbortStateMsg("Deal has incorrect miner as its provider.")
	}
}

func _rtAbortIfDealStartElapsed(rt Runtime, dealP StorageDealProposal) {
	if rt.CurrEpoch() > dealP.StartEpoch {
		rt.AbortStateMsg("Deal start epoch has already elapsed.")
	}
}

func _rtAbortIfDealEndElapsed(rt Runtime, dealP StorageDealProposal) {
	if dealP.EndEpoch > rt.CurrEpoch() {
		rt.AbortStateMsg("Deal end epoch has already elapsed.")
	}
}

func _rtAbortIfDealExceedsSectorLifetime(rt Runtime, dealP StorageDealProposal, sectorExpiration abi.ChainEpoch) {
	if dealP.EndEpoch > sectorExpiration {
		rt.AbortStateMsg("Deal would outlive its containing sector.")
	}
}

func _rtAbortIfDealInvalidForNewSectorSeal(
	rt Runtime, minerAddr addr.Address, sectorExpiration abi.ChainEpoch, deal OnChainDeal) {

	dealP := deal.Deal.Proposal

	_rtAbortIfDealNotFromProvider(rt, dealP, minerAddr)
	_rtAbortIfDealAlreadyProven(rt, deal)
	_rtAbortIfDealStartElapsed(rt, dealP)
	_rtAbortIfDealExceedsSectorLifetime(rt, dealP, sectorExpiration)
}

func _rtAbortIfNewDealInvalid(rt Runtime, deal StorageDeal) {
	dealP := deal.Proposal

	if !_rtDealProposalIsInternallyValid(rt, dealP) {
		rt.AbortStateMsg("Invalid deal proposal.")
	}

	_rtAbortIfDealStartElapsed(rt, dealP)
	_rtAbortIfDealFailsParamBounds(rt, dealP)
}

func _rtAbortIfDealFailsParamBounds(rt Runtime, dealP StorageDealProposal) {
	inds := rt.CurrIndices()

	minDuration, maxDuration := inds.StorageDeal_DurationBounds(dealP.PieceSize, dealP.StartEpoch)
	if dealP.Duration() < minDuration || dealP.Duration() > maxDuration {
		rt.AbortStateMsg("Deal duration out of bounds.")
	}

	minPrice, maxPrice := inds.StorageDeal_StoragePricePerEpochBounds(dealP.PieceSize, dealP.StartEpoch, dealP.EndEpoch)
	if dealP.StoragePricePerEpoch.LessThan(minPrice) || dealP.StoragePricePerEpoch.GreaterThan(maxPrice) {
		rt.AbortStateMsg("Storage price out of bounds.")
	}

	minProviderCollateral, maxProviderCollateral := inds.StorageDeal_ProviderCollateralBounds(
		dealP.PieceSize, dealP.StartEpoch, dealP.EndEpoch)
	if dealP.ProviderCollateral.LessThan(minProviderCollateral) || dealP.ProviderCollateral.GreaterThan(maxProviderCollateral) {
		rt.AbortStateMsg("Provider collateral out of bounds.")
	}

	minClientCollateral, maxClientCollateral := inds.StorageDeal_ClientCollateralBounds(
		dealP.PieceSize, dealP.StartEpoch, dealP.EndEpoch)
	if dealP.ClientCollateral.LessThan(minClientCollateral) || dealP.ClientCollateral.GreaterThan(maxClientCollateral) {
		rt.AbortStateMsg("Client collateral out of bounds.")
	}
}
