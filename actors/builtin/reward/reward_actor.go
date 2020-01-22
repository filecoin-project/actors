package reward

import (
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"math"

	addr "github.com/filecoin-project/go-address"
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	vmr "github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	serde "github.com/filecoin-project/specs-actors/actors/serde"
	autil "github.com/filecoin-project/specs-actors/actors/util"
	cid "github.com/ipfs/go-cid"
)

var TODO = autil.TODO

type VestingFunction int64

const (
	None VestingFunction = iota
	Linear
	// TODO: potential options
	// PieceWise
	// Quadratic
	// Exponential
)

type Reward struct {
	VestingFunction
	StartEpoch      abi.ChainEpoch
	EndEpoch        abi.ChainEpoch
	Value           abi.TokenAmount
	AmountWithdrawn abi.TokenAmount
}

func (r *Reward) AmountVested(elapsedEpoch abi.ChainEpoch) abi.TokenAmount {
	switch r.VestingFunction {
	case None:
		return r.Value
	case Linear:
		TODO() // BigInt
		vestedProportion := math.Max(1.0, float64(elapsedEpoch)/float64(r.StartEpoch-r.EndEpoch))
		return big.Mul(r.Value, big.NewInt(int64(vestedProportion)))
	default:
		return abi.NewTokenAmount(0)
	}
}

// ownerAddr to a collection of Reward
// TODO: AMT
type RewardBalanceAMT map[addr.Address][]Reward

type RewardActorState struct {
	RewardMap RewardBalanceAMT
}

func (st *RewardActorState) _withdrawReward(rt vmr.Runtime, ownerAddr addr.Address) abi.TokenAmount {
	rewards, found := st.RewardMap[ownerAddr]
	if !found {
		rt.AbortStateMsg("ra._withdrawReward: ownerAddr not found in RewardMap.")
	}

	rewardToWithdrawTotal := abi.NewTokenAmount(0)
	indicesToRemove := make([]int, len(rewards))

	for i, r := range rewards {
		elapsedEpoch := rt.CurrEpoch() - r.StartEpoch
		unlockedReward := r.AmountVested(elapsedEpoch)
		withdrawableReward := big.Sub(unlockedReward, r.AmountWithdrawn)

		if withdrawableReward.LessThan(big.NewInt(0)) {
			rt.AbortStateMsg("ra._withdrawReward: negative withdrawableReward.")
		}

		r.AmountWithdrawn = unlockedReward // modify rewards in place
		rewardToWithdrawTotal = big.Add(rewardToWithdrawTotal, withdrawableReward)

		if r.AmountWithdrawn == r.Value {
			indicesToRemove = append(indicesToRemove, i)
		}
	}

	updatedRewards := removeIndices(rewards, indicesToRemove)
	st.RewardMap[ownerAddr] = updatedRewards

	return rewardToWithdrawTotal
}

type RewardActor struct{}

func (a *RewardActor) Constructor(rt vmr.Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)
	// initialize Reward Map with investor accounts
	autil.IMPL_FINISH()
	return &vmr.EmptyReturn{}
}

func (a *RewardActor) State(rt vmr.Runtime) (vmr.ActorStateHandle, RewardActorState) {
	h := rt.AcquireState()
	stateCID := cid.Cid(h.Take())
	var state RewardActorState
	if !rt.IpldGet(stateCID, &state) {
		rt.Abort(exitcode.ErrPlaceholder, "state not found")
	}
	return h, state
}

func (a *RewardActor) WithdrawReward(rt vmr.Runtime) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerType(builtin.CallerTypesSignable...)
	ownerAddr := rt.ImmediateCaller()

	h, st := a.State(rt)

	// withdraw available funds from RewardMap
	withdrawableReward := st._withdrawReward(rt, ownerAddr)
	UpdateReleaseRewardActorState(rt, h, st)

	_, code := rt.Send(ownerAddr, builtin.MethodSend, nil, withdrawableReward)
	vmr.RequireSuccess(rt, code, "failed to send funds to owner")
	return &vmr.EmptyReturn{}
}

func (a *RewardActor) AwardBlockReward(
	rt vmr.Runtime,
	miner addr.Address,
	penalty abi.TokenAmount,
	minerNominalPower abi.StoragePower,
	currPledge abi.TokenAmount,
) *vmr.EmptyReturn {
	rt.ValidateImmediateCallerIs(builtin.SystemActorAddr)

	inds := rt.CurrIndices()
	pledgeReq := inds.PledgeCollateralReq(minerNominalPower)
	currReward := inds.GetCurrBlockRewardForMiner(minerNominalPower, currPledge)
	TODO() // BigInt

	// 0 if over collateralized
	underPledge := big.Max(big.Zero(), big.Sub(pledgeReq, currPledge))
	rewardToGarnish := big.Min(currReward, underPledge)

	TODO()
	// handle penalty here
	// also handle penalty greater than reward
	actualReward := big.Sub(currReward, rewardToGarnish)
	if rewardToGarnish.GreaterThan(big.NewInt(0)) {
		// Send fund to SPA for collateral
		_, code := rt.Send(
			builtin.StoragePowerActorAddr,
			builtin.Method_StoragePowerActor_AddBalance,
			serde.MustSerializeParams(miner),
			abi.TokenAmount(rewardToGarnish),
		)
		vmr.RequireSuccess(rt, code, "failed to add balance to power actor")
	}

	h, st := a.State(rt)
	if actualReward.GreaterThan(abi.NewTokenAmount(0)) {
		// put Reward into RewardMap
		newReward := &Reward{
			StartEpoch:      rt.CurrEpoch(),
			EndEpoch:        rt.CurrEpoch(),
			Value:           actualReward,
			AmountWithdrawn: abi.NewTokenAmount(0),
			VestingFunction: None,
		}
		rewards, found := st.RewardMap[miner]
		if !found {
			rewards = make([]Reward, 0)
		}
		rewards = append(rewards, *newReward)
		st.RewardMap[miner] = rewards
	}
	UpdateReleaseRewardActorState(rt, h, st)
	return &vmr.EmptyReturn{}
}

func UpdateReleaseRewardActorState(rt vmr.Runtime, h vmr.ActorStateHandle, st RewardActorState) {
	newCID := abi.ActorSubstateCID(rt.IpldPut(&st))
	h.UpdateRelease(newCID)
}

func removeIndices(rewards []Reward, indices []int) []Reward {
	// remove fully paid out Rewards by indices
	panic("TODO")
}
