package metamodel

import "sort"

// unionFind is a disjoint-set over element IDs whose representative is always
// the lexicographically smallest member of its set.
//
// Composition needs that property specifically. Links are pairwise, but fusing
// them is a set operation: linking A→B and B→C must produce one three-element
// class regardless of the order the links appear in. Choosing the smallest
// member as the canonical name makes the result independent of link order, so
// Flatten is associative. (tokenmodel/subnet names wires pairwise as
// "wire:<from_subnet>.<from_port>", which yields two different names for one
// class when links chain — see subnet.go:242.)
type unionFind struct {
	parent map[string]string
}

func newUnionFind() *unionFind {
	return &unionFind{parent: map[string]string{}}
}

// add registers an element as its own singleton set. Idempotent.
func (u *unionFind) add(x string) {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
}

// find returns the canonical (smallest) member of x's set, compressing the path.
func (u *unionFind) find(x string) string {
	u.add(x)
	root := x
	for u.parent[root] != root {
		root = u.parent[root]
	}
	// Path compression.
	for u.parent[x] != root {
		u.parent[x], x = root, u.parent[x]
	}
	return root
}

// union merges the sets containing a and b. The smaller root wins, which is what
// keeps find() returning the lexicographic minimum.
func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if rb < ra {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
}

// groups returns canonical representative → sorted members, for every set with
// at least one element.
func (u *unionFind) groups() map[string][]string {
	out := map[string][]string{}
	for x := range u.parent {
		root := u.find(x)
		out[root] = append(out[root], x)
	}
	for _, members := range out {
		sort.Strings(members)
	}
	return out
}
