package reachability

import (
	"sort"
)

// DefaultFarkasLimit caps the number of intermediate rows retained during
// Farkas iteration. The algorithm is worst-case exponential, so a limit keeps
// pathological nets from exhausting memory. Results are still sound when the
// limit is hit — just potentially incomplete (see FarkasResult.Truncated).
const DefaultFarkasLimit = 20000

// FarkasResult holds the outcome of a minimal-support invariant computation.
type FarkasResult struct {
	// Basis contains the minimal-support semi-positive invariants, each as a
	// coefficient vector indexed in the same order as Labels.
	Basis [][]int

	// Labels names the dimension of each vector (places for P-invariants,
	// transitions for T-invariants).
	Labels []string

	// Truncated is true if the row limit was reached and the basis may be
	// incomplete. Every returned invariant is still valid; some may be missing.
	Truncated bool
}

// farkasRow is one row of the augmented matrix [C | I] during elimination.
// coef is the partially eliminated matrix row; annot accumulates which linear
// combination of the original basis vectors produced it. When coef reaches the
// zero vector, annot is an invariant.
type farkasRow struct {
	coef  []int
	annot []int
}

// farkas computes the minimal-support semi-positive solutions of y*M = 0
// (y >= 0) using the Farkas / Martinez-Silva algorithm.
//
// M is given as rows x cols. The returned vectors are indexed over rows.
//
// The algorithm eliminates one column at a time. For each column j, rows are
// partitioned by the sign of their entry at j; every positive/negative pair is
// combined into a new row whose entry at j is zero, and rows already zero at j
// pass through unchanged. After all columns are eliminated, every surviving row
// has coef == 0, so its annot satisfies y*M = 0.
//
// Minimality is enforced after each column: a row whose annotation support is a
// strict superset of another row's is not a minimal-support invariant and is
// discarded. Without this filter the row count explodes and the output contains
// redundant sums of simpler invariants.
func farkas(matrix [][]int, rowCount, colCount, limit int) ([][]int, bool) {
	if rowCount == 0 {
		return nil, false
	}
	if limit <= 0 {
		limit = DefaultFarkasLimit
	}

	rows := make([]farkasRow, rowCount)
	for i := 0; i < rowCount; i++ {
		coef := make([]int, colCount)
		copy(coef, matrix[i])
		annot := make([]int, rowCount)
		annot[i] = 1
		rows[i] = farkasRow{coef: coef, annot: annot}
	}

	truncated := false

	for j := 0; j < colCount; j++ {
		var zero, pos, neg []farkasRow
		for _, r := range rows {
			switch {
			case r.coef[j] > 0:
				pos = append(pos, r)
			case r.coef[j] < 0:
				neg = append(neg, r)
			default:
				zero = append(zero, r)
			}
		}

		// Rows already zero at column j carry over untouched.
		next := zero

		for _, p := range pos {
			for _, n := range neg {
				if len(next) >= limit {
					truncated = true
					break
				}
				// Scale so the column-j entries cancel exactly:
				//   a = -n.coef[j] > 0, b = p.coef[j] > 0
				//   a*p[j] + b*n[j] = (-n[j])*p[j] + p[j]*n[j] = 0
				a, b := -n.coef[j], p.coef[j]
				next = append(next, combineRows(p, n, a, b))
			}
			if truncated {
				break
			}
		}

		rows = minimalRows(next)
	}

	// Every surviving row is an invariant; return its annotation.
	basis := make([][]int, 0, len(rows))
	for _, r := range rows {
		if isZeroVec(r.annot) {
			continue
		}
		basis = append(basis, r.annot)
	}

	sortVectors(basis)
	return basis, truncated
}

// combineRows returns a*p + b*n, reduced by the gcd of all its entries so
// invariants come back in lowest terms.
func combineRows(p, n farkasRow, a, b int) farkasRow {
	coef := make([]int, len(p.coef))
	for i := range coef {
		coef[i] = a*p.coef[i] + b*n.coef[i]
	}
	annot := make([]int, len(p.annot))
	for i := range annot {
		annot[i] = a*p.annot[i] + b*n.annot[i]
	}

	// Normalize by the gcd across both halves so that, e.g., 2P1+2P2 and
	// P1+P2 are recognized as the same invariant.
	g := 0
	for _, v := range coef {
		g = gcd(g, abs(v))
	}
	for _, v := range annot {
		g = gcd(g, abs(v))
	}
	if g > 1 {
		for i := range coef {
			coef[i] /= g
		}
		for i := range annot {
			annot[i] /= g
		}
	}

	return farkasRow{coef: coef, annot: annot}
}

// minimalRows removes duplicates and any row whose annotation support strictly
// contains another row's. This is the minimality criterion that keeps the
// Farkas basis from filling up with sums of simpler invariants.
func minimalRows(rows []farkasRow) []farkasRow {
	if len(rows) <= 1 {
		return rows
	}

	// Deduplicate identical annotations first — cheaper than the subset test.
	seen := make(map[string]bool, len(rows))
	unique := rows[:0:0]
	for _, r := range rows {
		key := vecKey(r.annot)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, r)
	}

	supports := make([]map[int]bool, len(unique))
	for i, r := range unique {
		s := make(map[int]bool)
		for k, v := range r.annot {
			if v != 0 {
				s[k] = true
			}
		}
		supports[i] = s
	}

	var result []farkasRow
	for i := range unique {
		minimal := true
		for k := range unique {
			if i == k {
				continue
			}
			// Discard row i if row k's support is a strict subset of it.
			if len(supports[k]) < len(supports[i]) && isSubset(supports[k], supports[i]) {
				minimal = false
				break
			}
		}
		if minimal {
			result = append(result, unique[i])
		}
	}

	return result
}

func isSubset(a, b map[int]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func isZeroVec(v []int) bool {
	for _, x := range v {
		if x != 0 {
			return false
		}
	}
	return true
}

func vecKey(v []int) string {
	b := make([]byte, 0, len(v)*3)
	for _, x := range v {
		b = appendInt(b, x)
		b = append(b, ',')
	}
	return string(b)
}

func appendInt(b []byte, x int) []byte {
	if x < 0 {
		b = append(b, '-')
		x = -x
	}
	if x >= 10 {
		b = appendInt(b, x/10)
	}
	return append(b, byte('0'+x%10))
}

// sortVectors orders a basis deterministically: fewer non-zero entries first
// (simpler invariants lead), then lexicographically.
func sortVectors(vs [][]int) {
	sort.SliceStable(vs, func(i, j int) bool {
		si, sj := supportSize(vs[i]), supportSize(vs[j])
		if si != sj {
			return si < sj
		}
		for k := range vs[i] {
			if vs[i][k] != vs[j][k] {
				return vs[i][k] < vs[j][k]
			}
		}
		return false
	})
}

func supportSize(v []int) int {
	n := 0
	for _, x := range v {
		if x != 0 {
			n++
		}
	}
	return n
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}
