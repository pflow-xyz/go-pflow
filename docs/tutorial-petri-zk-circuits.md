# Tutorial: Generate ZK Circuits from Petri Nets

This tutorial walks you through creating a Petri net model and generating zero-knowledge proof circuits that can verify valid execution.

**Time:** 15-20 minutes
**Prerequisites:** Go 1.21+, basic understanding of Petri nets

## What You'll Build

A simple order processing workflow with ZK proofs that verify:
- Orders transition through valid states (pending → approved → shipped)
- No one can skip steps or forge state transitions

```
[pending] --approve--> [approved] --ship--> [shipped]
     |
     +----cancel----> [cancelled]
```

## Step 1: Create Your Project

```bash
mkdir order-workflow && cd order-workflow
go mod init order-workflow
go get github.com/pflow-xyz/go-pflow@latest
go get github.com/consensys/gnark@latest
go get github.com/consensys/gnark-crypto@latest
```

## Step 2: Define the Petri Net Model

Create `model.json`:

```json
{
  "name": "order-workflow",
  "places": [
    {"id": "pending", "initial": 1},
    {"id": "approved"},
    {"id": "shipped"},
    {"id": "cancelled"}
  ],
  "transitions": [
    {"id": "approve"},
    {"id": "ship"},
    {"id": "cancel"}
  ],
  "arcs": [
    {"from": "pending", "to": "approve"},
    {"from": "approve", "to": "approved"},
    {"from": "approved", "to": "ship"},
    {"from": "ship", "to": "shipped"},
    {"from": "pending", "to": "cancel"},
    {"from": "cancel", "to": "cancelled"}
  ]
}
```

**Understanding the model:**
- `places`: States an order can be in (pending has 1 token initially)
- `transitions`: Actions that change state
- `arcs`: Flow from places to transitions and back

## Step 3: Generate ZK Circuits

Create `generate.go`:

```go
//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/zkcompile/petrigen"
)

func main() {
	// Load model
	data, err := os.ReadFile("model.json")
	if err != nil {
		panic(err)
	}

	var model metamodel.Model
	if err := json.Unmarshal(data, &model); err != nil {
		panic(err)
	}

	// Generate circuits
	gen, err := petrigen.New(petrigen.Options{
		PackageName:  "main",
		OutputDir:    ".",
		IncludeTests: true,
	})
	if err != nil {
		panic(err)
	}

	files, err := gen.Generate(&model)
	if err != nil {
		panic(err)
	}

	for _, f := range files {
		fmt.Println("Generated:", f.Name)
	}
}
```

Run it:

```bash
go run generate.go
```

You should see:

```
Generated: petri_state.go
Generated: petri_circuits.go
Generated: petri_game.go
Generated: petri_circuits_test.go
```

## Step 4: Understand the Generated Code

### petri_state.go

Defines your Petri net topology:

```go
const NumPlaces = 4
const NumTransitions = 3

const (
    Pending   = 0
    Approved  = 1
    Shipped   = 2
    Cancelled = 3
)

const (
    Approve = 0
    Ship    = 1
    Cancel  = 2
)

var Topology = [NumTransitions]ArcDef{
    Approve: {Inputs: []int{0}, Outputs: []int{1}},  // pending → approved
    Ship:    {Inputs: []int{1}, Outputs: []int{2}},  // approved → shipped
    Cancel:  {Inputs: []int{0}, Outputs: []int{3}},  // pending → cancelled
}
```

### petri_circuits.go

The ZK circuits:

- `PetriTransitionCircuit`: Proves a transition fired correctly
- `PetriReadCircuit`: Proves a place has tokens

### petri_game.go

Helper for tracking state and generating witnesses:

- `PetriGame`: Tracks the current marking
- `FireTransition()`: Fires a transition and returns a witness
- `ToPetriTransitionAssignment()`: Converts witness to circuit input

## Step 5: Run the Generated Tests

```bash
go test -v
```

You should see tests passing for circuit compilation and proof generation.

## Step 6: Write Your Own Proof Demo

Create `main.go`:

```go
package main

import (
	"fmt"
	"log"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

func main() {
	// 1. Compile the circuit
	fmt.Println("Compiling circuit...")
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &PetriTransitionCircuit{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Circuit has %d constraints\n", ccs.GetNbConstraints())

	// 2. Setup (generates proving/verification keys)
	fmt.Println("Running setup...")
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Create a game and make moves
	fmt.Println("\n--- Order Workflow Simulation ---")
	game := NewPetriGame()
	fmt.Println("Initial state:", game.Marking)

	// Approve the order
	fmt.Println("\nFiring 'approve' transition...")
	witness, err := game.FireTransition(Approve)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("New state:", game.Marking)

	// 4. Generate proof
	fmt.Println("\nGenerating ZK proof...")
	assignment := witness.ToPetriTransitionAssignment()
	w, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		log.Fatal(err)
	}

	proof, err := groth16.Prove(ccs, pk, w)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Proof generated!")

	// 5. Verify proof
	fmt.Println("Verifying proof...")
	publicWitness, _ := w.Public()
	err = groth16.Verify(proof, vk, publicWitness)
	if err != nil {
		log.Fatal("Verification failed:", err)
	}
	fmt.Println("✓ Proof verified!")

	// 6. Continue the workflow
	fmt.Println("\n--- Continuing workflow ---")

	// Ship the order
	witness2, err := game.FireTransition(Ship)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("After 'ship':", game.Marking)

	// Generate and verify second proof
	assignment2 := witness2.ToPetriTransitionAssignment()
	w2, _ := frontend.NewWitness(assignment2, ecc.BN254.ScalarField())
	proof2, _ := groth16.Prove(ccs, pk, w2)
	publicWitness2, _ := w2.Public()
	err = groth16.Verify(proof2, vk, publicWitness2)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Ship transition verified!")

	// 7. Try an invalid transition
	fmt.Println("\n--- Testing invalid transition ---")
	_, err = game.FireTransition(Approve) // Can't approve an already shipped order
	if err != nil {
		fmt.Println("✓ Correctly rejected:", err)
	}

	fmt.Println("\n--- Summary ---")
	fmt.Printf("Pre-state root:  %s...\n", witness.PreStateRoot.String()[:20])
	fmt.Printf("Post-state root: %s...\n", witness.PostStateRoot.String()[:20])
	fmt.Println("All transitions cryptographically verified!")
}
```

Run it:

```bash
go run .
```

Expected output:

```
Compiling circuit...
Circuit has 1847 constraints

Running setup...

--- Order Workflow Simulation ---
Initial state: pending: 1

Firing 'approve' transition...
New state: approved: 1

Generating ZK proof...
Proof generated!
Verifying proof...
✓ Proof verified!

--- Continuing workflow ---
After 'ship': shipped: 1
✓ Ship transition verified!

--- Testing invalid transition ---
✓ Correctly rejected: transition approve is not enabled

--- Summary ---
Pre-state root:  12345678901234567890...
Post-state root: 98765432109876543210...
All transitions cryptographically verified!
```

## Step 7: Export Solidity Verifier (Optional)

For on-chain verification, export a Solidity contract:

```go
// Add to main.go
import "github.com/consensys/gnark/backend/groth16/bn254"

// After setup:
f, _ := os.Create("Verifier.sol")
err = vk.ExportSolidity(f)
f.Close()
fmt.Println("Exported Verifier.sol")
```

## What You've Learned

1. **Model → Circuits**: Petri net JSON becomes working ZK circuits
2. **State Commitments**: Markings are hashed to state roots
3. **Transition Proofs**: Each state change can be cryptographically verified
4. **Invalid Transitions**: The Petri net semantics prevent illegal moves

## Next Steps

- **Add more places/transitions** to model complex workflows
- **Chain proofs** to verify entire execution histories
- **Deploy on-chain** using the Solidity verifier
- **Add selective disclosure** (see roadmap)

## Common Issues

**"circuit compilation failed"**
- Check your model.json has valid arc references

**"transition not enabled"**
- The current marking doesn't have tokens in the input places

**"proof verification failed"**
- The witness doesn't match the public inputs (state roots)

## Resources

- [go-pflow repository](https://github.com/pflow-xyz/go-pflow)
- [gnark documentation](https://docs.gnark.consensys.io/)
- [Petri net basics](https://en.wikipedia.org/wiki/Petri_net)
- [Paper: Verifiable Petri Net Execution](./paper-draft-verifiable-petri-nets.md)
