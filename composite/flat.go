package composite

// The flat net of a bundle, and the arithmetic done on it: the incidence
// matrix, the firing rule used to replay a trace, and the size of an answer.

import (
	"fmt"
	"math"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Flat is a flattened bundle: the net and how it was made.
type Flat struct {
	Model *metamodel.Model
	Map   *metamodel.FlattenMap
}

// Flatten is Bundle.FlattenWithMap.
func Flatten(b *metamodel.Bundle) (*Flat, error) {
	m, fm, err := b.FlattenWithMap()
	if err != nil {
		return nil, err
	}
	return &Flat{m, fm}, nil
}

func member(m map[string]map[string]string, name string) (string, bool) {
	s, local, ok := strings.Cut(name, "/")
	if !ok {
		return "", false
	}
	flat, ok := m[s][local]
	return flat, ok
}

// invert turns a subnet -> local -> flat map into flat -> sorted "subnet/local".
func invert(m map[string]map[string]string) map[string][]string {
	out := map[string][]string{}
	for s, locals := range m {
		for local, flat := range locals {
			out[flat] = append(out[flat], s+"/"+local)
		}
	}
	for _, v := range out {
		sortStrings(v)
	}
	return out
}

// weightless counts the arcs between a token place and a transition with no
// weight (unless movingOnly, the read and inhibitor arcs among them): every
// firing rule reads that as 1, a sum over the arcs as 0.
func weightless(m *metamodel.Model, movingOnly bool) int {
	token, transition := map[string]bool{}, map[string]bool{}
	for i := range m.Places {
		if m.Places[i].IsToken() {
			token[m.Places[i].ID] = true
		}
	}
	for i := range m.Transitions {
		transition[m.Transitions[i].ID] = true
	}
	n := 0
	for _, a := range m.Arcs {
		touches := (token[a.From] && transition[a.To]) || (transition[a.From] && token[a.To])
		if a.Weight == 0 && touches && !(movingOnly && a.IsReadOnly()) {
			n++
		}
	}
	return n
}

// incidence is C = post - pre of a model (rows: places, columns:
// transitions, in model order). Read and inhibitor arcs move nothing. An arc
// with no weight counts as 0, as the hosted pflow_bundle_incidence does.
func incidence(m *metamodel.Model) [][]int {
	c := make([][]int, len(m.Places))
	for i := range c {
		c[i] = make([]int, len(m.Transitions))
	}
	pi := func(id string) int {
		for i := range m.Places {
			if m.Places[i].ID == id {
				return i
			}
		}
		return -1
	}
	ti := func(id string) int {
		for i := range m.Transitions {
			if m.Transitions[i].ID == id {
				return i
			}
		}
		return -1
	}
	for _, a := range m.Arcs {
		if a.IsReadOnly() {
			continue
		}
		if p, t := pi(a.From), ti(a.To); p >= 0 && t >= 0 {
			c[p][t] -= a.Weight
		} else if t, p := ti(a.From), pi(a.To); t >= 0 && p >= 0 {
			c[p][t] += a.Weight
		}
	}
	return c
}

// answerBytes estimates the size of the flat net's answer before it is built.
func (f *Flat) answerBytes() int {
	m := f.Model
	n := 0
	for _, p := range m.Places {
		n += 160 + len(p.ID) + len(p.Description)
	}
	for _, t := range m.Transitions {
		n += 320 + 3*len(t.ID) + len(t.Description)
	}
	for _, a := range m.Arcs {
		n += 64 + len(a.From) + len(a.To)
	}
	for _, c := range m.Constraints {
		n += 32 + len(c.ID) + len(c.Expr)
	}
	for _, groups := range []map[string][]string{f.Map.Wires, f.Map.FusedGroups} {
		for name, members := range groups {
			n += 16 + len(name)
			for _, x := range members {
				n += 6 + len(x)
			}
		}
	}
	for _, mp := range []map[string]map[string]string{f.Map.Place, f.Map.Transition} {
		for subnet, locals := range mp {
			for local, flat := range locals {
				n += 2*(len(subnet)+len(local)) + len(flat) + 24
			}
		}
	}
	for _, w := range f.Map.Warnings {
		n += 64 + len(w.Code) + len(w.Element) + len(w.Message)
	}
	return n
}

func (f *Flat) checkAnswer(tool string) error {
	if n := f.answerBytes(); n > MaxAnswerBytes {
		return fmt.Errorf("%s: the flat net's answer would be about %d bytes (at most %d): long fused transition names are repeated in every list and map that mentions them; use shorter ids or fewer event links", tool, n, MaxAnswerBytes)
	}
	return nil
}

// ---- the firing rule, as pflow-rs hardens it (a negative arc weight never
// enables; counts saturate)

func satAdd(a, b int) int {
	if b > 0 && a > math.MaxInt-b {
		return math.MaxInt
	}
	if b < 0 && a < math.MinInt-b {
		return math.MinInt
	}
	return a + b
}

func satSub(a, b int) int {
	if b < 0 {
		if a > math.MaxInt+b {
			return math.MaxInt
		}
	} else if a < math.MinInt+b {
		return math.MinInt
	}
	return a - b
}

// enabledWhyNot is nil when transition can fire at mk, else the reason.
func enabledWhyNot(m *metamodel.Model, transition string, mk metamodel.Marking) error {
	if m.TransitionByID(transition) == nil {
		return fmt.Errorf("no transition %q in model %q", transition, m.Name)
	}
	negative := func(place string, weight int) error {
		return fmt.Errorf("%s: arc to/from %s has negative weight %d", transition, place, weight)
	}
	inputs := m.Inputs(transition)
	for _, in := range inputs {
		if in.Weight < 0 {
			return negative(in.Place, in.Weight)
		}
		if have := mk[in.Place]; have < in.Weight {
			return fmt.Errorf("%s needs %d token(s) on %s, have %d", transition, in.Weight, in.Place, have)
		}
	}
	outputs := m.Outputs(transition)
	for _, out := range outputs {
		if out.Weight < 0 {
			return negative(out.Place, out.Weight)
		}
	}
	for _, t := range m.Tests(transition) {
		if t.Weight < 0 {
			return negative(t.Place, t.Weight)
		}
		have := mk[t.Place]
		if t.Type == metamodel.InhibitorArc {
			if have >= t.Weight {
				return fmt.Errorf("%s is inhibited: %s holds %d (>= %d)", transition, t.Place, have, t.Weight)
			}
		} else if have < t.Weight {
			return fmt.Errorf("%s reads %d token(s) from %s, have %d", transition, t.Weight, t.Place, have)
		}
	}
	net := map[string]int{}
	for _, in := range inputs {
		net[in.Place] = satSub(net[in.Place], in.Weight)
	}
	for _, out := range outputs {
		net[out.Place] = satAdd(net[out.Place], out.Weight)
	}
	for i := range m.Places {
		p := &m.Places[i]
		if p.Capacity <= 0 || !p.IsToken() {
			continue
		}
		delta, ok := net[p.ID]
		if !ok || delta <= 0 {
			continue
		}
		if after := satAdd(mk[p.ID], delta); after > p.Capacity {
			return fmt.Errorf("%s would leave %d token(s) on %s, over its capacity of %d", transition, after, p.ID, p.Capacity)
		}
	}
	return nil
}

func fire(m *metamodel.Model, transition string, mk metamodel.Marking) metamodel.Marking {
	next := mk.Clone()
	for _, in := range m.Inputs(transition) {
		next[in.Place] = satSub(next[in.Place], in.Weight)
	}
	for _, out := range m.Outputs(transition) {
		next[out.Place] = satAdd(next[out.Place], out.Weight)
	}
	return next
}
