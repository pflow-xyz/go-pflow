# go-pflow

A Go library for Petri net modeling, ODE simulation, process mining, and zero-knowledge proofs.

> go-pflow is **not** an AI/ML library. It implements structural, dynamical computation based on Petri nets and differential equations. See [Why Petri Nets?](https://book.pflow.xyz/ch01-why-petri-nets.html) for the motivation.

For comprehensive documentation, see **[the book](https://book.pflow.xyz)**.

## Installation

```bash
go get github.com/pflow-xyz/go-pflow
```

## Quick Start

```go
// Build a Petri net and simulate with ODE solver
net, rates := petri.Build().
    Place("S", 999).Place("I", 1).Place("R", 0).
    Transition("infect").Transition("recover").
    Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
    Arc("I", "recover", 1).Arc("recover", "R", 1).
    WithCustomRates(map[string]float64{"infect": 0.3, "recover": 0.1})

prob := solver.NewProblem(net, net.SetState(nil), [2]float64{0, 100}, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
fmt.Println("Final state:", sol.GetFinalState())
```

See [The go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) for the full API guide.

## Packages

| Package | Purpose | Book Chapter |
|---------|---------|--------------|
| `petri` | Core types, fluent Builder API | [Ch 1: Why Petri Nets?](https://book.pflow.xyz/ch01-why-petri-nets.html) |
| `solver` | ODE solvers (Tsit5, RK45, implicit) | [Ch 3: Discrete to Continuous](https://book.pflow.xyz/ch03-discrete-to-continuous.html) |
| `stateutil` | State map utilities | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `hypothesis` | Move evaluation for game AI | [Ch 6: Game Mechanics](https://book.pflow.xyz/ch06-game-mechanics.html) |
| `sensitivity` | Parameter sensitivity analysis | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `cache` | Simulation memoization | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `reachability` | Deadlock/liveness analysis | [Ch 2: Mathematics of Flow](https://book.pflow.xyz/ch02-mathematics-of-flow.html) |
| `workflow` | Task dependencies, SLA tracking | [Ch 10: Complex State Machines](https://book.pflow.xyz/ch10-complex-state-machines.html) |
| `statemachine` | Hierarchical state machines | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `actor` | Message-passing actor model | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `eventlog` | Event log parsing | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) |
| `mining` | Process discovery, rate learning | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) |
| `monitoring` | Real-time prediction, SLA alerts | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) |
| `learn` | Neural ODE-ish parameter fitting | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `tokenmodel` | Token model schemas, DSL | [Ch 4: Token Language](https://book.pflow.xyz/ch04-token-language.html) |
| `codegen/solidity` | Solidity smart contract generation | [Ch 17: Code Generation](https://book.pflow.xyz/ch16-code-generation.html) |
| `prover` | Groth16 ZK proofs with gnark | [Ch 12: Zero-Knowledge Proofs](https://book.pflow.xyz/ch12-zero-knowledge-proofs.html) |
| `visualization` | SVG rendering | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |
| `plotter` | Time series SVG plots | [Ch 18: go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html) |

## Examples

Each example maps to a book chapter demonstrating the modeling pattern:

| Example | Domain | Book Chapter | Run |
|---------|--------|--------------|-----|
| [basic](examples/basic/) | Token flow fundamentals | [Ch 1](https://book.pflow.xyz/ch01-why-petri-nets.html) | `cd examples/basic && go run main.go` |
| [coffeeshop](examples/coffeeshop/) | Resource modeling | [Ch 5](https://book.pflow.xyz/ch05-resource-modeling.html) | `cd examples/coffeeshop/cmd && go run main.go` |
| [tictactoe](examples/tictactoe/) | Game AI, move evaluation | [Ch 6](https://book.pflow.xyz/ch06-game-mechanics.html) | `cd examples/tictactoe && go run ./cmd` |
| [sudoku](examples/sudoku/) | Constraint satisfaction | [Ch 7](https://book.pflow.xyz/ch07-constraint-satisfaction.html) | `cd examples/sudoku/cmd && go run *.go` |
| [knapsack](examples/knapsack/) | Combinatorial optimization | [Ch 8](https://book.pflow.xyz/ch08-optimization.html) | `cd examples/knapsack/cmd && go run *.go` |
| [poker](examples/poker/) | Complex state machines | [Ch 10](https://book.pflow.xyz/ch10-complex-state-machines.html) | `cd examples/poker && go run ./cmd` |
| [monitoring_demo](examples/monitoring_demo/) | Process mining, SLA prediction | [Ch 11](https://book.pflow.xyz/ch11-process-mining.html) | `cd examples/monitoring_demo && go run main.go` |
| [erc](examples/erc/) | Token standards, Solidity codegen | [Ch 4](https://book.pflow.xyz/ch04-token-language.html) | `go run ./examples/erc` |
| [connect4](examples/connect4/) | Pattern recognition | [Ch 6](https://book.pflow.xyz/ch06-game-mechanics.html) | `cd examples/connect4 && go run ./cmd` |
| [nim](examples/nim/) | Optimal strategy | [Ch 6](https://book.pflow.xyz/ch06-game-mechanics.html) | `cd examples/nim && go run ./cmd` |
| [chess](examples/chess/) | N-Queens, Knight's Tour | [Ch 7](https://book.pflow.xyz/ch07-constraint-satisfaction.html) | `cd examples/chess/cmd && go run *.go` |
| [neural](examples/neural/) | Parameter learning | [Ch 3](https://book.pflow.xyz/ch03-discrete-to-continuous.html) | `cd examples/neural && go run main.go` |
| [visualization_demo](examples/visualization_demo/) | SVG generation | [Ch 16](https://book.pflow.xyz/ch15-visual-editor.html) | `make run-visualization` |

See [examples/README.md](examples/README.md) for more details.

## The Book

**[book.pflow.xyz](https://book.pflow.xyz)** covers everything from foundations to advanced topics:

**Part I: Foundations** — [Why Petri Nets](https://book.pflow.xyz/ch01-why-petri-nets.html), [Mathematics of Flow](https://book.pflow.xyz/ch02-mathematics-of-flow.html), [Discrete to Continuous](https://book.pflow.xyz/ch03-discrete-to-continuous.html), [Token Language](https://book.pflow.xyz/ch04-token-language.html)

**Part II: Applications** — [Resource Modeling](https://book.pflow.xyz/ch05-resource-modeling.html), [Game Mechanics](https://book.pflow.xyz/ch06-game-mechanics.html), [Constraint Satisfaction](https://book.pflow.xyz/ch07-constraint-satisfaction.html), [Optimization](https://book.pflow.xyz/ch08-optimization.html), [Enzyme Kinetics](https://book.pflow.xyz/ch09-enzyme-kinetics.html), [Complex State Machines](https://book.pflow.xyz/ch10-complex-state-machines.html)

**Part III: Advanced** — [Process Mining](https://book.pflow.xyz/ch11-process-mining.html), [Zero-Knowledge Proofs](https://book.pflow.xyz/ch12-zero-knowledge-proofs.html), [Topology-Driven Verification](https://book.pflow.xyz/ch13-topology-driven-verification.html), [Declarative Infrastructure](https://book.pflow.xyz/ch14-declarative-infrastructure.html)

**Part IV: Building** — [Visual Editor](https://book.pflow.xyz/ch15-visual-editor.html), [Code Generation](https://book.pflow.xyz/ch16-code-generation.html), [go-pflow Library](https://book.pflow.xyz/ch17-go-pflow-library.html), [Dual Implementation](https://book.pflow.xyz/ch18-dual-implementation.html)

## Testing

```bash
go test ./...
```

## CLI

The `pflow` CLI provides simulation, analysis, and plotting from the command line. See [cmd/pflow/README.md](cmd/pflow/README.md).

## Compatibility

- Go 1.23+
- JSON format compatible with [pflow.xyz](https://pflow.xyz)

## License

MIT License - see [LICENSE](LICENSE) for details.

## Related

- [book.pflow.xyz](https://book.pflow.xyz) — Technical book
- [pflow.xyz](https://pflow.xyz) — Visual editor and JavaScript implementation
- [RESEARCH_PAPER_OUTLINE.md](RESEARCH_PAPER_OUTLINE.md) — Research paper draft
