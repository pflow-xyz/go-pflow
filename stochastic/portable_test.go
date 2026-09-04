package stochastic

import (
	"math"
	"math/rand"
	"runtime"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// The vectors here are ssa-spec.md §1.1, §1.5, §2.5, §3.2 and §3.8. Every
// comparison is on bits or with ==; a tolerance would defeat the point.

func TestSplitMix64State(t *testing.T) {
	cases := []struct {
		seed uint64
		want [4]uint64
	}{
		{42, [4]uint64{0xBDD732262FEB6E95, 0x28EFE333B266F103, 0x47526757130F9F52, 0x581CE1FF0E4AE394}},
		{0, [4]uint64{0xE220A8397B1DCDAF, 0x6E789E6AA1B965F4, 0x06C45D188009454F, 0xF88BB8A8724C81EC}},
	}
	for _, c := range cases {
		if got := splitmix64(c.seed); got != c.want {
			t.Errorf("splitmix64(%d) = %#x, want %#x", c.seed, got, c.want)
		}
	}
}

func TestXoshiroSeed42Stream(t *testing.T) {
	wantNext := []uint64{
		0x15780B2E0C2EC716, 0x6104D9866D113A7E, 0xAE17533239E499A1, 0xECB8AD4703B360A1, 0xFDE6DC7FE2EC5E64,
	}
	wantUniform := []uint64{
		0x3FB5780B2E0C2EC0, 0x3FD84136619B444E, 0x3FE5C2EA66473C93, 0x3FED9715A8E0766C, 0x3FEFBCDB8FFC5D8B,
	}
	wantDecimal := []float64{
		0.08386297105988216, 0.3789802506626686, 0.6800434110281394, 0.9246929453253876, 0.9918039142821028,
	}

	x := newXoshiro256(42)
	for i, w := range wantNext {
		if got := x.next(); got != w {
			t.Errorf("next()[%d] = %#x, want %#x", i, got, w)
		}
	}
	x = newXoshiro256(42)
	for i := range wantUniform {
		u := x.uniform()
		if got := math.Float64bits(u); got != wantUniform[i] {
			t.Errorf("uniform()[%d] bits = %#x, want %#x", i, got, wantUniform[i])
		}
		if u != wantDecimal[i] {
			t.Errorf("uniform()[%d] = %v, want %v", i, u, wantDecimal[i])
		}
		if u < 0 || u >= 1 {
			t.Errorf("uniform()[%d] = %v out of [0,1)", i, u)
		}
	}
}

func TestXoshiroFirstOutputs(t *testing.T) {
	cases := []struct{ seed, want uint64 }{
		{0, 0x99EC5F36CB75F2B4},
		{1, 0xB3F2AF6D0FC710C5},
		{0xFFFFFFFFFFFFFFFF, 0x8F5520D52A7EAD08}, // wraps in SplitMix64's first addition
	}
	for _, c := range cases {
		x := newXoshiro256(c.seed)
		if got := x.next(); got != c.want {
			t.Errorf("seed %#x: first next() = %#x, want %#x", c.seed, got, c.want)
		}
	}
}

// plogVectors is §2.5, all thirteen rows. The 3.0 row is the one a leaked
// runtime log fails: glibc gives 1.0986122886681098.
var plogVectors = []struct {
	x    float64
	want uint64
}{
	{0.5, 0xBFE62E42FEFA39EF},
	{2.0, 0x3FE62E42FEFA39EF},
	{0.1, 0xC0026BB1BBB55515},
	{1e-300, 0xC085963447F87FB5},
	{0.999999, 0xBEB0C6F82D74D230},
	{3.0, 0x3FF193EA7AAD030A},
	{10.0, 0x40026BB1BBB55516},
	{1.0, 0x0000000000000000},
	{0.7071067811865476, 0xBFD62E42FEFA39EE}, // = Sqrt2/2, no-doubling branch
	{0.7071067811865475, 0xBFD62E42FEFA39F1}, // one ulp below, doubling branch
	{1.0000000000000002, 0x3CAFFFFFFFFFFFFF},
	{0.9999999999999999, 0xBCA0000000000000},
	{1e-09, 0xC034B927F32BFFB8},
}

func TestPlogVectors(t *testing.T) {
	for _, c := range plogVectors {
		if got := math.Float64bits(plog(c.x)); got != c.want {
			t.Errorf("plog(%v) bits = %#x, want %#x", c.x, got, c.want)
		}
	}
	// Decimal forms of the rows, as a second reading of the same table.
	decimals := map[float64]float64{
		0.5: -0.6931471805599453, 2.0: 0.6931471805599453, 3.0: 1.0986122886681096,
		10.0: 2.302585092994046, 1e-300: -690.7755278982137, 1e-09: -20.72326583694641,
	}
	for x, want := range decimals {
		if got := plog(x); got != want {
			t.Errorf("plog(%v) = %v, want %v", x, got, want)
		}
	}
}

func TestPlogSpecialCases(t *testing.T) {
	if !math.IsNaN(plog(math.NaN())) {
		t.Error("plog(NaN) should be NaN")
	}
	if !math.IsInf(plog(math.Inf(1)), 1) {
		t.Error("plog(+Inf) should be +Inf")
	}
	if !math.IsNaN(plog(-1)) {
		t.Error("plog(-1) should be NaN")
	}
	if !math.IsInf(plog(0), -1) {
		t.Error("plog(0) should be -Inf")
	}
	// Worst case the SSA can reach: u = 2^-53.
	if got := plog(twoToMinus53); got != -36.7368005696771 {
		t.Errorf("plog(2^-53) = %v", got)
	}
	// Subnormal input goes through the normalising branch of frexp.
	if got := plog(5e-324); math.IsNaN(got) || math.IsInf(got, 0) || got > -744 || got < -745 {
		t.Errorf("plog(min subnormal) = %v, want about -744.44", got)
	}
}

func TestFrexp(t *testing.T) {
	for _, x := range []float64{0.5, 1, 2, 3, 0.1, 1e-300, 5e-324, 1e300, 0.7071067811865476} {
		wf, we := math.Frexp(x)
		gf, ge := frexp(x)
		if gf != wf || ge != we {
			t.Errorf("frexp(%v) = (%v, %d), want (%v, %d)", x, gf, ge, wf, we)
		}
	}
}

// TestPlogMatchesMathLog is a platform statement, not part of the contract:
// on this GOARCH the runtime's math.Log agrees bit for bit with the pure
// function plog copies, on the spec vectors and on 10k random inputs. amd64
// uses assembly (log_amd64.s) and arm64 the pure function; s390x has a third
// implementation the check does not cover. The explicit copy is what ships
// either way — this test exists to make a divergence visible, not to license
// calling math.Log.
//
// The agreement holds on the normal range only. On linux/amd64 (go1.26)
// math.Log(5e-324) returns -709.0895657128241 (0xC08628B76E3A7B61): the
// assembly does not normalise a subnormal before extracting the exponent.
// plog returns the correct -744.4400719213812 — see TestPlogSubnormal. So a
// subnormal must never be added to plogVectors, and plog must never be
// "fixed" to match the assembly; the SSA never reaches this range anyway
// (u = 1 - x1 >= 2^-53).
func TestPlogMatchesMathLog(t *testing.T) {
	if runtime.GOARCH == "s390x" {
		t.Skip("s390x has its own math.Log; plog is the portable one regardless")
	}
	for _, c := range plogVectors {
		if a, b := math.Float64bits(plog(c.x)), math.Float64bits(math.Log(c.x)); a != b {
			t.Errorf("plog(%v) = %#x but math.Log = %#x on %s", c.x, a, b, runtime.GOARCH)
		}
	}
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // a fixed exploration seed
	for i := 0; i < 10_000; i++ {
		var x float64
		switch i % 3 {
		case 0:
			x = rng.Float64() // the SSA's domain, (0,1]
		case 1:
			x = rng.Float64() * 1e6
		default:
			x = math.Exp((rng.Float64() - 0.5) * 1000) // wide dynamic range
		}
		if x == 0 {
			continue
		}
		if a, b := math.Float64bits(plog(x)), math.Float64bits(math.Log(x)); a != b {
			t.Errorf("plog(%v) = %#x but math.Log = %#x on %s", x, a, b, runtime.GOARCH)
		}
	}
}

// TestPlogSubnormal pins plog on the subnormal range, where the §2.3 frexp's
// scale-by-2^52 branch runs and amd64's math.Log is wrong (see
// TestPlogMatchesMathLog). Expected bits are the correctly rounded log, which
// the pure-Go math.log and glibc both produce.
func TestPlogSubnormal(t *testing.T) {
	cases := []struct {
		x    float64
		bits uint64
	}{
		{5e-324, 0xC0874385446D71C3},                                   // smallest subnormal, log = -744.4400719213812
		{math.Float64frombits(0x000FFFFFFFFFFFFF), 0xC086232BDD7ABCD2}, // largest subnormal, log = -708.3964185322641
	}
	for _, c := range cases {
		if got := math.Float64bits(plog(c.x)); got != c.bits {
			t.Errorf("plog(%g) = %#x (%v), want %#x", c.x, got, plog(c.x), c.bits)
		}
	}
}

func TestCombinationsSpec(t *testing.T) {
	cases := []struct {
		m, w int
		want float64
	}{
		{5, 0, 1}, {5, -1, 1}, {1, 2, 0}, {0, 1, 0},
		{50, 2, 1225}, {48, 2, 1128}, {1000, 20, 3.3948281130245768e+41},
		{990, 1, 990}, {10, 1, 10},
	}
	for _, c := range cases {
		if got := combinations(c.m, c.w); got != c.want {
			t.Errorf("combinations(%d, %d) = %v, want %v", c.m, c.w, got, c.want)
		}
	}
}

// TestPortableSamplerFirstStep replays §1.4 + §3.8 step 1 of the chain fixture
// through the sampler: u = 1 - x1 and dt = (-plog(u)) / a0 with a0 = 100.
func TestPortableSamplerFirstStep(t *testing.T) {
	s := &portableSampler{x: newXoshiro256(42)}
	w := s.wait()
	if got := math.Float64bits(1 - 0.08386297105988216); got != 0x3FED50FE9A3E7A28 {
		t.Fatalf("u bits = %#x", got)
	}
	dt := w / 100
	if got := math.Float64bits(dt); got != 0x3F4CB3868D3A58D8 {
		t.Errorf("dt bits = %#x, want 0x3F4CB3868D3A58D8 (%v)", got, dt)
	}
	if r := s.uniform() * 100; r != 37.89802506626686 {
		t.Errorf("r = %v, want 37.89802506626686", r)
	}
}

// TestPortableChainReference checks the ensemble values §3.8 quotes for the
// chain fixture, against Simulate on the portable path.
func TestPortableChainReference(t *testing.T) {
	m := &metamodel.Model{
		Name: "chain",
		Places: []metamodel.Place{
			{ID: "a", Initial: 100}, {ID: "b", Initial: 0}, {ID: "c", Initial: 0},
		},
		Transitions: []metamodel.Transition{{ID: "ab", Rate: 1}, {ID: "bc", Rate: 1}},
		Arcs: []metamodel.Arc{
			{From: "a", To: "ab"}, {From: "ab", To: "b"}, {From: "b", To: "bc"}, {From: "bc", To: "c"},
		},
	}
	res, err := Simulate(m, nil, Options{Horizon: 10, Samples: 11, Realizations: 3, Seed: 42, Portable: true})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Series[0]
	if a.Place != "a" {
		t.Fatalf("series[0] is %q", a.Place)
	}
	if got := math.Float64bits(a.Values[1]); got != 0x4041D55555555555 {
		t.Errorf("a.values[1] = %v (%#x), want 35.666666666666664", a.Values[1], got)
	}
	if got := math.Float64bits(a.StdDev[1]); got != 0x400A660E223F1B70 {
		t.Errorf("a.stddev[1] = %v (%#x), want 3.2998316455372603", a.StdDev[1], got)
	}
	if res.Final["a"] != 0 || res.Final["b"] != 0 || res.Final["c"] != 100 {
		t.Errorf("final = %v, want a:0 b:0 c:100", res.Final)
	}
}

// TestPortableOptionZeroValueIsDefaultPath pins that Options{} still runs the
// math/rand path: the default-path golden test (parity_test.go) is the
// authority, this just proves the two paths are not the same stream.
func TestPortableOptionZeroValueIsDefaultPath(t *testing.T) {
	m := paritySIR()
	opts := Options{Horizon: 40, Samples: 81, Realizations: 8, Seed: 11}
	std, err := Simulate(m, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.Portable = true
	portable, err := Simulate(m, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	same := true
	for i := range std.Series {
		for j := range std.Series[i].Values {
			if std.Series[i].Values[j] != portable.Series[i].Values[j] {
				same = false
			}
		}
	}
	if same {
		t.Error("portable and default paths produced identical trajectories; the seam is not selecting")
	}
	// §3.8 sir ensemble reference values.
	for _, s := range portable.Series {
		if s.Place == "S" {
			if got := math.Float64bits(s.StdDev[1]); got != 0x3FF52A7FA9D2F8EA {
				t.Errorf("S.stddev[1] = %v (%#x), want 1.3228756555322954", s.StdDev[1], got)
			}
		}
	}
	if portable.Final["S"] != 9.625 || portable.Final["I"] != 71.5 || portable.Final["R"] != 918.875 {
		t.Errorf("sir final = %v, want S:9.625 I:71.5 R:918.875", portable.Final)
	}
}

// TestPortableNormalVectors pins normal()'s output at seed 42 — the
// reference values for pflow-rs/pflow-xyz/pflow-jl to match if SDE is ever
// ported to be byte-exact there too (not yet done; see go-pflow ROADMAP.md
// G6 and CAPABILITIES.md). Go is the reference implementation here, the
// same role it played for wait()/uniform() in ssa-spec.md — there is no
// external SDE spec these come from, only this implementation's own Marsaglia
// polar draws over the already-pinned xoshiro256**/plog primitives.
//
// Index 0-1 and 2-3 are each one accepted (u1, u2) pair (the polar method's
// spare-caching means indices 0 and 1 share one rejection-sampling draw,
// as do 2 and 3); index 4 starts a third pair whose spare (index 5) is not
// checked here.
func TestPortableNormalVectors(t *testing.T) {
	s := &portableSampler{x: newXoshiro256(42)}
	want := []uint64{
		0xbfe73d2feb0fb377,
		0xbfcb088028693f9c,
		0x3fcc5e21f7812a4c,
		0x3fe0ba8bb0c5fa51,
		0x3fddb514bfac5b4e,
	}
	for i, w := range want {
		v := s.normal()
		if got := math.Float64bits(v); got != w {
			t.Errorf("normal()[%d] = %v (%#x), want %#x", i, v, got, w)
		}
	}
}
