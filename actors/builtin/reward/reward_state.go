package reward

import (
	cid "github.com/ipfs/go-cid"

	abi "github.com/filecoin-project/specs-actors/actors/abi"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	adt "github.com/filecoin-project/specs-actors/actors/util/adt"
)

type State struct {
	BaselinePower        abi.StoragePower
	RealizedPower        abi.StoragePower
	CumsumBaseline       abi.Spacetime
	CumsumRealized       abi.Spacetime
	EffectiveNetworkTime abi.ChainEpoch

	SimpleSupply   abi.TokenAmount // current supply
	BaselineSupply abi.TokenAmount // current supply

	LastPerEpochReward abi.TokenAmount
}

type AddrKey = adt.AddrKey

// These numbers are placeholders, but should be in units of attoFIL
var SimpleTotal, _ = big.FromString("1000000000000000000000000")
var BaselineTotal, _ = big.FromString("1000000000000000000000000")

func ConstructState(emptyMultiMapCid cid.Cid) *State {
	return &State{
		BaselinePower:        big.Zero(),
		RealizedPower:        big.Zero(),
		CumsumBaseline:       big.Zero(),
		CumsumRealized:       big.Zero(),
		EffectiveNetworkTime: abi.ChainEpoch(int64(0)),

		SimpleSupply:   big.Zero(),
		BaselineSupply: big.Zero(),
	}
}

// Minting Function: Taylor series expansion
//
// Intent
//   The intent of the following code is to compute the desired fraction of
//   coins that should have been minted at a given epoch according to the
//   simple exponential decay supply. This function is used both directly,
//   to compute simple minting, and indirectly, to compute baseline minting
//   by providing a synthetic "effective network time" instead of an actual
//   epoch number. The prose specification of the simple exponential decay is
//   that the unminted supply should decay exponentially with a half-life of
//   6 years. The formalization of the minted fraction at epoch t is thus:
//
//                            (            t             )
//                            ( ------------------------ )
//                      ( 1 )^( [# of epochs in 6 years] )
//           f(t) = 1 - ( - )
//                      ( 2 )
// Challenges
//
// 1. Since we cannot rely on floating-point computations in this setting, we
//    have resorted to using an ad-hoc fixed-point standard. Experimental
//    testing with the relevant scales of inputs and with a desired "atto"
//    level of output precision yielded a recommendation of a 97-bit fractional
//    part, which was stored in the constant "FixedPoint".
// !IMPORTANT!: the return value from this function is a factor of 2^FixedPoint
//    greater than the number it is semantically intended to represent (which
//    will always be between 0 and 1). The expectation is that callers will
//    multiply the result by some number, and THEN right-shift the result of
//    the multiplication by FixedPoint bits, thus implementing fixed-point
//    multiplication by the returned fraction.
//
// 2. Since we do not have a math library in this setting, we cannot directly
//    implement the intended closed form using stock implementations of
//    elementary functions like exp and ln. Not even powf is available.
//    Instead, we have manipulated the function into a form with a tractable
//    Taylor expansion, and then implemented the fixed-point Taylor expansion
//    in an efficient way.
//
// Mathematical Derivation
//
//   Note that the function f above is analytically equivalent to:
//
//                    (   ( 1 )              1                 )
//                    ( ln( - ) * ------------------------ * t )
//                    (   ( 2 )   [# of epochs in 6 years]     )
//        f(t) = 1 - e
//
//   We define λ = -ln(1/2)/[# of epochs in 6 years]
//               = -ln(1/2)*([Seconds per epoch] / (6 * [Seconds per year]))
//   such that
//                    -λt
//        f(t) = 1 - e
//
//   Now, we substitute for the exponential its well-known Taylor series at 0:
//
//                 infinity     n
//                   \```` (-λt)
//        f(t) = 1 -  >    ------
//                   /,,,,   n!
//                   n = 0
//
//   Simplifying, and truncating to the empirically necessary precision:
//
//                  24           n
//                \```` (-1)(-λt)
//        f(t) =   >    ----------
//                /,,,,     n!
//                n = 1
//
//   This is the final mathematical form of what is computed below. What remains
//   is to explain how the series calculation is carried out in fixed-point.
//
// Algorithm
//
//   The key observation is that each successive term can be represented as a
//   rational number, and derived from the previous term by simple
//   multiplications on the numerator and denominator. In particular:
//   * the numerator is the previous numerator multiplied by (-λt)
//   * the denominator is the previous denominator multiplied by n
//   We also need to represent λ itself as a rational, so the denominator of
//   the series term is actually multiplied by both n and the denominator of
//   lambda.
//
//   When we have the numerator and denominator for a given term set up, we
//   compute their fixed-point fraction by left-shifting the numerator before
//   performing integer division.
//
//   Finally, at the end of each loop, we remove unnecessary bits of precision
//   from both the numerator and denominator accumulators to control the
//   computational complexity of the bigint multiplications.

// Fixed-point precision (in bits) used internally and for output
const FixedPoint = 97

// Used in the definition of λ
const BlockTimeSeconds = 30
const SecondsInYear = 31556925

// The following are the numerator and denominator of -ln(1/2)=ln(2),
// represented as a rational with sufficient precision. They are parsed from
// strings because literals cannot be this long; they are stored as separate
// variables only because the string parsing function has multiple returns.
var LnTwoNum, _ = big.FromString("6931471805599453094172321215")
var LnTwoDen, _ = big.FromString("10000000000000000000000000000")

// We multiply the fraction ([Seconds per epoch] / (6 * [Seconds per year]))
// into the rational representation of -ln(1/2) which was just loaded, to
// produce the final, constant, rational representation of λ.
var LambdaNum = big.Mul(big.NewInt(BlockTimeSeconds), LnTwoNum)
var LambdaDen = big.Mul(big.NewInt(6*SecondsInYear), LnTwoDen)

// This function implements f(t) as described in the large comment block above,
// with the important caveat that its return value must not be interpreted
// semantically as an integer, but rather as a fixed-point number with
// FixedPoint bits of fractional part.
func taylorSeriesExpansion(t abi.ChainEpoch) big.Int {
	// `numeratorBase` is the numerator of the rational representation of (-λt).
	numeratorBase := big.Mul(LambdaNum.Neg(), big.NewInt(int64(t)))
	// The denominator of (-λt) is simply the denominator of λ, as -t is integral.
	denominatorBase := LambdaDen

	// `numerator` is the accumulator for numerators of the series terms. The
	// first term is simply (-1)(-λt). To include that factor of (-1), which
	// appears in every term, we introduce this negation into the numerator of
	// the first term. (All terms will be negated by this, because each term is
	// derived from the last by multiplying into it.)
	numerator := numeratorBase.Neg()
	// `denominator` is the accumulator for denominators of the series terms.
	denominator := denominatorBase

	// `ret` is an _additive_ accumulator for partial sums of the series, and
	// carries a _fixed-point_ representation rather than a rational
	// representation. This just means it has an implicit denominator of
	// 2^(FixedPoint).
	ret := big.Zero()

	// The first term computed has order 1; the final term has order 24.
	for n := int64(1); n < int64(25); n++ {

		// Multiplying the denominator by `n` on every loop accounts for the
		// `n!` (factorial) in the denominator of the series.
		denominator = big.Mul(denominator, big.NewInt(n))

		// Left-shift and divide to convert rational into fixed-point.
		term := big.Div(big.Lsh(numerator, FixedPoint), denominator)

		// Accumulate the fixed-point result into the return accumulator.
		ret = big.Add(ret, term)

		// Multiply the rational representation of (-λt) into the term accumulators
		// for the next iteration.  Doing this here in the loop allows us to save a
		// couple bigint operations by initializing numerator and denominator
		// directly instead of multiplying by 1.
		numerator = big.Mul(numerator, numeratorBase)
		denominator = big.Mul(denominator, denominatorBase)

		// If the denominator has grown beyond the necessary precision, then we can
		// truncate it by right-shifting. As long as we right-shift the numerator
		// by the same number of bits, all we have done is lose unnecessary
		// precision that would slow down the next iteration's multiplies.
		denominatorLen := big.BitLen(denominator)
		unnecessaryBits := denominatorLen - FixedPoint
		if unnecessaryBits < 0 {
			unnecessaryBits = 0
		}
		numerator = big.Rsh(numerator, unnecessaryBits)
		denominator = big.Rsh(denominator, unnecessaryBits)

	}

	return ret
}
