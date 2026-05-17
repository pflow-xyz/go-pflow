// Replayable command log for streaming pipelines.
//
// A Pipeline emits one PipelineEvent per externally-observable operation
// (Send / AdvanceWatermark / AdvanceProcessingTime). The log is the
// authoritative input history: replaying it from scratch on an identically
// configured Pipeline reproduces the marking bit-for-bit, because the
// compile step is deterministic from the builder state. Snapshots
// (L2.2) shorten replay; conformance checks (L2.3) verify replay correctness.
package dataflow

import (
	"errors"
	"fmt"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
)

// PipelineOp names the kind of command recorded.
type PipelineOp string

const (
	OpSend        PipelineOp = "send"
	OpAdvanceWM   PipelineOp = "advance_wm"
	OpAdvanceProc PipelineOp = "advance_proc"
)

// PipelineEvent is a single command in the pipeline's input history. Seq
// is a monotonic per-pipeline sequence used as the canonical ordering when
// timestamps tie. Key is set only for OpSend.
type PipelineEvent struct {
	Seq int        `json:"seq"`
	Op  PipelineOp `json:"op"`
	Key string     `json:"key,omitempty"`
	TS  int        `json:"ts"`
}

// Events returns a defensive copy of the pipeline's input history. Safe to
// serialize and feed into Replay on a fresh pipeline.
func (p *Pipeline) Events() []PipelineEvent {
	out := make([]PipelineEvent, len(p.events))
	copy(out, p.events)
	return out
}

// Replay drives a fresh pipeline through a previously captured log.
// Returns an error if any step rejects (typically: declared keys or window
// strategy don't match the recording). Calling Replay after the pipeline
// has already processed events is rejected to keep semantics simple.
func (p *Pipeline) Replay(events []PipelineEvent) error {
	if len(p.events) != 0 {
		return errors.New("dataflow: Replay called on a pipeline that already has history")
	}
	for _, e := range events {
		switch e.Op {
		case opRestored:
			continue // header from a snapshot restore; marking already set
		case OpSend:
			if err := p.Send(e.Key, e.TS); err != nil {
				return fmt.Errorf("replay seq=%d send(%q,%d): %w", e.Seq, e.Key, e.TS, err)
			}
		case OpAdvanceWM:
			if err := p.AdvanceWatermark(e.TS); err != nil {
				return fmt.Errorf("replay seq=%d advance_wm(%d): %w", e.Seq, e.TS, err)
			}
		case OpAdvanceProc:
			if err := p.AdvanceProcessingTime(e.TS); err != nil {
				return fmt.Errorf("replay seq=%d advance_proc(%d): %w", e.Seq, e.TS, err)
			}
		default:
			return fmt.Errorf("replay seq=%d: unknown op %q", e.Seq, e.Op)
		}
	}
	return nil
}

// ToEventLog converts the pipeline's input history into the generic
// process-mining EventLog format so mining/conformance tools can consume
// it. Each pipeline event becomes one Event whose CaseID is the pipeline
// name (one trace per pipeline) and whose Activity is the op name. The
// pipeline event's int TS becomes a Unix-epoch second on the Event so the
// Trace's monotonic ordering matches Seq.
//
// Every event carries an "op" attribute (matching e.Op) so consumers can
// filter to the data plane without parsing Activity strings. The "key"
// attribute is set only on data-send events; control events (watermark
// advances, processing-time advances, restored snapshots) omit it
// entirely. mining.DiscoverPipeline relies on this to skip control
// events instead of mistaking them for phantom keys.
func (p *Pipeline) ToEventLog() *eventlog.EventLog {
	log := eventlog.NewEventLog()
	caseID := p.name
	if caseID == "" {
		caseID = "pipeline"
	}
	for _, e := range p.events {
		attrs := map[string]any{
			"op": string(e.Op),
			"ts": e.TS,
		}
		if e.Op == OpSend {
			attrs["key"] = e.Key
		}
		log.AddEvent(eventlog.Event{
			CaseID:     caseID,
			Activity:   string(e.Op),
			Timestamp:  time.Unix(int64(e.Seq), 0),
			Attributes: attrs,
		})
	}
	return log
}

// Snapshot is a serializable checkpoint of pipeline runtime state — marking
// of every place in the flattened model, the explicit watermark, and the
// number of events that produced this state. Restore replays no events;
// it directly seeds the new pipeline's marking. To resume from a snapshot
// and continue, the caller appends post-cursor events and replays them.
//
// Snapshot does NOT capture the builder config (keys, windows, triggers).
// The recovering process is expected to reconstruct an identically
// configured Pipeline and call RestoreSnapshot before resuming.
type Snapshot struct {
	Marking      map[string]int `json:"marking"`
	ExplicitWM   int            `json:"explicit_wm"`
	EventCursor  int            `json:"event_cursor"`
	PipelineName string         `json:"pipeline_name"`
}

// SnapshotState captures the pipeline's runtime state. Returns an empty
// snapshot if the pipeline has not yet been compiled — the caller can
// distinguish via EventCursor == 0 && len(Marking) == 0.
func (p *Pipeline) SnapshotState() *Snapshot {
	s := &Snapshot{
		Marking:      map[string]int{},
		ExplicitWM:   p.explicitWM,
		EventCursor:  len(p.events),
		PipelineName: p.name,
	}
	if p.built && p.state != nil {
		for place, n := range p.state.Marking {
			if n != 0 {
				s.Marking[place] = n
			}
		}
	}
	return s
}

// RestoreSnapshot seeds a freshly built pipeline with the marking and
// watermark from a snapshot. Errors if the pipeline already has any
// recorded events — restore is only safe on a virgin builder.
func (p *Pipeline) RestoreSnapshot(s *Snapshot) error {
	if s == nil {
		return errors.New("dataflow: nil snapshot")
	}
	if len(p.events) != 0 {
		return errors.New("dataflow: RestoreSnapshot on a pipeline with existing history")
	}
	if err := p.ensureBuilt(); err != nil {
		return err
	}
	for place := range p.state.Marking {
		p.state.SetTokens(place, 0)
	}
	for place, n := range s.Marking {
		p.state.SetTokens(place, n)
	}
	p.explicitWM = s.ExplicitWM
	// Seed the event log with a placeholder header so EventCursor stays
	// monotonic and downstream consumers can tell "I picked up at N".
	// We synthesize cursor entries as no-op markers; replay rejects them.
	if s.EventCursor > 0 {
		p.events = make([]PipelineEvent, s.EventCursor)
		for i := range p.events {
			p.events[i] = PipelineEvent{Seq: i, Op: opRestored}
		}
	}
	return nil
}

// opRestored marks an event slot consumed by a restored snapshot. Replay
// skips these because the marking they would have produced is already set.
const opRestored PipelineOp = "restored"

func (p *Pipeline) recordSend(key string, ts int) {
	p.events = append(p.events, PipelineEvent{
		Seq: len(p.events), Op: OpSend, Key: key, TS: ts,
	})
}

func (p *Pipeline) recordAdvanceWM(to int) {
	p.events = append(p.events, PipelineEvent{
		Seq: len(p.events), Op: OpAdvanceWM, TS: to,
	})
}

func (p *Pipeline) recordAdvanceProc(to int) {
	p.events = append(p.events, PipelineEvent{
		Seq: len(p.events), Op: OpAdvanceProc, TS: to,
	})
}
