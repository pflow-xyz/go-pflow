package crmath

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

type powCase struct{ x, y, want float64 }

// testdata/pow.txt is a byte-identical copy of pflow-rs'
// crates/pflow-solver/tests/fixtures/crmath/pow.txt (scripts/crmath/corpus.py
// there): "x y x^y" as hex bit patterns, x^y correctly rounded from Python's
// decimal at 120 digits.
func loadCases(t *testing.T) []powCase {
	fh, err := os.Open("testdata/pow.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	var out []powCase
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		w := strings.Fields(sc.Text())
		var v [3]float64
		for i := range v {
			b, err := strconv.ParseUint(w[i], 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			v[i] = math.Float64frombits(b)
		}
		out = append(out, powCase{v[0], v[1], v[2]})
	}
	if len(out) < 13000 {
		t.Fatalf("corpus truncated: %d cases", len(out))
	}
	return out
}

func same(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b) || (math.IsNaN(a) && math.IsNaN(b))
}

func TestPowMatchesTheCorpus(t *testing.T) {
	cases := loadCases(t)
	bad, mathPowWrong := 0, 0
	for _, c := range cases {
		if got := Pow(c.x, c.y); !same(got, c.want) {
			bad++
			if bad <= 20 {
				t.Errorf("Pow(%v [%016x], %v [%016x]) = %v, want %v", c.x, math.Float64bits(c.x), c.y, math.Float64bits(c.y), got, c.want)
			}
		}
		if !same(math.Pow(c.x, c.y), c.want) {
			mathPowWrong++
		}
	}
	t.Logf("math.Pow differs from the correctly rounded result on %d of %d cases", mathPowWrong, len(cases))
	if bad > 0 {
		t.Fatalf("%d mismatches", bad)
	}
}

func TestTheSlowPathMatchesTheCorpus(t *testing.T) {
	for _, c := range loadCases(t) {
		if c.x > 0 && !math.IsInf(c.x, 0) && c.x != 1 && !math.IsInf(c.y, 0) && !math.IsNaN(c.y) && c.y != 0 &&
			!math.IsInf(c.want, 0) && c.want != 0 {
			if got := powSlow(c.x, c.y); !same(got, c.want) {
				t.Fatalf("powSlow(%v, %v) = %v, want %v", c.x, c.y, got, c.want)
			}
		}
	}
}

func TestSlowAgreesWithFast(t *testing.T) {
	s := uint64(0x9E3779B97F4A7C15)
	next := func() uint64 {
		s ^= s << 13
		s ^= s >> 7
		s ^= s << 17
		return s
	}
	for i := 0; i < 20000; i++ {
		x := math.Float64frombits(next() % 0x7FF0000000000000)
		y := float64(next())/float64(math.MaxUint64)*8 - 4
		if !(x > 0) {
			continue
		}
		if a, b := Pow(x, y), powSlow(x, y); !same(a, b) {
			t.Fatalf("Pow(%v, %v): fast %v, slow %v", x, y, a, b)
		}
	}
}
