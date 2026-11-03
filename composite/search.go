package composite

// One bounded search of a flat net, on go-pflow's reachability engine (the
// model is converted by metamodel/metapetri, which carries capacities, read
// arcs and inhibitors across losslessly).

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

type tri int

const (
	triTrue tri = iota
	triFalse
	triUnknown
)

// search is what one bounded search of a flat net found.
type search struct {
	states       int
	complete     bool
	bounded      tri
	deadlockFree reachability.Verdict
	quasiLive    reachability.Verdict
	// a shortest trace to a dead marking, and the marking
	dead       bool
	deadTrace  []string
	deadMark   reachability.Marking
	deadCount  int
	fired      map[string]bool
	neverFires []string
	maxTokens  map[string]int
	// every explored marking (for declared-law checks); small by the state cap
	markings []reachability.Marking
}

// Opts bound a proof's searches.
type Opts struct {
	MaxStates int
	MaxTokens int
	Ablate    bool
}

const (
	defaultStates = 2000
	maxStatesCap  = 20000
	defaultTokens = 1000
	maxAblated    = 60 // links judged one by one (each is a reachability run)
	maxLaws       = 24 // laws per part kept in the record
)

func (o Opts) view() map[string]any {
	return map[string]any{"max_states": o.MaxStates, "max_tokens": o.MaxTokens, "ablate": o.Ablate}
}

// DefaultOpts are the options bundle_prove uses when none are given.
func DefaultOpts() Opts {
	return Opts{MaxStates: defaultStates, MaxTokens: defaultTokens, Ablate: true}
}

// convert makes the analysable net of a model. It refuses what the analysis
// cannot read faithfully (a guard, a data place): an answer about another net
// would be worse than none.
func convert(m *metamodel.Model) (*metapetri.Result, error) {
	if why := Unrepresentable(m); len(why) > 0 {
		return nil, fmt.Errorf("the net has what the analyses cannot read (%s)", why[0])
	}
	return metapetri.Convert(m, metapetri.Options{})
}

func runSearch(m *metamodel.Model, o Opts) (*search, error) {
	res, err := convert(m)
	if err != nil {
		return nil, err
	}
	r := reachability.NewAnalyzer(res.Net).
		WithInitialMarking(res.Marking).
		WithMaxStates(o.MaxStates).
		WithMaxTokens(o.MaxTokens).
		Analyze()
	lv := r.Liveness
	s := &search{
		states:       r.StateCount,
		complete:     r.IsComplete,
		deadlockFree: lv.DeadlockFree,
		quasiLive:    lv.QuasiLive,
		deadCount:    len(r.DeadMarkings),
		fired:        map[string]bool{},
		neverFires:   append([]string{}, r.DeadTrans...),
		maxTokens:    r.MaxTokens,
	}
	sort.Strings(s.neverFires)
	switch {
	case !r.IsComplete:
		s.bounded = triUnknown
	case r.Bounded:
		s.bounded = triTrue
	default:
		s.bounded = triFalse
	}
	for _, st := range r.Graph.StatesList() {
		s.markings = append(s.markings, st.Marking)
	}
	if lv.Witness != nil && lv.Witness.Kind == reachability.WitnessDeadlock {
		s.dead, s.deadTrace, s.deadMark = true, lv.Witness.Trace, lv.Witness.Marking
	}
	for _, t := range r.FiredTransitions {
		s.fired[t] = true
	}
	return s, nil
}

func markingView(mk reachability.Marking) map[string]any {
	out := map[string]any{}
	for k, v := range mk {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

// traceTo is a shortest trace from the initial marking to target, or nil.
func traceTo(m *metamodel.Model, o Opts, target reachability.Marking) any {
	res, err := convert(m)
	if err != nil {
		return nil
	}
	if res.Marking.Equals(target) {
		return []any{}
	}
	a := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).WithMaxStates(o.MaxStates).WithMaxTokens(o.MaxTokens)
	t := a.PathTo(target)
	if t == nil {
		return nil
	}
	return strs(t)
}

// ---- documents

// toDoc is v as the plain JSON value other tools read, after fn.
func toDoc(v any, fn func(map[string]any)) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if fn != nil {
		fn(out)
	}
	return out, nil
}
