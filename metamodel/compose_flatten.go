package metamodel

import (
	"fmt"
	"sort"
	"strings"
)

// Flatten lowers the bundle to a single Model.
func (b *Bundle) Flatten() (*Model, error) {
	m, _, err := b.FlattenWithMap()
	return m, err
}

// FlattenWithMap lowers the bundle and also returns the rewrite map.
//
// Pipeline:
//
//  0. identity short-circuit (one subnet, no links)
//  1. validate
//  2. place equivalence classes   (TokenLink + DataLink)
//  3. transition equivalence classes (EventLink)
//  4. emit merged places
//  5. emit fused transitions
//  6. emit retargeted, weight-merged arcs
//  7. lower guard links
//  8. rewrite guards, constraints and simulation through the alias maps
func (b *Bundle) FlattenWithMap() (*Model, *FlattenMap, error) {
	// 0. Identity. A lone subnet with no links flattens to itself, with no
	// namespacing — this is what keeps every existing single-net model, and the
	// apps generated from them, byte-for-byte unchanged. It is an explicit case
	// rather than a consequence of the general algorithm, which would prefix.
	if len(b.Subnets) == 1 && len(b.Links) == 0 {
		if b.Subnets[0].Model == nil {
			return nil, nil, fmt.Errorf("subnet %q has no model", b.Subnets[0].ID)
		}
		out := b.Subnets[0].Model.Clone()
		if b.Name != "" {
			out.Name = b.Name
		}
		out.Constraints = append(out.Constraints, b.Constraints...)
		return out, identityFlattenMap(b), nil
	}

	// 1. Validate.
	res := b.Validate()
	if !res.Valid {
		return nil, nil, b.MustValidate()
	}
	fm := newFlattenMap()
	fm.Warnings = res.Warnings

	subnets := b.sortedSubnets()

	// 2. Place classes. Every place starts in its own class; fusion links merge.
	places := newUnionFind()
	for _, s := range subnets {
		for _, p := range s.Model.Places {
			places.add(s.ID + "/" + p.ID)
		}
	}
	for i := range b.Links {
		l := &b.Links[i]
		if l.Kind != TokenLink && l.Kind != DataLink {
			continue
		}
		from, _ := b.resolve(l.From)
		to, _ := b.resolve(l.To)
		places.union(from.subnet.ID+"/"+from.place, to.subnet.ID+"/"+to.place)
	}

	// 3. Transition classes, merged by EventLink.
	transitions := newUnionFind()
	for _, s := range subnets {
		for _, t := range s.Model.Transitions {
			transitions.add(s.ID + "/" + t.ID)
		}
	}
	eventLinkIDs := map[string]string{} // class root → explicit Link.ID
	for i := range b.Links {
		l := &b.Links[i]
		if l.Kind != EventLink {
			continue
		}
		from, _ := b.resolve(l.From)
		to, _ := b.resolve(l.To)
		transitions.union(from.subnet.ID+"/"+from.transition, to.subnet.ID+"/"+to.transition)
	}
	for i := range b.Links {
		l := &b.Links[i]
		if l.Kind != EventLink || l.ID == "" {
			continue
		}
		from, _ := b.resolve(l.From)
		root := transitions.find(from.subnet.ID + "/" + from.transition)
		if prev, ok := eventLinkIDs[root]; ok && prev != l.ID {
			return nil, nil, fmt.Errorf(
				"event links %q and %q fuse the same transitions but name the result differently", prev, l.ID)
		}
		eventLinkIDs[root] = l.ID
	}

	placeGroups := places.groups()
	transGroups := transitions.groups()

	// Flat names.
	placeFlat := map[string]string{} // "<subnet>/<place>" → flat ID
	for root, members := range placeGroups {
		name := b.flatPlaceName(root, members)
		for _, member := range members {
			placeFlat[member] = name
		}
		if len(members) > 1 {
			fm.Wires[name] = members
		}
	}
	transFlat := map[string]string{} // "<subnet>/<transition>" → flat ID
	for root, members := range transGroups {
		name := b.flatTransitionName(root, members, eventLinkIDs[root])
		for _, member := range members {
			transFlat[member] = name
		}
		if len(members) > 1 {
			fm.FusedGroups[name] = members
			fm.Warnings = append(fm.Warnings, ValidationError{
				Code:    WarnEventLinkCycle,
				Message: fmt.Sprintf("transitions %s fire as one", strings.Join(members, ", ")),
				Element: name,
			})
		}
	}

	for _, s := range subnets {
		fm.Place[s.ID] = map[string]string{}
		fm.Transition[s.ID] = map[string]string{}
		fm.PlacePrefix[s.ID] = b.prefix(s.ID)
		for _, p := range s.Model.Places {
			fm.Place[s.ID][p.ID] = placeFlat[s.ID+"/"+p.ID]
		}
		for _, t := range s.Model.Transitions {
			fm.Transition[s.ID][t.ID] = transFlat[s.ID+"/"+t.ID]
		}
	}

	out := &Model{Name: b.Name, Version: b.Version, Description: b.Description}

	// 4. Merged places.
	mergedPlaces, err := b.mergePlaces(subnets, placeFlat)
	if err != nil {
		return nil, nil, err
	}
	out.Places = mergedPlaces

	// 5. Fused transitions.
	fusedTransitions, err := b.fuseTransitions(subnets, transGroups, transFlat, placeFlat, fm)
	if err != nil {
		return nil, nil, err
	}
	out.Transitions = fusedTransitions

	// 6. Arcs.
	out.Arcs = b.mergeArcs(subnets, placeFlat, transFlat)

	// 7. Guard links.
	if err := b.applyGuardLinks(out, placeFlat, transFlat); err != nil {
		return nil, nil, err
	}

	// 8. Constraints, events, simulation.
	out.Constraints = b.rewriteConstraints(subnets, placeFlat)
	events, err := b.mergeEvents(subnets)
	if err != nil {
		return nil, nil, err
	}
	out.Events = events
	out.Simulation = b.mergeSimulation(subnets, placeFlat, transFlat)
	b.mergeTokenDisplay(subnets, out)

	NormalizeKinds(out)
	return out, fm, nil
}

// identityFlattenMap is the trivial rewrite map for the single-subnet case.
func identityFlattenMap(b *Bundle) *FlattenMap {
	fm := newFlattenMap()
	s := b.Subnets[0]
	fm.Place[s.ID] = map[string]string{}
	fm.Transition[s.ID] = map[string]string{}
	fm.PlacePrefix[s.ID] = ""
	if s.Model != nil {
		for _, p := range s.Model.Places {
			fm.Place[s.ID][p.ID] = p.ID
		}
		for _, t := range s.Model.Transitions {
			fm.Transition[s.ID][t.ID] = t.ID
		}
	}
	return fm
}

// flatPlaceName names a place class: the qualified ID for a singleton, a
// "wire:" name keyed on the canonical member for a fused class.
func (b *Bundle) flatPlaceName(root string, members []string) string {
	subnetID, localID := splitQualified(root)
	if len(members) == 1 {
		return b.qualifiedID(subnetID, localID)
	}
	return "wire:" + b.qualifiedID(subnetID, localID)
}

// flatTransitionName names a transition class.
func (b *Bundle) flatTransitionName(root string, members []string, explicitID string) string {
	subnetID, localID := splitQualified(root)
	if len(members) == 1 {
		return b.qualifiedID(subnetID, localID)
	}
	if explicitID != "" {
		return explicitID
	}
	qualified := make([]string, 0, len(members))
	for _, m := range members {
		s, l := splitQualified(m)
		qualified = append(qualified, b.qualifiedID(s, l))
	}
	sort.Strings(qualified)
	return "fused:" + strings.Join(qualified, "+")
}

func splitQualified(id string) (subnet, local string) {
	i := strings.Index(id, "/")
	if i < 0 {
		return "", id
	}
	return id[:i], id[i+1:]
}

// mergePlaces folds each place class into a single Place.
//
// Merge rules, and why each is what it is:
//
//	Kind         must agree — the runtime dispatches on it, and fusing a counter
//	             with a data cell has no semantics
//	Type         non-empty values must agree — it drives generated struct fields,
//	             so last-writer-wins would silently miscompile
//	Initial      sum — two producers may each contribute starting stock
//	Capacity     min of non-zero — fusion means both bounds must hold, so the
//	             effective bound is the tightest one
//	Exported     OR — a wire is by definition a boundary
//	Persisted    OR — if any component needs it stored, store it
//	Resource     OR
//	InitialValue conflict is an error; there is no sum for a map or a record
func (b *Bundle) mergePlaces(subnets []*Subnet, placeFlat map[string]string) ([]Place, error) {
	var order []string
	acc := map[string]*Place{}
	members := map[string][]string{}

	for _, s := range subnets {
		for _, p := range s.Model.Places {
			flat := placeFlat[s.ID+"/"+p.ID]
			existing, ok := acc[flat]
			if !ok {
				merged := p
				merged.ID = flat
				acc[flat] = &merged
				order = append(order, flat)
				members[flat] = []string{s.ID + "/" + p.ID}
				continue
			}
			members[flat] = append(members[flat], s.ID+"/"+p.ID)

			if existing.IsToken() != p.IsToken() {
				return nil, fmt.Errorf("[%s] fusing %s: %s is a %s place but %s is a %s place",
					ErrKindMismatch, flat, members[flat][0], kindName(existing), s.ID+"/"+p.ID, kindName(&p))
			}
			if existing.Type != "" && p.Type != "" && existing.Type != p.Type {
				return nil, fmt.Errorf("[%s] fusing %s: types %q and %q differ",
					ErrTypeMismatch, flat, existing.Type, p.Type)
			}
			if existing.Type == "" {
				existing.Type = p.Type
			}
			if existing.InitialValue != nil && p.InitialValue != nil &&
				fmt.Sprintf("%v", existing.InitialValue) != fmt.Sprintf("%v", p.InitialValue) {
				return nil, fmt.Errorf("[%s] fusing %s: initial values %v and %v differ",
					ErrInitialValueConflit, flat, existing.InitialValue, p.InitialValue)
			}
			if existing.InitialValue == nil {
				existing.InitialValue = p.InitialValue
			}

			existing.Initial += p.Initial
			existing.Capacity = minNonZero(existing.Capacity, p.Capacity)
			existing.Exported = existing.Exported || p.Exported
			existing.Persisted = existing.Persisted || p.Persisted
			existing.Resource = existing.Resource || p.Resource
			if existing.Description == "" {
				existing.Description = p.Description
			}
		}
	}

	out := make([]Place, 0, len(order))
	for _, flat := range order {
		p := acc[flat]
		if len(members[flat]) > 1 {
			p.Description = describeFusion(p.Description, "fused", members[flat])
		}
		out = append(out, *p)
	}
	return out, nil
}

// minNonZero treats 0 as "unbounded", so the tightest real bound wins.
func minNonZero(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func describeFusion(desc, verb string, members []string) string {
	note := verb + ": " + strings.Join(members, ", ")
	if desc == "" {
		return note
	}
	return desc + " (" + note + ")"
}

// mergeArcs retargets every arc onto flat IDs and merges duplicates.
//
// After fusion two components can produce the same (place, transition) pair.
// Under MergeSum their weights add, which is what preserves each component's
// P-invariants: component A still consumes exactly w_A from the fused place, so
// its conservation law survives projection. MergeMax models a genuinely shared
// consumption instead.
func (b *Bundle) mergeArcs(subnets []*Subnet, placeFlat, transFlat map[string]string) []Arc {
	type arcKey struct {
		from, to string
		typ      ArcType
		keys     string
		value    string
	}

	var order []arcKey
	acc := map[arcKey]*Arc{}

	for _, s := range subnets {
		for _, a := range s.Model.Arcs {
			flat := a
			flat.From = b.translateRef(s, a.From, placeFlat, transFlat)
			flat.To = b.translateRef(s, a.To, placeFlat, transFlat)
			if flat.Weight == 0 {
				flat.Weight = 1
			}

			key := arcKey{
				from:  flat.From,
				to:    flat.To,
				typ:   flat.Type,
				keys:  strings.Join(a.Keys, "\x00"),
				value: a.Value,
			}
			if existing, ok := acc[key]; ok {
				if b.arcMerge() == MergeMax {
					if flat.Weight > existing.Weight {
						existing.Weight = flat.Weight
					}
				} else {
					existing.Weight += flat.Weight
				}
				continue
			}
			copied := flat
			acc[key] = &copied
			order = append(order, key)
		}
	}

	out := make([]Arc, 0, len(order))
	for _, key := range order {
		out = append(out, *acc[key])
	}
	return out
}

// translateRef maps a subnet-local element ID to its flat ID.
func (b *Bundle) translateRef(s *Subnet, localID string, placeFlat, transFlat map[string]string) string {
	if flat, ok := placeFlat[s.ID+"/"+localID]; ok {
		return flat
	}
	if flat, ok := transFlat[s.ID+"/"+localID]; ok {
		return flat
	}
	return b.qualifiedID(s.ID, localID)
}

// applyGuardLinks lowers each guard link onto the flattened model.
func (b *Bundle) applyGuardLinks(out *Model, placeFlat, transFlat map[string]string) error {
	for i := range b.Links {
		l := &b.Links[i]
		if l.Kind != GuardLink {
			continue
		}
		from, err := b.resolve(l.From)
		if err != nil {
			return err
		}
		to, err := b.resolve(l.To)
		if err != nil {
			return err
		}

		flatPlace := placeFlat[to.subnet.ID+"/"+to.place]
		flatTrans := transFlat[from.subnet.ID+"/"+from.transition]

		lowering, err := resolveLowering(l)
		if err != nil {
			return err
		}

		switch lowering {
		case LoweringInhibitor:
			out.Arcs = append(out.Arcs, Arc{
				From:   flatPlace,
				To:     flatTrans,
				Weight: 1,
				Type:   InhibitorArc,
			})
		case LoweringExpr:
			conjunct, err := guardConjunct(flatPlace, l.Condition)
			if err != nil {
				return err
			}
			t := out.TransitionByID(flatTrans)
			if t == nil {
				return fmt.Errorf("guard link targets transition %q, which is not in the flattened model", flatTrans)
			}
			t.Guard = andGuards(t.Guard, conjunct)
		}
	}
	return nil
}

// rewriteConstraints namespaces constraint IDs and rewrites the place
// references inside their expressions.
//
// tokenmodel/subnet deliberately skips this (subnet.go:335-346, "the expression
// language doesn't have a rewrite hook"), which leaves every composed
// conservation law pointing at names that no longer exist.
func (b *Bundle) rewriteConstraints(subnets []*Subnet, placeFlat map[string]string) []Constraint {
	var out []Constraint
	for _, s := range subnets {
		exact := subnetPlaceAlias(s, placeFlat)
		for _, c := range s.Model.Constraints {
			out = append(out, Constraint{
				ID:   b.qualifiedID(s.ID, c.ID),
				Expr: RewritePlaceRefs(c.Expr, exact, b.prefix(s.ID)),
			})
		}
	}
	// Bundle-level constraints are authored against flat IDs already.
	out = append(out, b.Constraints...)
	return out
}

// subnetPlaceAlias is local place ID → flat ID for one subnet.
func subnetPlaceAlias(s *Subnet, placeFlat map[string]string) map[string]string {
	exact := make(map[string]string, len(s.Model.Places))
	for _, p := range s.Model.Places {
		exact[p.ID] = placeFlat[s.ID+"/"+p.ID]
	}
	return exact
}

// mergeEvents unions the subnets' event definitions.
//
// Event IDs are NOT namespaced: they become Go type names downstream, and
// "orders/OrderPlaced" is not an identifier. Identical definitions dedupe;
// a genuine collision is an error, since silently picking one would make the
// other component's events decode wrongly.
func (b *Bundle) mergeEvents(subnets []*Subnet) ([]Event, error) {
	var out []Event
	seen := map[string]Event{}
	origin := map[string]string{}

	for _, s := range subnets {
		for _, e := range s.Model.Events {
			prev, ok := seen[e.ID]
			if !ok {
				seen[e.ID] = e
				origin[e.ID] = s.ID
				out = append(out, e)
				continue
			}
			if !sameEvent(prev, e) {
				return nil, fmt.Errorf("[%s] event %q is defined differently in %q and %q",
					ErrEventIDCollision, e.ID, origin[e.ID], s.ID)
			}
		}
	}
	return out, nil
}

func sameEvent(a, b Event) bool {
	if a.Name != b.Name || a.Description != b.Description || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		if a.Fields[i] != b.Fields[i] {
			return false
		}
	}
	return true
}

// mergeSimulation carries at most one objective through, and remaps solver rate
// keys (which are transition IDs) and player turn places onto flat IDs.
func (b *Bundle) mergeSimulation(subnets []*Subnet, placeFlat, transFlat map[string]string) *Simulation {
	var out *Simulation
	ensure := func() *Simulation {
		if out == nil {
			out = &Simulation{}
		}
		return out
	}

	for _, s := range subnets {
		sim := s.Model.Simulation
		if sim == nil {
			continue
		}
		o := ensure()

		if sim.Objective != "" {
			o.Objective = RewritePlaceRefs(sim.Objective, subnetPlaceAlias(s, placeFlat), b.prefix(s.ID))
		}

		for name, player := range sim.Players {
			p := player
			if p.TurnPlace != "" {
				p.TurnPlace = placeFlat[s.ID+"/"+p.TurnPlace]
			}
			mapped := make([]string, 0, len(player.Transitions))
			for _, t := range player.Transitions {
				mapped = append(mapped, transFlat[s.ID+"/"+t])
			}
			p.Transitions = mapped
			if o.Players == nil {
				o.Players = map[string]Player{}
			}
			o.Players[b.qualifiedID(s.ID, name)] = p
		}

		if sim.Solver != nil {
			if o.Solver == nil {
				o.Solver = &SolverConfig{Tspan: sim.Solver.Tspan, Dt: sim.Solver.Dt}
			}
			for tid, rate := range sim.Solver.Rates {
				if o.Solver.Rates == nil {
					o.Solver.Rates = map[string]float64{}
				}
				flat := transFlat[s.ID+"/"+tid]
				if flat == "" {
					flat = b.qualifiedID(s.ID, tid)
				}
				// A fused transition fires no faster than its slowest participant.
				if prev, ok := o.Solver.Rates[flat]; !ok || rate < prev {
					o.Solver.Rates[flat] = rate
				}
			}
		}
	}
	return out
}

// mergeTokenDisplay carries Decimals/Unit through when the subnets agree.
func (b *Bundle) mergeTokenDisplay(subnets []*Subnet, out *Model) {
	for _, s := range subnets {
		if out.Decimals == 0 && s.Model.Decimals != 0 {
			out.Decimals = s.Model.Decimals
		}
		if out.Unit == "" && s.Model.Unit != "" {
			out.Unit = s.Model.Unit
		}
	}
}
