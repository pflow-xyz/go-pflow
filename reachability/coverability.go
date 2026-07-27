package reachability

// UnboundedWitness is a finite proof that a net is unbounded.
//
// It records a firing sequence that reaches a marking strictly covering an
// earlier marking on the same path. Because the covering marking has at least
// as many tokens everywhere as its ancestor and strictly more somewhere, the
// intervening firing sequence is enabled again from the covering marking, and
// repeating it pumps tokens into Places without limit.
//
// This is the Karp–Miller unboundedness criterion. The value over "we explored
// N states and the count kept rising" is that it terminates with a definite
// answer on a bounded search: the witness is finite and checkable, so
// unboundedness can be *refuted-proved* rather than merely suspected.
type UnboundedWitness struct {
	// Prefix is the firing sequence from the initial marking to the smaller
	// (ancestor) marking.
	Prefix []string

	// Pump is the firing sequence from the ancestor marking to the marking
	// that strictly covers it. Repeating Pump grows Places without bound.
	Pump []string

	// From is the ancestor marking; To strictly covers it.
	From Marking
	To   Marking

	// Places names the places whose token count strictly increases across
	// Pump — these are the unbounded ones.
	Places []string
}

// FindUnboundedWitness searches for a proof that the net is unbounded from the
// analyzer's initial marking. It returns nil if no witness was found within the
// state limit, which is *not* a proof of boundedness — use StructuralBoundedness
// or a complete reachability graph for that.
//
// The search is breadth-first so the witness returned is a shortest one, which
// keeps counterexample traces short enough to replay by hand.
func (a *Analyzer) FindUnboundedWitness() *UnboundedWitness {
	graph := NewGraph(a.net, a.initial)

	type node struct {
		marking Marking
		// path records the markings from the initial marking to this one,
		// and trace the transitions that produced them. path[i] is the
		// marking before firing trace[i].
		path  []Marking
		trace []string
	}

	start := node{marking: a.initial.Copy()}
	queue := []node{start}
	visited := map[string]bool{a.initial.Hash(): true}

	for len(queue) > 0 && len(visited) < a.maxStates {
		cur := queue[0]
		queue = queue[1:]

		state := graph.AddState(cur.marking)
		for _, trans := range state.Enabled {
			next := graph.Fire(cur.marking, trans)
			if next == nil {
				continue
			}

			newPath := make([]Marking, len(cur.path)+1)
			copy(newPath, cur.path)
			newPath[len(cur.path)] = cur.marking

			newTrace := make([]string, len(cur.trace)+1)
			copy(newTrace, cur.trace)
			newTrace[len(cur.trace)] = trans

			// Does this marking strictly cover any ancestor on its own path?
			// Checking ancestors (not all visited states) is what makes the
			// pump sequence genuinely repeatable.
			for i, ancestor := range newPath {
				if !next.StrictlyCovers(ancestor) {
					continue
				}
				return &UnboundedWitness{
					Prefix: append([]string(nil), newTrace[:i]...),
					Pump:   append([]string(nil), newTrace[i:]...),
					From:   ancestor.Copy(),
					To:     next.Copy(),
					Places: growingPlaces(ancestor, next),
				}
			}

			hash := next.Hash()
			if visited[hash] {
				continue
			}
			visited[hash] = true
			queue = append(queue, node{marking: next, path: newPath, trace: newTrace})
		}
	}

	return nil
}

// growingPlaces returns the places whose token count strictly increases from
// one marking to the next, sorted for determinism.
func growingPlaces(from, to Marking) []string {
	var places []string
	for _, p := range to.SortedKeys() {
		if to[p] > from[p] {
			places = append(places, p)
		}
	}
	return places
}
