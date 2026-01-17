package metamodel

import (
	"os"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel/dsl"
	mpetri "github.com/pflow-xyz/go-pflow/metamodel/petri"
	"github.com/pflow-xyz/go-pflow/parser"
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
