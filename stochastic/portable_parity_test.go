package stochastic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// The goldens in testdata/portable/ are the byte-exact contract between this
// package's portable path and the ports in pflow-rs, pflow-xyz and pflow-jl
// (ssa-spec.md §4). They are written by cmd/ssa-goldens — `make ssa-goldens`
// — and never by a test. Every double is compared with ==; a difference is a
// finding about the engine, not about the file.

// portableFixtures are the files that must exist. A glob alone would let a
// deleted golden pass silently.
var portableFixtures = []string{"chain", "sir", "dimer", "gates", "coffeeshop"}

type portableFixture struct {
	Model   json.RawMessage `json:"model"`
	Options struct {
		Horizon      float64 `json:"horizon"`
		Samples      int     `json:"samples"`
		Realizations int     `json:"realizations"`
		Seed         uint64  `json:"seed"`
	} `json:"options"`
	Expected struct {
		Times  []float64 `json:"times"`
		Series map[string]struct {
			Values []float64 `json:"values"`
			StdDev []float64 `json:"stddev"`
		} `json:"series"`
		Final map[string]float64 `json:"final"`
	} `json:"expected"`
}

func loadPortable(t *testing.T, name string) (*portableFixture, *metamodel.Model) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "portable", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx portableFixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	// The model uses metamodel key names and only those, so it loads unchanged.
	var m metamodel.Model
	if err := json.Unmarshal(fx.Model, &m); err != nil {
		t.Fatalf("%s: model: %v", name, err)
	}
	// What a loader must reject (§3.1/§4.1): guards and non-token places. A
	// fixture carrying either is a contract violation whatever its numbers say.
	for i := range m.Transitions {
		if m.Transitions[i].Guard != "" {
			t.Fatalf("%s: transition %q carries a guard; portable fixtures may not", name, m.Transitions[i].ID)
		}
	}
	for i := range m.Places {
		if !m.Places[i].IsToken() {
			t.Fatalf("%s: place %q is not a token place; portable fixtures may not carry one", name, m.Places[i].ID)
		}
	}
	if fx.Options.Samples < 2 || fx.Options.Realizations < 2 {
		t.Fatalf("%s: options %+v; every fixture needs samples >= 2 and realizations >= 2", name, fx.Options)
	}
	if fx.Options.Seed >= 1<<53 {
		t.Fatalf("%s: seed %d is not exact in every JSON parser", name, fx.Options.Seed)
	}
	return &fx, &m
}

func TestPortableGoldensPresent(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "portable", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, f := range files {
		have = append(have, filepath.Base(f[:len(f)-len(".json")]))
	}
	want := append([]string(nil), portableFixtures...)
	sort.Strings(have)
	sort.Strings(want)
	if len(have) != len(want) {
		t.Fatalf("testdata/portable holds %v, want exactly %v", have, want)
	}
	for i := range want {
		if have[i] != want[i] {
			t.Fatalf("testdata/portable holds %v, want exactly %v", have, want)
		}
	}
}

func TestPortableParity(t *testing.T) {
	for _, name := range portableFixtures {
		t.Run(name, func(t *testing.T) {
			fx, m := loadPortable(t, name)
			got, err := Simulate(m, nil, Options{
				Horizon:      fx.Options.Horizon,
				Samples:      fx.Options.Samples,
				Realizations: fx.Options.Realizations,
				Seed:         int64(fx.Options.Seed),
				Portable:     true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Method != "ssa" {
				t.Errorf("method: got %q, want ssa", got.Method)
			}
			if len(got.Caveats) > 0 {
				t.Errorf("caveats: %v; the portable contract forbids any", got.Caveats)
			}

			if len(fx.Expected.Times) != fx.Options.Samples {
				t.Errorf("golden times has %d entries, options.samples is %d", len(fx.Expected.Times), fx.Options.Samples)
			}
			exactFloats(t, "times", got.Times, fx.Expected.Times)

			// The set of place ids equals the model's token places.
			var modelPlaces, goldenPlaces []string
			for i := range m.Places {
				modelPlaces = append(modelPlaces, m.Places[i].ID)
			}
			for p := range fx.Expected.Series {
				goldenPlaces = append(goldenPlaces, p)
			}
			sort.Strings(modelPlaces)
			sort.Strings(goldenPlaces)
			if len(modelPlaces) != len(goldenPlaces) {
				t.Fatalf("golden series places %v, model places %v", goldenPlaces, modelPlaces)
			}
			for i := range modelPlaces {
				if modelPlaces[i] != goldenPlaces[i] {
					t.Fatalf("golden series places %v, model places %v", goldenPlaces, modelPlaces)
				}
			}
			if len(fx.Expected.Final) != len(modelPlaces) {
				t.Errorf("golden final has %d places, model %d", len(fx.Expected.Final), len(modelPlaces))
			}

			if len(got.Series) != len(fx.Expected.Series) {
				t.Fatalf("series: got %d, want %d", len(got.Series), len(fx.Expected.Series))
			}
			last := fx.Options.Samples - 1
			for _, s := range got.Series {
				want, ok := fx.Expected.Series[s.Place]
				if !ok {
					t.Errorf("series %q: not in golden", s.Place)
					continue
				}
				exactFloats(t, "series."+s.Place+".values", s.Values, want.Values)
				exactFloats(t, "series."+s.Place+".stddev", s.StdDev, want.StdDev)
				// final is mean[S-1], bit for bit, in the golden and in the run.
				if wf := fx.Expected.Final[s.Place]; wf != want.Values[last] {
					t.Errorf("golden final[%s] = %v but values[%d] = %v", s.Place, wf, last, want.Values[last])
				}
				if got.Final[s.Place] != s.Values[last] {
					t.Errorf("final[%s] = %v but values[%d] = %v", s.Place, got.Final[s.Place], last, s.Values[last])
				}
			}
			exactMap(t, "final", got.Final, fx.Expected.Final)
		})
	}
}
