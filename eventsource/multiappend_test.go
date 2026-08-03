package eventsource

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
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

// Two store instances over one database file stand in for two processes: the
// in-process mutex does not span them, so only the database's own write lock
// can keep the version check honest. A deferred transaction takes that lock
// only at its first INSERT — by which point both writers have already read the
// same version and passed the check — so the loser dies on SQLITE_BUSY halfway
// through instead of being told, correctly and retryably, that it lost.
//
// Several streams per batch, several rounds: the wider the read phase, the more
// reliably the two writers actually overlap.
func TestMultiAppendConcurrentProcesses(t *testing.T) {
	ctx := context.Background()

	for round := 0; round < 20; round++ {
		path := filepath.Join(t.TempDir(), "events.db")

		stores := make([]*SQLiteStore, 2)
		for i := range stores {
			s, err := NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			stores[i] = s
		}

		var batch []StreamAppend
		for i := 0; i < 20; i++ {
			batch = append(batch, StreamAppend{
				StreamID:        fmt.Sprintf("member/%d", i),
				ExpectedVersion: -1,
				Events:          []*Event{evt("member.fire")},
			})
		}

		start := make(chan struct{})
		errs := make(chan error, len(stores))
		var wg sync.WaitGroup
		for _, s := range stores {
			wg.Add(1)
			go func(s *SQLiteStore) {
				defer wg.Done()
				<-start
				// Fresh Event values per writer; MultiAppend stamps them.
				own := make([]StreamAppend, len(batch))
				for i, a := range batch {
					own[i] = StreamAppend{StreamID: a.StreamID, ExpectedVersion: -1, Events: []*Event{evt("member.fire")}}
				}
				errs <- s.MultiAppend(ctx, own)
			}(s)
		}
		close(start)
		wg.Wait()
		close(errs)

		succeeded := 0
		for err := range errs {
			switch {
			case err == nil:
				succeeded++
			case err != ErrConcurrencyConflict:
				t.Fatalf("round %d: loser got %v, want ErrConcurrencyConflict", round, err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("round %d: %d of %d appends succeeded, want exactly 1", round, succeeded, len(stores))
		}

		for _, stream := range []string{"member/0", "member/19"} {
			v, err := stores[0].StreamVersion(ctx, stream)
			if err != nil || v != 0 {
				t.Fatalf("round %d: %s version = %d (%v), want 0", round, stream, v, err)
			}
		}
	}
}

// A coordinator that fires a member subnet and fences a read on that same
// subnet names one stream twice. That is one intent, and it must not be
// mistaken for two conflicting appends.
func TestMultiAppendReadFenceOnAppendedStream(t *testing.T) {
	for name, store := range multiAppendStores(t) {
		t.Run(name, func(t *testing.T) {
			ma := store.(MultiAppender)
			ctx := context.Background()

			err := ma.MultiAppend(ctx, []StreamAppend{
				{StreamID: "order/1", ExpectedVersion: -1, Events: []*Event{evt("order.place_order")}},
				{StreamID: "inventory/1", ExpectedVersion: -1, Events: []*Event{evt("inventory.reserve_stock")}},
				// Same subnet, read-fenced at the version it was fired at.
				{StreamID: "order/1", ExpectedVersion: -1},
			})
			if err != nil {
				t.Fatalf("member append + read fence on one stream: %v", err)
			}

			events, err := store.Read(ctx, "order/1", 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].Type != "order.place_order" {
				t.Fatalf("order/1 = %d events, want exactly the member's 1", len(events))
			}

			// A fence carrying a version the append does not expect is a
			// genuine disagreement and stays an error.
			err = ma.MultiAppend(ctx, []StreamAppend{
				{StreamID: "order/1", ExpectedVersion: 0, Events: []*Event{evt("order.ship")}},
				{StreamID: "order/1", ExpectedVersion: 7},
			})
			if err == nil {
				t.Fatal("fence at a different expected version must error")
			}
			if v, _ := store.StreamVersion(ctx, "order/1"); v != 0 {
				t.Fatalf("order/1 version = %d after rejected batch, want 0", v)
			}
		})
	}
}

// Append is a check-then-append too, so it needs the same cross-process
// discipline MultiAppend has: two store instances over one file, one winner,
// and the loser told it lost rather than dying on a raw SQLITE_BUSY partway
// through its INSERTs.
func TestAppendConcurrentProcesses(t *testing.T) {
	ctx := context.Background()

	for round := 0; round < 20; round++ {
		path := filepath.Join(t.TempDir(), "events.db")

		stores := make([]*SQLiteStore, 2)
		for i := range stores {
			s, err := NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			stores[i] = s
		}
		if _, err := stores[0].Append(ctx, "order/1", -1, []*Event{evt("order.place_order")}); err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		errs := make(chan error, len(stores))
		var wg sync.WaitGroup
		for _, s := range stores {
			wg.Add(1)
			go func(s *SQLiteStore) {
				defer wg.Done()
				<-start
				// Both writers read version 0 and both intend version 1.
				var events []*Event
				for i := 0; i < 20; i++ {
					events = append(events, evt(fmt.Sprintf("order.step%d", i)))
				}
				_, err := s.Append(ctx, "order/1", 0, events)
				errs <- err
			}(s)
		}
		close(start)
		wg.Wait()
		close(errs)

		succeeded := 0
		for err := range errs {
			switch {
			case err == nil:
				succeeded++
			case err != ErrConcurrencyConflict:
				t.Fatalf("round %d: loser got %v, want ErrConcurrencyConflict", round, err)
			}
		}
		if succeeded != 1 {
			t.Fatalf("round %d: %d of %d appends succeeded, want exactly 1", round, succeeded, len(stores))
		}
		if v, err := stores[0].StreamVersion(ctx, "order/1"); err != nil || v != 20 {
			t.Fatalf("round %d: version = %d (%v), want 20 (exactly one writer's events)", round, v, err)
		}
	}
}
