package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/tokenmodel/guard"
	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// SubnetRunner drives ONE subnet on ONE goroutine. Each tick it:
//
//  1. Polls every in-port's bound link queues via Transport.Recv and adds
//     the received counts to the local marking (Recv ADDS — a queued count
//     of N raises the in-port's tokens by N for Enabled() purposes).
//  2. Fires every enabled internal transition to quiescence.
//  3. Drains every out-port place down to zero, broadcasting the drained
//     count on the wire(s) bound to that out-port.
//
// Invariants disabled on the runner's private state: the flattened model
// is the canonical invariant scope, and a partitioned runner only sees its
// own places, so invariants written against cross-subnet sums would always
// fail. Bundle authors check invariants at the flattened level.
type SubnetRunner struct {
	subnet *subnet.Subnet
	tx     Transport

	mu      sync.RWMutex
	state   *tmpetri.State
	kinds   map[string]portPlaceKind
	inMap   map[string][]string // place -> []linkID
	outMap  map[string][]string // place -> []wireID

	// tick controls how often the runner re-polls when idle.
	tick time.Duration

	stopped chan struct{}
}

// NewRunner builds a SubnetRunner. The bundle is only needed at construction
// time to discover this subnet's link bindings; after that the runner uses
// the Transport alone.
func NewRunner(b *subnet.Bundle, s *subnet.Subnet, t Transport) *SubnetRunner {
	st := tmpetri.NewState(s.Model)
	st.CheckInvariants = false
	return &SubnetRunner{
		subnet:  s,
		tx:      t,
		state:   st,
		kinds:   classifyPlaces(s),
		inMap:   inLinks(b, s),
		outMap:  outWires(b, s),
		tick:    200 * time.Microsecond,
		stopped: make(chan struct{}),
	}
}

// Start blocks until ctx is cancelled, running the poll/fire/drain loop.
// Always returns nil on clean shutdown.
func (r *SubnetRunner) Start(ctx context.Context) error {
	defer close(r.stopped)
	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// One final drain so any in-flight tokens are flushed downstream.
			r.step()
			return nil
		case <-ticker.C:
			r.step()
		}
	}
}

// Stopped returns a channel closed when Start has returned.
func (r *SubnetRunner) Stopped() <-chan struct{} { return r.stopped }

// step performs one poll-fire-drain cycle. Exposed for the deterministic
// equivalence test.
func (r *SubnetRunner) step() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pollInputs()
	r.fireToQuiescence()
	r.drainOutputs()
}

// pollInputs drains every pending count from this subnet's link queues
// and adds them to the matching in-port place. Multiple producers fanned
// in to the same in-port simply sum.
func (r *SubnetRunner) pollInputs() {
	for place, lids := range r.inMap {
		for _, lid := range lids {
			for {
				n, ok := r.tx.Recv(lid)
				if !ok {
					break
				}
				r.state.Marking[place] += n
			}
		}
	}
}

// fireToQuiescence fires every enabled internal transition until none
// remain. Guards are evaluated against the local marking via
// FireWithGuardFuncs — without this, inputless guarded transitions
// (Pipeline's source `assign:[s,e)` and watermark `advance`) would fire
// to safetyCap on every tick because plain Fire ignores guards.
//
// Transitions whose guards reference unbound bindings (e.g.
// `event_time`) will fail evaluation and are skipped: those transitions
// belong to subnets the orchestrator drives directly (sources and
// watermark), not to subnets that should run autonomously here.
//
// Bounded by a generous safety cap so a runaway model can't hang the
// goroutine — in normal use this exits naturally.
func (r *SubnetRunner) fireToQuiescence() {
	const safetyCap = 1 << 20
	for i := 0; i < safetyCap; i++ {
		fired := false
		funcs := guard.MakeAggregates(r.state.Marking.AsGuardMarking())
		for _, t := range r.subnet.Model.Transitions {
			if !r.state.Enabled(t.ID) {
				continue
			}
			var err error
			if t.Guard == "" {
				err = r.state.Fire(t.ID)
			} else {
				err = r.state.FireWithGuardFuncs(t.ID, nil, funcs)
			}
			if err == nil {
				fired = true
				funcs = guard.MakeAggregates(r.state.Marking.AsGuardMarking())
			}
		}
		if !fired {
			return
		}
	}
}


// drainOutputs ships every token sitting on an out-port place onto its
// bound wire(s) and zeroes the local count. Out-ports that have no bound
// wires (terminal sinks — typical for a result port that nothing reads
// downstream) are NOT zeroed; their tokens remain available for the
// orchestrator to observe via Marking(). isQuiescent treats unwired
// out-ports as quiescent so a finished pipeline can settle.
func (r *SubnetRunner) drainOutputs() {
	for place, wires := range r.outMap {
		if len(wires) == 0 {
			continue
		}
		n := r.state.Marking[place]
		if n <= 0 {
			continue
		}
		for _, w := range wires {
			if err := r.tx.Send(w, n); err != nil {
				// Buffer full / closed — leave tokens in place so a later
				// drain can retry once the receiver catches up.
				return
			}
		}
		r.state.Marking[place] = 0
	}
}

// Marking returns a snapshot of the runner's private marking, including
// in-port and out-port places. Safe to call concurrently with Start.
func (r *SubnetRunner) Marking() tmpetri.Marking {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state.Marking.Clone()
}

// InjectInPort directly raises an in-port place's count. Used by
// DistributedBundle.SendToInPort for orchestrator-supplied source data
// (the equivalent of writing into a Pipeline before Run).
func (r *SubnetRunner) InjectInPort(portID string, tokens int) error {
	port := r.subnet.PortByID(portID)
	if port == nil || port.Kind != subnet.PortIn {
		return fmt.Errorf("transport: %q is not an in-port of subnet %q", portID, r.subnet.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Marking[port.Place] += tokens
	return nil
}

// DistributedBundle wires up one SubnetRunner per subnet plus a single
// Transport binding them by Link. Start launches all runners; Stop cancels
// the context and joins.
type DistributedBundle struct {
	bundle  *subnet.Bundle
	tx      *LocalChannelTransport
	runners map[string]*SubnetRunner

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex
}

// NewDistributedBundle constructs runners for every subnet in b, bound by
// a single LocalChannelTransport.
func NewDistributedBundle(b *subnet.Bundle, bufferSize int) *DistributedBundle {
	tx := NewLocal(b.Links, bufferSize)
	d := &DistributedBundle{
		bundle:  b,
		tx:      tx,
		runners: make(map[string]*SubnetRunner, len(b.Subnets)),
	}
	for i := range b.Subnets {
		s := &b.Subnets[i]
		d.runners[s.ID] = NewRunner(b, s, tx)
	}
	return d
}

// Runner returns the runner for subnetID.
func (d *DistributedBundle) Runner(subnetID string) *SubnetRunner {
	return d.runners[subnetID]
}

// Transport returns the underlying LocalChannelTransport (test/debug hook).
func (d *DistributedBundle) Transport() *LocalChannelTransport { return d.tx }

// SendToInPort injects tokens into an in-port place of a subnet —
// the orchestrator API for source data.
func (d *DistributedBundle) SendToInPort(subnetID, portID string, tokens int) error {
	r, ok := d.runners[subnetID]
	if !ok {
		return fmt.Errorf("transport: subnet %q not found", subnetID)
	}
	return r.InjectInPort(portID, tokens)
}

// Start launches one goroutine per runner. Returns once they are all
// scheduled.
func (d *DistributedBundle) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return fmt.Errorf("transport: bundle already started")
	}
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	for _, r := range d.runners {
		d.wg.Add(1)
		go func(r *SubnetRunner) {
			defer d.wg.Done()
			_ = r.Start(ctx)
		}(r)
	}
	d.started = true
	return nil
}

// Stop cancels every runner's context, waits for them to drain, and
// closes the transport. Safe to call multiple times.
func (d *DistributedBundle) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		return nil
	}
	d.cancel()
	d.wg.Wait()
	d.started = false
	return d.tx.Close()
}

// Marking returns a snapshot of a subnet's local marking.
func (d *DistributedBundle) Marking(subnetID string) tmpetri.Marking {
	r, ok := d.runners[subnetID]
	if !ok {
		return nil
	}
	return r.Marking()
}

// Quiesce waits until either timeout elapses or no runner has any pending
// recv queue activity and every runner is at quiescence (no enabled
// transitions). Used by tests to deterministically observe a settled
// distributed marking without sleeping arbitrary durations.
func (d *DistributedBundle) Quiesce(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if d.isQuiescent() {
			// One more confirmation tick to catch a token in transit.
			time.Sleep(2 * d.tickInterval())
			if d.isQuiescent() {
				return true
			}
		}
		time.Sleep(d.tickInterval())
	}
	return false
}

func (d *DistributedBundle) tickInterval() time.Duration {
	for _, r := range d.runners {
		return r.tick
	}
	return time.Millisecond
}

func (d *DistributedBundle) isQuiescent() bool {
	// Any pending queue? Any enabled transition? If yes -> not quiescent.
	d.tx.mu.RLock()
	for _, ch := range d.tx.queues {
		if len(ch) > 0 {
			d.tx.mu.RUnlock()
			return false
		}
	}
	d.tx.mu.RUnlock()
	for _, r := range d.runners {
		r.mu.RLock()
		funcs := guard.MakeAggregates(r.state.Marking.AsGuardMarking())
		for _, t := range r.subnet.Model.Transitions {
			if !r.state.Enabled(t.ID) {
				continue
			}
			// A guarded transition whose guard can't evaluate cleanly with
			// the current marking (typically because it references an
			// unbound binding like `event_time`) is owned by the
			// orchestrator, not by this runner. Treat as quiescent here.
			if t.Guard != "" {
				ok, err := guard.Evaluate(t.Guard, nil, funcs)
				if err != nil || !ok {
					continue
				}
			}
			r.mu.RUnlock()
			return false
		}
		// Any out-port still holding tokens pending drain? Skip terminal
		// out-ports (no bound wires) — they're sinks the orchestrator
		// reads via Marking(), not work the bundle still has to do.
		for place, kind := range r.kinds {
			if kind != kindOut || r.state.Marking[place] == 0 {
				continue
			}
			if len(r.outMap[place]) == 0 {
				continue
			}
			r.mu.RUnlock()
			return false
		}
		r.mu.RUnlock()
	}
	return true
}
