// Compile lowers a Pipeline to a subnet.Bundle without flattening or
// firing. Self-contained lowering of builder state into the substrate IR.
package dataflow

import (
	"errors"
	"fmt"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// Compile lowers the pipeline to a subnet.Bundle without flattening or
// firing. Exported for inspection / serialization.
func (p *Pipeline) Compile() (*subnet.Bundle, error) {
	if p.window == nil {
		return nil, errors.New("dataflow: no window strategy")
	}
	switch p.window.kind() {
	case "fixed", "sliding", "sessions":
	default:
		return nil, fmt.Errorf("dataflow: window kind %q: %w", p.window.kind(), ErrNotImplemented)
	}
	if p.stage != stageCountPerKey {
		return nil, fmt.Errorf("dataflow: stage not set: %w", ErrNotImplemented)
	}
	if len(p.keys) == 0 {
		return nil, errors.New("dataflow: no keys declared")
	}
	if p.horizon <= 0 {
		return nil, errors.New("dataflow: horizon must be positive")
	}

	windows := p.window.Materialize(p.horizon)
	perKey, perKeyOK := p.window.(PerKeyWindowFn)
	windowsForKey := func(key string) []Window {
		if perKeyOK {
			return perKey.WindowsForKey(key)
		}
		return windows
	}

	trigger := p.trigger
	if trigger == nil {
		trigger = AfterWatermark{}
	}
	needProc := trigger.needsProcessingClock()

	b := subnet.NewBundle(p.name)

	// Watermark subnet: a single `wm` place, an `advance` transition that
	// produces a token into `wm`. `wm` is exposed as an out-port so every
	// downstream (key,window) subnet aliases its `wm` in-port to this slot.
	wmModel := tmpetri.NewModel("watermark")
	wmModel.AddPlace(tmpetri.Place{ID: "wm"})
	wmModel.AddTransition(tmpetri.Transition{ID: "advance"})
	wmModel.AddArc(tmpetri.Arc{Source: "advance", Target: "wm"})
	b.AddSubnet(subnet.Subnet{
		ID:    "watermark",
		Model: wmModel,
		Ports: []subnet.Port{{ID: "out", Kind: subnet.PortOut, Place: "wm", Schema: "watermark"}},
	})

	// Processing-time clock subnet: parallel to the watermark. Only emitted
	// if the trigger needs it (AfterProcessingTime or any composite of one).
	if needProc {
		procModel := tmpetri.NewModel("proc-clock")
		procModel.AddPlace(tmpetri.Place{ID: "proc"})
		procModel.AddTransition(tmpetri.Transition{ID: "tick"})
		procModel.AddArc(tmpetri.Arc{Source: "tick", Target: "proc"})
		b.AddSubnet(subnet.Subnet{
			ID:    "proc-clock",
			Model: procModel,
			Ports: []subnet.Port{{ID: "out", Kind: subnet.PortOut, Place: "proc", Schema: "proc-time"}},
		})
	}

	// Per-key source subnet: one place `arrivals` (initial 0). For each
	// window, an inputless `assign:[s,e)` transition with guard on
	// event_time produces a token into per-window out-port `to:[s,e)`.
	// Sources don't actually need an internal place: the assign transitions
	// fire directly into the out-port places, which are aliased away on
	// link. Each source subnet thus declares one out-port per window.
	for _, k := range p.keys {
		// Filter: if a keep-set is declared and this key isn't in it, the
		// per-key source subnet is omitted entirely. Sending an element for
		// this key still errors at Send (unknown-or-dropped key, same path).
		if p.keepKeys != nil && !p.keepKeys[k] {
			continue
		}
		kw := windowsForKey(k)
		if len(kw) == 0 {
			// Per-key window strategy with no sessions for this key: emit
			// only an empty source subnet (no assigns, no ports). Send
			// will reject this key at Compile-time naturally because no
			// assign exists.
			continue
		}
		srcModel := tmpetri.NewModel("source:" + k)
		ports := make([]subnet.Port, 0, len(kw))
		for _, w := range kw {
			outID := "to:" + w.String()
			assignID := "assign:" + w.String()
			srcModel.AddPlace(tmpetri.Place{ID: outID})
			srcModel.AddTransition(tmpetri.Transition{
				ID:    assignID,
				Guard: fmt.Sprintf("event_time >= %d && event_time < %d", w.Start, w.End),
			})
			srcModel.AddArc(tmpetri.Arc{Source: assignID, Target: outID})
			ports = append(ports, subnet.Port{
				ID:     "w:" + w.String(),
				Kind:   subnet.PortOut,
				Place:  outID,
				Schema: "element:" + k,
			})
		}
		b.AddSubnet(subnet.Subnet{
			ID:    "src:" + k,
			Model: srcModel,
			Ports: ports,
		})
	}

	// Per-(key,window) window subnet: `in` (arrivals from source), `wm`
	// (watermark feed), accumulator `acc`, `out` (drained emission). One
	// `emit` transition consumes one `acc` token per fire, gated by
	// tokens("wm") >= w.End; output goes to `out`. Discarding semantics:
	// emit fires until `acc` is empty, and the residual count lands in
	// `out`. Each emit fire is a per-element emission to the collector.
	if p.windowTxn == nil {
		p.windowTxn = map[string]paneKey{}
	}
	for _, k := range p.keys {
		if p.keepKeys != nil && !p.keepKeys[k] {
			continue
		}
		for _, w := range windowsForKey(k) {
			subID := "win:" + k + ":" + w.String()
			m := tmpetri.NewModel(subID)
			pk := paneKey{Key: k, Window: w}
			p.windowTxn[subID+"/emit"] = pk
			p.windowTxn[subID+"/close"] = pk
			m.AddPlace(tmpetri.Place{ID: "in"})
			m.AddPlace(tmpetri.Place{ID: "wm"})
			m.AddPlace(tmpetri.Place{ID: "acc"})
			m.AddPlace(tmpetri.Place{ID: "out"})
			if needProc {
				m.AddPlace(tmpetri.Place{ID: "proc"})
			}

			// receive: in -> acc. Always enabled when `in` has a token.
			m.AddTransition(tmpetri.Transition{ID: "receive"})
			m.AddArc(tmpetri.Arc{Source: "in", Target: "receive"})
			m.AddArc(tmpetri.Arc{Source: "receive", Target: "acc"})

			// emit: acc -> out, gated by the trigger's compiled guard.
			// Local port names (wm, proc) and internal names (acc) are all
			// rewritten through the alias map at flatten time.
			m.AddTransition(tmpetri.Transition{ID: "emit", Guard: trigger.emitGuard(w.End)})
			m.AddArc(tmpetri.Arc{Source: "acc", Target: "emit"})
			m.AddArc(tmpetri.Arc{Source: "emit", Target: "out"})

			// close: GC the window once the watermark has advanced past
			// w.End + allowedLateness. Drains any leftover acc into out
			// (force-emit on close), guaranteeing acc is structurally empty
			// after close. Combined with the lateness gate at assign-time,
			// the window subnet's per-place marking is bounded once closed.
			closeGuard := fmt.Sprintf(`tokens("wm") > %d`, w.End+p.allowedLateness)
			m.AddTransition(tmpetri.Transition{ID: "close", Guard: closeGuard})
			m.AddArc(tmpetri.Arc{Source: "acc", Target: "close"})
			m.AddArc(tmpetri.Arc{Source: "close", Target: "out"})

			ports := []subnet.Port{
				{ID: "feed", Kind: subnet.PortIn, Place: "in", Schema: "element:" + k},
				{ID: "wm", Kind: subnet.PortIn, Place: "wm", Schema: "watermark"},
				{ID: "result", Kind: subnet.PortOut, Place: "out", Schema: "count:" + k},
			}
			if needProc {
				ports = append(ports, subnet.Port{ID: "proc", Kind: subnet.PortIn, Place: "proc", Schema: "proc-time"})
			}

			b.AddSubnet(subnet.Subnet{ID: subID, Model: m, Ports: ports})

			// Links: source.w:[..] -> window.feed; watermark.out -> window.wm;
			// proc-clock.out -> window.proc if the trigger reads proc time.
			b.AddLink(subnet.Link{
				FromSubnet: "src:" + k, FromPort: "w:" + w.String(),
				ToSubnet: subID, ToPort: "feed",
			})
			b.AddLink(subnet.Link{
				FromSubnet: "watermark", FromPort: "out",
				ToSubnet: subID, ToPort: "wm",
			})
			if needProc {
				b.AddLink(subnet.Link{
					FromSubnet: "proc-clock", FromPort: "out",
					ToSubnet: subID, ToPort: "proc",
				})
			}
		}
	}

	return b, nil
}

// ensureBuilt compiles + flattens + replays buffered inputs. Called by
