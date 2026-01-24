# Metamodel

A declarative schema system for defining state machines as Petri nets.

## Overview

The metamodel package provides:
- **Schema Definition** - Declarative state machine specification
- **Struct Tag DSL** - Define schemas as Go types with morphisms (categorical style)
- **Fluent Builder** - Programmatic schema construction
- **S-Expression DSL** - Human-readable schema language
- **Guard Expressions** - Transition preconditions
- **Petri Net Execution** - Formal execution semantics

## Quick Start

Three equivalent syntaxes for defining schemas:

| Syntax | Speed | Best For |
|--------|-------|----------|
| Struct Tags | ~5.5μs | Static schemas, type safety, categorical style |
| Fluent Builder | ~1.5μs | Dynamic schemas, runtime generation |
| S-Expression | ~2μs | Human-readable config files |

### Using Struct Tags (Categorical Style)

Define schemas as types with their morphisms:

```go
import "github.com/pflow-xyz/go-pflow/metamodel/dsl"

type ERC20 struct {
    _ struct{} `meta:"name:ERC-20,version:v1.0.0"`

    TotalSupply dsl.DataState `meta:"type:uint256"`
    Balances    dsl.DataState `meta:"type:map[address]uint256,exported"`
    Allowances  dsl.DataState `meta:"type:map[address]map[address]uint256,exported"`

    Transfer     dsl.Action `meta:"guard:balances[from] >= amount && to != address(0)"`
    Approve      dsl.Action `meta:""`
    TransferFrom dsl.Action `meta:"guard:balances[from] >= amount && allowances[from][caller] >= amount"`
}

// Define morphisms (flows between objects)
func (ERC20) Flows() []dsl.Flow {
    return []dsl.Flow{
        {From: "Balances", To: "Transfer", Keys: []string{"from"}},
        {From: "Transfer", To: "Balances", Keys: []string{"to"}},
        {From: "Approve", To: "Allowances", Keys: []string{"owner", "spender"}},
    }
}

// Define constraints (equations/relations)
func (ERC20) Constraints() []dsl.Invariant {
    return []dsl.Invariant{
        {ID: "conservation", Expr: "sum(balances) == totalSupply"},
    }
}

schema, _ := dsl.SchemaFromStruct(ERC20{})
```

**Marker Types:**
- `dsl.DataState` — data container (maps, values)
- `dsl.TokenState` — discrete counter
- `dsl.Action` — state transformation

**Tag Format:** `meta:"key:value,key2:value2,flag"`
- `type:T` — type schema
- `initial:V` — initial value
- `exported` — include in state root
- `guard:EXPR` — precondition

### Using the Fluent Builder

```go
import "github.com/pflow-xyz/go-pflow/metamodel/dsl"

schema := dsl.Build("ERC-20").
    Version("1.0.0").
    Data("balances", "map[address]uint256").Exported().
    Data("totalSupply", "uint256").
    Action("transfer").Guard("balances[from] >= amount && to != address(0)").
    Flow("balances", "transfer").Keys("from").
    Flow("transfer", "balances").Keys("to").
    Constraint("conservation", "sum(balances) == totalSupply").
    MustSchema()
```

### Hybrid: Struct Tags + Builder

```go
// Start with static structure, add dynamic elements
schema := dsl.BuilderFromStruct(ERC20{}).
    Action("emergencyPause").Guard("caller == admin").
    MustSchema()
```

### Using the S-Expression DSL

```go
import "github.com/pflow-xyz/go-pflow/metamodel/dsl"

schema, err := dsl.ParseSchema(`
(schema ERC-20
  (version v1.0.0)
  (states
    (state balances :type map[address]uint256 :exported)
    (state totalSupply :type uint256))
  (actions
    (action transfer :guard {balances[from] >= amount && to != address(0)}))
  (arcs
    (arc balances -> transfer :keys (from))
    (arc transfer -> balances :keys (to)))
  (constraints
    (constraint conservation {sum(balances) == totalSupply})))
`)
```

## Package Structure

```
metamodel/
├── schema.go       # Core types: Schema, State, Action, Arc, Constraint
├── runtime.go      # Runtime model execution
├── cid.go          # Content-addressed model IDs
├── snapshot.go     # State snapshots
├── validate.go     # Schema validation
├── errors.go       # Error types
├── dsl/            # Schema definition DSLs
│   ├── tags.go     # Struct tag dialect (categorical style)
│   ├── builder.go  # Fluent schema builder
│   ├── lexer.go    # S-expression tokenizer
│   ├── parser.go   # S-expression parser
│   ├── interpret.go# DSL interpreter
│   ├── ast.go      # AST types
│   └── codegen.go  # Code generation
├── guard/          # Guard expression DSL
│   ├── lexer.go    # Guard tokenizer
│   ├── parser.go   # Guard parser
│   ├── eval.go     # Guard evaluation
│   └── invariant.go# Invariant checking
└── petri/          # Petri net execution model
    ├── model.go    # Place/Transition model
    ├── fire.go     # Transition firing
    ├── analysis.go # Reachability analysis
    └── bridge.go   # Schema-to-model conversion
```

## Core Concepts

### Schema

A schema defines a state machine with:
- **States** (Places) - Data containers with types
- **Actions** (Transitions) - Operations that transform state
- **Arcs** (Flows) - Connections between states and actions
- **Constraints** - Invariants that must hold

### State Types

```go
// Data state - stores values
Data("balances", "map[address]uint256")

// Token state - discrete counter
Token("counter").Initial(100)

// Exported - included in state root computation
Data("balances", "map[address]uint256").Exported()
```

### Actions and Guards

```go
// Action with guard precondition
Action("transfer").Guard("balances[from] >= amount && to != address(0)")

// Action without guard (always enabled)
Action("mint")

// Action linked to blockchain event
Action("transfer").Guard("...").OnEvent("Transfer")
```

### Arcs (Flows)

Arcs connect states to actions (input) or actions to states (output):

```go
// Input arc: state -> action (consumes tokens)
Flow("balances", "transfer").Keys("from")

// Output arc: action -> state (produces tokens)
Flow("transfer", "balances").Keys("to")
```

### Constraints

Invariants that must hold across all states:

```go
Constraint("conservation", "sum(balances) == totalSupply")
Constraint("non_negative", "forall a: balances[a] >= 0")
```

## S-Expression DSL Syntax

```lisp
(schema <name>
  (version <version>)

  (states
    (state <id> :type <type> [:exported] [:kind token] [:initial <value>])
    ...)

  (actions
    (action <id> [:guard {<expression>}])
    ...)

  (arcs
    (arc <source> -> <target> [:keys (<key1> <key2> ...)] [:value <expr>])
    ...)

  (constraints
    (constraint <id> {<expression>})
    ...))
```

### Example: ERC-721 NFT

```lisp
(schema ERC-721
  (version v1.0.0)

  (states
    (state owners :type map[uint256]address :exported)
    (state balances :type map[address]uint256 :exported)
    (state approved :type map[uint256]address)
    (state operators :type map[address]map[address]bool))

  (actions
    (action transferFrom
      :guard {owners[tokenId] == caller || approved[tokenId] == caller || operators[owners[tokenId]][caller]})
    (action approve
      :guard {owners[tokenId] == caller})
    (action setApprovalForAll)
    (action mint)
    (action burn
      :guard {owners[tokenId] == caller}))

  (arcs
    (arc owners -> transferFrom :keys (tokenId))
    (arc transferFrom -> owners :keys (tokenId))
    (arc balances -> transferFrom :keys (from))
    (arc transferFrom -> balances :keys (to))
    (arc approved -> approve :keys (tokenId))
    (arc approve -> approved :keys (tokenId))
    (arc operators -> setApprovalForAll :keys (owner spender))
    (arc setApprovalForAll -> operators :keys (owner spender))
    (arc mint -> owners :keys (tokenId))
    (arc mint -> balances :keys (to))
    (arc owners -> burn :keys (tokenId))
    (arc balances -> burn :keys (from)))

  (constraints
    (constraint ownership {forall t: owners[t] != 0 => balances[owners[t]] >= 1})))
```

## Guard Expression Syntax

Guards are boolean expressions that must evaluate to true for a transition to fire:

```
// Comparisons
balances[from] >= amount
owners[tokenId] == caller
to != address(0)

// Logical operators
condition1 && condition2
condition1 || condition2
!condition

// Map/field access
balances[addr]
approved[tokenId]
operators[owner][spender]
schedule.revocable

// Arithmetic
balance - amount >= 0
totalSupply + amount <= maxSupply

// Function calls
sum(balances)
length(array)
```

## Petri Net Execution

Convert a schema to an executable Petri net:

```go
import (
    "github.com/pflow-xyz/go-pflow/metamodel/dsl"
    "github.com/pflow-xyz/go-pflow/metamodel/petri"
)

// Build schema
schema := dsl.Build("Counter").
    Token("count").Initial(10).
    Action("increment").
    Action("decrement").Guard("count > 0").
    Flow("increment", "count").
    Flow("count", "decrement").
    MustSchema()

// Convert to Petri net
model := petri.FromSchema(schema)

// Create engine
engine := petri.NewEngine(model)

// Fire transition
err := engine.Fire("increment", nil)

// Check state
marking := engine.Marking()
```

## Content-Addressed IDs

Schemas have deterministic content IDs for versioning and caching:

```go
schema := dsl.Build("MySchema").
    // ... definition ...
    MustSchema()

cid := schema.CID()  // e.g., "bafyreig..."
```

## Validation

Schemas are validated on construction:

```go
schema, err := dsl.ParseSchema(input)
if err != nil {
    // Parse error or validation error
}

// Manual validation
err := schema.Validate()
```

Validation checks:
- All arc sources/targets exist
- No duplicate state/action IDs
- Guard expressions parse correctly
- Type annotations are valid

## Integration with go-pflow

The metamodel integrates with go-pflow's ODE solver for continuous simulation:

```go
import (
    "github.com/pflow-xyz/go-pflow/metamodel/petri"
    "github.com/pflow-xyz/go-pflow/solver"
)

// Convert metamodel to go-pflow petri net
model := petri.FromSchema(schema)
net := model.ToPflowNet()

// Simulate with ODE solver
prob := solver.NewProblem(net, initialState, timeSpan, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
```

## Origin

This package was extracted from [arcnet](https://github.com/stackdump/arcnet), a blockchain toolkit that uses Petri nets to model token standards (ERC-20, ERC-721, ERC-1155, etc.) as executable state machines.
