// Declarative pipeline specification. PipelineSpec is a JSON-LD-tagged
// describable form of everything you'd otherwise build via the fluent
// NewPipeline().WithKeys()...CountPerKey() chain. The two surfaces are
// equivalent: pipeline.Spec() emits a spec, spec.Build() constructs an
// identical pipeline. The spec is the wire format for storing /
// transporting pipeline definitions.
//
// Limitations:
//   - SessionWindows requires PlanSessions on a concrete element stream,
//     so a session spec round-trips the gap but the caller must replay
//     PlanSessions before Send / Run.
package dataflow

import (
	"errors"
	"fmt"
)

// JSON-LD envelope.
const (
	PipelineContext = "https://pflow.xyz/schema"
	PipelineType    = "DataflowPipeline"
)

// PipelineSpec is the declarative form of a Pipeline. It is the serialized
// equivalent of the builder calls and round-trips through encoding/json.
type PipelineSpec struct {
	Context         string       `json:"@context,omitempty"`
	Type            string       `json:"@type,omitempty"`
	Name            string       `json:"name"`
	Keys            []string     `json:"keys"`
	Filter          []string     `json:"filter,omitempty"`
	Window          WindowSpec   `json:"window"`
	Horizon         int          `json:"horizon"`
	Trigger         *TriggerSpec `json:"trigger,omitempty"`
	Stage           string       `json:"stage"`
	AllowedLateness int          `json:"allowed_lateness,omitempty"`
	// AccumulationMode is "discarding" (default) or "accumulating". Controls
	// what Pane.Count reports; does not affect Result.Counts.
	AccumulationMode string `json:"accumulation_mode,omitempty"`
}

// Accumulation mode tags for PipelineSpec.AccumulationMode.
const (
	AccumulationDiscarding  = "discarding"
	AccumulationAccumulating = "accumulating"
)

// WindowSpec describes a windowing strategy.
type WindowSpec struct {
	Kind   string `json:"kind"`             // "fixed" | "sliding" | "sessions"
	Size   int    `json:"size,omitempty"`   // fixed, sliding
	Period int    `json:"period,omitempty"` // sliding
	Gap    int    `json:"gap,omitempty"`    // sessions
}

// TriggerSpec describes an emit trigger. Composite forms ("any", "all")
// nest via Children.
type TriggerSpec struct {
	Kind     string        `json:"kind"` // "after_watermark" | "after_count" | "after_processing_time" | "any" | "all"
	N        int           `json:"n,omitempty"`
	Delay    int           `json:"delay,omitempty"`
	Children []TriggerSpec `json:"children,omitempty"`
}

const (
	StageCountPerKey = "count_per_key"
)

// Spec extracts the pipeline's declarative form. Safe to call on an
// uncompiled pipeline.
func (p *Pipeline) Spec() *PipelineSpec {
	s := &PipelineSpec{
		Context:         PipelineContext,
		Type:            PipelineType,
		Name:            p.name,
		Keys:            append([]string(nil), p.keys...),
		Horizon:         p.horizon,
		AllowedLateness: p.allowedLateness,
	}
	if p.keepKeys != nil {
		filter := make([]string, 0, len(p.keepKeys))
		for k := range p.keepKeys {
			filter = append(filter, k)
		}
		s.Filter = filter
	}
	switch p.stage {
	case stageCountPerKey:
		s.Stage = StageCountPerKey
	}
	s.Window = encodeWindow(p.window)
	if p.trigger != nil {
		t := encodeTrigger(p.trigger)
		s.Trigger = &t
	}
	if p.accMode == Accumulating {
		s.AccumulationMode = AccumulationAccumulating
	}
	return s
}

// Build constructs a Pipeline from a spec. Errors on unknown stages /
// windows / triggers so silent misconfiguration is impossible. The
// returned pipeline is not yet compiled — call Send / Run as usual.
func (s *PipelineSpec) Build() (*Pipeline, error) {
	if s == nil {
		return nil, errors.New("dataflow: nil spec")
	}
	if s.Name == "" {
		return nil, errors.New("dataflow: spec missing name")
	}
	p := NewPipeline(s.Name).WithKeys(s.Keys...)
	if len(s.Filter) > 0 {
		p.Filter(s.Filter...)
	}
	w, err := decodeWindow(s.Window)
	if err != nil {
		return nil, err
	}
	p.WindowInto(w, s.Horizon)
	if s.Trigger != nil {
		t, err := decodeTrigger(*s.Trigger)
		if err != nil {
			return nil, err
		}
		p.Triggering(t)
	}
	if s.AllowedLateness > 0 {
		p.WithAllowedLateness(s.AllowedLateness)
	}
	switch s.AccumulationMode {
	case "", AccumulationDiscarding:
		// default
	case AccumulationAccumulating:
		p.WithAccumulationMode(Accumulating)
	default:
		return nil, fmt.Errorf("dataflow: unknown accumulation_mode %q", s.AccumulationMode)
	}
	switch s.Stage {
	case StageCountPerKey, "":
		p.CountPerKey()
	default:
		return nil, fmt.Errorf("dataflow: unknown stage %q", s.Stage)
	}
	return p, nil
}

func encodeWindow(w WindowFn) WindowSpec {
	switch v := w.(type) {
	case FixedWindows:
		return WindowSpec{Kind: "fixed", Size: v.Size}
	case SlidingWindows:
		return WindowSpec{Kind: "sliding", Size: v.Size, Period: v.Period}
	case *SessionWindows:
		return WindowSpec{Kind: "sessions", Gap: v.Gap}
	case nil:
		return WindowSpec{}
	default:
		return WindowSpec{Kind: w.kind()}
	}
}

func decodeWindow(s WindowSpec) (WindowFn, error) {
	switch s.Kind {
	case "fixed":
		if s.Size <= 0 {
			return nil, fmt.Errorf("dataflow: fixed window requires positive size")
		}
		return NewFixedWindows(s.Size), nil
	case "sliding":
		if s.Size <= 0 || s.Period <= 0 {
			return nil, fmt.Errorf("dataflow: sliding window requires positive size and period")
		}
		return NewSlidingWindows(s.Size, s.Period), nil
	case "sessions":
		if s.Gap <= 0 {
			return nil, fmt.Errorf("dataflow: sessions require positive gap")
		}
		// Caller must run PlanSessions before driving the pipeline. We
		// return the empty plan here so the spec round-trips; firing
		// without PlanSessions will surface at compile time.
		return NewSessionWindows(s.Gap), nil
	case "":
		return nil, errors.New("dataflow: spec missing window kind")
	default:
		return nil, fmt.Errorf("dataflow: unknown window kind %q", s.Kind)
	}
}

func encodeTrigger(t Trigger) TriggerSpec {
	switch v := t.(type) {
	case AfterWatermark:
		return TriggerSpec{Kind: "after_watermark"}
	case AfterCount:
		return TriggerSpec{Kind: "after_count", N: v.N}
	case AfterProcessingTime:
		return TriggerSpec{Kind: "after_processing_time", Delay: v.Delay}
	case Any:
		out := TriggerSpec{Kind: "any"}
		for _, c := range v.Triggers {
			out.Children = append(out.Children, encodeTrigger(c))
		}
		return out
	case All:
		out := TriggerSpec{Kind: "all"}
		for _, c := range v.Triggers {
			out.Children = append(out.Children, encodeTrigger(c))
		}
		return out
	default:
		return TriggerSpec{Kind: "unknown"}
	}
}

func decodeTrigger(s TriggerSpec) (Trigger, error) {
	switch s.Kind {
	case "after_watermark":
		return AfterWatermark{}, nil
	case "after_count":
		if s.N <= 0 {
			return nil, fmt.Errorf("dataflow: after_count trigger requires positive n")
		}
		return AfterCount{N: s.N}, nil
	case "after_processing_time":
		return AfterProcessingTime{Delay: s.Delay}, nil
	case "any", "all":
		children := make([]Trigger, 0, len(s.Children))
		for _, c := range s.Children {
			ch, err := decodeTrigger(c)
			if err != nil {
				return nil, err
			}
			children = append(children, ch)
		}
		if s.Kind == "any" {
			return Any{Triggers: children}, nil
		}
		return All{Triggers: children}, nil
	default:
		return nil, fmt.Errorf("dataflow: unknown trigger kind %q", s.Kind)
	}
}
