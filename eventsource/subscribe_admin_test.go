package eventsource_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/eventsource"
)

func mustEvent(t *testing.T, stream, typ string, data any) *eventsource.Event {
	t.Helper()
	e, err := eventsource.NewEvent(stream, typ, data)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return e
}

// --- Repository.Execute -----------------------------------------------------

func TestRepositoryExecute(t *testing.T) {
	ctx := context.Background()
	store := eventsource.NewMemoryStore()
	defer store.Close()

	repo := eventsource.NewRepository(store, func(id string) eventsource.Aggregate {
		return NewOrderAggregate(id)
	})

	handler := func(ctx context.Context, agg eventsource.Aggregate, cmd eventsource.Command) ([]*eventsource.Event, error) {
		switch cmd.Type {
		case "create":
			e := mustEvent(t, cmd.AggregateID, "OrderCreated", map[string]any{"items": []string{"x"}})
			return []*eventsource.Event{e}, nil
		default:
			return nil, errors.New("unknown command")
		}
	}

	if err := repo.Execute(ctx, "o1", eventsource.Command{Type: "create", AggregateID: "o1"}, handler); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Events persisted and replayable: a fresh load sees the state.
	agg, err := repo.Load(ctx, "o1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if agg.Version() != 0 {
		t.Errorf("version = %d, want 0 after one event", agg.Version())
	}

	// Handler errors propagate and persist nothing.
	if err := repo.Execute(ctx, "o1", eventsource.Command{Type: "nope"}, handler); err == nil {
		t.Error("Execute should surface handler errors")
	}
	if v, _ := store.StreamVersion(ctx, "o1"); v != 0 {
		t.Errorf("failed command must not append events; version = %d", v)
	}
}

// --- MemoryStore.Subscribe ----------------------------------------------------

func TestMemoryStoreSubscribe(t *testing.T) {
	ctx := context.Background()
	store := eventsource.NewMemoryStore()
	defer store.Close()

	sub, err := store.Subscribe(ctx, eventsource.EventFilter{Types: []string{"Wanted"}})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if _, err := store.Append(ctx, "s1", -1, []*eventsource.Event{
		mustEvent(t, "s1", "Ignored", nil),
		mustEvent(t, "s1", "Wanted", map[string]any{"n": 1}),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	select {
	case e := <-sub.Events():
		if e.Type != "Wanted" {
			t.Errorf("filter leaked event type %q", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered within 2s")
	}

	// The filtered-out event must not arrive next.
	select {
	case e := <-sub.Events():
		t.Errorf("unexpected second event: %q", e.Type)
	case <-time.After(100 * time.Millisecond):
	}

	if err := sub.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMemoryStoreSubscribeStreamFilter(t *testing.T) {
	ctx := context.Background()
	store := eventsource.NewMemoryStore()
	defer store.Close()

	sub, err := store.Subscribe(ctx, eventsource.EventFilter{StreamID: "watched"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	_, _ = store.Append(ctx, "other", -1, []*eventsource.Event{mustEvent(t, "other", "E", nil)})
	_, _ = store.Append(ctx, "watched", -1, []*eventsource.Event{mustEvent(t, "watched", "E", nil)})

	select {
	case e := <-sub.Events():
		if e.StreamID != "watched" {
			t.Errorf("stream filter leaked %q", e.StreamID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered")
	}
}

// --- Admin queries -------------------------------------------------------------

func TestMemoryStoreListInstancesAndStats(t *testing.T) {
	ctx := context.Background()
	store := eventsource.NewMemoryStore()
	defer store.Close()

	// Three instances; the state place comes from the event payload shape the
	// store indexes.
	for _, id := range []string{"a", "b", "c"} {
		if _, err := store.Append(ctx, id, -1, []*eventsource.Event{
			mustEvent(t, id, "Created", map[string]any{"v": id}),
		}); err != nil {
			t.Fatal(err)
		}
	}

	instances, total, err := store.ListInstances(ctx, "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if total != 3 || len(instances) != 3 {
		t.Errorf("total=%d len=%d, want 3/3", total, len(instances))
	}

	// Pagination.
	page1, total, err := store.ListInstances(ctx, "", "", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(page1) != 2 {
		t.Errorf("page1: total=%d len=%d, want 3/2", total, len(page1))
	}
	page2, _, err := store.ListInstances(ctx, "", "", "", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Errorf("page2 len=%d, want 1", len(page2))
	}

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalInstances != 3 {
		t.Errorf("stats.TotalInstances = %d, want 3", stats.TotalInstances)
	}
}

// --- SQLite parity ---------------------------------------------------------------

func TestSQLiteSubscribeAndSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := eventsource.NewSQLiteStore(filepath.Join(t.TempDir(), "es.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	// Subscribe delivers appended events, same contract as memory.
	sub, err := store.Subscribe(ctx, eventsource.EventFilter{Types: []string{"Wanted"}})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	if _, err := store.Append(ctx, "s1", -1, []*eventsource.Event{
		mustEvent(t, "s1", "Wanted", map[string]any{"n": 1}),
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-sub.Events():
		if e.Type != "Wanted" {
			t.Errorf("got %q", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sqlite subscription delivered nothing")
	}

	// Snapshot round trip.
	snap := &eventsource.Snapshot{StreamID: "s1", Version: 0, State: []byte(`{"x":1}`)}
	if err := store.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	back, err := store.LoadSnapshot(ctx, "s1")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if back == nil || back.Version != 0 || string(back.State) != `{"x":1}` {
		t.Errorf("snapshot round trip: %+v", back)
	}

	// Newer snapshot supersedes.
	if err := store.SaveSnapshot(ctx, &eventsource.Snapshot{StreamID: "s1", Version: 5, State: []byte(`{"x":2}`)}); err != nil {
		t.Fatal(err)
	}
	back, err = store.LoadSnapshot(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if back.Version != 5 {
		t.Errorf("LoadSnapshot returned version %d, want the latest (5)", back.Version)
	}

	// Missing stream: distinguishable from an error.
	if snap, err := store.LoadSnapshot(ctx, "ghost"); err == nil && snap != nil {
		t.Errorf("LoadSnapshot(ghost) = %+v, want nil", snap)
	}
}
