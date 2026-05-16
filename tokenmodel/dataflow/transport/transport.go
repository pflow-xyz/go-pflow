// Package transport is the L3.1 wire layer for subnet.Bundle execution.
//
// Layering goal (from the dataflow roadmap, L3):
//
//	Each subnet.Link becomes a typed channel — first an in-process Go
//	channel (this package), then a tiny gRPC/NATS shim. Same Transport
//	interface, two implementations. Distribution becomes a swap.
//
// Where Bundle.Flatten() collapses linked port-places into a single shared
// marking slot — fine for a single-goroutine driver — the Transport model
// keeps port-places LOCAL to each runner and moves tokens between them as
// counted messages on wires. The semantic contract is equivalence: for the
// same input sequence (Sends into source in-ports), a DistributedBundle and
// a Flatten()+State.Fire driver must arrive at the same final markings on
// every subnet's out-ports. The test TestDistributedBundleMatchesFlatten
// pins that invariant.
//
// IDs used on the wire:
//
//   - wireID  = "<fromSubnet>.<fromPort>"        (the producer side)
//   - linkID  = "<fromSubnet>.<fromPort>-><toSubnet>.<toPort>" (one consumer)
//
// A Send is addressed to a wireID and is broadcast to every receiver
// subscribed to that wire — this is the 1→N fan-out required by the
// watermark subnet pattern (one watermark out-port feeding many windowed
// subnets). A Recv is addressed to a linkID and returns the per-receiver
// queued count.
package transport

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// Transport is the abstraction that lets a subnet.Link become a wire rather
// than a marking-slot alias. The Go-channel implementation in this package
// is the prototype; a later gRPC/NATS implementation satisfies the same
// interface so distribution is a configuration swap.
//
// Send addresses a wire (producer side) and delivers `tokens` to every
// receiver registered for that wire. Recv addresses a single receiver
// (linkID) and non-blockingly drains the next pending count from its
// private queue.
type Transport interface {
	Send(wireID string, tokens int) error
	Recv(linkID string) (tokens int, ok bool)
	Close() error
}

// WireID returns the canonical producer-side wire identifier for a Link.
func WireID(l subnet.Link) string {
	return l.FromSubnet + "." + l.FromPort
}

// LinkID returns the canonical per-receiver identifier for a Link.
func LinkID(l subnet.Link) string {
	return l.FromSubnet + "." + l.FromPort + "->" + l.ToSubnet + "." + l.ToPort
}

// LocalChannelTransport implements Transport with one buffered Go channel
// per linkID (receiver). Send fans out to all receivers on the wire.
type LocalChannelTransport struct {
	mu       sync.RWMutex
	queues   map[string]chan int // linkID -> per-receiver queue
	wires    map[string][]string // wireID -> receiver linkIDs
	closed   bool
	bufSize  int
}

// NewLocal constructs a LocalChannelTransport from a slice of links.
// bufferSize is applied uniformly to every per-receiver channel; if it is
// <= 0 a default of 1024 is used.
func NewLocal(links []subnet.Link, bufferSize int) *LocalChannelTransport {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	t := &LocalChannelTransport{
		queues:  make(map[string]chan int),
		wires:   make(map[string][]string),
		bufSize: bufferSize,
	}
	for _, l := range links {
		lid := LinkID(l)
		wid := WireID(l)
		if _, ok := t.queues[lid]; !ok {
			t.queues[lid] = make(chan int, bufferSize)
			t.wires[wid] = append(t.wires[wid], lid)
		}
	}
	// Sort receiver lists for deterministic fan-out order.
	for w := range t.wires {
		sort.Strings(t.wires[w])
	}
	return t
}

// Send broadcasts a token count to every receiver on wireID. Returns an
// error if the transport is closed or if a receiver's buffer is full
// (blocking would mask the buffer-sizing decision).
func (t *LocalChannelTransport) Send(wireID string, tokens int) error {
	if tokens <= 0 {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.closed {
		return ErrClosed
	}
	receivers, ok := t.wires[wireID]
	if !ok {
		// No subscribers — treated as a dangling write (silently dropped).
		// This matches Flatten semantics where an unlinked out-port simply
		// accumulates tokens that nothing reads.
		return nil
	}
	for _, lid := range receivers {
		ch := t.queues[lid]
		select {
		case ch <- tokens:
		default:
			return fmt.Errorf("transport: receiver %q buffer full (size=%d)", lid, t.bufSize)
		}
	}
	return nil
}

// Recv non-blockingly drains the next queued count for linkID. Returns
// (0, false) when nothing is pending.
func (t *LocalChannelTransport) Recv(linkID string) (int, bool) {
	t.mu.RLock()
	ch, ok := t.queues[linkID]
	t.mu.RUnlock()
	if !ok {
		return 0, false
	}
	select {
	case n := <-ch:
		return n, true
	default:
		return 0, false
	}
}

// Close releases all per-receiver channels. Idempotent.
func (t *LocalChannelTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	for _, ch := range t.queues {
		close(ch)
	}
	return nil
}

// ErrClosed is returned by Send on a closed Transport.
var ErrClosed = errors.New("transport: closed")

// inLinks returns the links whose ToSubnet equals subnetID, indexed by
// (in-port place ID) -> []linkID. Multiple producers may target the same
// in-port (fan-in); all of their queues drain into that place.
func inLinks(b *subnet.Bundle, s *subnet.Subnet) map[string][]string {
	out := map[string][]string{}
	for _, l := range b.Links {
		if l.ToSubnet != s.ID {
			continue
		}
		port := s.PortByID(l.ToPort)
		if port == nil {
			continue
		}
		out[port.Place] = append(out[port.Place], LinkID(l))
	}
	return out
}

// outWires returns the wireIDs to broadcast on when a token appears in
// out-port place P, indexed by (out-port place ID) -> []wireID. With
// fan-out (one out-port linked to N in-ports) the producer side still
// has a single wireID, but multiple Link entries; we dedupe.
func outWires(b *subnet.Bundle, s *subnet.Subnet) map[string][]string {
	wireSeen := map[string]map[string]bool{}
	out := map[string][]string{}
	for _, l := range b.Links {
		if l.FromSubnet != s.ID {
			continue
		}
		port := s.PortByID(l.FromPort)
		if port == nil {
			continue
		}
		w := WireID(l)
		if wireSeen[port.Place] == nil {
			wireSeen[port.Place] = map[string]bool{}
		}
		if wireSeen[port.Place][w] {
			continue
		}
		wireSeen[port.Place][w] = true
		out[port.Place] = append(out[port.Place], w)
	}
	return out
}

// portPlaceKind classifies each place in a subnet as in-port / out-port /
// internal. Used by SubnetRunner to know where to drain to / pull from.
type portPlaceKind int

const (
	kindInternal portPlaceKind = iota
	kindIn
	kindOut
)

func classifyPlaces(s *subnet.Subnet) map[string]portPlaceKind {
	out := map[string]portPlaceKind{}
	for _, p := range s.Model.Places {
		out[p.ID] = kindInternal
	}
	for _, p := range s.Ports {
		switch p.Kind {
		case subnet.PortIn:
			out[p.Place] = kindIn
		case subnet.PortOut:
			out[p.Place] = kindOut
		}
	}
	return out
}
