# tokenmodel

A Go submodule providing abstract building blocks for formal token models. It generalizes Petri nets to support both discrete token counting and structured data state, making it suitable for modeling ERC token standards and smart contract state machines.

## Installation

```bash
go get github.com/pflow-xyz/go-pflow/tokenmodel
```

## Core Concepts

### Schema

A `Schema` defines the structure and behavior of a formal model:

```go
schema := tokenmodel.NewSchema("erc20")
schema.AddDataState("balances", "map[address]uint256", nil, true)
schema.AddAction(tokenmodel.Action{ID: "transfer", Guard: "balances[from] >= amount"})
schema.AddArc(tokenmodel.Arc{Source: "balances", Target: "transfer", Keys: []string{"from"}, Value: "amount"})
schema.AddArc(tokenmodel.Arc{Source: "transfer", Target: "balances", Keys: []string{"to"}, Value: "amount"})
```

### State Types

- **TokenState**: Integer counters (classic Petri net semantics)
- **DataState**: Structured data like maps and records

### Runtime

Execute actions against a schema:

```go
runtime := tokenmodel.NewRuntime(schema)
err := runtime.ExecuteWithBindings("transfer", tokenmodel.Bindings{
    "from":   "0xAlice",
    "to":     "0xBob",
    "amount": int64(100),
})
```

## Subpackages

- `guard/` - Guard expression parser and evaluator
- `petri/` - Petri net model and analysis
- `dsl/` - Domain-specific language for schema definition

## Features

- Content-addressed identifiers (CID) for schemas
- Guard expression evaluation with custom functions
- Constraint checking after action execution
- Reachability analysis
- Snapshot cloning for state exploration

## Example

```go
package main

import (
    "fmt"
    "github.com/pflow-xyz/go-pflow/tokenmodel"
)

func main() {
    // Create a simple counter schema
    s := tokenmodel.NewSchema("counter")
    s.AddTokenState("count", 0)
    s.AddAction(tokenmodel.Action{ID: "increment"})
    s.AddArc(tokenmodel.Arc{Source: "increment", Target: "count"})

    // Create runtime and execute
    r := tokenmodel.NewRuntime(s)
    r.Execute("increment")
    r.Execute("increment")

    fmt.Printf("Count: %d\n", r.Tokens("count")) // Output: Count: 2
}
```

## License

MIT
