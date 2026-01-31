package graphql

import (
	"context"
	"testing"

	"github.com/pflow-xyz/go-pflow/eventsource"
	"github.com/pflow-xyz/go-pflow/petri"
)

func TestEventSourceStore_Create(t *testing.T) {
	ctx := context.Background()

	// Create a simple model
	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)

	// Create memory event store
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()

	// Create GraphQL store
	store := NewEventSourceStore(memStore, model, "approval")

	// Create instance
	id, err := store.Create(ctx, "approval")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if id == "" {
		t.Fatal("Expected non-empty ID")
	}

	// Get instance
	instance, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if instance.ID != id {
		t.Errorf("ID = %q, want %q", instance.ID, id)
	}

	if instance.Marking["pending"] != 1 {
		t.Errorf("pending tokens = %d, want 1", instance.Marking["pending"])
	}

	if instance.Marking["approved"] != 0 {
		t.Errorf("approved tokens = %d, want 0", instance.Marking["approved"])
	}

	// Check enabled transitions
	found := false
	for _, et := range instance.EnabledTransitions {
		if et == "approve" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("EnabledTransitions = %v, want to contain 'approve'", instance.EnabledTransitions)
	}
}

func TestEventSourceStore_Fire(t *testing.T) {
	ctx := context.Background()

	// Create a simple model
	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddPlace("rejected", 0, 0, 100, 100, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddTransition("reject", "", 50, 100, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)
	model.AddArc("pending", "reject", 1, false)
	model.AddArc("reject", "rejected", 1, false)

	// Create stores
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()
	store := NewEventSourceStore(memStore, model, "approval")

	// Create instance
	id, err := store.Create(ctx, "approval")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Fire approve transition
	instance, err := store.Fire(ctx, id, "approve", nil)
	if err != nil {
		t.Fatalf("Fire() error = %v", err)
	}

	// Check state changed
	if instance.Marking["pending"] != 0 {
		t.Errorf("pending tokens = %d, want 0", instance.Marking["pending"])
	}

	if instance.Marking["approved"] != 1 {
		t.Errorf("approved tokens = %d, want 1", instance.Marking["approved"])
	}

	// Approve should no longer be enabled
	for _, et := range instance.EnabledTransitions {
		if et == "approve" {
			t.Error("approve should not be enabled after firing")
		}
	}

	// Try to fire again - should fail
	_, err = store.Fire(ctx, id, "approve", nil)
	if err == nil {
		t.Error("Expected error when firing disabled transition")
	}
}

func TestEventSourceStore_List(t *testing.T) {
	ctx := context.Background()

	// Create model
	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)

	// Create stores
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()
	store := NewEventSourceStore(memStore, model, "approval")

	// Create multiple instances
	id1, _ := store.Create(ctx, "approval")
	id2, _ := store.Create(ctx, "approval")
	id3, _ := store.Create(ctx, "approval")

	// Approve one instance
	store.Fire(ctx, id2, "approve", nil)

	// List all
	instances, total, err := store.List(ctx, InstanceFilter{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}

	if len(instances) != 3 {
		t.Errorf("len(instances) = %d, want 3", len(instances))
	}

	// List by place filter
	pendingInstances, pendingTotal, err := store.List(ctx, InstanceFilter{Place: "pending"})
	if err != nil {
		t.Fatalf("List(pending) error = %v", err)
	}

	if pendingTotal != 2 {
		t.Errorf("pending total = %d, want 2", pendingTotal)
	}

	// Verify the approved instance is not in pending list
	for _, inst := range pendingInstances {
		if inst.ID == id2 {
			t.Error("Approved instance should not be in pending filter")
		}
	}

	_ = id1
	_ = id3
}

func TestEventSourceStore_Delete(t *testing.T) {
	ctx := context.Background()

	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)

	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()
	store := NewEventSourceStore(memStore, model, "test")

	// Create and delete
	id, _ := store.Create(ctx, "test")

	err := store.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Should not be found
	_, err = store.Get(ctx, id)
	if err == nil {
		t.Error("Expected error getting deleted instance")
	}
}

func TestEventSourceStore_Persistence(t *testing.T) {
	ctx := context.Background()

	// Create model
	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)

	// Create memory store (simulating persistence)
	memStore := eventsource.NewMemoryStore()
	defer memStore.Close()

	// First store instance - create and modify
	store1 := NewEventSourceStore(memStore, model, "approval")
	id, _ := store1.Create(ctx, "approval")
	store1.Fire(ctx, id, "approve", nil)

	// Second store instance - should reload from events
	store2 := NewEventSourceStore(memStore, model, "approval")

	instance, err := store2.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// State should be replayed correctly
	if instance.Marking["pending"] != 0 {
		t.Errorf("pending = %d, want 0", instance.Marking["pending"])
	}

	if instance.Marking["approved"] != 1 {
		t.Errorf("approved = %d, want 1", instance.Marking["approved"])
	}
}
