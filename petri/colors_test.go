package petri

import "testing"

func TestExpandColorsSingleColorIsIdentity(t *testing.T) {
	net := Build().Place("a", 2).Transition("t").Arc("a", "t", 1).Done()
	out, cm := net.ExpandColors()
	if out != net || cm != nil {
		t.Error("single-color net must be returned as-is with nil ColorMap")
	}
	if net.IsMultiColor() {
		t.Error("IsMultiColor = true for a scalar net")
	}
}

func TestExpandColorsBasic(t *testing.T) {
	net := NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{3, 1}, nil, 0, 0, nil)
	net.AddPlace("out", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("move", "default", 0, 0, nil)
	net.AddArc("pool", "move", []float64{1, 2}, false)
	net.AddArc("move", "out", []float64{1, 2}, false)

	if !net.IsMultiColor() {
		t.Fatal("IsMultiColor = false")
	}

	out, cm := net.ExpandColors()
	if cm == nil {
		t.Fatal("nil ColorMap for a 2-color net")
	}

	// 2 places x 2 colors.
	if len(out.Places) != 4 {
		t.Fatalf("expanded places = %d, want 4: %v", len(out.Places), keys(out.Places))
	}
	if out.Places["pool.red"].GetTokenCount() != 3 || out.Places["pool.blue"].GetTokenCount() != 1 {
		t.Errorf("initial markings wrong: red=%f blue=%f",
			out.Places["pool.red"].GetTokenCount(), out.Places["pool.blue"].GetTokenCount())
	}

	// One transition, shared; four arcs (both colors non-zero both ways).
	if len(out.Transitions) != 1 {
		t.Errorf("transitions = %d, want 1", len(out.Transitions))
	}
	if len(out.Arcs) != 4 {
		t.Errorf("arcs = %d, want 4", len(out.Arcs))
	}

	// Mapping round trip.
	base, color, ok := cm.BaseName("pool.blue")
	if !ok || base != "pool" || color != "blue" {
		t.Errorf("BaseName(pool.blue) = %q,%q,%v", base, color, ok)
	}
	sum := cm.SumByBase(map[string]int{"pool.red": 2, "pool.blue": 1, "out.red": 1})
	if sum["pool"] != 3 || sum["out"] != 1 {
		t.Errorf("SumByBase = %v", sum)
	}
}

// TestExpandColorsZeroWeightComponentsSkipped: a weight component of zero
// imposes nothing and moves nothing, so no arc is created for that color —
// matching petri-sim.js where wVal ?? 0 contributes no constraint.
func TestExpandColorsZeroWeightComponentsSkipped(t *testing.T) {
	net := NewPetriNet()
	net.AddPlace("p", []float64{1, 1}, nil, 0, 0, nil)
	net.AddPlace("q", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("p", "t", []float64{1, 0}, false) // only color 0 consumed
	net.AddArc("t", "q", []float64{0, 2}, false) // only color 1 produced

	out, _ := net.ExpandColors()
	if len(out.Arcs) != 2 {
		t.Fatalf("arcs = %d, want 2 (zero components skipped)", len(out.Arcs))
	}
}

// TestExpandColorsShorterVectors: places and arcs with shorter vectors than
// the net's color count get zeros/unbounded for the missing components.
func TestExpandColorsShorterVectors(t *testing.T) {
	net := NewPetriNet()
	net.AddPlace("wide", []float64{1, 2}, []float64{5, 0}, 0, 0, nil) // cap: color0=5, color1 unbounded
	net.AddPlace("narrow", 4.0, nil, 0, 0, nil)                       // scalar: color0 only
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("narrow", "t", 1.0, false) // scalar weight: color 0

	out, cm := net.ExpandColors()

	if out.Places["narrow.c1"].GetTokenCount() != 0 {
		t.Errorf("missing color component should start at 0")
	}
	if got := out.Places["wide.c0"].Capacity; len(got) != 1 || got[0] != 5 {
		t.Errorf("wide.c0 capacity = %v, want [5]", got)
	}
	if got := out.Places["wide.c1"].Capacity; len(got) != 0 {
		t.Errorf("wide.c1 capacity = %v, want unbounded (empty)", got)
	}
	if len(out.Arcs) != 1 {
		t.Errorf("scalar arc should expand to color 0 only, got %d arcs", len(out.Arcs))
	}
	if cm.Colors[0] != "c0" || cm.Colors[1] != "c1" {
		t.Errorf("default color names = %v", cm.Colors)
	}
}

// TestExpandColorsNameCollision: a literal place named "pool.red" must not
// collide with the expansion of a colored "pool".
func TestExpandColorsNameCollision(t *testing.T) {
	net := NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{1, 1}, nil, 0, 0, nil)
	net.AddPlace("pool.red", 7.0, nil, 0, 0, nil) // adversarial literal name
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("pool", "t", []float64{1, 1}, false)

	out, cm := net.ExpandColors()

	// The literal place survives with its own tokens, distinct from the
	// expansion of "pool".
	total := 0.0
	for name, p := range out.Places {
		_ = name
		total += p.GetTokenCount()
	}
	if total != 9 { // 1+1 from pool colors (+0 for pool.red colors 1) + 7 literal color0
		t.Errorf("token mass corrupted by collision handling: %f", total)
	}
	// Every expanded name must be unique.
	seen := map[string]bool{}
	for _, names := range cm.Expanded {
		for _, n := range names {
			if seen[n] {
				t.Fatalf("duplicate expanded name %q", n)
			}
			seen[n] = true
		}
	}
}

func keys(m map[string]*Place) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestExpandStateRoundTrip pins the property that makes ExpandState safe as
// the default translation in solver.NewProblem: expanding the net's OWN state
// reproduces each place's declared per-color vector exactly, so the ordinary
// NewProblem(net, net.SetState(nil), …) call loses nothing.
func TestExpandStateRoundTrip(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{2, 6}, nil, 0, 0, nil)
	n.AddPlace("sink", []float64{0, 0}, nil, 0, 0, nil)

	got := n.ExpandState(n.SetState(nil))

	want := map[string]float64{
		"pool.red": 2, "pool.blue": 6,
		"sink.red": 0, "sink.blue": 0,
	}
	for name, v := range want {
		if got[name] != v {
			t.Errorf("%s = %v, want %v", name, got[name], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d entries %v, want %d", len(got), got, len(want))
	}
}

// A base total that is not the declared total scales every color by the same
// factor — "run this model at half the population" keeps the color ratio.
func TestExpandStateScalesProportionally(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{2, 6}, nil, 0, 0, nil)

	got := n.ExpandState(map[string]float64{"pool": 4})

	if got["pool.red"] != 1 || got["pool.blue"] != 3 {
		t.Errorf("got red=%v blue=%v, want 1 and 3", got["pool.red"], got["pool.blue"])
	}
}

// A place that declares no tokens has no proportions to follow, so the whole
// total goes to the first color rather than being silently dropped.
func TestExpandStateEmptyDeclarationGoesToFirstColor(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{2, 6}, nil, 0, 0, nil)
	n.AddPlace("sink", []float64{0, 0}, nil, 0, 0, nil)

	got := n.ExpandState(map[string]float64{"sink": 5})

	if got["sink.red"] != 5 || got["sink.blue"] != 0 {
		t.Errorf("got red=%v blue=%v, want 5 and 0", got["sink.red"], got["sink.blue"])
	}
}

// Already-expanded keys pass through untouched, which is what makes it safe
// for a caller to hand ExpandState a state it has already expanded.
func TestExpandStateIsIdempotent(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{2, 6}, nil, 0, 0, nil)

	once := n.ExpandState(n.SetState(nil))
	twice := n.ExpandState(once)

	if len(once) != len(twice) {
		t.Fatalf("second expansion changed size: %v vs %v", once, twice)
	}
	for k, v := range once {
		if twice[k] != v {
			t.Errorf("%s: %v -> %v", k, v, twice[k])
		}
	}
}

func TestExpandStateSingleColorIsNoOp(t *testing.T) {
	n := NewPetriNet()
	n.AddPlace("pool", 5.0, nil, 0, 0, nil)

	in := n.SetState(nil)
	got := n.ExpandState(in)

	if got["pool"] != 5 || len(got) != 1 {
		t.Errorf("single-color net was rewritten: %v", got)
	}
}

func TestSumByBaseFloatAndLookup(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{2, 6}, nil, 0, 0, nil)
	_, cm := n.ExpandColors()

	folded := cm.SumByBaseFloat(map[string]float64{"pool.red": 1.5, "pool.blue": 2.5})
	if folded["pool"] != 4 {
		t.Errorf("SumByBaseFloat: got %v, want 4", folded["pool"])
	}

	if got := cm.Lookup("pool"); len(got) != 2 {
		t.Errorf("Lookup(base): got %v, want 2 names", got)
	}
	// An expanded name is already a single color; an unknown name is its own.
	if got := cm.Lookup("pool.red"); len(got) != 1 || got[0] != "pool.red" {
		t.Errorf("Lookup(expanded): got %v", got)
	}
	var nilMap *ColorMap
	if got := nilMap.Lookup("pool"); len(got) != 1 || got[0] != "pool" {
		t.Errorf("Lookup on nil ColorMap: got %v", got)
	}
	if got := nilMap.SumByBaseFloat(map[string]float64{"pool": 3}); got["pool"] != 3 {
		t.Errorf("SumByBaseFloat on nil ColorMap: got %v", got)
	}
}

// IsMultiColor answers from any of the three vector-valued fields, since a net
// can declare colors on places, on arcs, or only via Token.
func TestIsMultiColor(t *testing.T) {
	tests := []struct {
		name string
		mk   func() *PetriNet
		want bool
	}{
		{"empty net", func() *PetriNet { return NewPetriNet() }, false},
		{"single color place", func() *PetriNet {
			n := NewPetriNet()
			n.AddPlace("p", 1.0, nil, 0, 0, nil)
			return n
		}, false},
		{"two token names", func() *PetriNet {
			n := NewPetriNet()
			n.Token = []string{"red", "blue"}
			n.AddPlace("p", 1.0, nil, 0, 0, nil)
			return n
		}, true},
		{"multi-color initial", func() *PetriNet {
			n := NewPetriNet()
			n.AddPlace("p", []float64{1, 2}, nil, 0, 0, nil)
			return n
		}, true},
		{"multi-color capacity", func() *PetriNet {
			n := NewPetriNet()
			n.AddPlace("p", 1.0, []float64{3, 3}, 0, 0, nil)
			return n
		}, true},
		{"multi-color arc weight", func() *PetriNet {
			n := NewPetriNet()
			n.AddPlace("p", 1.0, nil, 0, 0, nil)
			n.AddTransition("t", "default", 0, 0, nil)
			n.AddArc("p", "t", []float64{1, 1}, false)
			return n
		}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			net := tc.mk()
			if got := net.IsMultiColor(); got != tc.want {
				t.Errorf("IsMultiColor() = %v, want %v", got, tc.want)
			}
			// colorCount and IsMultiColor must agree on where the line is.
			if got := net.colorCount() > 1; got != tc.want {
				t.Errorf("colorCount() > 1 = %v, disagrees with IsMultiColor", got)
			}
		})
	}
}

// BaseName has to answer for three kinds of input: an expanded name, a name
// the map has never seen, and a nil map (the single-color case, where callers
// hold a nil *ColorMap and must not have to check).
func TestBaseName(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{1, 1}, nil, 0, 0, nil)
	_, cm := n.ExpandColors()

	place, color, ok := cm.BaseName("pool.blue")
	if !ok || place != "pool" || color != "blue" {
		t.Errorf("BaseName(expanded) = %q, %q, %v", place, color, ok)
	}

	// An unknown name is returned unchanged rather than reported as an error,
	// so a caller can pass any label through.
	place, color, ok = cm.BaseName("somewhere-else")
	if ok || place != "somewhere-else" || color != "" {
		t.Errorf("BaseName(unknown) = %q, %q, %v", place, color, ok)
	}

	var nilMap *ColorMap
	place, color, ok = nilMap.BaseName("pool")
	if ok || place != "pool" || color != "" {
		t.Errorf("BaseName on nil ColorMap = %q, %q, %v", place, color, ok)
	}
}

// SumByBase passes non-place keys through and is a no-op on a nil map.
func TestSumByBase(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{1, 1}, nil, 0, 0, nil)
	_, cm := n.ExpandColors()

	got := cm.SumByBase(map[string]int{"pool.red": 2, "pool.blue": 3, "unrelated": 7})
	if got["pool"] != 5 {
		t.Errorf("SumByBase(pool) = %d, want 5", got["pool"])
	}
	if got["unrelated"] != 7 {
		t.Errorf("an unmapped key was dropped: %v", got)
	}

	var nilMap *ColorMap
	in := map[string]int{"pool": 5}
	if out := nilMap.SumByBase(in); out["pool"] != 5 {
		t.Errorf("SumByBase on nil ColorMap = %v", out)
	}
	// Same passthrough for the float variant.
	if out := cm.SumByBaseFloat(map[string]float64{"unrelated": 1.5}); out["unrelated"] != 1.5 {
		t.Errorf("SumByBaseFloat dropped an unmapped key: %v", out)
	}
}

// An arc with no declared weight defaults to [1] — one token of color 0 —
// rather than being skipped as a zero vector. The Builder API creates such
// arcs routinely, so a colored net can easily contain one.
func TestExpandColorsDefaultsUndeclaredArcWeight(t *testing.T) {
	n := NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("a", []float64{1, 1}, nil, 0, 0, nil)
	n.AddPlace("b", []float64{0, 0}, nil, 0, 0, nil)
	n.AddTransition("t", "default", 0, 0, nil)
	n.AddArc("a", "t", nil, false) // no weight declared
	n.AddArc("t", "b", nil, false)

	out, cm := n.ExpandColors()
	if cm == nil {
		t.Fatal("colored net was not unfolded")
	}

	// One arc per direction, on color 0 only — the default is [1], not [1,1].
	if len(out.Arcs) != 2 {
		t.Fatalf("got %d arcs, want 2: %+v", len(out.Arcs), out.Arcs)
	}
	for _, arc := range out.Arcs {
		if arc.GetWeightSum() != 1 {
			t.Errorf("default weight became %v, want 1", arc.GetWeightSum())
		}
		if arc.Source != "a.red" && arc.Target != "b.red" {
			t.Errorf("default weight landed on a color other than the first: %s -> %s",
				arc.Source, arc.Target)
		}
	}
}
