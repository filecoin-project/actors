package market

import (
	"bytes"
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	xerrors "golang.org/x/xerrors"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	. "github.com/filecoin-project/specs-actors/actors/util"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

const epochUndefined = abi.ChainEpoch(-1)

// Market mutations
// add / rm balance
// pub deal (always provider)
// activate deal (miner)
// end deal (miner terminate, expire(no activation))

// BalanceLockingReason is the reason behind locking an amount.
type BalanceLockingReason int

const (
	ClientCollateral BalanceLockingReason = iota
	ClientStorageFee
	ProviderCollateral
)

type State struct {
	Proposals cid.Cid // AMT[DealID]DealProposal
	States    cid.Cid // AMT[DealID]DealState

	// PendingProposals tracks proposals that have not yet reached their deal start date.
	// We track them here to ensure that miners can't publish the same deal proposal twice
	PendingProposals cid.Cid // HAMT[DealCid]DealProposal

	// Total amount held in escrow, indexed by actor address (including both locked and unlocked amounts).
	EscrowTable cid.Cid // BalanceTable

	// Amount locked, indexed by actor address.
	// Note: the amounts in this table do not affect the overall amount in escrow:
	// only the _portion_ of the total escrow amount that is locked.
	LockedTable cid.Cid // BalanceTable

	NextID abi.DealID

	// Metadata cached for efficient iteration over deals.
	DealOpsByEpoch cid.Cid // SetMultimap, HAMT[epoch]Set
	LastCron       abi.ChainEpoch

	// Total Client Collateral that is locked -> unlocked when deal is terminated
	TotalClientLockedCollateral abi.TokenAmount
	// Total Provider Collateral that is locked -> unlocked when deal is terminated
	TotalProviderLockedCollateral abi.TokenAmount
	// Total storage fee that is locked in escrow -> unlocked when payments are made
	TotalClientStorageFee abi.TokenAmount
}

func ConstructState(emptyArrayCid, emptyMapCid, emptyMSetCid cid.Cid) *State {
	return &State{
		Proposals:        emptyArrayCid,
		States:           emptyArrayCid,
		PendingProposals: emptyMapCid,
		EscrowTable:      emptyMapCid,
		LockedTable:      emptyMapCid,
		NextID:           abi.DealID(0),
		DealOpsByEpoch:   emptyMSetCid,
		LastCron:         abi.ChainEpoch(-1),

		TotalClientLockedCollateral:   abi.NewTokenAmount(0),
		TotalProviderLockedCollateral: abi.NewTokenAmount(0),
		TotalClientStorageFee:         abi.NewTokenAmount(0),
	}
}

////////////////////////////////////////////////////////////////////////////////
// Deal state operations
////////////////////////////////////////////////////////////////////////////////

func (st *State) updatePendingDealState(rt Runtime, state *DealState, deal *DealProposal, dealID abi.DealID, et, lt *adt.BalanceTable, epoch abi.ChainEpoch) (amountSlashed abi.TokenAmount,
	nextEpoch abi.ChainEpoch, removeDeal bool) {
	amountSlashed = abi.NewTokenAmount(0)

	everUpdated := state.LastUpdatedEpoch != epochUndefined
	everSlashed := state.SlashEpoch != epochUndefined

	Assert(!everUpdated || (state.LastUpdatedEpoch <= epoch)) // if the deal was ever updated, make sure it didn't happen in the future

	// This would be the case that the first callback somehow triggers before it is scheduled to
	// This is expected not to be able to happen
	if deal.StartEpoch > epoch {
		return amountSlashed, epochUndefined, false
	}

	paymentEndEpoch := deal.EndEpoch
	if everSlashed {
		AssertMsg(epoch >= state.SlashEpoch, "current epoch less than slash epoch")
		Assert(state.SlashEpoch <= deal.EndEpoch)
		paymentEndEpoch = state.SlashEpoch
	} else if epoch < paymentEndEpoch {
		paymentEndEpoch = epoch
	}

	paymentStartEpoch := deal.StartEpoch
	if everUpdated && state.LastUpdatedEpoch > paymentStartEpoch {
		paymentStartEpoch = state.LastUpdatedEpoch
	}

	numEpochsElapsed := paymentEndEpoch - paymentStartEpoch

	{
		// Process deal payment for the elapsed epochs.
		totalPayment := big.Mul(big.NewInt(int64(numEpochsElapsed)), deal.StoragePricePerEpoch)

		// the transfer amount can be less than or equal to zero if a deal is slashed before or at the deal's start epoch.
		if totalPayment.GreaterThan(big.Zero()) {
			st.transferBalance(rt, deal.Client, deal.Provider, totalPayment, et, lt)
		}
	}

	if everSlashed {
		// unlock client collateral and locked storage fee
		paymentRemaining := dealGetPaymentRemaining(deal, state.SlashEpoch)

		// unlock remaining storage fee
		if err := st.unlockBalance(lt, deal.Client, paymentRemaining, ClientStorageFee); err != nil {
			rt.Abortf(exitcode.ErrIllegalState, "failed to unlock remaining client storage fee: %s", err)
		}
		// unlock client collateral
		if err := st.unlockBalance(lt, deal.Client, deal.ClientCollateral, ClientCollateral); err != nil {
			rt.Abortf(exitcode.ErrIllegalState, "failed to unlock client collateral: %s", err)
		}

		// slash provider collateral
		amountSlashed = deal.ProviderCollateral
		if err := st.slashBalance(et, lt, deal.Provider, amountSlashed, ProviderCollateral); err != nil {
			rt.Abortf(exitcode.ErrIllegalState, "slashing balance: %s", err)
		}

		return amountSlashed, epochUndefined, true
	}

	if epoch >= deal.EndEpoch {
		st.processDealExpired(rt, deal, state, lt, dealID)
		return amountSlashed, epochUndefined, true
	}

	nextEpoch = epoch + DealUpdatesInterval
	if nextEpoch > deal.EndEpoch {
		nextEpoch = deal.EndEpoch
	}

	return amountSlashed, nextEpoch, false
}

// Deal start deadline elapsed without appearing in a proven sector.
// Delete deal, slash a portion of provider's collateral, and unlock remaining collaterals
// for both provider and client.
func (st *State) processDealInitTimedOut(rt Runtime, et, lt *adt.BalanceTable, deal *DealProposal) abi.TokenAmount {
	if err := st.unlockBalance(lt, deal.Client, deal.TotalStorageFee(), ClientStorageFee); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failure unlocking client storage fee: %s", err)
	}
	if err := st.unlockBalance(lt, deal.Client, deal.ClientCollateral, ClientCollateral); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failure unlocking client collateral: %s", err)
	}

	amountSlashed := collateralPenaltyForDealActivationMissed(deal.ProviderCollateral)
	amountRemaining := big.Sub(deal.ProviderBalanceRequirement(), amountSlashed)

	if err := st.slashBalance(et, lt, deal.Provider, amountSlashed, ProviderCollateral); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to slash balance: %s", err)
	}

	if err := st.unlockBalance(lt, deal.Provider, amountRemaining, ProviderCollateral); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed to unlock deal provider balance: %s", err)
	}

	return amountSlashed
}

// Normal expiration. Delete deal and unlock collaterals for both miner and client.
func (st *State) processDealExpired(rt Runtime, deal *DealProposal, state *DealState, lt *adt.BalanceTable, dealID abi.DealID) {
	Assert(state.SectorStartEpoch != epochUndefined)

	// Note: payment has already been completed at this point (_rtProcessDealPaymentEpochsElapsed)
	if err := st.unlockBalance(lt, deal.Provider, deal.ProviderCollateral, ProviderCollateral); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed unlocking deal provider balance: %s", err)
	}

	if err := st.unlockBalance(lt, deal.Client, deal.ClientCollateral, ClientCollateral); err != nil {
		rt.Abortf(exitcode.ErrIllegalState, "failed unlocking deal client balance: %s", err)
	}
}

func (st *State) generateStorageDealID() abi.DealID {
	ret := st.NextID
	st.NextID = st.NextID + abi.DealID(1)
	return ret
}

type dealWithId struct {
	id       abi.DealID
	proposal DealProposal
}

func (st *State) putDealProposalsAndFlush(store adt.Store, deals ...*dealWithId) error {
	proposals, err := AsDealProposalArray(store, st.Proposals)
	if err != nil {
		return fmt.Errorf("failed to load proposals: %w", err)
	}

	for i := range deals {
		d := deals[i]
		if err := proposals.Set(d.id, &d.proposal); err != nil {
			return fmt.Errorf("failed to set proposal %v, err: %w", *d, err)
		}
	}

	st.Proposals, err = proposals.Root()
	return err
}

func (st *State) putDealOpsAndFlush(store adt.Store, deals ...*dealWithId) error {
	dealOps, err := AsSetMultimap(store, st.DealOpsByEpoch)
	if err != nil {
		return fmt.Errorf("failed to load deal ops: %w", err)
	}

	for i := range deals {
		d := deals[i]
		if err := dealOps.Put(d.proposal.StartEpoch, d.id); err != nil {
			return fmt.Errorf("failed to put deal ops %v, err: %w", *d, err)
		}
	}

	st.DealOpsByEpoch, err = dealOps.Root()
	return err
}

////////////////////////////////////////////////////////////////////////////////
// Method utility functions
////////////////////////////////////////////////////////////////////////////////

func (st *State) mustGetDeal(rt Runtime, proposals *DealArray, dealID abi.DealID) *DealProposal {
	proposal, found, err := proposals.Get(dealID)
	builtin.RequireNoErr(rt, err, exitcode.ErrIllegalState, "failed to load proposal for dealId %d", dealID)
	if !found {
		rt.Abortf(exitcode.ErrIllegalState, "dealId %d not found", dealID)
	}

	return proposal
}

////////////////////////////////////////////////////////////////////////////////
// State utility functions
////////////////////////////////////////////////////////////////////////////////

func dealProposalIsInternallyValid(rt Runtime, proposal ClientDealProposal) error {
	// Note: we do not verify the provider signature here, since this is implicit in the
	// authenticity of the on-chain message publishing the deal.
	buf := bytes.Buffer{}
	err := proposal.Proposal.MarshalCBOR(&buf)
	if err != nil {
		return xerrors.Errorf("proposal signature verification failed to marshal proposal: %w", err)
	}
	err = rt.Syscalls().VerifySignature(proposal.ClientSignature, proposal.Proposal.Client, buf.Bytes())
	if err != nil {
		return xerrors.Errorf("signature proposal invalid: %w", err)
	}
	return nil
}

func dealGetPaymentRemaining(deal *DealProposal, slashEpoch abi.ChainEpoch) abi.TokenAmount {
	Assert(slashEpoch <= deal.EndEpoch)

	// Payments are always for start -> end epoch irrespective of when the deal is slashed.
	if slashEpoch < deal.StartEpoch {
		slashEpoch = deal.StartEpoch
	}

	durationRemaining := deal.EndEpoch - slashEpoch
	Assert(durationRemaining > 0)

	return big.Mul(big.NewInt(int64(durationRemaining)), deal.StoragePricePerEpoch)
}
