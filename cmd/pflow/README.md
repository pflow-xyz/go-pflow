# pflow CLI

AI-native command-line tool for Petri net modeling and simulation.

## Installation

```bash
go build -o pflow ./cmd/pflow
# Or install to GOPATH/bin:
go install ./cmd/pflow
```

## Quick Start

```bash
# Run simulation
pflow simulate --time 100 --rates "infection=0.0003,recovery=0.1" --output results.json model.json

# View summary
pflow summary results.json

# Full analysis
pflow analyze results.json

# Generate plot
pflow plot --output plot.svg results.json

# Compare variants
pflow compare baseline.json variant.json
```

## Commands

### `simulate` - Run ODE simulation

Simulates a Petri net model using adaptive ODE integration.

```bash
pflow simulate [flags] <model.json>

Flags:
  --time float        End time (default: 100.0)
  --start float       Start time (default: 0.0)
  --output string     Output file (required)
  --rates string      Override rates (format: "t1=0.5,t2=0.3")
  --initial string    Override initial state (format: "p1=100,p2=50")
  --name string       Model name (inferred from filename if not provided)
  --analyze           Compute automatic analysis (default: true)
  --downsample int    Downsampling target points (default: 150)
```

**Output**: JSON file with simulation results, analysis, and metadata

**Examples**:
```bash
# Basic simulation
pflow simulate --output results.json model.json

# Custom parameters
pflow simulate --time 200 --rates "arrive=2.0,process=1.5" --output results.json queue.json

# Skip analysis for speed
pflow simulate --analyze=false --output results.json large_model.json
```

### `analyze` - Compute insights

Displays human-readable analysis of simulation results.

```bash
pflow analyze [flags] <results.json>

Flags:
  --recompute    Recompute analysis even if present
  --save string  Save updated results to file
```

**Output**: Formatted analysis including:
- Peaks and troughs
- Variable crossings
- Steady state detection
- Conservation checking
- Statistical summary
- Final state

**Examples**:
```bash
# Show analysis
pflow analyze results.json

# Recompute and save
pflow analyze --recompute --save updated.json results.json
```

### `plot` - Generate visualization

Creates SVG plot from simulation results.

```bash
pflow plot [flags] <results.json>

Flags:
  --output string   SVG output file (required)
  --width int       Width in pixels (default: 800)
  --height int      Height in pixels (default: 600)
  --title string    Plot title (default: model name)
  --xlabel string   X-axis label (default: "Time")
  --ylabel string   Y-axis label (default: "Value")
  --vars string     Variables to plot, comma-separated (default: all)
```

**Output**: SVG file with downsampled data (typically 85% smaller than full resolution)

**Examples**:
```bash
# Basic plot
pflow plot --output plot.svg results.json

# Custom size and labels
pflow plot --output plot.svg --width 1200 --height 800 --title "SIR Model" results.json

# Plot subset of variables
pflow plot --output plot.svg --vars "S,I,R" results.json
```

### `summary` - Quick overview

Shows brief summary of simulation results.

```bash
pflow summary <results.json>
```

**Output**: Model name, status, timespan, final state, steady state status

**Example**:
```bash
pflow summary results.json
```

### `compare` - Compare simulations

Compares two simulation results and highlights differences.

```bash
pflow compare <baseline.json> <variant.json>
```

**Output**: Side-by-side comparison of:
- Peak values and timing
- Steady state differences
- Conservation properties
- Final states
- Parameter changes

**Example**:
```bash
# Compare baseline vs variant
pflow compare baseline.json variant.json
```

### `events` - Show event timeline

Displays chronological list of events from monitored simulation.

```bash
pflow events [flags] <results.json>

Flags:
  --type string  Filter by event type
```

**Output**: Timeline with timestamps, types, and descriptions

**Examples**:
```bash
# Show all events
pflow events monitoring.json

# Filter by type
pflow events --type threshold_exceeded monitoring.json
```

### `expand` - Unfold a colored net

Turns a multi-color (colored) Petri net into an equivalent single-color net.
Each place becomes one place per color (`pool.red`, `pool.blue`), each arc
becomes one arc per non-zero weight component, and transitions are shared so a
firing still moves every color atomically.

```bash
pflow expand [flags] <model.json>

Flags:
  --summary        Print the color mapping instead of the unfolded model
  --output string  Write the unfolded model to a file instead of stdout
```

`validate`, `verify` and `simulate` already unfold internally — this command is
for *seeing* the unfolding, or for handing the expanded net to a tool with no
color support of its own. A single-color model passes through unchanged.

**Examples**:
```bash
# Inspect a model's color structure
pflow expand model.json --summary

# Write the unfolded model out
pflow expand model.json --output unfolded.json
```

In `verify`, you don't need to expand first: an expanded name pins one color
and a base name means the total across colors.

```bash
pflow verify model.json -p "pool.red == 3"   # exactly 3 red
pflow verify model.json -p "pool == 3"       # 3 tokens of any color
```

### `create` - Create model from template

Generates a Petri net model from a built-in template instead of hand-writing
JSON.

```bash
pflow create [flags]

Flags:
  --template string  Template name (required)
  --output string    Output file (required)
  --params string    Template parameters (format: "key=value,key2=value2")
  --list             List available templates
  --show string      Show a template's parameters
```

**Templates**: `sir`, `seir`, `queue`, `producer-consumer`, `workflow`

**Examples**:
```bash
# List templates
pflow create --list

# Show a template's parameters
pflow create --show sir

# Create an SIR model with custom parameters
pflow create --template sir --params "population=5000,infection_rate=0.0005" --output sir.json

# Create a queue with 3 servers
pflow create --template queue --params "servers=3,queue_capacity=50" --output queue.json
```

### `validate` - Validate model structure

Checks structural integrity (negative tokens, invalid weights), connectivity,
deadlocks, unbounded places and token conservation. `--reachability`
additionally explores the full discrete state space.

```bash
pflow validate <model.json> [flags]

Flags:
  --reachability    Perform reachability analysis (explores state space)
  --max-states int  Maximum states to explore for reachability (default: 10000)
  --json            Output results as JSON
  --output string   Write JSON results to file
```

**Output**: Structural warnings and info; with `--reachability`, also states
explored, boundedness, deadlock states and maximum tokens per place.

**Examples**:
```bash
# Basic validation
pflow validate model.json

# With reachability analysis
pflow validate model.json --reachability

# Limit state exploration and save a JSON report
pflow validate model.json --reachability --max-states 5000 --json --output validation.json
```

### `verify` - Check declarative properties

Checks declarative properties against a model's reachable state space. Each
property returns **proved**, **refuted** or **unknown** — a refutation carries
a replayable firing sequence. The process exits non-zero if any property is
refuted.

```bash
pflow verify <model.json> -p <property> [-p <property> ...] [flags]

Flags:
  --json    Output results as JSON

Property forms:
  deadlock-free              no reachable marking is a deadlock
  bounded                    no place accumulates tokens without limit
  live                       every transition can fire from some marking
  terminating                every execution eventually stops
  conserves                  total token count never changes
  reachable:<marking>        some reachable marking matches, e.g. reachable:done=1
  unreachable:<marking>      no reachable marking matches (safety)
  mutex:<p1,p2,...>[<=n]     at most n of these places hold a token at once
  <linear expression>        holds at every reachable marking, e.g. "a + 2*b == 10"
```

A verdict's `method` says how far it generalizes: `structural` (proved by
linear algebra, for any initial marking), `exhaustive` (the marking's full
state space was enumerated), `witness` (a constructive witness, e.g. an
unbounded pump), or `partial` (exploration was truncated — only refutations
found under a partial search are sound).

**Examples**:
```bash
pflow verify model.json -p deadlock-free -p bounded
pflow verify model.json -p "mutex:busy1,busy2"
pflow verify model.json -p "minted == circulating + burned"
pflow verify model.json -p unreachable:busy1=1,busy2=1 --json
```

### `sweep` - Parameter sweep and optimization

Runs a model across a grid of rate or initial-state values and ranks the
variants against an optimization objective.

```bash
pflow sweep <model.json> [flags]

Flags:
  --rates string       Sweep rates: "name=min:max:count,..."
  --initial string     Sweep initial state: "name=min:max:count,..."
  --objective string   Optimization objective (default: "minimize_peak")
  --time float         End time for simulation (default: 100)
  --parallel int       Number of parallel simulations (default: 4)
  --output string      Output file for sweep results (default: "sweep_results.json")
  --save-variants      Save individual variant results
  --variant-dir string Directory for variant results (default: "variants")

Objectives:
  minimize_peak            Minimize maximum peak across all variables
  maximize_peak            Maximize peak (useful for throughput)
  minimize_final           Minimize sum of final state
  maximize_throughput      Maximize "Completed" or "Output" place
  minimize_time_to_steady  Minimize time to reach steady state
```

**Output**: Ranked variants (best/worst configuration, score, peak) and, with
`--save-variants`, each variant's full simulation result under `--variant-dir`.

**Examples**:
```bash
# Sweep a single rate
pflow sweep model.json --rates "infection=0.0001:0.001:10" --output sweep.json

# Sweep multiple parameters
pflow sweep model.json --rates "arrive=1:5:5,process=0.5:2:4" --output sweep.json

# Sweep initial state
pflow sweep model.json --initial "Queue=0:100:11" --output sweep.json

# Custom objective, saving every variant
pflow sweep model.json --rates "r=0.1:0.5:5" --objective minimize_time_to_steady --save-variants --variant-dir variants/
```

### `visualize` - Render Petri net structure

Renders the net's places, transitions and arcs as an SVG using the pflow-xyz
layout, independent of any simulation run.

```bash
pflow visualize <model.json> [flags]

Flags:
  --output string  Output SVG file (required)
```

**Output**: SVG file of the net structure (not a plot of simulation results —
see `plot` for that).

**Examples**:
```bash
# Visualize model structure
pflow visualize model.json --output model.svg

# Visualize from JSON-LD
pflow visualize model.jsonld --output model.svg
```

## AI-Assisted Workflows

The CLI is designed to work seamlessly with AI assistants like Claude:

### Example 1: Claude generates and simulates model

```
User: "Create an SIR model with population 1000, infection rate 0.0003, recovery rate 0.1"

Claude:
1. Generates model JSON
2. Runs: pflow simulate --rates "infection=0.0003,recovery=0.1" --output results.json model.json
3. Reads results.json
4. Explains: "Peak infection occurs at day 27 with ~304 cases..."
```

### Example 2: Claude debugs unstable model

```
User: "Why is my model unstable?"

Claude:
1. Runs: pflow analyze results.json
2. Reads analysis
3. Identifies: "Conservation violated - tokens being created"
4. Suggests fix: "Increase consumption rate or add capacity limits"
```

### Example 3: Claude optimizes parameters

```
User: "How can I reduce peak infection below 200?"

Claude:
1. Generates variants with different parameters
2. Runs simulations in parallel
3. Runs: pflow compare baseline.json variant1.json
4. Recommends best option with explanation
```

## JSON Schema

All simulation results use a structured JSON format optimized for AI consumption:

```json
{
  "version": "1.0.0",
  "metadata": { "solver": "tsit5", "status": "success", ... },
  "model": { "places": [...], "transitions": [...], ... },
  "simulation": { "timespan": [0, 100], "rates": {...}, ... },
  "results": {
    "summary": { "finalState": {...}, ... },
    "timeseries": {
      "time": { "downsampled": [...], "full": [...] },
      "variables": { "S": {...}, "I": {...}, ... }
    }
  },
  "analysis": {
    "peaks": [...],
    "crossings": [...],
    "steadyState": {...},
    "conservation": {...},
    "statistics": {...}
  }
}
```

See `schema/README.md` for full documentation.

## Performance

- **Downsampling**: SVG plots are 85% smaller (16KB vs 107KB)
- **Fast execution**: Typical SIR simulation in ~8ms
- **Efficient format**: JSON output with both full and downsampled data

## Tips

1. **Flags before or after**: Both work, but flags-first is more reliable:
   ```bash
   pflow simulate --output results.json model.json  # ✓
   pflow simulate model.json --output results.json  # ✓ but flags must come last
   ```

2. **Pipe-friendly**: Status messages go to stderr, data to stdout:
   ```bash
   pflow simulate --output /dev/stdout model.json 2>/dev/null | jq .results.summary
   ```

3. **Reuse results**: Analysis is cached in JSON:
   ```bash
   pflow simulate --output results.json model.json
   pflow plot --output plot1.svg results.json
   pflow plot --output plot2.svg --vars "S,I" results.json  # No re-simulation
   ```

4. **Compose commands**: Results from one command feed into another:
   ```bash
   pflow simulate --output results.json model.json
   pflow analyze results.json > analysis.txt
   pflow plot --output plot.svg results.json
   ```

## Examples

See `examples/` directory for sample models:
- `basic/sir_model.json` - SIR epidemic model
- `tictactoe/` - Game AI using ODEs

## Related

- [Schema Documentation](../../schema/README.md) - JSON format specification
- [Main README](../../README.md) - Library documentation
- [Examples](../../examples/) - Sample models and use cases
