package graphql

import (
	"context"
	"testing"

	"github.com/pflow-xyz/go-pflow/eventsource"
	"github.com/pflow-xyz/go-pflow/petri"
)

// coloredApprovalModel: "approve" consumes RED only.
func coloredApprovalModel(pendingRed, pendingBlue float64) *petri.PetriNet {
	m := petri.NewPetriNet()
	m.Token = []string{"red", "blue"}
	m.AddPlace("pending", []float64{pendingRed, pendingBlue}, nil, 0, 0, nil)
	m.AddPlace("approved", []float64{0, 0}, nil, 100, 0, nil)
	m.AddTransition("approve", "", 50, 0, nil)
	m.AddArc("pending", "approve", []float64{1, 0}, false)
	m.AddArc("approve", "approved", []float64{1, 0}, false)
	return m
}

// A colored model is unfolded, so the instance's marking is keyed by expanded
// names and each color is tracked separately.
func TestEventSourceStoreUnfoldsColors(t *testing.T) {
	ctx := context.Background()
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()

	store := NewEventSourceStore(memStore, coloredApprovalModel(1, 2), "approval")

	if store.ColorMap() == nil {
		t.Fatal("colored model was not unfolded")
	}

	id, err := store.Create(ctx, "approval")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	instance, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if instance.Marking["pending.red"] != 1 || instance.Marking["pending.blue"] != 2 {
		t.Errorf("marking lost the color split: %v", instance.Marking)
	}
	if _, ok := instance.Marking["pending"]; ok {
		t.Errorf("marking mixes base and expanded keys, which double-counts: %v", instance.Marking)
	}

	// The ColorMap folds it back to per-place totals for reporting.
	if total := store.ColorMap().SumByBase(instance.Marking)["pending"]; total != 3 {
		t.Errorf("SumByBase(pending) = %d, want 3", total)
	}
}

// Firing is decided per color: a pending pool holding only blue cannot satisfy
// a red arc, so "approve" is not enabled and firing it fails.
func TestEventSourceFiringIsPerColor(t *testing.T) {
	ctx := context.Background()
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()

	store := NewEventSourceStore(memStore, coloredApprovalModel(0, 5), "approval")

	id, err := store.Create(ctx, "approval")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	instance, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	for _, et := range instance.EnabledTransitions {
		if et == "approve" {
			t.Fatal("a red-only transition was enabled by blue tokens")
		}
	}

	if _, err := store.Fire(ctx, id, "approve", nil); err == nil {
		t.Error("firing a red transition on a blue-only marking succeeded")
	}
}

// With red present it fires, moving red and leaving blue alone.
func TestEventSourceFireMovesOnlyTheNamedColor(t *testing.T) {
	ctx := context.Background()
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()

	store := NewEventSourceStore(memStore, coloredApprovalModel(1, 5), "approval")

	id, err := store.Create(ctx, "approval")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	instance, err := store.Fire(ctx, id, "approve", nil)
	if err != nil {
		t.Fatalf("Fire() error = %v", err)
	}

	if instance.Marking["pending.red"] != 0 || instance.Marking["approved.red"] != 1 {
		t.Errorf("red did not move: %v", instance.Marking)
	}
	if instance.Marking["pending.blue"] != 5 || instance.Marking["approved.blue"] != 0 {
		t.Errorf("blue was disturbed by a red transition: %v", instance.Marking)
	}
}

func TestEventSourceSingleColorHasNoColorMap(t *testing.T) {
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()

	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)

	if NewEventSourceStore(memStore, model, "approval").ColorMap() != nil {
		t.Error("single-color model produced a ColorMap")
	}
}
