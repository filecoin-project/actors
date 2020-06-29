package miner

import (
	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	power "github.com/filecoin-project/specs-actors/actors/builtin/power"
)

// An approximation to chain state finality (should include message propagation time as well).
const ChainFinalityish = abi.ChainEpoch(500) // PARAM_FINISH

// Maximum duration to allow for the sealing process for seal algorithms.
// Dependent on algorithm and sector size
var MaxSealDuration = map[abi.RegisteredProof]abi.ChainEpoch{
	abi.RegisteredProof_StackedDRG32GiBSeal:  abi.ChainEpoch(10000), // PARAM_FINISH
	abi.RegisteredProof_StackedDRG2KiBSeal:   abi.ChainEpoch(10000),
	abi.RegisteredProof_StackedDRG8MiBSeal:   abi.ChainEpoch(10000),
	abi.RegisteredProof_StackedDRG512MiBSeal: abi.ChainEpoch(10000),
}

// Number of epochs between publishing the precommit and when the challenge for interactive PoRep is drawn
// used to ensure it is not predictable by miner.
const PreCommitChallengeDelay = abi.ChainEpoch(10)

// Lookback from the current epoch from which to obtain a PoSt challenge.
const PoStLookback = abi.ChainEpoch(1) // PARAM_FINISH

// Lookback from the current epoch for state view for elections; for Election PoSt, same as the PoSt lookback.
const ElectionLookback = PoStLookback // PARAM_FINISH

// Number of sectors to be sampled as part of windowed PoSt
const NumWindowedPoStSectors = 200 // PARAM_FINISH

// Delay between declaration of a temporary sector fault and effectiveness of reducing the active proving set for PoSts.
const DeclaredFaultEffectiveDelay = abi.ChainEpoch(20) // PARAM_FINISH

// Staging period for a miner worker key change.
const WorkerKeyChangeDelay = 2 * ElectionLookback // PARAM_FINISH

// Deposit per sector required at pre-commitment, refunded after the commitment is proven (else burned).
func precommitDeposit(sectorSize abi.SectorSize, duration abi.ChainEpoch) abi.TokenAmount {
	depositPerByte := abi.NewTokenAmount(0) // PARAM_FINISH
	return big.Mul(depositPerByte, big.NewInt(int64(sectorSize)))
}

func temporaryFaultFee(weights []*power.SectorStorageWeightDesc, duration abi.ChainEpoch) abi.TokenAmount {
	return big.Zero() // PARAM_FINISH
}

// MaxFaultsCount is the maximum number of faults that can be declared
const MaxFaultsCount = 32 << 20

// ProvingPeriod defines the frequency of PoSt challenges that a miner will have to respond to
const ProvingPeriod = 300

// WindowedPoStChallengeCount defines the number of windowed PoSt challenges
const WindowedPoStChallengeCount = 2000
