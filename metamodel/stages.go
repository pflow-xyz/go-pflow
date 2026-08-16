package metamodel

import (
	"fmt"
	"sort"
)

// StageExpansion records how ExpandStages rewrote a model, so an engine can
// run the expanded net and report in the original vocabulary.
type StageExpansion struct {
	// CarrierOf maps each stage place to the staged transition's input place
	// — the place a job "still counts as" while mid-service. Reporting folds
	// stage-place tokens back into the carrier.
	CarrierOf map[string]string
	// FinalStage maps the last stage transition of each staged transition
	// back to the original id. Its firings are the original's completions;
	// intermediate stage firings are internal and reported as nothing.
	FinalStage map[string]string
	// StageIDs lists every stage transition (all k of them) per original id,
	// in order. A rate override addressed to the original applies to each,
	// scaled by Stages.
	StageIDs map[string][]string
	// Stages is the declared k per staged transition.
	Stages map[string]int
}

// IsStagePlace reports whether id is an internal stage place.
func (e *StageExpansion) IsStagePlace(id string) bool {
	if e == nil {
		return false
	}
	_, ok := e.CarrierOf[id]
	return ok
}

// TranslateRates rewrites a rate-override map addressed to original
// transition ids into one addressed to the expanded net: an override for a
// staged transition lands on every stage, scaled by k so the mean duration
// still matches the override. Unstaged entries pass through.
func (e *StageExpansion) TranslateRates(rates map[string]float64) map[string]float64 {
	if e == nil || len(rates) == 0 {
		return rates
	}
	out := make(map[string]float64, len(rates))
	for id, v := range rates {
		if stages, ok := e.StageIDs[id]; ok {
			k := float64(e.Stages[id])
			for _, sid := range stages {
				out[sid] = v * k
			}
			continue
		}
		out[id] = v
	}
	return out
}

// ExpandStages materialises every Stages declaration as a structural Erlang
// chain: the staged transition T with input place p becomes
//
//	p -> T@1 -> T@stage1 -> T@2 -> ... -> T@stageK-1 -> T@K -> outputs
//
// with each stage transition at Stages x Rate, so the mean is unchanged and
// the variance falls by Stages. The chain is ordinary mass action — every
// engine that understands the firing rule understands the expansion.
//
// A model with no Stages declarations is returned unchanged (same pointer,
// nil expansion), so the call is free for everything that exists today.
//
// The declaration is deliberately narrow, and every violation is an error
// naming the transition rather than a quietly different semantics. Stages
// goes on the service transition draining a dedicated in-progress place —
// the brewing_X -> finish_X idiom — so it requires: exactly one consuming
// input arc, weight 1, kinetic; no guard; no read or inhibitor arcs on the
// transition; and an input place with no capacity that no other arc reads,
// tests or consumes. Each restriction exists because relaxing it changes
// when a mid-service job can be stolen, gated or counted, and those are
// modelling decisions the expansion must not make silently.
func (m *Model) ExpandStages() (*Model, *StageExpansion, error) {
	var staged []string
	for i := range m.Transitions {
		if m.Transitions[i].Stages > 1 {
			staged = append(staged, m.Transitions[i].ID)
		}
		if m.Transitions[i].Stages < 0 {
			return nil, nil, fmt.Errorf("transition %q declares %d stages; negative stages mean nothing", m.Transitions[i].ID, m.Transitions[i].Stages)
		}
	}
	if len(staged) == 0 {
		return m, nil, nil
	}
	sort.Strings(staged)

	// Consumers and referencers of every place, for the exclusivity checks.
	consumersOf := map[string][]string{}
	touchesPlace := map[string][]string{}
	placeSet := map[string]bool{}
	for _, p := range m.Places {
		placeSet[p.ID] = true
	}
	for _, a := range m.Arcs {
		if placeSet[a.From] {
			touchesPlace[a.From] = append(touchesPlace[a.From], a.To)
			if a.Type == "" {
				consumersOf[a.From] = append(consumersOf[a.From], a.To)
			}
		}
	}

	out := m.Clone()
	exp := &StageExpansion{
		CarrierOf:  map[string]string{},
		FinalStage: map[string]string{},
		StageIDs:   map[string][]string{},
		Stages:     map[string]int{},
	}

	for _, id := range staged {
		t := out.TransitionByID(id)
		k := t.Stages
		if t.Guard != "" {
			return nil, nil, fmt.Errorf("transition %q: stages cannot be declared with a guard — the expansion cannot decide which stage the guard gates", id)
		}

		// Classify the original's arcs.
		var input *Arc
		for i := range out.Arcs {
			a := &out.Arcs[i]
			switch {
			case a.To == id && a.Type != "":
				return nil, nil, fmt.Errorf("transition %q: stages cannot be declared with a %s arc — whether the gate holds for the whole service or only its start is a modelling decision the expansion must not make", id, a.Type)
			case a.To == id:
				if input != nil {
					return nil, nil, fmt.Errorf("transition %q: stages need exactly one consuming input (the in-progress place); found several", id)
				}
				input = a
			}
		}
		if input == nil {
			return nil, nil, fmt.Errorf("transition %q: stages need a consuming input place; a source transition has no duration to shape", id)
		}
		if w := input.Weight; w > 1 {
			return nil, nil, fmt.Errorf("transition %q: stages need an input weight of 1; a batch of %d entering one Erlang chain is a modelling decision the expansion must not make", id, input.Weight)
		}
		if !input.IsKinetic() {
			return nil, nil, fmt.Errorf("transition %q: stages need a kinetic input — each in-service job progresses independently, which is what kinetic means", id)
		}
		p := input.From
		if pl := out.PlaceByID(p); pl == nil || pl.Capacity > 0 {
			return nil, nil, fmt.Errorf("transition %q: input place %q has a capacity; stage places would hold tokens the bound cannot see", id, p)
		}
		if len(consumersOf[p]) != 1 {
			return nil, nil, fmt.Errorf("transition %q: input place %q feeds other transitions too; whether a mid-service job can still be taken is a modelling decision the expansion must not make", id, p)
		}
		if len(touchesPlace[p]) != len(consumersOf[p]) {
			return nil, nil, fmt.Errorf("transition %q: input place %q is read or tested by other arcs, which would not see mid-service jobs after expansion", id, p)
		}

		rate := t.Rate * float64(k)
		desc := t.Description

		// Rewrite T into T@1..T@k with stage places between. T itself becomes
		// the first stage (keeps its input arc), so external references to
		// the input place stay valid; the final stage inherits the outputs.
		firstID := id + "@1"
		t.ID = firstID
		t.Rate = rate
		t.Stages = 0
		t.Description = fmt.Sprintf("%s (stage 1 of %d)", desc, k)
		input.To = firstID

		ids := []string{firstID}
		for s := 1; s < k; s++ {
			stagePlace := fmt.Sprintf("%s@stage%d", id, s)
			nextID := fmt.Sprintf("%s@%d", id, s+1)
			out.Places = append(out.Places, Place{ID: stagePlace,
				Description: fmt.Sprintf("mid-service: %s, %d of %d stages done", id, s, k)})
			out.Transitions = append(out.Transitions, Transition{ID: nextID, Rate: rate,
				Description: fmt.Sprintf("%s (stage %d of %d)", desc, s+1, k)})
			prev := ids[len(ids)-1]
			out.Arcs = append(out.Arcs,
				Arc{From: prev, To: stagePlace},
				Arc{From: stagePlace, To: nextID},
			)
			exp.CarrierOf[stagePlace] = p
			ids = append(ids, nextID)
		}
		finalID := ids[len(ids)-1]
		// The original's outputs move to the final stage.
		for i := range out.Arcs {
			if out.Arcs[i].From == id {
				out.Arcs[i].From = finalID
			}
		}
		exp.FinalStage[finalID] = id
		exp.StageIDs[id] = ids
		exp.Stages[id] = k
	}
	return out, exp, nil
}
