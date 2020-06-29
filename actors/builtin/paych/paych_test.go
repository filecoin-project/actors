package paych_test

import (
	"context"
	"reflect"
	"testing"

	addr "github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	. "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/filecoin-project/specs-actors/support/mock"
	tutil "github.com/filecoin-project/specs-actors/support/testing"
)

func TestPaymentChannelActor_Constructor(t *testing.T) {
	ctx := context.Background()
	actor := pcActorHarness{Actor{}, t}

	paychAddr := tutil.NewIDAddr(t, 100)
	payerAddr := tutil.NewIDAddr(t, 101)
	callerAddr := tutil.NewIDAddr(t, 102)

	t.Run("can create a payment channel actor", func(t *testing.T) {
		builder := mock.NewBuilder(ctx, paychAddr).
			WithCaller(callerAddr, builtin.InitActorCodeID).
			WithActorType(paychAddr, builtin.AccountActorCodeID).
			WithActorType(payerAddr, builtin.AccountActorCodeID)
		rt := builder.Build(t)
		actor.constructAndVerify(t, rt, payerAddr, paychAddr)
	})

	testCases := []struct {
		desc               string
		paymentChannelAddr addr.Address
		callerCode         cid.Cid
		newActorCode       cid.Cid
		payerCode          cid.Cid
		expExitCode        exitcode.ExitCode
	}{
		{"fails if target (to) is not account actor",
			paychAddr,
			builtin.InitActorCodeID,
			builtin.MultisigActorCodeID,
			builtin.AccountActorCodeID,
			exitcode.ErrIllegalArgument,
		}, {"fails if sender (from) is not account actor",
			paychAddr,
			builtin.InitActorCodeID,
			builtin.MultisigActorCodeID,
			builtin.AccountActorCodeID,
			exitcode.ErrIllegalArgument,
		}, {"fails if addr is not ID type",
			tutil.NewSECP256K1Addr(t, "beach blanket babylon"),
			builtin.InitActorCodeID,
			builtin.AccountActorCodeID,
			builtin.AccountActorCodeID,
			exitcode.ErrIllegalArgument,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			builder := mock.NewBuilder(ctx, paychAddr).
				WithCaller(callerAddr, tc.callerCode).
				WithActorType(tc.paymentChannelAddr, tc.newActorCode).
				WithActorType(payerAddr, tc.payerCode)
			rt := builder.Build(t)
			rt.ExpectValidateCallerType(builtin.InitActorCodeID)
			rt.ExpectAbort(tc.expExitCode, func() {
				rt.Call(actor.Constructor, &ConstructorParams{To: tc.paymentChannelAddr})
			})
		})
	}

	t.Run("fails if actor does not exist with: no code for address", func(t *testing.T) {
		builder := mock.NewBuilder(ctx, paychAddr).
			WithCaller(callerAddr, builtin.InitActorCodeID).
			WithActorType(payerAddr, builtin.AccountActorCodeID)
		rt := builder.Build(t)
		rt.ExpectValidateCallerType(builtin.InitActorCodeID)
		rt.ExpectAbort(exitcode.ErrIllegalArgument, func() {
			rt.Call(actor.Constructor, &ConstructorParams{To: paychAddr})
		})
	})
}

func TestPaymentChannelActor_CreateLane(t *testing.T) {
	ctx := context.Background()
	actor := pcActorHarness{Actor{}, t}

	initActorAddr := tutil.NewIDAddr(t, 100)
	paychAddr := tutil.NewIDAddr(t, 101)
	payerAddr := tutil.NewIDAddr(t, 102)
	payChBalance := abi.NewTokenAmount(9)

	sig := &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte("doesn't matter")}

	testCases := []struct {
		desc       string
		targetCode cid.Cid

		balance  int64
		received int64
		epoch    int64

		tl    int64
		lane  uint64
		nonce uint64
		amt   int64

		secretPreimage []byte
		sig            *crypto.Signature
		verifySig      bool
		expExitCode    exitcode.ExitCode
	}{
		{desc: "succeeds", targetCode: builtin.AccountActorCodeID,
			amt: 1, epoch: 1, tl: 1,
			sig: sig, verifySig: true,
			expExitCode: exitcode.Ok},
		{desc: "fails if balance too low", targetCode: builtin.AccountActorCodeID,
			amt: 10, epoch: 1, tl: 1,
			sig: sig, verifySig: true,
			expExitCode: exitcode.ErrIllegalState},
		{desc: "fails if new send balance is negative", targetCode: builtin.AccountActorCodeID,
			amt: -1, epoch: 1, tl: 1,
			sig: sig, verifySig: true,
			expExitCode: exitcode.ErrIllegalState},
		{desc: "fails if signature not valid", targetCode: builtin.AccountActorCodeID,
			amt: 1, epoch: 1, tl: 1,
			sig: nil, verifySig: true,
			expExitCode: exitcode.ErrIllegalArgument},
		{desc: "fails if too early for voucher", targetCode: builtin.AccountActorCodeID,
			amt: 1, epoch: 1, tl: 10,
			sig: sig, verifySig: true,
			expExitCode: exitcode.ErrIllegalArgument},
		{desc: "fails if signature not verified", targetCode: builtin.AccountActorCodeID,
			amt: 1, epoch: 1, tl: 1, sig: sig, verifySig: false,
			expExitCode: exitcode.ErrIllegalArgument},
		{desc: "fails if SigningBytes fails", targetCode: builtin.AccountActorCodeID,
			amt: 1, epoch: 1, tl: 1, sig: sig, verifySig: true,
			secretPreimage: make([]byte, 2<<21),
			expExitCode:    exitcode.ErrIllegalArgument},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			versig := func(sig crypto.Signature, signer addr.Address, plaintext []byte) bool { return tc.verifySig }
			hasher := func(data []byte) [32]byte { return [32]byte{} }

			builder := mock.NewBuilder(ctx, paychAddr).
				WithBalance(payChBalance, abi.NewTokenAmount(tc.received)).
				WithEpoch(abi.ChainEpoch(tc.epoch)).
				WithCaller(initActorAddr, builtin.InitActorCodeID).
				WithActorType(paychAddr, builtin.AccountActorCodeID).
				WithActorType(payerAddr, builtin.AccountActorCodeID).
				WithVerifiesSig(versig).
				WithHasher(hasher)

			rt := builder.Build(t)
			actor.constructAndVerify(t, rt, payerAddr, paychAddr)

			sv := SignedVoucher{
				TimeLock:       abi.ChainEpoch(tc.tl),
				Lane:           tc.lane,
				Nonce:          tc.nonce,
				Amount:         big.NewInt(tc.amt),
				Signature:      tc.sig,
				SecretPreimage: tc.secretPreimage,
			}
			ucp := &UpdateChannelStateParams{Sv: sv}

			rt.SetCaller(payerAddr, tc.targetCode)
			rt.ExpectValidateCallerAddr(payerAddr, paychAddr)

			if tc.expExitCode == exitcode.Ok {
				rt.Call(actor.UpdateChannelState, ucp)
				var st State
				rt.GetState(&st)
				assert.Len(t, st.LaneStates, 1)
				ls := st.LaneStates[0]
				assert.Equal(t, sv.Amount, ls.Redeemed)
				assert.Equal(t, sv.Nonce, ls.Nonce)
				assert.Equal(t, sv.Lane, ls.ID)
			} else {
				rt.ExpectAbort(tc.expExitCode, func() {
					rt.Call(actor.UpdateChannelState, ucp)
				})
				// verify state unchanged; no lane was created
				verifyInitialState(t, rt, payerAddr, paychAddr)
			}
		})
	}
}
func TestActor_UpdateChannelStateRedeem(t *testing.T) {
	ctx := context.Background()
	newVoucherAmt := big.NewInt(9)

	t.Run("redeeming voucher updates correctly with one lane", func(t *testing.T) {
		rt, actor, sv := requireCreateChannelWithLanes(t, ctx, 1)
		var st1 State
		rt.GetState(&st1)

		ucp := &UpdateChannelStateParams{Sv: *sv}
		ucp.Sv.Amount = newVoucherAmt

		// Sending to same lane updates the lane with "new" state
		rt.ExpectValidateCallerAddr(st1.From, st1.To)
		constructRet := rt.Call(actor.UpdateChannelState, ucp).(*adt.EmptyValue)
		require.Equal(t, adt.EmptyValue{}, *constructRet)
		rt.Verify()

		expLs := LaneState{
			ID:       0,
			Redeemed: newVoucherAmt,
			Nonce:    1,
		}
		expState := State{
			From:            st1.From,
			To:              st1.To,
			ToSend:          newVoucherAmt,
			SettlingAt:      st1.SettlingAt,
			MinSettleHeight: st1.MinSettleHeight,
			LaneStates:      []*LaneState{&expLs},
		}
		verifyState(t, rt, 1, expState)
	})

	t.Run("redeems voucher for correct lane", func(t *testing.T) {
		rt, actor, sv := requireCreateChannelWithLanes(t, ctx, 3)
		var st1, st2 State
		rt.GetState(&st1)

		initialAmt := st1.ToSend

		ucp := &UpdateChannelStateParams{Sv: *sv}
		ucp.Sv.Amount = newVoucherAmt
		ucp.Sv.Lane = 1
		lsToUpdate := st1.LaneStates[ucp.Sv.Lane]
		ucp.Sv.Nonce = lsToUpdate.Nonce + 1

		// Sending to same lane updates the lane with "new" state
		rt.ExpectValidateCallerAddr(st1.From, st1.To)
		constructRet := rt.Call(actor.UpdateChannelState, ucp).(*adt.EmptyValue)
		require.Equal(t, adt.EmptyValue{}, *constructRet)
		rt.Verify()

		rt.GetState(&st2)
		lUpdated := st2.LaneStates[ucp.Sv.Lane]

		bDelta := big.Sub(ucp.Sv.Amount, lsToUpdate.Redeemed)
		expToSend := big.Add(initialAmt, bDelta)
		assert.Equal(t, expToSend, st2.ToSend)
		assert.Equal(t, ucp.Sv.Amount, lUpdated.Redeemed)
		assert.Equal(t, ucp.Sv.Nonce, lUpdated.Nonce)
	})
}

func TestActor_UpdateChannelStateMergeSuccess(t *testing.T) {
	// Check that a lane merge correctly updates lane states
	numLanes := 3
	rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), numLanes)

	var st1 State
	rt.GetState(&st1)
	rt.SetCaller(st1.From, builtin.AccountActorCodeID)

	mergeTo := st1.LaneStates[0]
	mergeFrom := st1.LaneStates[1]

	// Note sv.Amount = 4
	sv.Lane = mergeTo.ID
	mergeNonce := mergeTo.Nonce + 10

	merges := []Merge{{Lane: mergeFrom.ID, Nonce: mergeNonce}}
	sv.Merges = merges

	ucp := &UpdateChannelStateParams{Sv: *sv}
	rt.ExpectValidateCallerAddr(st1.From, st1.To)
	_ = rt.Call(actor.UpdateChannelState, ucp).(*adt.EmptyValue)
	rt.Verify()

	expMergeTo := LaneState{ID: mergeTo.ID, Redeemed: sv.Amount, Nonce: sv.Nonce}
	expMergeFrom := LaneState{ID: mergeFrom.ID, Redeemed: mergeFrom.Redeemed, Nonce: mergeNonce}

	// calculate ToSend amount
	redeemed := big.Add(mergeFrom.Redeemed, mergeTo.Redeemed)
	expDelta := big.Sub(sv.Amount, redeemed)
	expSendAmt := big.Add(st1.ToSend, expDelta)

	// last lane should be unchanged
	expState := st1
	expState.ToSend = expSendAmt
	expState.LaneStates = []*LaneState{&expMergeTo, &expMergeFrom, st1.LaneStates[2]}
	verifyState(t, rt, numLanes, expState)
}

func TestActor_UpdateChannelStateMergeFailure(t *testing.T) {
	testCases := []struct {
		name                           string
		balance                        int64
		lane, voucherNonce, mergeNonce uint64
		expExitCode                    exitcode.ExitCode
	}{
		{
			name: "fails: merged lane in voucher has outdated nonce, cannot redeem",
			lane: 1, voucherNonce: 10, mergeNonce: 1,
			expExitCode: exitcode.ErrIllegalArgument,
		},
		{
			name: "fails: voucher has an outdated nonce, cannot redeem",
			lane: 1, voucherNonce: 0, mergeNonce: 10,
			expExitCode: exitcode.ErrIllegalArgument,
		},
		{
			name: "fails: not enough funds in channel to cover voucher",
			lane: 1, balance: 1, voucherNonce: 10, mergeNonce: 10,
			expExitCode: exitcode.ErrIllegalState,
		},
		{
			name: "fails: voucher cannot merge lanes into its own lane",
			lane: 0, voucherNonce: 10, mergeNonce: 10,
			expExitCode: exitcode.ErrIllegalArgument,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), 2)
			if tc.balance > 0 {
				rt.SetBalance(abi.NewTokenAmount(tc.balance))
			}

			var st1 State
			rt.GetState(&st1)
			mergeTo := st1.LaneStates[0]
			mergeFrom := st1.LaneStates[tc.lane]

			sv.Lane = mergeTo.ID
			sv.Nonce = tc.voucherNonce
			merges := []Merge{{Lane: mergeFrom.ID, Nonce: tc.mergeNonce}}
			sv.Merges = merges
			ucp := &UpdateChannelStateParams{Sv: *sv}

			rt.SetCaller(st1.From, builtin.AccountActorCodeID)
			rt.ExpectValidateCallerAddr(st1.From, st1.To)
			rt.ExpectAbort(tc.expExitCode, func() {
				rt.Call(actor.UpdateChannelState, ucp)
			})

		})
	}
	t.Run("When lane doesn't exist, fails with: voucher specifies invalid merge lane 999", func(t *testing.T) {
		rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), 2)

		var st1 State
		rt.GetState(&st1)
		mergeTo := st1.LaneStates[0]
		mergeFrom := LaneState{ID: 999, Nonce: 2}

		sv.Lane = mergeTo.ID
		sv.Nonce = 10
		merges := []Merge{{Lane: mergeFrom.ID, Nonce: sv.Nonce}}
		sv.Merges = merges
		ucp := &UpdateChannelStateParams{Sv: *sv}

		rt.SetCaller(st1.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st1.From, st1.To)
		rt.ExpectAbort(exitcode.ErrIllegalArgument, func() {
			rt.Call(actor.UpdateChannelState, ucp)
		})
	})

	t.Run("Too many lanes, fails with: lane limit exceeded", func(t *testing.T) {
		rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), LaneLimit)

		var st1 State
		rt.GetState(&st1)
		sv.Lane++
		sv.Nonce++
		sv.Amount = abi.NewTokenAmount(100)
		ucp := &UpdateChannelStateParams{Sv: *sv}
		rt.SetCaller(st1.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st1.From, st1.To)
		rt.ExpectAbort(exitcode.ErrIllegalArgument, func() {
			rt.Call(actor.UpdateChannelState, ucp)
		})
	})
}

func TestActor_UpdateChannelStateExtra(t *testing.T) {
	rt1, actor1, sv1 := requireCreateChannelWithLanes(t, context.Background(), 1)
	var st1 State
	rt1.GetState(&st1)

	mnum := abi.MethodNum(2)
	fakeParams := runtime.CBORBytes([]byte{1, 2, 3, 4})
	expSendParams := &PaymentVerifyParams{fakeParams, nil}
	otherAddr := tutil.NewIDAddr(t, 104)
	ex := &ModVerifyParams{
		Actor:  otherAddr,
		Method: mnum, //UpdateChannelState
		Data:   fakeParams,
	}
	ucp := &UpdateChannelStateParams{Sv: *sv1}
	ucp.Sv.Extra = ex

	rt1.SetCaller(st1.From, builtin.AccountActorCodeID)

	t.Run("Succeeds if extra call succeeds", func(t *testing.T) {
		rt1.ExpectValidateCallerAddr(st1.From, st1.To)
		rt1.ExpectSend(otherAddr, mnum, expSendParams, big.Zero(), nil, exitcode.Ok)
		rt1.Call(actor1.UpdateChannelState, ucp)
	})
	t.Run("If Extra call fails, fails with: spend voucher verification failed", func(t *testing.T) {
		rt1.ExpectValidateCallerAddr(st1.From, st1.To)
		rt1.ExpectSend(otherAddr, mnum, expSendParams, big.Zero(), nil, exitcode.ErrPlaceholder)
		rt1.ExpectAbort(exitcode.ErrPlaceholder, func() {
			rt1.Call(actor1.UpdateChannelState, ucp)
		})
	})
}

func TestActor_UpdateChannelStateSettling(t *testing.T) {
	rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), 1)

	ep := abi.ChainEpoch(10)
	rt.SetEpoch(ep)
	var st State
	rt.GetState(&st)

	rt.SetCaller(st.From, builtin.AccountActorCodeID)
	rt.ExpectValidateCallerAddr(st.From, st.To)
	rt.Call(actor.Settle, &adt.EmptyValue{})

	expSettlingAt := ep + SettleDelay
	rt.GetState(&st)
	require.Equal(t, expSettlingAt, st.SettlingAt)
	require.Equal(t, abi.ChainEpoch(0), st.MinSettleHeight)

	ucp := &UpdateChannelStateParams{Sv: *sv}

	testCases := []struct {
		name                                               string
		minSettleHeight, expSettlingAt, expMinSettleHeight abi.ChainEpoch
		//expExitCode                                        exitcode.ExitCode
	}{
		{name: "No change",
			minSettleHeight: 0, expMinSettleHeight: st.MinSettleHeight,
			expSettlingAt: st.SettlingAt},
		{name: "Updates MinSettleHeight only",
			minSettleHeight: abi.ChainEpoch(2), expMinSettleHeight: abi.ChainEpoch(2),
			expSettlingAt: st.SettlingAt},
		{name: "Updates both SettlingAt and MinSettleHeight",
			minSettleHeight: abi.ChainEpoch(12), expMinSettleHeight: abi.ChainEpoch(12),
			expSettlingAt: abi.ChainEpoch(12)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var newSt State
			ucp.Sv.MinSettleHeight = tc.minSettleHeight
			rt.ExpectValidateCallerAddr(st.From, st.To)
			rt.Call(actor.UpdateChannelState, ucp)
			rt.GetState(&newSt)
			assert.Equal(t, tc.expSettlingAt, newSt.SettlingAt)
			assert.Equal(t, tc.expMinSettleHeight, newSt.MinSettleHeight)
		})
	}
}

func TestActor_UpdateChannelStateSecretPreimage(t *testing.T) {
	rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), 1)
	var st State
	rt.GetState(&st)

	rt.SetHasher(func(data []byte) [32]byte {
		aux := []byte("XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
		var res [32]byte
		copy(res[:], aux)
		copy(res[:], data)
		return res
	})
	secret := []byte("Profesr")

	ucp := &UpdateChannelStateParams{
		Sv:     *sv,
		Secret: secret,
		Proof:  nil,
	}
	t.Run("Succeeds with correct secret", func(t *testing.T) {
		ucp.Sv.SecretPreimage = []byte("ProfesrXXXXXXXXXXXXXXXXXXXXXXXXX")
		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.Call(actor.UpdateChannelState, ucp)
	})

	t.Run("If bad secret preimage, fails with: incorrect secret!", func(t *testing.T) {
		ucp.Sv.SecretPreimage = []byte("Magneto")
		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.ExpectAbort(exitcode.ErrIllegalArgument, func() {
			rt.Call(actor.UpdateChannelState, ucp)
		})
	})
}

func TestActor_Settle(t *testing.T) {
	ep := abi.ChainEpoch(10)

	t.Run("Settle adjusts SettlingAt", func(t *testing.T) {
		rt, actor, _ := requireCreateChannelWithLanes(t, context.Background(), 1)
		rt.SetEpoch(ep)
		var st State
		rt.GetState(&st)

		rt.SetCaller(st.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.Call(actor.Settle, &adt.EmptyValue{})

		expSettlingAt := ep + SettleDelay
		rt.GetState(&st)
		assert.Equal(t, expSettlingAt, st.SettlingAt)
		assert.Equal(t, abi.ChainEpoch(0), st.MinSettleHeight)
	})

	t.Run("settle fails if called twice: channel already settling", func(t *testing.T) {
		rt, actor, _ := requireCreateChannelWithLanes(t, context.Background(), 1)
		rt.SetEpoch(ep)
		var st State
		rt.GetState(&st)

		rt.SetCaller(st.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.Call(actor.Settle, &adt.EmptyValue{})

		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.ExpectAbort(exitcode.ErrIllegalState, func() {
			rt.Call(actor.Settle, &adt.EmptyValue{})
		})
	})

	t.Run("Settle changes SettleHeight again if MinSettleHeight is less", func(t *testing.T) {
		rt, actor, sv := requireCreateChannelWithLanes(t, context.Background(), 1)
		rt.SetEpoch(ep)
		var st State
		rt.GetState(&st)

		// UpdateChannelState to increase MinSettleHeight only
		ucp := &UpdateChannelStateParams{Sv: *sv}
		ucp.Sv.MinSettleHeight = (ep + SettleDelay) + 1

		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.Call(actor.UpdateChannelState, ucp)

		var newSt State
		rt.GetState(&newSt)
		// SettlingAt should remain the same.
		require.Equal(t, abi.ChainEpoch(0), newSt.SettlingAt)
		require.Equal(t, ucp.Sv.MinSettleHeight, newSt.MinSettleHeight)

		// Settle.
		rt.SetCaller(st.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.Call(actor.Settle, &adt.EmptyValue{})

		// SettlingAt should = MinSettleHeight, not epoch + SettleDelay.
		rt.GetState(&newSt)
		assert.Equal(t, ucp.Sv.MinSettleHeight, newSt.SettlingAt)
	})
}

func TestActor_Collect(t *testing.T) {
	t.Run("Happy path", func(t *testing.T) {
		rt, actor, _ := requireCreateChannelWithLanes(t, context.Background(), 1)
		rt.SetEpoch(10)
		var st State
		rt.GetState(&st)

		// Settle.
		rt.SetCaller(st.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st.From, st.To)
		rt.Call(actor.Settle, &adt.EmptyValue{})

		rt.GetState(&st)
		require.Equal(t, abi.ChainEpoch(11), st.SettlingAt)
		rt.ExpectValidateCallerAddr(st.From, st.To)

		// "wait" for SettlingAt epoch
		rt.SetEpoch(12)

		bal := rt.GetBalance()
		sentToFrom := big.Sub(bal, st.ToSend)
		rt.ExpectSend(st.From, builtin.MethodSend, nil, sentToFrom, nil, exitcode.Ok)
		rt.ExpectSend(st.To, builtin.MethodSend, nil, st.ToSend, nil, exitcode.Ok)

		// Collect.
		rt.SetCaller(st.From, builtin.AccountActorCodeID)
		rt.ExpectValidateCallerAddr(st.From, st.To)
		res := rt.Call(actor.Collect, &adt.EmptyValue{})
		require.Equal(t, &adt.EmptyValue{}, res)

		var newSt State
		rt.GetState(&newSt)
		assert.Equal(t, big.Zero(), newSt.ToSend)
	})

	testCases := []struct {
		name                                           string
		expSendToCode, expSendFromCode, expCollectExit exitcode.ExitCode
		dontSettle                                     bool
	}{
		{name: "fails if not settling with: payment channel not settling or settled", dontSettle: true, expCollectExit: exitcode.ErrForbidden},
		{name: "fails if Failed to send balance to `From`", expSendFromCode: exitcode.ErrPlaceholder, expCollectExit: exitcode.ErrPlaceholder},
		{name: "fails if Failed to send funds to `To`", expSendToCode: exitcode.ErrPlaceholder, expCollectExit: exitcode.ErrPlaceholder},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rt, actor, _ := requireCreateChannelWithLanes(t, context.Background(), 1)
			rt.SetEpoch(10)
			var st State
			rt.GetState(&st)

			if !tc.dontSettle {
				rt.SetCaller(st.From, builtin.AccountActorCodeID)
				rt.ExpectValidateCallerAddr(st.From, st.To)
				rt.Call(actor.Settle, &adt.EmptyValue{})
				rt.GetState(&st)
				require.Equal(t, abi.ChainEpoch(11), st.SettlingAt)
			}

			// "wait" for SettlingAt epoch
			rt.SetEpoch(12)

			sentToFrom := big.Sub(rt.GetBalance(), st.ToSend)
			rt.ExpectSend(st.From, builtin.MethodSend, nil, sentToFrom, nil, tc.expSendFromCode)
			rt.ExpectSend(st.To, builtin.MethodSend, nil, st.ToSend, nil, tc.expSendToCode)

			// Collect.
			rt.SetCaller(st.From, builtin.AccountActorCodeID)
			rt.ExpectValidateCallerAddr(st.From, st.To)
			rt.ExpectAbort(tc.expCollectExit, func() {
				rt.Call(actor.Collect, &adt.EmptyValue{})
			})
		})
	}
}

type pcActorHarness struct {
	Actor
	t testing.TB
}

type laneParams struct {
	epochNum    int64
	from, to    addr.Address
	amt         big.Int
	lane, nonce uint64
}

func requireCreateChannelWithLanes(t *testing.T, ctx context.Context, numLanes int) (*mock.Runtime, *pcActorHarness, *SignedVoucher) {
	actor := pcActorHarness{Actor{}, t}

	paychAddr := tutil.NewIDAddr(t, 100)
	callerAddr := tutil.NewIDAddr(t, 101)
	payerAddr := tutil.NewIDAddr(t, 102)
	balance := abi.NewTokenAmount(100000)
	received := abi.NewTokenAmount(0)
	curEpoch := 2

	versig := func(sig crypto.Signature, signer addr.Address, plaintext []byte) bool { return true }
	hasher := func(data []byte) [32]byte { return [32]byte{} }

	builder := mock.NewBuilder(ctx, paychAddr).
		WithBalance(balance, received).
		WithEpoch(abi.ChainEpoch(curEpoch)).
		WithCaller(callerAddr, builtin.InitActorCodeID).
		WithActorType(paychAddr, builtin.AccountActorCodeID).
		WithActorType(payerAddr, builtin.AccountActorCodeID).
		WithVerifiesSig(versig).
		WithHasher(hasher)

	rt := builder.Build(t)
	actor.constructAndVerify(t, rt, payerAddr, paychAddr)

	var lastSv *SignedVoucher
	for i := 0; i < numLanes; i++ {
		amt := big.NewInt(int64(i + 1))
		lastSv = requireAddNewLane(t, rt, &actor, laneParams{
			epochNum: int64(curEpoch),
			from:     payerAddr,
			to:       paychAddr,
			amt:      amt,
			lane:     uint64(i),
			nonce:    uint64(i + 1),
		})
	}
	return rt, &actor, lastSv
}

func requireAddNewLane(t *testing.T, rt *mock.Runtime, actor *pcActorHarness, params laneParams) *SignedVoucher {
	sig := &crypto.Signature{Type: crypto.SigTypeBLS, Data: []byte("doesn't matter")}
	tl := abi.ChainEpoch(params.epochNum)
	sv := SignedVoucher{TimeLock: tl, Lane: params.lane, Nonce: params.nonce, Amount: params.amt, Signature: sig}
	ucp := &UpdateChannelStateParams{Sv: sv}

	rt.SetCaller(params.from, builtin.AccountActorCodeID)
	rt.ExpectValidateCallerAddr(params.from, params.to)
	constructRet := rt.Call(actor.UpdateChannelState, ucp).(*adt.EmptyValue)
	require.Equal(t, adt.EmptyValue{}, *constructRet)
	rt.Verify()
	return &sv
}

func (h *pcActorHarness) constructAndVerify(t *testing.T, rt *mock.Runtime, sender, receiver addr.Address) {
	params := &ConstructorParams{To: receiver, From: sender}

	rt.ExpectValidateCallerType(builtin.InitActorCodeID)
	constructRet := rt.Call(h.Actor.Constructor, params).(*adt.EmptyValue)
	assert.Equal(h.t, adt.EmptyValue{}, *constructRet)
	rt.Verify()
	verifyInitialState(t, rt, sender, receiver)
}

func verifyInitialState(t *testing.T, rt *mock.Runtime, sender, receiver addr.Address) {
	var st State
	rt.GetState(&st)
	expectedState := State{From: sender, To: receiver, ToSend: abi.NewTokenAmount(0)}
	verifyState(t, rt, -1, expectedState)
}

func verifyState(t *testing.T, rt *mock.Runtime, expLanes int, expectedState State) {
	var st State
	rt.GetState(&st)
	assert.Equal(t, expectedState.To, st.To)
	assert.Equal(t, expectedState.From, st.From)
	assert.Equal(t, expectedState.MinSettleHeight, st.MinSettleHeight)
	assert.Equal(t, expectedState.SettlingAt, st.SettlingAt)
	assert.Equal(t, expectedState.ToSend, st.ToSend)
	if expLanes >= 0 {
		require.Len(t, st.LaneStates, expLanes)
		assert.True(t, reflect.DeepEqual(expectedState.LaneStates, st.LaneStates))
	} else {
		assert.Len(t, st.LaneStates, 0)
	}
}
