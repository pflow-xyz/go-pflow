package saga

import "github.com/pflow-xyz/go-pflow/reachability"

// Stranded returns the reachable states from which none of the target places can
// ever be marked — the half-done states a saga exists to rule out.
//
// Pass the saga's terminals (completed and rolled_back, under whatever flat
// names the composition gave them) to ask "can every state still finish one way
// or the other?", or rolled_back alone to ask "can every state that has not
// completed still be undone?". The graph must be complete: on a truncated one a
// state may look stranded only because its way out was not explored, so check
// Result.IsComplete before trusting an empty or non-empty answer.
//
// It is a backward search from the target states over the graph's predecessor
// edges, so it costs one pass over the graph however many states there are.
func Stranded(g *reachability.Graph, places ...string) []*reachability.State {
	can := map[*reachability.State]bool{}
	var frontier []*reachability.State
	for _, st := range g.StatesList() {
		for _, p := range places {
			if st.Marking.Get(p) > 0 {
				can[st] = true
				frontier = append(frontier, st)
				break
			}
		}
	}
	for len(frontier) > 0 {
		st := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		for _, e := range st.Predecessors {
			if !can[e.From] {
				can[e.From] = true
				frontier = append(frontier, e.From)
			}
		}
	}

	var out []*reachability.State
	for _, st := range g.StatesList() {
		if !can[st] {
			out = append(out, st)
		}
	}
	return out
}
