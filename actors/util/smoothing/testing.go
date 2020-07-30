package smoothing

import (
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/util/math"
)

// Returns an estimate with position val and velocity 0
func TestingConstantEstimate(val big.Int) *FilterEstimate {
	estimate := InitialEstimate()
	estimate.PositionEstimate = big.Lsh(val, math.Precision) // Q.0 => Q.128
	return estimate
}

// Returns and estimate with postion x and velocity v
func TestingEstimate(x, v big.Int) *FilterEstimate {
	estimate := InitialEstimate()
	estimate.PositionEstimate = big.Lsh(x, math.Precision) // Q.0 => Q.128
	estimate.VelocityEstimate = big.Lsh(v, math.Precision) // Q.0 => Q.128
	return estimate
}
