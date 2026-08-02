package eventsource

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func multiAppendStores(t *testing.T) map[string]Store {
	t.Helper()
	sqlite, err := NewSQLiteStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlite.Close() })
	return map[string]Store{
		"memory": NewMemoryStore(),
		"sqlite": sqlite,
	}
}

func evt(typ string) *Event {
	return &Event{Type: typ, Data: json.RawMessage(`{}`)}
}

// The whole point: both streams advance, or neither does.
func TestMultiAppendAtomicity(t *testing.T) {
	for name, store := range multiAppendStores(t) {
		t.Run(name, func(t *testing.T) {
			ma, ok := store.(MultiAppender)
			if !ok {
				t.Fatalf("%T does not implement MultiAppender", store)
			}
			ctx := context.Background()

			// Happy path: two new streams.
			err := ma.MultiAppend(ctx, []StreamAppend{
				{StreamID: "order/1", ExpectedVersion: -1, Events: []*Event{evt("order.place_order")}},
				{StreamID: "inventory/1", ExpectedVersion: -1, Events: []*Event{evt("inventory.reserve_stock")}},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range []string{"order/1", "inventory/1"} {
				v, err := store.StreamVersion(ctx, stream)
				if err != nil || v != 0 {
					t.Fatalf("%s version = %d (%v), want 0", stream, v, err)
				}
			}

			// Conflict on the SECOND stream must roll back the first too.
			err = ma.MultiAppend(ctx, []StreamAppend{
				{StreamID: "order/1", ExpectedVersion: 0, Events: []*Event{evt("order.ship")}},
				{StreamID: "inventory/1", ExpectedVersion: 99, Events: []*Event{evt("inventory.release")}},
			})
			if err != ErrConcurrencyConflict {
				t.Fatalf("err = %v, want ErrConcurrencyConflict", err)
			}
			v, _ := store.StreamVersion(ctx, "order/1")
			if v != 0 {
				t.Fatalf("order/1 version = %d after failed multi-append, want 0 (nothing appended)", v)
			}

			// Duplicate stream in one batch is an error.
			err = ma.MultiAppend(ctx, []StreamAppend{
				{StreamID: "order/1", ExpectedVersion: -2, Events: []*Event{evt("a")}},
				{StreamID: "order/1", ExpectedVersion: -2, Events: []*Event{evt("b")}},
			})
			if err == nil {
				t.Fatal("duplicate stream must error")
			}

			// -2 skips the version check.
			if err := ma.MultiAppend(ctx, []StreamAppend{
				{StreamID: "order/1", ExpectedVersion: -2, Events: []*Event{evt("order.ship")}},
			}); err != nil {
				t.Fatal(err)
			}

			// Events are readable through the normal path.
			events, err := store.Read(ctx, "order/1", 0)
			if err != nil || len(events) != 2 {
				t.Fatalf("read order/1: %d events (%v), want 2", len(events), err)
			}
			if events[0].Type != "order.place_order" || events[1].Type != "order.ship" {
				t.Fatalf("event types: %s, %s", events[0].Type, events[1].Type)
			}
		})
	}
}
