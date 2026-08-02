package metamodel

import (
	"fmt"
	"sort"
	"strings"
)

// EventLink transition fusion.
//
// "When one fires, the other fires too" (ch04). The transitions in an EventLink
// class become one transition whose pre/postset is the union of theirs and whose
// guard is their conjunction — a rendezvous, so it is enabled only when every
// participant is.
//
// Fusion is computed over *equivalence classes*, not pairwise. That is what
// makes it associative: linking a→b and b→c yields one three-member class no
// matter which order the links appear in, so (A;B);C and A;(B;C) agree. It also
// makes an EventLink cycle a non-event — a↔b is simply a two-member class,
// rather than something to detect and reject.

// fusionMember is one transition inside an EventLink equivalence class, kept
// with its owning subnet so guards and bindings can be rewritten in context.
type fusionMember struct {
	subnet *Subnet
	trans  *Transition
}

// fuseTransitions builds one Transition per equivalence class.
func (b *Bundle) fuseTransitions(
	subnets []*Subnet,
	groups map[string][]string,
	transFlat, placeFlat map[string]string,
	fm *FlattenMap,
) ([]Transition, error) {
	// Index members for lookup, and keep emission order stable: subnet order,
	// then declaration order within a subnet.
	index := map[string]fusionMember{}
	var order []string
	emitted := map[string]bool{}

	for _, s := range subnets {
		for i := range s.Model.Transitions {
			t := &s.Model.Transitions[i]
			key := s.ID + "/" + t.ID
			index[key] = fusionMember{subnet: s, trans: t}

			flat := transFlat[key]
			if !emitted[flat] {
				emitted[flat] = true
				order = append(order, flat)
			}
		}
	}

	// Which class each flat name belongs to.
	classOf := map[string][]string{}
	for _, members := range groups {
		flat := transFlat[members[0]]
		classOf[flat] = members
	}

	out := make([]Transition, 0, len(order))
	for _, flat := range order {
		members := classOf[flat]
		if len(members) == 1 {
			m := index[members[0]]
			t := m.trans.Clone()
			t.ID = flat
			t.Guard = RewritePlaceRefs(t.Guard, subnetPlaceAlias(m.subnet, placeFlat), b.prefix(m.subnet.ID))
			out = append(out, *t)
			continue
		}

		fused, err := b.fuseClass(flat, members, index, placeFlat, fm)
		if err != nil {
			return nil, err
		}
		out = append(out, *fused)
	}
	return out, nil
}

// fuseClass merges an equivalence class of transitions into one.
func (b *Bundle) fuseClass(
	flat string,
	members []string,
	index map[string]fusionMember,
	placeFlat map[string]string,
	fm *FlattenMap,
) (*Transition, error) {
	sorted := append([]string(nil), members...)
	sort.Strings(sorted) // canonical order makes every merge below associative

	fused := &Transition{ID: flat}

	var (
		guards       []string
		descriptions []string
		emits        []string
		bindings     []Binding
		bindingIndex = map[string]int{}
		fields       []TransitionField
		fieldIndex   = map[string]int{}
		rate         float64
		haveRate     bool
	)

	initiator := b.initiatorOf(sorted)

	for _, key := range sorted {
		m, ok := index[key]
		if !ok {
			return nil, fmt.Errorf("internal: fused member %q not found", key)
		}
		t := m.trans
		alias := subnetPlaceAlias(m.subnet, placeFlat)
		rename := b.renamesFor(key)

		if g := RewritePlaceRefs(t.Guard, alias, b.prefix(m.subnet.ID)); g != "" {
			guards = append(guards, g)
		}
		if t.Description != "" {
			descriptions = append(descriptions, t.Description)
		}

		// Each component keeps emitting its own event. The fused transition is
		// one firing, but a component whose event stopped being written would no
		// longer be replayable on its own, and assume-guarantee would be a lie at
		// the persistence layer.
		if evt := componentEvent(t); evt != "" {
			emits = append(emits, evt)
		}

		if err := mergeBindings(&bindings, bindingIndex, t.Bindings, rename, alias, key); err != nil {
			return nil, err
		}
		if err := mergeTransitionFields(&fields, fieldIndex, t.Fields, key); err != nil {
			return nil, err
		}

		// The initiator owns the HTTP route: "when one fires, the other fires
		// too" is directional. Non-initiator routes disappear, which is real lost
		// API surface, so it is recorded as a warning rather than done silently.
		if key == initiator {
			fused.HTTPMethod = t.HTTPMethod
			fused.HTTPPath = t.HTTPPath
			fused.Event = t.Event
		} else if t.HTTPPath != "" {
			fm.Warnings = append(fm.Warnings, ValidationError{
				Code: WarnRouteDropped,
				Message: fmt.Sprintf("%s %s from %s is dropped: it fused into %s, whose route comes from the initiator %s",
					t.HTTPMethod, t.HTTPPath, key, flat, initiator),
				Element: flat,
			})
		}

		if t.Rate != 0 {
			// A rendezvous fires no faster than its slowest participant.
			if !haveRate || t.Rate < rate {
				rate, haveRate = t.Rate, true
			}
		}
		fused.ClearsHistory = fused.ClearsHistory || t.ClearsHistory
	}

	dur, err := mergeDurations(sorted, index, flat)
	if err != nil {
		return nil, err
	}
	fused.Duration, fused.MinDuration, fused.MaxDuration = dur.duration, dur.min, dur.max

	fused.Guard = andGuards(guards...)
	fused.Bindings = bindings
	fused.Fields = fields
	fused.Emits = emits
	fused.Rate = rate
	fused.Description = describeFusion(strings.Join(descriptions, " ⊗ "), "fires with", sorted)

	if len(emits) > 0 {
		fm.MemberEvents[flat] = emits
	}
	return fused, nil
}

// componentEvent returns the event ID a transition emits, falling back to the
// deprecated EventType and finally to the transition ID.
func componentEvent(t *Transition) string {
	if t.Event != "" {
		return t.Event
	}
	if t.EventType != "" {
		return t.EventType
	}
	return t.ID
}

// initiatorOf picks the class member that owns the fused HTTP route: the unique
// member with no incoming EventLink. On a cycle, or with several sources, the
// canonically smallest member wins so the result stays deterministic.
func (b *Bundle) initiatorOf(members []string) string {
	inClass := map[string]bool{}
	for _, m := range members {
		inClass[m] = true
	}

	hasIncoming := map[string]bool{}
	for i := range b.Links {
		l := &b.Links[i]
		if l.Kind != EventLink {
			continue
		}
		to, err := b.resolve(l.To)
		if err != nil {
			continue
		}
		key := to.subnet.ID + "/" + to.transition
		if inClass[key] {
			hasIncoming[key] = true
		}
	}

	for _, m := range members {
		if !hasIncoming[m] {
			return m
		}
	}
	return members[0]
}

// renamesFor collects the binding renames that apply to one class member, from
// every EventLink whose From side is that member.
func (b *Bundle) renamesFor(memberKey string) map[string]string {
	var out map[string]string
	for i := range b.Links {
		l := &b.Links[i]
		if l.Kind != EventLink || len(l.Rename) == 0 {
			continue
		}
		from, err := b.resolve(l.From)
		if err != nil {
			continue
		}
		if from.subnet.ID+"/"+from.transition != memberKey {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		for k, v := range l.Rename {
			out[k] = v
		}
	}
	return out
}

// mergeBindings unifies bindings by name.
//
// This is the mechanism that makes orders.confirm ⊗ inventory.reserve share
// order_id: same name means same variable. It is CPN variable unification,
// restricted to the flat binding model this schema actually has — so two
// bindings that share a name must agree on Type, on the Value flag and on Keys.
// Place may differ only when both places fused into the same flat place.
func mergeBindings(out *[]Binding, index map[string]int, in []Binding, rename, alias map[string]string, member string) error {
	for _, bnd := range in {
		b := bnd
		if renamed, ok := rename[b.Name]; ok {
			b.Name = renamed
		}
		if b.Place != "" {
			if flat, ok := alias[b.Place]; ok {
				b.Place = flat
			}
		}

		pos, seen := index[b.Name]
		if !seen {
			index[b.Name] = len(*out)
			*out = append(*out, b)
			continue
		}

		prev := (*out)[pos]
		if prev.Type != b.Type {
			return fmt.Errorf("[%s] binding %q is %q in one fused transition and %q in %s",
				ErrBindingConflict, b.Name, prev.Type, b.Type, member)
		}
		if prev.Value != b.Value {
			return fmt.Errorf("[%s] binding %q is the transfer value in one fused transition but not in %s",
				ErrBindingConflict, b.Name, member)
		}
		if strings.Join(prev.Keys, ",") != strings.Join(b.Keys, ",") {
			return fmt.Errorf("[%s] binding %q has keys [%s] in one fused transition and [%s] in %s",
				ErrBindingConflict, b.Name, strings.Join(prev.Keys, ","), strings.Join(b.Keys, ","), member)
		}
		if prev.Place != b.Place {
			return fmt.Errorf("[%s] binding %q reads place %q in one fused transition and %q in %s; "+
				"fuse those places with a data link, or rename one binding on the event link",
				ErrBindingConflict, b.Name, prev.Place, b.Place, member)
		}
	}
	return nil
}

// mergeTransitionFields unions input form fields by name, so the fused action
// presents one form.
func mergeTransitionFields(out *[]TransitionField, index map[string]int, in []TransitionField, member string) error {
	for _, f := range in {
		pos, seen := index[f.Name]
		if !seen {
			index[f.Name] = len(*out)
			*out = append(*out, f)
			continue
		}
		prev := (*out)[pos]
		if prev.Type != f.Type || prev.Required != f.Required {
			return fmt.Errorf("[%s] input field %q is declared differently in %s (type %q/%q, required %v/%v)",
				ErrBindingConflict, f.Name, member, prev.Type, f.Type, prev.Required, f.Required)
		}
	}
	return nil
}

type durations struct{ duration, min, max string }

// mergeDurations combines SLA timings across a fused class: the slowest expected
// duration, the tightest minimum and the tightest maximum.
//
// Durations are compared as strings only when they are equal; otherwise the
// first non-empty value wins per field and a genuine conflict between a min and
// a max is reported. Parsing is deliberately avoided here — Duration is a free
// string in this schema and go-pflow does not own its grammar.
func mergeDurations(members []string, index map[string]fusionMember, flat string) (durations, error) {
	var d durations
	var minSource, maxSource string

	for _, key := range members {
		t := index[key].trans
		if d.duration == "" {
			d.duration = t.Duration
		}
		if t.MinDuration != "" {
			if d.min == "" {
				d.min, minSource = t.MinDuration, key
			} else if d.min != t.MinDuration {
				return d, fmt.Errorf("[%s] fused transition %s has incompatible minimum durations: %q from %s and %q from %s",
					ErrDurationConflict, flat, d.min, minSource, t.MinDuration, key)
			}
		}
		if t.MaxDuration != "" {
			if d.max == "" {
				d.max, maxSource = t.MaxDuration, key
			} else if d.max != t.MaxDuration {
				return d, fmt.Errorf("[%s] fused transition %s has incompatible maximum durations: %q from %s and %q from %s",
					ErrDurationConflict, flat, d.max, maxSource, t.MaxDuration, key)
			}
		}
	}
	return d, nil
}
