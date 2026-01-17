package metamodel

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/metamodel/dsl"
	mpetri "github.com/pflow-xyz/go-pflow/metamodel/petri"
	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/solver"
)

// TestSemanticEquivalence verifies the metamodel and JSONLD models are semantically equivalent.
// The two models use different naming conventions but should have identical topology.
func TestSemanticEquivalence(t *testing.T) {
	// Load metamodel version
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	metamodel := mpetri.FromSchema(schema)

	// Load JSONLD version
	jsonData, err := os.ReadFile("../../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld")
	if err != nil {
		t.Fatalf("Failed to read JSONLD: %v", err)
	}
	jsonNet, err := parser.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse JSONLD: %v", err)
	}

	// Compute signatures
	metaSig := metamodel.ComputeSignature()
	jsonSig := mpetri.ComputeSignatureFromPetriNet(jsonNet)

	t.Logf("Metamodel: %d places, %d transitions, %d arcs, %d tokens",
		metaSig.PlaceCount, metaSig.TransitionCount, metaSig.ArcCount, metaSig.TotalTokens)
	t.Logf("JSONLD:    %d places, %d transitions, %d arcs, %d tokens",
		jsonSig.PlaceCount, jsonSig.TransitionCount, jsonSig.ArcCount, jsonSig.TotalTokens)

	// Check equivalence
	result := metaSig.SemanticEquivalent(jsonSig)

	if !result.PlaceMatch {
		t.Errorf("Place count mismatch: metamodel=%d, jsonld=%d",
			metaSig.PlaceCount, jsonSig.PlaceCount)
	}
	if !result.TransMatch {
		t.Errorf("Transition count mismatch: metamodel=%d, jsonld=%d",
			metaSig.TransitionCount, jsonSig.TransitionCount)
	}
	if !result.ArcMatch {
		t.Errorf("Arc count mismatch: metamodel=%d, jsonld=%d",
			metaSig.ArcCount, jsonSig.ArcCount)
	}
	if !result.TokenMatch {
		t.Errorf("Total token mismatch: metamodel=%d, jsonld=%d",
			metaSig.TotalTokens, jsonSig.TotalTokens)
	}

	if result.Equivalent {
		t.Log("Models are semantically equivalent")
	} else {
		t.Log("Models differ in structure:")
		for _, diff := range result.Differences {
			t.Logf("  - %s", diff)
		}
	}

	// Log signature details for debugging
	t.Log("Place signatures (in, out, initial):")
	t.Logf("  Metamodel: %v", metaSig.PlaceSignatures)
	t.Logf("  JSONLD:    %v", jsonSig.PlaceSignatures)

	t.Log("Transition signatures (in, out):")
	t.Logf("  Metamodel: %v", metaSig.TransSignatures)
	t.Logf("  JSONLD:    %v", jsonSig.TransSignatures)
}

// TestMetamodelSelfEquivalence verifies a model is equivalent to itself.
func TestMetamodelSelfEquivalence(t *testing.T) {
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	result := model.IsSemanticEquivalent(model)
	if !result.Equivalent {
		t.Errorf("Model should be equivalent to itself: %v", result.Differences)
	}
}

// TicTacToeMapping returns the explicit node mapping between metamodel and JSONLD.
// This serves as a witness for the isomorphism proof.
func TicTacToeMapping() *mpetri.NodeMapping {
	mapping := &mpetri.NodeMapping{
		Places:      make(map[string]string),
		Transitions: make(map[string]string),
	}

	// Board positions: p00 -> P00, etc.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			meta := fmt.Sprintf("p%d%d", i, j)
			json := fmt.Sprintf("P%d%d", i, j)
			mapping.Places[meta] = json
		}
	}

	// X history: x00 -> _X00, etc.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			meta := fmt.Sprintf("x%d%d", i, j)
			json := fmt.Sprintf("_X%d%d", i, j)
			mapping.Places[meta] = json
		}
	}

	// O history: o00 -> _O00, etc.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			meta := fmt.Sprintf("o%d%d", i, j)
			json := fmt.Sprintf("_O%d%d", i, j)
			mapping.Places[meta] = json
		}
	}

	// Control and win places
	mapping.Places["next"] = "Next"
	mapping.Places["winX"] = "win_x"
	mapping.Places["winO"] = "win_o"

	// X move transitions: playX00 -> X00, etc.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			meta := fmt.Sprintf("playX%d%d", i, j)
			json := fmt.Sprintf("X%d%d", i, j)
			mapping.Transitions[meta] = json
		}
	}

	// O move transitions: playO00 -> O00, etc.
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			meta := fmt.Sprintf("playO%d%d", i, j)
			json := fmt.Sprintf("O%d%d", i, j)
			mapping.Transitions[meta] = json
		}
	}

	// X win detection transitions
	mapping.Transitions["xRow0"] = "X00_X01_X02"
	mapping.Transitions["xRow1"] = "X10_X11_X12"
	mapping.Transitions["xRow2"] = "X20_X21_X22"
	mapping.Transitions["xCol0"] = "X00_X10_X20"
	mapping.Transitions["xCol1"] = "X01_X11_X21"
	mapping.Transitions["xCol2"] = "X02_X12_X22"
	mapping.Transitions["xDg0"] = "X00_X11_X22"
	mapping.Transitions["xDg1"] = "X20_X11_X02"

	// O win detection transitions
	mapping.Transitions["oRow0"] = "O00_O01_O02"
	mapping.Transitions["oRow1"] = "O10_O11_O12"
	mapping.Transitions["oRow2"] = "O20_O21_O22"
	mapping.Transitions["oCol0"] = "O00_O10_O20"
	mapping.Transitions["oCol1"] = "O01_O11_O21"
	mapping.Transitions["oCol2"] = "O02_O12_O22"
	mapping.Transitions["oDg0"] = "O00_O11_O22"
	mapping.Transitions["oDg1"] = "O20_O11_O02"

	return mapping
}

// TestIsomorphismWithMapping proves isomorphism using explicit witness mapping.
func TestIsomorphismWithMapping(t *testing.T) {
	// Load metamodel version
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	// Load JSONLD version
	jsonData, err := os.ReadFile("../../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld")
	if err != nil {
		t.Fatalf("Failed to read JSONLD: %v", err)
	}
	jsonNet, err := parser.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse JSONLD: %v", err)
	}

	// Get the witness mapping
	mapping := TicTacToeMapping()

	t.Logf("Mapping: %d places, %d transitions",
		len(mapping.Places), len(mapping.Transitions))

	// Verify isomorphism
	result := model.VerifyIsomorphismWithPetriNet(jsonNet, mapping)

	if !result.PlaceBijection {
		t.Error("Place bijection failed")
	}
	if !result.TransBijection {
		t.Error("Transition bijection failed")
	}
	if !result.ArcsPreserved {
		t.Error("Arc preservation failed")
	}
	if !result.InitialPreserved {
		t.Error("Initial marking preservation failed")
	}

	if result.Isomorphic {
		t.Log("Isomorphism verified: metamodel ≅ JSONLD")
	} else {
		t.Error("Models are NOT isomorphic")
		for _, err := range result.Errors {
			t.Logf("  - %s", err)
		}
	}
}

// TestODEBehavioralEquivalence verifies both models produce identical ODE trajectories.
// This is a behavioral test: if two systems evolve identically, they are equivalent.
func TestODEBehavioralEquivalence(t *testing.T) {
	// Load metamodel version
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	// Load JSONLD version
	jsonData, err := os.ReadFile("../../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld")
	if err != nil {
		t.Fatalf("Failed to read JSONLD: %v", err)
	}
	jsonNet, err := parser.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse JSONLD: %v", err)
	}
	jsonRates := make(map[string]float64)
	for trans := range jsonNet.Transitions {
		jsonRates[trans] = 1.0
	}

	// Get mapping
	mapping := TicTacToeMapping()

	// Test with default options (final state only)
	t.Run("DefaultOptions", func(t *testing.T) {
		result := model.VerifyBehavioralEquivalence(jsonNet, jsonRates, mapping, nil)

		if result.Equivalent {
			t.Logf("Behavioral equivalence confirmed (default): max diff = %.6f", result.MaxDifference)
		} else {
			t.Errorf("Behavioral mismatch: %d differences", len(result.Differences))
			for _, d := range result.Differences {
				t.Logf("  %s→%s at t=%.1f: %.4f vs %.4f (diff=%.4f)",
					d.SourcePlace, d.TargetPlace, d.Time, d.SourceValue, d.TargetValue, d.Difference)
			}
		}
	})

	// Test with strict options (multiple sample points)
	t.Run("StrictOptions", func(t *testing.T) {
		opts := mpetri.StrictBehavioralOptions()
		result := model.VerifyBehavioralEquivalence(jsonNet, jsonRates, mapping, opts)

		if result.Equivalent {
			t.Logf("Behavioral equivalence confirmed (strict): max diff = %.6f", result.MaxDifference)
			t.Logf("All %d sample points matched", len(opts.SampleAt))
		} else {
			t.Errorf("Behavioral mismatch: %d differences", len(result.Differences))
		}
	})

	// Test with custom options
	t.Run("CustomOptions", func(t *testing.T) {
		opts := &mpetri.BehavioralOptions{
			Tspan:      [2]float64{0, 3.0},
			Tolerance:  0.01, // looser tolerance
			SampleAt:   []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0},
			SolverOpts: solver.FastOptions(),
		}
		result := model.VerifyBehavioralEquivalence(jsonNet, jsonRates, mapping, opts)

		if result.Equivalent {
			t.Logf("Behavioral equivalence confirmed (custom): max diff = %.6f", result.MaxDifference)
		} else {
			t.Errorf("Behavioral mismatch with custom options")
		}
	})
}

// TestDiscoverMappingByTrajectory tests automatic discovery of place mappings via ODE.
func TestDiscoverMappingByTrajectory(t *testing.T) {
	// Load metamodel version
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	// Load JSONLD version
	jsonData, err := os.ReadFile("../../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld")
	if err != nil {
		t.Fatalf("Failed to read JSONLD: %v", err)
	}
	jsonNet, err := parser.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse JSONLD: %v", err)
	}

	// Create rates for JSONLD net
	jsonRates := make(map[string]float64)
	for trans := range jsonNet.Transitions {
		jsonRates[trans] = 1.0
	}

	// Discover mapping by trajectory
	result := model.DiscoverMappingByTrajectory(jsonNet, jsonRates, [2]float64{0, 5.0})

	t.Logf("Discovery confidence: %.1f%% (%d/%d unique mappings)",
		result.Confidence*100,
		int(result.Confidence*float64(len(result.PlaceMappings))),
		len(result.PlaceMappings))

	if len(result.Ambiguous) > 0 {
		t.Logf("Ambiguous places (symmetric trajectories): %v", result.Ambiguous)
	}

	// Get the known correct mapping for comparison
	knownMapping := TicTacToeMapping()

	// Check discovered mappings against known mapping
	correct := 0
	incorrect := 0
	for metaPlace, candidates := range result.PlaceMappings {
		expected := knownMapping.Places[metaPlace]
		found := false
		for _, c := range candidates {
			if c == expected {
				found = true
				break
			}
		}
		if found {
			correct++
		} else {
			incorrect++
			t.Logf("  Mismatch: %s expected %s, got candidates %v",
				metaPlace, expected, candidates)
		}
	}

	t.Logf("Mapping accuracy: %d/%d correct (%.1f%%)",
		correct, correct+incorrect, float64(correct)/float64(correct+incorrect)*100)

	// Log some discovered mappings
	t.Log("Sample discovered mappings:")
	count := 0
	for meta, candidates := range result.PlaceMappings {
		if count >= 5 {
			break
		}
		t.Logf("  %s -> %v", meta, candidates)
		count++
	}

	// The discovery should find all places, even if some are ambiguous
	if len(result.PlaceMappings) != 30 {
		t.Errorf("Expected 30 place mappings, got %d", len(result.PlaceMappings))
	}
}

// BaseSeed for reproducible shuffle tests. Change this to get a different test sequence.
const BaseSeed int64 = 42

// shuffleModel creates a copy of the model with randomized ordering of places, transitions, and arcs.
// Uses deterministic seeding for reproducible results.
func shuffleModel(m *mpetri.Model, seed int64) *mpetri.Model {
	r := rand.New(rand.NewSource(seed))

	shuffled := mpetri.NewModel(m.Name)
	shuffled.Version = m.Version

	// Copy places in random order
	placeIndices := r.Perm(len(m.Places))
	for _, i := range placeIndices {
		shuffled.AddPlace(m.Places[i])
	}

	// Copy transitions in random order
	transIndices := r.Perm(len(m.Transitions))
	for _, i := range transIndices {
		shuffled.AddTransition(m.Transitions[i])
	}

	// Copy arcs in random order
	arcIndices := r.Perm(len(m.Arcs))
	for _, i := range arcIndices {
		shuffled.AddArc(m.Arcs[i])
	}

	// Copy invariants in random order
	invIndices := r.Perm(len(m.Invariants))
	for _, i := range invIndices {
		shuffled.AddInvariant(m.Invariants[i])
	}

	return shuffled
}

// TestShuffledModelEquivalence verifies that equivalence detection is order-independent.
// Creates multiple randomly shuffled versions and confirms they're all equivalent.
func TestShuffledModelEquivalence(t *testing.T) {
	// Load original model
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	original := mpetri.FromSchema(schema)

	iterations := 20
	t.Logf("Testing %d random shuffles (BaseSeed=%d)", iterations, BaseSeed)
	t.Logf("Original: %d places, %d transitions, %d arcs",
		len(original.Places), len(original.Transitions), len(original.Arcs))

	for i := 0; i < iterations; i++ {
		seed := BaseSeed + int64(i)
		shuffled := shuffleModel(original, seed)

		t.Run(fmt.Sprintf("Shuffle%d", i), func(t *testing.T) {
			// Verify structural validity
			if err := shuffled.Validate(); err != nil {
				t.Fatalf("Shuffled model invalid: %v", err)
			}

			// Test semantic equivalence
			result := original.IsSemanticEquivalent(shuffled)
			if !result.Equivalent {
				t.Errorf("Semantic equivalence failed: %v", result.Differences)
			}

			// Test behavioral equivalence via ODE
			origNet := original.ToPetriNet()
			shuffledNet := shuffled.ToPetriNet()
			origRates := original.DefaultRates(1.0)
			shuffledRates := shuffled.DefaultRates(1.0)

			// Create identity mapping (same names, just different order)
			mapping := &mpetri.NodeMapping{
				Places:      make(map[string]string),
				Transitions: make(map[string]string),
			}
			for _, p := range original.Places {
				mapping.Places[p.ID] = p.ID
			}
			for _, tr := range original.Transitions {
				mapping.Transitions[tr.ID] = tr.ID
			}

			behavResult := mpetri.VerifyBehavioralEquivalence(
				origNet, origRates,
				shuffledNet, shuffledRates,
				mapping,
				mpetri.DefaultBehavioralOptions(),
			)

			if !behavResult.Equivalent {
				t.Errorf("Behavioral equivalence failed: max diff = %f", behavResult.MaxDifference)
			}

			// Test trajectory-based discovery can find mappings
			discoveryResult := mpetri.DiscoverMappingByTrajectory(
				origNet, origRates,
				shuffledNet, shuffledRates,
				[2]float64{0, 3.0},
			)

			// All places should have at least themselves as a candidate
			if len(discoveryResult.PlaceMappings) != len(original.Places) {
				t.Errorf("Discovery found %d mappings, expected %d",
					len(discoveryResult.PlaceMappings), len(original.Places))
			}
		})
	}
}

// TestLargeScaleShuffleAnalysis runs extensive shuffle testing with statistics.
func TestLargeScaleShuffleAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large-scale analysis in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	original := mpetri.FromSchema(schema)

	iterations := 100
	var semanticPass, behavioralPass, discoveryPass int
	var totalMaxDiff float64

	t.Logf("Running %d shuffle iterations (BaseSeed=%d, reproducible)", iterations, BaseSeed)

	for i := 0; i < iterations; i++ {
		seed := BaseSeed + int64(i*1000) // spread seeds for variety
		shuffled := shuffleModel(original, seed)

		// Semantic check
		semResult := original.IsSemanticEquivalent(shuffled)
		if semResult.Equivalent {
			semanticPass++
		}

		// Behavioral check
		origNet := original.ToPetriNet()
		shuffledNet := shuffled.ToPetriNet()
		origRates := original.DefaultRates(1.0)
		shuffledRates := shuffled.DefaultRates(1.0)

		mapping := &mpetri.NodeMapping{
			Places:      make(map[string]string),
			Transitions: make(map[string]string),
		}
		for _, p := range original.Places {
			mapping.Places[p.ID] = p.ID
		}
		for _, tr := range original.Transitions {
			mapping.Transitions[tr.ID] = tr.ID
		}

		behavResult := mpetri.VerifyBehavioralEquivalence(
			origNet, origRates,
			shuffledNet, shuffledRates,
			mapping,
			nil,
		)
		if behavResult.Equivalent {
			behavioralPass++
		}
		totalMaxDiff += behavResult.MaxDifference

		// Discovery check
		discoveryResult := mpetri.DiscoverMappingByTrajectory(
			origNet, origRates,
			shuffledNet, shuffledRates,
			[2]float64{0, 3.0},
		)
		if len(discoveryResult.PlaceMappings) == len(original.Places) {
			discoveryPass++
		}
	}

	t.Logf("Results over %d iterations:", iterations)
	t.Logf("  Semantic equivalence:   %d/%d (%.1f%%)", semanticPass, iterations, float64(semanticPass)/float64(iterations)*100)
	t.Logf("  Behavioral equivalence: %d/%d (%.1f%%)", behavioralPass, iterations, float64(behavioralPass)/float64(iterations)*100)
	t.Logf("  Discovery complete:     %d/%d (%.1f%%)", discoveryPass, iterations, float64(discoveryPass)/float64(iterations)*100)
	t.Logf("  Average max diff:       %.9f", totalMaxDiff/float64(iterations))

	// All should pass
	if semanticPass != iterations {
		t.Errorf("Expected 100%% semantic pass rate")
	}
	if behavioralPass != iterations {
		t.Errorf("Expected 100%% behavioral pass rate")
	}
}

// TestBehavioralShuffleStatistics collects detailed statistics on behavioral equivalence.
func TestBehavioralShuffleStatistics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping detailed statistics in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	original := mpetri.FromSchema(schema)

	iterations := 50
	t.Logf("Collecting behavioral statistics over %d shuffles (BaseSeed=%d)", iterations, BaseSeed)

	// Track per-place max differences across all shuffles
	placeMaxDiffs := make(map[string]float64)
	placeTotalDiffs := make(map[string]float64)
	for _, p := range original.Places {
		placeMaxDiffs[p.ID] = 0
		placeTotalDiffs[p.ID] = 0
	}

	// Track timing
	var totalTime float64
	var maxDiffs []float64

	// Sample at multiple time points
	opts := &mpetri.BehavioralOptions{
		Tspan:      [2]float64{0, 5.0},
		Tolerance:  1e-9, // very tight to see actual differences
		SampleAt:   []float64{0.5, 1.0, 2.0, 3.0, 4.0, 5.0},
		SolverOpts: solver.DefaultOptions(),
	}

	origNet := original.ToPetriNet()
	origRates := original.DefaultRates(1.0)

	mapping := &mpetri.NodeMapping{
		Places:      make(map[string]string),
		Transitions: make(map[string]string),
	}
	for _, p := range original.Places {
		mapping.Places[p.ID] = p.ID
	}
	for _, tr := range original.Transitions {
		mapping.Transitions[tr.ID] = tr.ID
	}

	for i := 0; i < iterations; i++ {
		seed := BaseSeed + int64(i*777)
		shuffled := shuffleModel(original, seed)
		shuffledNet := shuffled.ToPetriNet()
		shuffledRates := shuffled.DefaultRates(1.0)

		start := float64(time.Now().UnixNano())
		result := mpetri.VerifyBehavioralEquivalence(
			origNet, origRates,
			shuffledNet, shuffledRates,
			mapping,
			opts,
		)
		elapsed := (float64(time.Now().UnixNano()) - start) / 1e6 // ms
		totalTime += elapsed

		maxDiffs = append(maxDiffs, result.MaxDifference)

		// Track per-place differences
		for _, diff := range result.Differences {
			if diff.Difference > placeMaxDiffs[diff.SourcePlace] {
				placeMaxDiffs[diff.SourcePlace] = diff.Difference
			}
			placeTotalDiffs[diff.SourcePlace] += diff.Difference
		}
	}

	// Compute statistics
	var sumDiff, maxDiff, minDiff float64
	minDiff = maxDiffs[0]
	for _, d := range maxDiffs {
		sumDiff += d
		if d > maxDiff {
			maxDiff = d
		}
		if d < minDiff {
			minDiff = d
		}
	}
	avgDiff := sumDiff / float64(iterations)

	t.Log("=== Behavioral Equivalence Statistics ===")
	t.Logf("Iterations:     %d", iterations)
	t.Logf("Sample points:  %v", opts.SampleAt)
	t.Logf("Tolerance:      %.0e", opts.Tolerance)
	t.Log("")
	t.Log("Max Difference Distribution:")
	t.Logf("  Min:     %.12f", minDiff)
	t.Logf("  Max:     %.12f", maxDiff)
	t.Logf("  Average: %.12f", avgDiff)
	t.Log("")
	t.Logf("Timing:")
	t.Logf("  Total:   %.2f ms", totalTime)
	t.Logf("  Average: %.2f ms per verification", totalTime/float64(iterations))
	t.Log("")

	// Find places with any differences
	placesWithDiff := 0
	var maxPlaceDiff float64
	var maxPlaceName string
	for place, diff := range placeMaxDiffs {
		if diff > 0 {
			placesWithDiff++
		}
		if diff > maxPlaceDiff {
			maxPlaceDiff = diff
			maxPlaceName = place
		}
	}

	t.Log("Per-Place Analysis:")
	t.Logf("  Places with any difference: %d/%d", placesWithDiff, len(original.Places))
	if maxPlaceDiff > 0 {
		t.Logf("  Highest variance place: %s (max diff: %.12f)", maxPlaceName, maxPlaceDiff)
	} else {
		t.Log("  All places: perfectly equivalent (diff = 0)")
	}

	// Categorize results
	t.Log("")
	t.Log("Equivalence Categories:")
	perfect := 0
	nearPerfect := 0
	acceptable := 0
	for _, d := range maxDiffs {
		if d == 0 {
			perfect++
		} else if d < 1e-10 {
			nearPerfect++
		} else if d < 1e-6 {
			acceptable++
		}
	}
	t.Logf("  Perfect (diff=0):     %d (%.1f%%)", perfect, float64(perfect)/float64(iterations)*100)
	t.Logf("  Near-perfect (<1e-10): %d (%.1f%%)", nearPerfect, float64(nearPerfect)/float64(iterations)*100)
	t.Logf("  Acceptable (<1e-6):    %d (%.1f%%)", acceptable, float64(acceptable)/float64(iterations)*100)
}

// deleteRandomElement creates a corrupted copy of the model by deleting a random element.
// Returns the corrupted model, what was deleted, and the element ID.
func deleteRandomElement(m *mpetri.Model, seed int64) (*mpetri.Model, string, string) {
	rng := rand.New(rand.NewSource(seed))

	// Count elements
	nPlaces := len(m.Places)
	nTransitions := len(m.Transitions)
	nArcs := len(m.Arcs)
	total := nPlaces + nTransitions + nArcs

	// Pick what to delete
	choice := rng.Intn(total)

	// Deep copy
	corrupted := &mpetri.Model{
		Name:    m.Name,
		Version: m.Version,
	}

	var deletedType, deletedID string

	if choice < nPlaces {
		// Delete a place
		deletedType = "place"
		deleteIdx := choice
		deletedID = m.Places[deleteIdx].ID

		for i, p := range m.Places {
			if i != deleteIdx {
				corrupted.Places = append(corrupted.Places, mpetri.Place{
					ID:      p.ID,
					Initial: p.Initial,
				})
			}
		}
		// Copy all transitions
		for _, t := range m.Transitions {
			corrupted.Transitions = append(corrupted.Transitions, mpetri.Transition{ID: t.ID})
		}
		// Copy arcs that don't reference deleted place
		for _, a := range m.Arcs {
			if a.Source != deletedID && a.Target != deletedID {
				corrupted.Arcs = append(corrupted.Arcs, mpetri.Arc{
					Source: a.Source,
					Target: a.Target,
					Keys:   a.Keys,
					Value:  a.Value,
				})
			}
		}
	} else if choice < nPlaces+nTransitions {
		// Delete a transition
		deletedType = "transition"
		deleteIdx := choice - nPlaces
		deletedID = m.Transitions[deleteIdx].ID

		// Copy all places
		for _, p := range m.Places {
			corrupted.Places = append(corrupted.Places, mpetri.Place{
				ID:      p.ID,
				Initial: p.Initial,
			})
		}
		for i, t := range m.Transitions {
			if i != deleteIdx {
				corrupted.Transitions = append(corrupted.Transitions, mpetri.Transition{ID: t.ID})
			}
		}
		// Copy arcs that don't reference deleted transition
		for _, a := range m.Arcs {
			if a.Source != deletedID && a.Target != deletedID {
				corrupted.Arcs = append(corrupted.Arcs, mpetri.Arc{
					Source: a.Source,
					Target: a.Target,
					Keys:   a.Keys,
					Value:  a.Value,
				})
			}
		}
	} else {
		// Delete an arc
		deletedType = "arc"
		deleteIdx := choice - nPlaces - nTransitions
		deletedID = fmt.Sprintf("%s->%s", m.Arcs[deleteIdx].Source, m.Arcs[deleteIdx].Target)

		// Copy all places
		for _, p := range m.Places {
			corrupted.Places = append(corrupted.Places, mpetri.Place{
				ID:      p.ID,
				Initial: p.Initial,
			})
		}
		// Copy all transitions
		for _, t := range m.Transitions {
			corrupted.Transitions = append(corrupted.Transitions, mpetri.Transition{ID: t.ID})
		}
		for i, a := range m.Arcs {
			if i != deleteIdx {
				corrupted.Arcs = append(corrupted.Arcs, mpetri.Arc{
					Source: a.Source,
					Target: a.Target,
					Keys:   a.Keys,
					Value:  a.Value,
				})
			}
		}
	}

	return corrupted, deletedType, deletedID
}

// TestRandomDeletionDetection tests how well equivalence checks detect random deletions.
func TestRandomDeletionDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deletion detection in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	original := mpetri.FromSchema(schema)

	iterations := 30
	t.Logf("Testing deletion detection over %d random deletions (BaseSeed=%d)", iterations, BaseSeed)
	t.Log("")

	// Track detection by type
	stats := map[string]struct {
		count             int
		semanticDetected  int
		behavioralDetected int
		maxBehavioralDiff float64
		examples          []string
	}{
		"place":      {},
		"transition": {},
		"arc":        {},
	}

	origNet := original.ToPetriNet()
	origRates := original.DefaultRates(1.0)

	opts := mpetri.DefaultBehavioralOptions()

	for i := 0; i < iterations; i++ {
		seed := BaseSeed + int64(i*333)
		corrupted, deletedType, deletedID := deleteRandomElement(original, seed)

		// Semantic check
		origSig := original.ComputeSignature()
		corrSig := corrupted.ComputeSignature()
		semResult := origSig.SemanticEquivalent(corrSig)
		semanticDetected := !semResult.Equivalent

		// Behavioral check (only if we can build a valid net)
		behavioralDetected := false
		var behavioralDiff float64

		corrNet := corrupted.ToPetriNet()
		corrRates := corrupted.DefaultRates(1.0)

		// Build identity mapping for remaining places
		mapping := &mpetri.NodeMapping{
			Places:      make(map[string]string),
			Transitions: make(map[string]string),
		}
		for _, p := range corrupted.Places {
			mapping.Places[p.ID] = p.ID
		}
		for _, tr := range corrupted.Transitions {
			mapping.Transitions[tr.ID] = tr.ID
		}

		behResult := mpetri.VerifyBehavioralEquivalence(
			origNet, origRates,
			corrNet, corrRates,
			mapping,
			opts,
		)
		behavioralDetected = !behResult.Equivalent
		behavioralDiff = behResult.MaxDifference

		// Update stats
		s := stats[deletedType]
		s.count++
		if semanticDetected {
			s.semanticDetected++
		}
		if behavioralDetected {
			s.behavioralDetected++
		}
		if behavioralDiff > s.maxBehavioralDiff {
			s.maxBehavioralDiff = behavioralDiff
		}
		if len(s.examples) < 3 {
			s.examples = append(s.examples, fmt.Sprintf("%s (sem=%v, beh=%.6f)", deletedID, semanticDetected, behavioralDiff))
		}
		stats[deletedType] = s
	}

	// Report results
	t.Log("=== Deletion Detection Results ===")
	t.Log("")

	totalSemantic := 0
	totalBehavioral := 0
	for _, dtype := range []string{"place", "transition", "arc"} {
		s := stats[dtype]
		if s.count == 0 {
			continue
		}
		t.Logf("%s deletions (%d total):", dtype, s.count)
		t.Logf("  Semantic detection:   %d/%d (%.1f%%)", s.semanticDetected, s.count, float64(s.semanticDetected)/float64(s.count)*100)
		t.Logf("  Behavioral detection: %d/%d (%.1f%%)", s.behavioralDetected, s.count, float64(s.behavioralDetected)/float64(s.count)*100)
		t.Logf("  Max behavioral diff:  %.6f", s.maxBehavioralDiff)
		t.Log("  Examples:")
		for _, ex := range s.examples {
			t.Logf("    - %s", ex)
		}
		t.Log("")
		totalSemantic += s.semanticDetected
		totalBehavioral += s.behavioralDetected
	}

	t.Log("=== Summary ===")
	t.Logf("Total deletions: %d", iterations)
	t.Logf("Semantic detection rate:   %d/%d (%.1f%%)", totalSemantic, iterations, float64(totalSemantic)/float64(iterations)*100)
	t.Logf("Behavioral detection rate: %d/%d (%.1f%%)", totalBehavioral, iterations, float64(totalBehavioral)/float64(iterations)*100)

	// At least semantic should catch all deletions
	if totalSemantic != iterations {
		t.Logf("Note: %d deletions were not detected by semantic check", iterations-totalSemantic)
	}
}

// TestParallelSpeedup compares parallel vs sequential sensitivity analysis.
func TestParallelSpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping speedup test in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	t.Log("=== Parallel vs Sequential Speedup ===")
	t.Logf("Model: %d places, %d transitions, %d arcs", len(model.Places), len(model.Transitions), len(model.Arcs))
	t.Log("")

	// Sequential
	seqOpts := mpetri.FastSensitivityOptions()
	seqOpts.Parallel = false

	seqStart := time.Now()
	seqResult := model.AnalyzeSensitivity(seqOpts)
	seqDuration := time.Since(seqStart)

	// Parallel
	parOpts := mpetri.FastSensitivityOptions()
	parOpts.Parallel = true

	parStart := time.Now()
	parResult := model.AnalyzeSensitivity(parOpts)
	parDuration := time.Since(parStart)

	speedup := float64(seqDuration) / float64(parDuration)

	t.Logf("Sequential: %v (%d elements)", seqDuration, len(seqResult.Elements))
	t.Logf("Parallel:   %v (%d elements)", parDuration, len(parResult.Elements))
	t.Logf("Speedup:    %.2fx", speedup)

	// Verify same results
	if len(seqResult.Elements) != len(parResult.Elements) {
		t.Errorf("Element count mismatch: seq=%d, par=%d", len(seqResult.Elements), len(parResult.Elements))
	}

	// Rate sensitivity comparison
	t.Log("")
	t.Log("Rate Sensitivity:")

	seqOpts2 := mpetri.DefaultRateSensitivityOptions()
	seqOpts2.Parallel = false
	seqStart = time.Now()
	seqRateResult := model.AnalyzeRateSensitivity(seqOpts2)
	seqDuration = time.Since(seqStart)

	parOpts2 := mpetri.DefaultRateSensitivityOptions()
	parOpts2.Parallel = true
	parStart = time.Now()
	parRateResult := model.AnalyzeRateSensitivity(parOpts2)
	parDuration = time.Since(parStart)

	speedup = float64(seqDuration) / float64(parDuration)
	t.Logf("Sequential: %v (%d transitions)", seqDuration, len(seqRateResult.Transitions))
	t.Logf("Parallel:   %v (%d transitions)", parDuration, len(parRateResult.Transitions))
	t.Logf("Speedup:    %.2fx", speedup)
}

// TestIsolatedElementDetection tests that sensitivity analysis can detect isolated elements.
func TestIsolatedElementDetection(t *testing.T) {
	// Create a model with an isolated transition (not connected to any place)
	model := &mpetri.Model{
		Name:    "test-isolated",
		Version: "v1.0.0",
		Places: []mpetri.Place{
			{ID: "A", Initial: 1},
			{ID: "B", Initial: 0},
			{ID: "orphan", Initial: 5}, // isolated place - no arcs
		},
		Transitions: []mpetri.Transition{
			{ID: "t1"},     // connected
			{ID: "unused"}, // isolated - no arcs
		},
		Arcs: []mpetri.Arc{
			{Source: "A", Target: "t1"},
			{Source: "t1", Target: "B"},
		},
	}

	t.Log("=== Isolated Element Detection ===")
	t.Logf("Model: %d places, %d transitions, %d arcs", len(model.Places), len(model.Transitions), len(model.Arcs))
	t.Log("Expected isolated: 'orphan' place, 'unused' transition")
	t.Log("")

	// Rate sensitivity should show "unused" has zero impact
	rateResult := model.AnalyzeRateSensitivity(nil)
	t.Log("Rate Sensitivity Results:")
	for _, ts := range rateResult.Transitions {
		status := "CONNECTED"
		if ts.AtZero < 0.001 && ts.AtHalf < 0.001 && ts.AtDouble < 0.001 {
			status = "ISOLATED"
		}
		t.Logf("  %s: rate=0 impact=%.4f [%s]", ts.ID, ts.AtZero, status)
	}

	// Deletion sensitivity should show "orphan" has zero impact
	delResult := model.AnalyzeSensitivity(mpetri.FastSensitivityOptions())
	t.Log("")
	t.Log("Deletion Sensitivity Results:")
	for _, elem := range delResult.Elements {
		status := "CONNECTED"
		if elem.Impact < 0.001 {
			status = "ISOLATED"
		}
		t.Logf("  %s (%s): impact=%.4f [%s]", elem.ID, elem.Type, elem.Impact, status)
	}

	// Verify isolated elements have near-zero impact
	for _, ts := range rateResult.Transitions {
		if ts.ID == "unused" && ts.AtZero > 0.001 {
			t.Errorf("Expected 'unused' transition to have near-zero impact, got %.4f", ts.AtZero)
		}
	}
}

// TestMarkingSensitivity tests initial marking sensitivity analysis.
func TestMarkingSensitivity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping marking sensitivity in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	t.Log("=== Initial Marking Sensitivity Analysis ===")
	t.Log("")

	result := model.AnalyzeMarkingSensitivity(nil)

	t.Logf("Most sensitive place: %s (sensitivity=%.4f)", result.MostSensitive, result.MaxSensitivity)
	t.Logf("Average sensitivity: %.4f", result.AvgSensitivity)
	t.Log("")

	// Show top 10 most sensitive places
	t.Log("Top 10 most sensitive to marking changes:")
	for i, ps := range result.Places {
		if i >= 10 {
			break
		}
		t.Logf("  %-10s (init=%d): zero=%.4f, double=%.4f, +1=%.4f",
			ps.ID, ps.InitialValue, ps.AtZero, ps.AtDouble, ps.AtPlus1)
	}

	// Places with initial=0 and high AtPlus1 are "trigger" places
	t.Log("")
	t.Log("Trigger places (initial=0, sensitive to +1):")
	triggers := 0
	for _, ps := range result.Places {
		if ps.InitialValue == 0 && ps.AtPlus1 > 0.1 {
			t.Logf("  %s: +1 impact=%.4f", ps.ID, ps.AtPlus1)
			triggers++
		}
	}
	if triggers == 0 {
		t.Log("  No trigger places found")
	}
}

// TestRateSensitivityAnalysis tests rate-based sensitivity (rate=0 is like deletion).
func TestRateSensitivityAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate sensitivity in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	t.Log("=== Rate-Based Sensitivity Analysis ===")
	t.Logf("Model: %d transitions", len(model.Transitions))
	t.Log("")

	result := model.AnalyzeRateSensitivity(nil)

	t.Logf("Most sensitive transition: %s (sensitivity=%.4f)", result.MostSensitive, result.MaxSensitivity)
	t.Logf("Average sensitivity: %.4f", result.AvgSensitivity)
	t.Log("")

	// Report by category
	for _, cat := range []string{"critical", "important", "moderate", "peripheral"} {
		elems := result.ByCategory[cat]
		if len(elems) == 0 {
			continue
		}
		t.Logf("%s (%d):", strings.ToUpper(cat), len(elems))
		for i, e := range elems {
			if i >= 3 {
				t.Logf("  ... and %d more", len(elems)-3)
				break
			}
			t.Logf("  %-15s rate=0: %.4f, rate=0.5x: %.4f, rate=2x: %.4f", e.ID, e.AtZero, e.AtHalf, e.AtDouble)
		}
	}

	// Identify potentially isolated transitions (zero impact at rate=0)
	t.Log("")
	t.Log("=== Potential Isolated Transitions (rate=0 has no effect) ===")
	isolated := 0
	for _, ts := range result.Transitions {
		if ts.AtZero < 0.001 {
			t.Logf("  %s: rate=0 impact=%.6f (may be unreachable or redundant)", ts.ID, ts.AtZero)
			isolated++
		}
	}
	if isolated == 0 {
		t.Log("  No isolated transitions found (all transitions affect behavior)")
	}
}

// TestElementSensitivityAnalysis performs full sensitivity analysis on the model using package functions.
func TestElementSensitivityAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sensitivity analysis in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	model := mpetri.FromSchema(schema)

	t.Log("=== Element Sensitivity Analysis ===")
	t.Logf("Model: %d places, %d transitions, %d arcs", len(model.Places), len(model.Transitions), len(model.Arcs))
	t.Log("")

	// Use package sensitivity analysis
	opts := mpetri.FastSensitivityOptions() // samples 30 arcs
	result := model.AnalyzeSensitivity(opts)

	// Report by category
	t.Log("=== Results by Category ===")
	for _, cat := range []string{"critical", "important", "moderate", "peripheral"} {
		elems := result.ByCategory[cat]
		if len(elems) == 0 {
			continue
		}
		t.Logf("\n%s (%d elements):", strings.ToUpper(cat), len(elems))
		for i, e := range elems {
			if i >= 5 {
				t.Logf("  ... and %d more", len(elems)-5)
				break
			}
			if math.IsInf(e.Impact, 1) {
				t.Logf("  %-20s [%s] impact=∞", e.ID, e.Type)
			} else {
				t.Logf("  %-20s [%s] impact=%.4f", e.ID, e.Type, e.Impact)
			}
		}
	}

	// Report symmetry groups
	t.Log("")
	t.Log("=== Symmetry Groups (identical impact) ===")
	for impact, members := range result.SymmetryGroups {
		t.Logf("Impact %.4f: %v", impact, members)
	}

	// Summary statistics
	t.Log("")
	t.Log("=== Summary ===")
	t.Logf("Average place impact:      %.4f", result.PlaceAvgImpact)
	t.Logf("Average transition impact: %.4f", result.TransitionAvgImpact)
	t.Logf("Average arc impact:        %.4f", result.ArcAvgImpact)

	// Verify we got reasonable results
	if len(result.Elements) == 0 {
		t.Error("Expected non-empty sensitivity results")
	}
	if len(result.SymmetryGroups) == 0 {
		t.Error("Expected to find symmetry groups in tic-tac-toe model")
	}
}

// TestMultipleDeletionSeverity tests cumulative impact of multiple deletions.
func TestMultipleDeletionSeverity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-deletion severity in short mode")
	}

	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}
	original := mpetri.FromSchema(schema)

	t.Log("=== Multiple Deletion Severity Analysis ===")
	t.Log("")
	t.Logf("Original model: %d places, %d transitions, %d arcs",
		len(original.Places), len(original.Transitions), len(original.Arcs))
	t.Log("")

	origNet := original.ToPetriNet()
	origRates := original.DefaultRates(1.0)
	opts := mpetri.DefaultBehavioralOptions()

	// Test increasing numbers of deletions
	for numDeletions := 1; numDeletions <= 10; numDeletions++ {
		// Apply multiple deletions
		corrupted := original
		var deletions []string
		seed := BaseSeed

		for d := 0; d < numDeletions; d++ {
			var deletedType, deletedID string
			corrupted, deletedType, deletedID = deleteRandomElement(corrupted, seed+int64(d*111))
			deletions = append(deletions, fmt.Sprintf("%s:%s", deletedType, deletedID))
		}

		// Check if still valid (has at least some places and transitions)
		if len(corrupted.Places) == 0 || len(corrupted.Transitions) == 0 {
			t.Logf("%d deletions: Model collapsed (no places or transitions left)", numDeletions)
			continue
		}

		// Semantic check
		origSig := original.ComputeSignature()
		corrSig := corrupted.ComputeSignature()
		semResult := origSig.SemanticEquivalent(corrSig)

		// Behavioral check
		corrNet := corrupted.ToPetriNet()
		corrRates := corrupted.DefaultRates(1.0)

		mapping := &mpetri.NodeMapping{
			Places:      make(map[string]string),
			Transitions: make(map[string]string),
		}
		for _, p := range corrupted.Places {
			mapping.Places[p.ID] = p.ID
		}
		for _, tr := range corrupted.Transitions {
			mapping.Transitions[tr.ID] = tr.ID
		}

		behResult := mpetri.VerifyBehavioralEquivalence(
			origNet, origRates,
			corrNet, corrRates,
			mapping,
			opts,
		)

		t.Logf("%d deletion(s): semantic=%v, behavioral_diff=%.4f, remaining=%d/%d/%d",
			numDeletions,
			!semResult.Equivalent,
			behResult.MaxDifference,
			len(corrupted.Places),
			len(corrupted.Transitions),
			len(corrupted.Arcs))

		if numDeletions <= 3 {
			t.Logf("   Deleted: %v", deletions)
		}
	}
}
