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
