package stochastic

import (
	"math"
	"math/bits"
)

// This file is the portable SSA and SDE path: everything the byte-exact
// contract with pflow-rs, pflow-xyz and pflow-jl needs that the default path
// leaves to the platform. The default path (math/rand + the runtime
// logarithm) stays the default and is untouched; Options.Portable selects
// this one.
//
// Three things are pinned here, because each of them varies across languages
// and none of them may:
//
//   - the random stream — SplitMix64 seeding into xoshiro256** (§1 of the spec);
//   - the logarithm — plog, an explicit copy of Go's pure-Go math.log, never
//     the runtime's (§2). On amd64 the runtime's Log is assembly; it agrees
//     with the pure function on every normal input tried (and disagrees on
//     subnormals, see portable_test.go), but "happens to" is not a contract
//     and s390x has a third implementation. The copy is what ships; the
//     runtime logarithm is not named in this file, so a grep proves it — the
//     default path's sampler, which does call it, lives in stochastic.go
//     beside the math/rand import;
//   - the arithmetic order — every operation is written in the order it must
//     be evaluated, with explicit float64 conversions wherever the Go compiler
//     would otherwise be free to fuse a multiply-add (it does, on arm64).
//
// normal() (SDE's draw) needs no fourth pinned primitive: it is built
// entirely from the three above plus math.Sqrt, which — unlike log — IEEE
// 754 requires to be correctly rounded, so it is already bit-identical
// across every conformant runtime and needs no port.
//
// The spec is ssa-spec.md; the vectors in portable_test.go are its §1.5 and
// §2.5 tables. normal()'s own vectors are in portable_test.go, not yet named
// in ssa-spec.md — SDE has no spec of its own the way SSA does.

// portableSampler implements the sampler seam declared in stochastic.go with
// the spec's stream: u = 1 - x1 is exact for every x1 in [0, 1) and never
// zero, so there is no clamp and no redraw — a clamp here would consume a
// draw the other languages do not.
type portableSampler struct {
	x xoshiro256

	// normal()'s cache: the Marsaglia polar method produces two independent
	// standard normals per accepted pair of uniforms, so the second is
	// stashed here rather than discarded.
	hasSpare bool
	spare    float64
}

func (s *portableSampler) wait() float64    { return -plog(1 - s.x.uniform()) }
func (s *portableSampler) uniform() float64 { return s.x.uniform() }

// normal draws a standard normal variate via the Marsaglia polar method:
// deliberately not Box-Muller, which needs sin/cos — a second transcendental
// port on top of plog, and one with no portable reference implementation
// this codebase already carries. Polar needs only sqrt (already exact and
// identical across every IEEE 754 implementation — never approximated the
// way log is, so it needs no port) and plog, both already in this file's
// contract. u1, u2 are drawn from (-1, 1) and accepted when 0 < u1²+u2² < 1;
// mul = sqrt(-2·plog(s)/s) turns the pair into two independent N(0,1)
// values, u1·mul and u2·mul.
func (s *portableSampler) normal() float64 {
	if s.hasSpare {
		s.hasSpare = false
		return s.spare
	}
	for {
		u1 := 2*s.x.uniform() - 1
		u2 := 2*s.x.uniform() - 1
		sq := u1*u1 + u2*u2
		if sq > 0 && sq < 1 {
			mul := math.Sqrt(-2 * plog(sq) / sq)
			s.spare = u2 * mul
			s.hasSpare = true
			return u1 * mul
		}
	}
}

// splitmix64 expands one 64-bit seed into the four xoshiro state words. All
// arithmetic wraps modulo 2^64, which is what uint64 does natively.
func splitmix64(seed uint64) [4]uint64 {
	var s [4]uint64
	x := seed
	for i := range s {
		x += 0x9E3779B97F4A7C15
		z := x
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		z ^= z >> 31
		s[i] = z
	}
	return s
}

// xoshiro256 is xoshiro256** with no jump functions.
type xoshiro256 struct{ s [4]uint64 }

func newXoshiro256(seed uint64) xoshiro256 {
	return xoshiro256{s: splitmix64(seed)}
}

// next is one step of the generator. result comes from the old s[1], before
// any state word is touched.
func (x *xoshiro256) next() uint64 {
	result := bits.RotateLeft64(x.s[1]*5, 7) * 9
	t := x.s[1] << 17
	x.s[2] ^= x.s[0]
	x.s[3] ^= x.s[1]
	x.s[1] ^= x.s[2]
	x.s[0] ^= x.s[3]
	x.s[2] ^= t
	x.s[3] = bits.RotateLeft64(x.s[3], 45)
	return result
}

// twoToMinus53 is 2^-53, bit pattern 0x3CA0000000000000.
const twoToMinus53 = 1.0 / 9007199254740992.0

// uniform is the top 53 bits of next() scaled into [0, 1). The integer is
// exactly representable and the scaling is exact, so this is one rounding-free
// operation on every platform.
//
// The outer float64 conversion is for the caller, not the product: after
// inlining, wait()'s 1 - uniform() is x*y-z shaped and the arm64 backend
// would fuse it into a single FMSUB. Fusion would happen to be harmless here
// (the product is exact, so "1 - exact" rounds once either way), but the
// conversion makes that a property of the code rather than of an argument.
func (x *xoshiro256) uniform() float64 {
	return float64(float64(x.next()>>11) * twoToMinus53)
}

// Constants of the FreeBSD/fdlibm log, as Go's math/log.go carries them. The
// bit patterns are the intended values; the decimal forms parse to them.
var (
	plogLn2Hi = math.Float64frombits(0x3FE62E42FEE00000) // 6.93147180369123816490e-01
	plogLn2Lo = math.Float64frombits(0x3DEA39EF35793C76) // 1.90821492927058770002e-10
	plogL1    = math.Float64frombits(0x3FE5555555555593) // 6.666666666666735130e-01
	plogL2    = math.Float64frombits(0x3FD999999997FA04) // 3.999999999940941908e-01
	plogL3    = math.Float64frombits(0x3FD2492494229359) // 2.857142874366239149e-01
	plogL4    = math.Float64frombits(0x3FCC71C51D8E78AF) // 2.222219843214978396e-01
	plogL5    = math.Float64frombits(0x3FC7466496CB03DE) // 1.818357216161805012e-01
	plogL6    = math.Float64frombits(0x3FC39A09D078C69F) // 1.531383769920937332e-01
	plogL7    = math.Float64frombits(0x3FC2F112DF3E5244) // 1.479819860511658591e-01
	// plogSqrt2Over2 is math.Sqrt2/2, the reduction threshold.
	plogSqrt2Over2 = math.Float64frombits(0x3FE6A09E667F3BCD) // 0.7071067811865476
)

// frexp is math.Frexp on the bit pattern, for a positive finite non-zero x —
// the only inputs plog hands it. Returns frac in [0.5, 1) and exp with
// x = frac * 2^exp. A subnormal is normalised first with an exact scaling.
func frexp(x float64) (frac float64, exp int) {
	b := math.Float64bits(x)
	e := int((b >> 52) & 0x7FF)
	if e == 0 {
		x *= 1 << 52 // exact
		b = math.Float64bits(x)
		e = int((b>>52)&0x7FF) - 52
	}
	exp = e - 1022
	frac = math.Float64frombits((b &^ (0x7FF << 52)) | (1022 << 52))
	return frac, exp
}

// plog is the natural logarithm, as a copy of the pure-Go math.log body
// ($GOROOT/src/math/log.go) so the result does not depend on GOARCH.
//
// Every product that feeds an addition or subtraction is wrapped in an
// explicit float64 conversion. The Go spec allows the compiler to fuse x*y+z
// into a single rounding, across statements, and the arm64 backend does; an
// explicit conversion rounds to float64 and forbids the fusion. On amd64 with
// the default GOAMD64=v1 the conversions change nothing, which is how the
// function can be checked against the runtime's assembly here and still be the
// same function on a machine where that check would not hold.
func plog(x float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsInf(x, 1):
		return x
	case x < 0:
		return math.NaN()
	case x == 0:
		return math.Inf(-1)
	}

	// reduce
	f1, ki := frexp(x)
	if f1 < plogSqrt2Over2 {
		f1 *= 2
		ki--
	}
	f := f1 - 1
	k := float64(ki)

	// compute
	s := f / (2 + f)
	s2 := float64(s * s)
	s4 := float64(s2 * s2)
	t1 := float64(s2 * (plogL1 + float64(s4*(plogL3+float64(s4*(plogL5+float64(s4*plogL7)))))))
	t2 := float64(s4 * (plogL2 + float64(s4*(plogL4+float64(s4*plogL6)))))
	R := t1 + t2
	hfsq := float64(float64(0.5*f) * f)
	return float64(k*plogLn2Hi) - ((hfsq - (float64(s*(hfsq+R)) + float64(k*plogLn2Lo))) - f)
}
