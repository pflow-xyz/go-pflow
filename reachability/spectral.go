package reachability

import (
	"math"
	"sort"

	"github.com/pflow-xyz/go-pflow/petri"
)

// SpectralResult holds eigenvector centrality results for nodes in a graph.
type SpectralResult struct {
	Labels      []string           // Node labels in canonical order
	Centrality  map[string]float64 // Label → centrality value
	Eigenvalue  float64            // Dominant eigenvalue (spectral radius)
	Iterations  int                // Power iteration steps used
	Convergence float64            // Final residual
}

// EigenvectorCentrality computes the dominant eigenvector of the adjacency matrix
// derived from a Petri net's bipartite graph (places ↔ transitions).
//
// The adjacency matrix is symmetric:
//
//	A = [ 0   B ]
//	    [ B^T 0 ]
//
// where B[p][t] = arc weight between place p and transition t (ignoring direction).
// Power iteration finds the Perron–Frobenius eigenvector for non-negative matrices.
func EigenvectorCentrality(net *petri.PetriNet, maxIter int, tol float64) *SpectralResult {
	// Canonical ordering
	places := sortedKeys(net.Places)
	transitions := sortedTransKeys(net.Transitions)
	n := len(places) + len(transitions)

	placeIdx := make(map[string]int, len(places))
	for i, p := range places {
		placeIdx[p] = i
	}
	transIdx := make(map[string]int, len(transitions))
	for i, t := range transitions {
		transIdx[t] = len(places) + i
	}

	// Build adjacency matrix (symmetric, undirected bipartite)
	adj := make([][]float64, n)
	for i := range adj {
		adj[i] = make([]float64, n)
	}

	for _, arc := range net.Arcs {
		w := arc.GetWeightSum()
		if arc.InhibitTransition {
			continue
		}
		// Place → Transition arc
		if pi, ok := placeIdx[arc.Source]; ok {
			if ti, ok := transIdx[arc.Target]; ok {
				adj[pi][ti] += w
				adj[ti][pi] += w
			}
		}
		// Transition → Place arc
		if ti, ok := transIdx[arc.Source]; ok {
			if pi, ok := placeIdx[arc.Target]; ok {
				adj[ti][pi] += w
				adj[pi][ti] += w
			}
		}
	}

	// Power iteration
	v := make([]float64, n)
	for i := range v {
		v[i] = 1.0 / math.Sqrt(float64(n))
	}

	var eigenvalue float64
	var residual float64
	iter := 0

	for iter < maxIter {
		// w = A * v
		w := make([]float64, n)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				w[i] += adj[i][j] * v[j]
			}
		}

		// Eigenvalue estimate = ||w||
		eigenvalue = 0
		for _, x := range w {
			eigenvalue += x * x
		}
		eigenvalue = math.Sqrt(eigenvalue)

		if eigenvalue < 1e-15 {
			break
		}

		// Normalize
		for i := range w {
			w[i] /= eigenvalue
		}

		// Convergence check
		residual = 0
		for i := range w {
			d := w[i] - v[i]
			residual += d * d
		}
		residual = math.Sqrt(residual)

		v = w
		iter++

		if residual < tol {
			break
		}
	}

	// Extract centrality values
	labels := make([]string, n)
	copy(labels, places)
	copy(labels[len(places):], transitions)

	centrality := make(map[string]float64, n)
	for i, label := range labels {
		centrality[label] = v[i]
	}

	return &SpectralResult{
		Labels:      labels,
		Centrality:  centrality,
		Eigenvalue:  eigenvalue,
		Iterations:  iter,
		Convergence: residual,
	}
}

// ProjectedCentrality computes eigenvector centrality on the entity projection
// of a bipartite entity-constraint graph.
//
// Given the biadjacency matrix B (entities × constraints), computes eigenvector
// centrality of M = B · B^T. Entry M[i][j] counts shared constraints between
// entities i and j; diagonal M[i][i] = n_i (number of constraints containing
// entity i = the "degree" used by integer reduction).
//
// entityPrefix filters places to those starting with the prefix (e.g., "_X" for
// accumulator places, or "" for all places). constraintRole filters transitions
// by role (e.g., "drain" or "" for all).
func ProjectedCentrality(net *petri.PetriNet, entityPrefix, constraintRole string, maxIter int, tol float64) *SpectralResult {
	// Identify entities and constraints
	var entities []string
	for _, p := range sortedKeys(net.Places) {
		if entityPrefix == "" || len(p) >= len(entityPrefix) && p[:len(entityPrefix)] == entityPrefix {
			entities = append(entities, p)
		}
	}

	var constraints []string
	for _, t := range sortedTransKeys(net.Transitions) {
		if constraintRole == "" || net.Transitions[t].Role == constraintRole {
			constraints = append(constraints, t)
		}
	}

	ne := len(entities)
	nc := len(constraints)

	entityIdx := make(map[string]int, ne)
	for i, e := range entities {
		entityIdx[e] = i
	}
	constraintIdx := make(map[string]int, nc)
	for i, c := range constraints {
		constraintIdx[c] = i
	}

	// Build biadjacency matrix B[entity][constraint]
	B := make([][]float64, ne)
	for i := range B {
		B[i] = make([]float64, nc)
	}

	for _, arc := range net.Arcs {
		if arc.InhibitTransition {
			continue
		}
		w := arc.GetWeightSum()
		// Entity → Constraint (place → transition)
		if ei, ok := entityIdx[arc.Source]; ok {
			if ci, ok := constraintIdx[arc.Target]; ok {
				B[ei][ci] += w
			}
		}
		// Constraint → Entity (transition → place)
		if ci, ok := constraintIdx[arc.Source]; ok {
			if ei, ok := entityIdx[arc.Target]; ok {
				B[ei][ci] += w
			}
		}
	}

	// Compute M = B * B^T (entity × entity)
	M := make([][]float64, ne)
	for i := range M {
		M[i] = make([]float64, ne)
		for j := range M[i] {
			for k := 0; k < nc; k++ {
				M[i][j] += B[i][k] * B[j][k]
			}
		}
	}

	// Power iteration on M
	v := make([]float64, ne)
	for i := range v {
		v[i] = 1.0 / math.Sqrt(float64(ne))
	}

	var eigenvalue, residual float64
	iter := 0

	for iter < maxIter {
		w := make([]float64, ne)
		for i := 0; i < ne; i++ {
			for j := 0; j < ne; j++ {
				w[i] += M[i][j] * v[j]
			}
		}

		eigenvalue = 0
		for _, x := range w {
			eigenvalue += x * x
		}
		eigenvalue = math.Sqrt(eigenvalue)

		if eigenvalue < 1e-15 {
			break
		}

		for i := range w {
			w[i] /= eigenvalue
		}

		residual = 0
		for i := range w {
			d := w[i] - v[i]
			residual += d * d
		}
		residual = math.Sqrt(residual)

		v = w
		iter++

		if residual < tol {
			break
		}
	}

	centrality := make(map[string]float64, ne)
	for i, e := range entities {
		centrality[e] = v[i]
	}

	return &SpectralResult{
		Labels:      entities,
		Centrality:  centrality,
		Eigenvalue:  eigenvalue,
		Iterations:  iter,
		Convergence: residual,
	}
}

// EntityConstraintCentrality computes eigenvector centrality from an explicit
// entity-constraint bipartite graph (not decomposed into per-entity drains).
//
// entities: labels for entities (e.g., board cells).
// constraints: each constraint is a list of entity indices it connects.
//
// Builds B[entity][constraint] = 1 if entity participates in constraint.
// Computes M = B · B^T (entity co-occurrence matrix) where:
//   - M[i][i] = n_i (number of constraints containing entity i)
//   - M[i][j] = number of constraints shared by entities i and j
//
// The dominant eigenvector of M gives eigenvector centrality that accounts
// for both direct degree AND shared-constraint structure between entities.
func EntityConstraintCentrality(entities []string, constraints [][]int, maxIter int, tol float64) *SpectralResult {
	ne := len(entities)
	nc := len(constraints)

	// Build biadjacency matrix B[entity][constraint]
	B := make([][]float64, ne)
	for i := range B {
		B[i] = make([]float64, nc)
	}
	for j, constraint := range constraints {
		for _, entityIdx := range constraint {
			if entityIdx >= 0 && entityIdx < ne {
				B[entityIdx][j] = 1.0
			}
		}
	}

	// M = B · B^T (entity × entity co-occurrence matrix)
	M := make([][]float64, ne)
	for i := range M {
		M[i] = make([]float64, ne)
		for j := range M[i] {
			for k := 0; k < nc; k++ {
				M[i][j] += B[i][k] * B[j][k]
			}
		}
	}

	// Power iteration on M
	v := make([]float64, ne)
	for i := range v {
		v[i] = 1.0 / math.Sqrt(float64(ne))
	}

	var eigenvalue, residual float64
	iter := 0

	for iter < maxIter {
		w := make([]float64, ne)
		for i := 0; i < ne; i++ {
			for j := 0; j < ne; j++ {
				w[i] += M[i][j] * v[j]
			}
		}

		eigenvalue = 0
		for _, x := range w {
			eigenvalue += x * x
		}
		eigenvalue = math.Sqrt(eigenvalue)

		if eigenvalue < 1e-15 {
			break
		}

		for i := range w {
			w[i] /= eigenvalue
		}

		residual = 0
		for i := range w {
			d := w[i] - v[i]
			residual += d * d
		}
		residual = math.Sqrt(residual)

		v = w
		iter++

		if residual < tol {
			break
		}
	}

	centrality := make(map[string]float64, ne)
	for i, e := range entities {
		centrality[e] = v[i]
	}

	return &SpectralResult{
		Labels:      entities,
		Centrality:  centrality,
		Eigenvalue:  eigenvalue,
		Iterations:  iter,
		Convergence: residual,
	}
}

// MatrixCentrality computes eigenvector centrality of an arbitrary square matrix.
// Useful for computing centrality of custom adjacency or co-occurrence matrices.
func MatrixCentrality(labels []string, M [][]float64, maxIter int, tol float64) *SpectralResult {
	n := len(labels)

	v := make([]float64, n)
	for i := range v {
		v[i] = 1.0 / math.Sqrt(float64(n))
	}

	var eigenvalue, residual float64
	iter := 0

	for iter < maxIter {
		w := make([]float64, n)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				w[i] += M[i][j] * v[j]
			}
		}

		eigenvalue = 0
		for _, x := range w {
			eigenvalue += x * x
		}
		eigenvalue = math.Sqrt(eigenvalue)

		if eigenvalue < 1e-15 {
			break
		}

		for i := range w {
			w[i] /= eigenvalue
		}

		residual = 0
		for i := range w {
			d := w[i] - v[i]
			residual += d * d
		}
		residual = math.Sqrt(residual)

		v = w
		iter++

		if residual < tol {
			break
		}
	}

	centrality := make(map[string]float64, n)
	for i, label := range labels {
		centrality[label] = v[i]
	}

	return &SpectralResult{
		Labels:      labels,
		Centrality:  centrality,
		Eigenvalue:  eigenvalue,
		Iterations:  iter,
		Convergence: residual,
	}
}

func sortedKeys(m map[string]*petri.Place) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedTransKeys(m map[string]*petri.Transition) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
