package tokenmodel

import "testing"

func firingSchema(in Arc, outWeight int) *Schema {
	s := NewSchema("t")
	s.AddTokenState("a", 3).AddTokenState("gate", 2).AddTokenState("b", 0)
	s.AddAction(Action{ID: "t"})
	in.Source, in.Target = "gate", "t"
	s.AddArc(Arc{Source: "a", Target: "t", Weight: 2})
	s.AddArc(in)
	s.AddArc(Arc{Source: "t", Target: "b", Weight: outWeight})
	return s
}

func TestWeightHonored(t *testing.T) {
	s := NewSchema("w")
	s.AddTokenState("a", 3).AddTokenState("b", 0).AddAction(Action{ID: "t"})
	s.AddArc(Arc{Source: "a", Target: "t", Weight: 2}).AddArc(Arc{Source: "t", Target: "b", Weight: 3})
	r := NewRuntime(s)
	if err := r.Execute("t"); err != nil {
		t.Fatal(err)
	}
	if r.Tokens("a") != 1 || r.Tokens("b") != 3 {
		t.Fatalf("a=%d b=%d", r.Tokens("a"), r.Tokens("b"))
	}
	if r.Enabled("t") { // 1 < weight 2; old code would say enabled
		t.Fatal("should not be enabled with 1 token and weight 2")
	}
	if r.Execute("t") != ErrActionNotEnabled {
		t.Fatal("expected ErrActionNotEnabled")
	}
}

func TestUnsetWeightIsOne(t *testing.T) {
	s := NewSchema("d")
	s.AddTokenState("a", 1).AddTokenState("b", 0).AddAction(Action{ID: "t"})
	s.AddArc(Arc{Source: "a", Target: "t"}).AddArc(Arc{Source: "t", Target: "b"})
	r := NewRuntime(s)
	if err := r.Execute("t"); err != nil || r.Tokens("a") != 0 || r.Tokens("b") != 1 {
		t.Fatal("default weight 1 broken")
	}
}

func TestReadArc(t *testing.T) {
	r := NewRuntime(firingSchema(Arc{Type: ReadArc, Weight: 2}, 1))
	if err := r.Execute("t"); err != nil {
		t.Fatal(err)
	}
	if r.Tokens("gate") != 2 {
		t.Fatalf("read arc consumed: gate=%d", r.Tokens("gate"))
	}
	r2 := NewRuntime(firingSchema(Arc{Type: ReadArc, Weight: 3}, 1))
	if r2.Enabled("t") {
		t.Fatal("read arc weight 3 with 2 tokens must block")
	}
}

func TestInhibitorArc(t *testing.T) {
	// blocks at >= weight
	if NewRuntime(firingSchema(Arc{Type: InhibitorArc, Weight: 2}, 1)).Enabled("t") {
		t.Fatal("inhibitor should block at 2 >= 2")
	}
	r := NewRuntime(firingSchema(Arc{Type: InhibitorArc, Weight: 3}, 1))
	if !r.Enabled("t") {
		t.Fatal("inhibitor should allow at 2 < 3")
	}
	if err := r.Execute("t"); err != nil || r.Tokens("gate") != 2 {
		t.Fatalf("inhibitor must not consume: gate=%d err=%v", r.Tokens("gate"), err)
	}
}

func TestExecuteWithBindingsWeights(t *testing.T) {
	r := NewRuntime(firingSchema(Arc{Type: ReadArc}, 2))
	if err := r.ExecuteWithBindings("t", nil); err != nil {
		t.Fatal(err)
	}
	if r.Tokens("a") != 1 || r.Tokens("b") != 2 || r.Tokens("gate") != 2 {
		t.Fatalf("a=%d b=%d gate=%d", r.Tokens("a"), r.Tokens("b"), r.Tokens("gate"))
	}
}
