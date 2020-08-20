package miner

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	exitcode "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/math"
	"github.com/filecoin-project/specs-actors/actors/util/smoothing"
)

// IP = IPBase(precommit time) + AdditionalIP(precommit time)
// IPBase(t) = BR(t, InitialPledgeProjectionPeriod)
// AdditionalIP(t) = LockTarget(t)*PledgeShare(t)
// LockTarget = (LockTargetFactorNum / LockTargetFactorDenom) * FILCirculatingSupply(t)
// PledgeShare(t) = sectorQAPower / max(BaselinePower(t), NetworkQAPower(t))
// PARAM_FINISH
var PreCommitDepositFactor = 20
var InitialPledgeFactor = 20
var PreCommitDepositProjectionPeriod = abi.ChainEpoch(PreCommitDepositFactor) * builtin.EpochsInDay
var InitialPledgeProjectionPeriod = abi.ChainEpoch(InitialPledgeFactor) * builtin.EpochsInDay
var LockTargetFactorNum = big.NewInt(3)
var LockTargetFactorDenom = big.NewInt(10)

// FF = BR(t, DeclaredFaultProjectionPeriod)
// projection period of 2.14 days:  2880 * 2.14 = 6163.2.  Rounded to nearest epoch 6163
var DeclaredFaultFactorNum = 214
var DeclaredFaultFactorDenom = 100
var DeclaredFaultProjectionPeriod = abi.ChainEpoch((builtin.EpochsInDay * DeclaredFaultFactorNum) / DeclaredFaultFactorDenom)

// SP = BR(t, UndeclaredFaultProjectionPeriod)
var UndeclaredFaultProjectionPeriod = abi.ChainEpoch(5) * builtin.EpochsInDay

// Maximum number of days of BR a terminated sector can be penalized
const TerminationLifetimeCap = abi.ChainEpoch(70)

// Number of whole per-winner rewards covered by consensus fault penalty
const ConsensusFaultFactor = 5

// This is the BR(t) value of the given sector for the current epoch.
// It is the expected reward this sector would pay out over a one day period.
// BR(t) = CurrEpochReward(t) * SectorQualityAdjustedPower * EpochsInDay / TotalNetworkQualityAdjustedPower(t)
func ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate *smoothing.FilterEstimate, qaSectorPower abi.StoragePower, projectionDuration abi.ChainEpoch) abi.TokenAmount {
	networkQAPowerSmoothed := networkQAPowerEstimate.Estimate()
	if networkQAPowerSmoothed.IsZero() {
		return rewardEstimate.Estimate()
	}
	expectedRewardForProvingPeriod := smoothing.ExtrapolatedCumSumOfRatio(projectionDuration, 0, rewardEstimate, networkQAPowerEstimate)
	br128 := big.Mul(qaSectorPower, expectedRewardForProvingPeriod) // Q.0 * Q.128 => Q.128
	br := big.Rsh(br128, math.Precision)
	return big.Max(br, big.Zero()) // negative BR is clamped at 0
}

// This is the FF(t) penalty for a sector expected to be in the fault state either because the fault was declared or because
// it has been previously detected by the network.
// FF(t) = DeclaredFaultFactor * BR(t)
func PledgePenaltyForDeclaredFault(rewardEstimate, networkQAPowerEstimate *smoothing.FilterEstimate, qaSectorPower abi.StoragePower) abi.TokenAmount {
	return ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate, qaSectorPower, DeclaredFaultProjectionPeriod)
}

// This is the SP(t) penalty for a newly faulty sector that has not been declared.
// SP(t) = UndeclaredFaultFactor * BR(t)
func PledgePenaltyForUndeclaredFault(rewardEstimate, networkQAPowerEstimate *smoothing.FilterEstimate, qaSectorPower abi.StoragePower) abi.TokenAmount {
	return ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate, qaSectorPower, UndeclaredFaultProjectionPeriod)
}

// Penalty to locked pledge collateral for the termination of a sector before scheduled expiry.
// SectorAge is the time between the sector's activation and termination.
// replacedDayReward and replacedSectorAge are the day reward and age of the replaced sector in a capacity upgrade.
// They must be zero if no upgrade occurred.
func PledgePenaltyForTermination(dayReward abi.TokenAmount, sectorAge abi.ChainEpoch,
	twentyDayRewardAtActivation abi.TokenAmount, networkQAPowerEstimate *smoothing.FilterEstimate,
	qaSectorPower abi.StoragePower, rewardEstimate *smoothing.FilterEstimate, replacedDayReward abi.TokenAmount,
	replacedSectorAge abi.ChainEpoch,
) abi.TokenAmount {
	// max(SP(t), BR(StartEpoch, 20d) + BR(StartEpoch, 1d)*min(SectorAgeInDays, 70))
	// and sectorAgeInDays = sectorAge / EpochsInDay
	lifetimeCap := TerminationLifetimeCap * builtin.EpochsInDay
	cappedSectorAge := minEpoch(sectorAge, lifetimeCap)
	// expected reward for lifetime of new sector (epochs*AttoFIL/day)
	expectedReward := big.Mul(dayReward, big.NewInt(int64(cappedSectorAge)))
	// if lifetime under cap and this sector replaced capacity, add expected reward for old sector's lifetime up to cap
	relevantReplacedAge := minEpoch(replacedSectorAge, lifetimeCap-cappedSectorAge)
	expectedReward = big.Add(expectedReward, big.Mul(replacedDayReward, big.NewInt(int64(relevantReplacedAge))))

	return big.Max(
		PledgePenaltyForUndeclaredFault(rewardEstimate, networkQAPowerEstimate, qaSectorPower),
		big.Add(
			twentyDayRewardAtActivation,
			big.Div(
				expectedReward,
				big.NewInt(builtin.EpochsInDay)))) // (epochs*AttoFIL/day -> AttoFIL)
}

// Computes the PreCommit Deposit given sector qa weight and current network conditions.
// PreCommit Deposit = 20 * BR(t)
func PreCommitDepositForPower(rewardEstimate, networkQAPowerEstimate *smoothing.FilterEstimate, qaSectorPower abi.StoragePower) abi.TokenAmount {
	return ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate, qaSectorPower, PreCommitDepositProjectionPeriod)
}

// Computes the pledge requirement for committing new quality-adjusted power to the network, given the current
// total power, total pledge commitment, epoch block reward, and circulating token supply.
// In plain language, the pledge requirement is a multiple of the block reward expected to be earned by the
// newly-committed power, holding the per-epoch block reward constant (though in reality it will change over time).
func InitialPledgeForPower(qaPower abi.StoragePower, baselinePower abi.StoragePower, networkTotalPledge abi.TokenAmount, rewardEstimate, networkQAPowerEstimate *smoothing.FilterEstimate, networkCirculatingSupplySmoothed abi.TokenAmount) abi.TokenAmount {
	networkQAPower := networkQAPowerEstimate.Estimate()
	ipBase := ExpectedRewardForPower(rewardEstimate, networkQAPowerEstimate, qaPower, InitialPledgeProjectionPeriod)

	lockTargetNum := big.Mul(LockTargetFactorNum, networkCirculatingSupplySmoothed)
	lockTargetDenom := LockTargetFactorDenom
	pledgeShareNum := qaPower
	pledgeShareDenom := big.Max(big.Max(networkQAPower, baselinePower), qaPower) // use qaPower in case others are 0
	additionalIPNum := big.Mul(lockTargetNum, pledgeShareNum)
	additionalIPDenom := big.Mul(lockTargetDenom, pledgeShareDenom)
	additionalIP := big.Div(additionalIPNum, additionalIPDenom)

	return big.Add(ipBase, additionalIP)
}

// Repays all fee debt and then verifies that the miner has amount needed to cover
// the pledge requirement after burning all fee debt.  If not aborts.
// Returns an amount that must be burnt by the actor.
// Note that this call does not compute recent vesting so reported unlocked balance
// may be slightly lower than the true amount. Computing vesting here would be
// almost always redundant since vesting is quantized to ~daily units.  Vesting
// will be at most one proving period old if computed in the cron callback.
func VerifyPledgeRequirementsAndRepayDebts(rt Runtime, st *State) abi.TokenAmount {
	currBalance := rt.CurrentBalance()
	toBurn, err := st.RepayDebt(currBalance)
	builtin.RequireNoErr(rt, err, exitcode.Unwrap(err, exitcode.ErrIllegalState), "unlocked balance can not repay fee debt")

	// IP requirements must be checked against balance after we account for fee debt repayment.
	// The toBurn fee debt repayment will be burned so subtract from working value for current balance.
	currBalance = big.Sub(currBalance, toBurn)
	if !st.MeetsInitialPledgeCondition(currBalance) {
		rt.Abortf(exitcode.ErrInsufficientFunds, "unlocked balance does not cover pledge requirements")
	}
	return toBurn
}

func ConsensusFaultPenalty(thisEpochReward abi.TokenAmount) abi.TokenAmount {
	return big.Div(
		big.Mul(thisEpochReward, big.NewInt(ConsensusFaultFactor)),
		big.NewInt(builtin.ExpectedLeadersPerEpoch),
	)
}
