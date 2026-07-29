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
