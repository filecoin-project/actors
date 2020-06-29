package multisig

import (
	"encoding/binary"

	addr "github.com/filecoin-project/go-address"
	cbg "github.com/whyrusleeping/cbor-gen"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type TxnID int64

func (t TxnID) Key() string {
	// convert a TxnID to a HAMT key.
	txnKey := make([]byte, binary.MaxVarintLen64)
	n := binary.PutVarint(txnKey, int64(t))
	return string(txnKey[:n])
}

type Transaction struct {
	To     addr.Address
	Value  abi.TokenAmount
	Method abi.MethodNum
	Params []byte

	// This address at index 0 is the transaction proposer, order of this slice must be preserved.
	Approved []addr.Address
}

type Actor struct{}

func (a Actor) Exports() []interface{} {
	return []interface{}{
		builtin.MethodConstructor: a.Constructor,
		2:                         a.Propose,
		3:                         a.Approve,
		4:                         a.Cancel,
		//5: msa.ClearCompleted, //TODO: sync with spec
		6: a.AddSigner,
		7: a.RemoveSigner,
		8: a.SwapSigner,
		9: a.ChangeNumApprovalsThreshold,
	}
}

var _ abi.Invokee = Actor{}

type ConstructorParams struct {
	Signers               []addr.Address
	NumApprovalsThreshold int64
	UnlockDuration        abi.ChainEpoch
}

func (a Actor) Constructor(rt vmr.Runtime, params *ConstructorParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerIs(builtin.InitActorAddr)

	var signers []addr.Address
	for _, sa := range params.Signers {
		signers = append(signers, sa)
	}

	rt.State().Construct(func() vmr.CBORMarshaler {
		pending, err := adt.MakeEmptyMap(adt.AsStore(rt))
		if err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to create empty map: %v", err)
		}

		var st State
		st.Signers = signers
		st.NumApprovalsThreshold = params.NumApprovalsThreshold
		st.PendingTxns = pending.Root()
		st.InitialBalance = abi.NewTokenAmount(0)
		if params.UnlockDuration != 0 {
			st.InitialBalance = rt.Message().ValueReceived()
			st.UnlockDuration = params.UnlockDuration
			st.StartEpoch = rt.CurrEpoch()
		}
		return &st
	})
	return &adt.EmptyValue{}
}

type ProposeParams struct {
	To     addr.Address
	Value  abi.TokenAmount
	Method abi.MethodNum
	Params []byte
}

func (a Actor) Propose(rt vmr.Runtime, params *ProposeParams) *cbg.CborInt {
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	callerAddr := rt.Message().Caller()

	var txnID TxnID
	var st State
	rt.State().Transaction(&st, func() interface{} {
		a.validateSigner(rt, &st, callerAddr)
		txnID = st.NextTxnID
		st.NextTxnID += 1

		if err := st.putPendingTransaction(adt.AsStore(rt), txnID, Transaction{
			To:       params.To,
			Value:    params.Value,
			Method:   params.Method,
			Params:   params.Params,
			Approved: []addr.Address{},
		}); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to put transaction for propose: %v", err)
		}
		return nil
	})

	// Proposal implicitly includes approval of a transaction.
	a.approveTransaction(rt, txnID)

	// Note: this ID may not be stable across chain re-orgs.
	// https://github.com/filecoin-project/specs-actors/issues/7

	v := cbg.CborInt(txnID)
	return &v
}

type TxnIDParams struct {
	ID TxnID
}

func (a Actor) Approve(rt vmr.Runtime, params *TxnIDParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	callerAddr := rt.Message().Caller()
	var st State
	rt.State().Transaction(&st, func() interface{} {
		a.validateSigner(rt, &st, callerAddr)
		return nil
	})
	a.approveTransaction(rt, params.ID)
	return &adt.EmptyValue{}
}

func (a Actor) Cancel(rt vmr.Runtime, params *TxnIDParams) *adt.EmptyValue {
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	callerAddr := rt.Message().Caller()

	var st State
	rt.State().Transaction(&st, func() interface{} {
		a.validateSigner(rt, &st, callerAddr)
		txn, err := st.getPendingTransaction(adt.AsStore(rt), params.ID)
		if err != nil {
			rt.Abort(exitcode.ErrNotFound, "failed to get transaction for cancel: %v", err)
		}
		proposer := txn.Approved[0]
		if proposer != callerAddr {
			rt.Abort(exitcode.ErrForbidden, "Cannot cancel another signers transaction")
		}

		if err = st.deletePendingTransaction(adt.AsStore(rt), params.ID); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to delete transaction for cancel: %v", err)
		}
		return nil
	})
	return &adt.EmptyValue{}
}

type AddSignerParams struct {
	Signer   addr.Address // must be an ID protocol address.
	Increase bool
}

func (a Actor) AddSigner(rt vmr.Runtime, params *AddSignerParams) *adt.EmptyValue {
	// Can only be called by the multisig wallet itself.
	rt.ValidateImmediateCallerIs(rt.Message().Receiver())

	var st State
	rt.State().Transaction(&st, func() interface{} {
		if st.isSigner(params.Signer) {
			rt.Abort(exitcode.ErrIllegalArgument, "party is already a signer")
		}
		st.Signers = append(st.Signers, params.Signer)
		if params.Increase {
			st.NumApprovalsThreshold = st.NumApprovalsThreshold + 1
		}
		return nil
	})
	return &adt.EmptyValue{}
}

type RemoveSignerParams struct {
	Signer   addr.Address // must be an ID protocol address.
	Decrease bool
}

func (a Actor) RemoveSigner(rt vmr.Runtime, params *RemoveSignerParams) *adt.EmptyValue {
	// Can only be called by the multisig wallet itself.
	rt.ValidateImmediateCallerIs(rt.Message().Receiver())

	var st State
	rt.State().Transaction(&st, func() interface{} {
		if !st.isSigner(params.Signer) {
			rt.Abort(exitcode.ErrNotFound, "Party not found")
		}

		newSigners := make([]addr.Address, 0, len(st.Signers))
		for _, s := range st.Signers {
			if s != params.Signer {
				newSigners = append(newSigners, s)
			}
		}
		if params.Decrease || int64(len(st.Signers)-1) < st.NumApprovalsThreshold {
			st.NumApprovalsThreshold = st.NumApprovalsThreshold - 1
		}
		st.Signers = newSigners
		return nil
	})

	return &adt.EmptyValue{}
}

type SwapSignerParams struct {
	From addr.Address
	To   addr.Address
}

func (a Actor) SwapSigner(rt vmr.Runtime, params *SwapSignerParams) *adt.EmptyValue {
	// Can only be called by the multisig wallet itself.
	rt.ValidateImmediateCallerIs(rt.Message().Receiver())

	var st State
	rt.State().Transaction(&st, func() interface{} {
		if !st.isSigner(params.From) {
			rt.Abort(exitcode.ErrNotFound, "Party not found")
		}

		if st.isSigner(params.To) {
			rt.Abort(exitcode.ErrIllegalArgument, "Party already present")
		}

		newSigners := make([]addr.Address, 0, len(st.Signers))
		for _, s := range st.Signers {
			if s != params.From {
				newSigners = append(newSigners, s)
			}
		}
		newSigners = append(newSigners, params.To)
		st.Signers = newSigners
		return nil
	})

	return &adt.EmptyValue{}
}

type ChangeNumApprovalsThresholdParams struct {
	NewThreshold int64
}

func (a Actor) ChangeNumApprovalsThreshold(rt vmr.Runtime, params *ChangeNumApprovalsThresholdParams) *adt.EmptyValue {
	// Can only be called by the multisig wallet itself.
	rt.ValidateImmediateCallerIs(rt.Message().Receiver())

	var st State
	rt.State().Transaction(&st, func() interface{} {
		if params.NewThreshold <= 0 || params.NewThreshold > int64(len(st.Signers)) {
			rt.Abort(exitcode.ErrIllegalArgument, "New threshold value not supported")
		}

		st.NumApprovalsThreshold = params.NewThreshold
		return nil
	})
	return &adt.EmptyValue{}
}

func (a Actor) approveTransaction(rt vmr.Runtime, txnID TxnID) {
	var st State
	var txn Transaction
	rt.State().Transaction(&st, func() interface{} {
		var err error
		txn, err = st.getPendingTransaction(adt.AsStore(rt), txnID)
		if err != nil {
			rt.Abort(exitcode.ErrNotFound, "failed to get transaction for approval: %v", err)
		}
		// abort duplicate approval
		for _, previousApprover := range txn.Approved {
			if previousApprover == rt.Message().Caller() {
				rt.Abort(exitcode.ErrIllegalState, "already approved this message")
			}
		}
		// update approved on the transaction
		txn.Approved = append(txn.Approved, rt.Message().Caller())
		if err = st.putPendingTransaction(adt.AsStore(rt), txnID, txn); err != nil {
			rt.Abort(exitcode.ErrIllegalState, "failed to put transaction for approval: %v", err)
		}
		return nil
	})

	thresholdMet := int64(len(txn.Approved)) >= st.NumApprovalsThreshold
	if thresholdMet {
		if err := st.assertAvailable(rt.CurrentBalance(), txn.Value, rt.CurrEpoch()); err != nil {
			rt.Abort(exitcode.ErrInsufficientFunds, "insufficient funds unlocked: %v", err)
		}

		// A sufficient number of approvals have arrived and sufficient funds have been unlocked: relay the message and delete from pending queue.
		_, code := rt.Send(
			txn.To,
			txn.Method,
			vmr.CBORBytes(txn.Params),
			txn.Value,
		)
		// The exit code is explicitly ignored. It's ok for the subcall to fail.
		_ = code

		// This could be rearranged to happen inside the first state transaction, before the send().
		rt.State().Transaction(&st, func() interface{} {
			if err := st.deletePendingTransaction(adt.AsStore(rt), txnID); err != nil {
				rt.Abort(exitcode.ErrIllegalState, "failed to delete transaction for cleanup: %v", err)
			}
			return nil
		})
	}
}

func (a Actor) validateSigner(rt vmr.Runtime, st *State, address addr.Address) {
	if !st.isSigner(address) {
		rt.Abort(exitcode.ErrForbidden, "party not a signer")
	}
}
