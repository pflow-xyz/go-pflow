# PFlow Results Schema

This directory defines the JSON schema for simulation results, optimized for AI-assisted analysis and CLI workflows.

## Design Goals

1. **AI-Readable**: Structured data that Claude (and other LLMs) can easily parse and explain
2. **Multi-Resolution**: Both full high-res data and downsampled views for different use cases
3. **Self-Describing**: Results include model, parameters, and automatic analysis
4. **Composable**: Output of one command becomes input to another
5. **Extensible**: Easy to add new analysis types without breaking existing tools

## Schema Structure

```
results.json
├── version: Schema version
├── metadata: Simulation info (solver, status, timing)
├── model: Petri net structure summary
├── simulation: Parameters used (timespan, initial state, rates)
├── results
│   ├── summary: Quick overview (final state, point count)
│   └── timeseries: Time series at multiple resolutions
├── analysis: Automatically computed insights
│   ├── peaks/troughs: Local extrema
│   ├── crossings: Variable intersections
│   ├── steadyState: Equilibrium analysis
│   ├── conservation: Mass balance
│   └── statistics: Min/max/mean/std per variable
└── events: Timeline of notable occurrences (for monitoring)
```

## CLI Workflow

### Basic Simulation

```bash
# Run simulation, output results to JSON
pflow simulate model.json --time 100 --output results.json

# View results summary
pflow summary results.json
# Output:
# Model: SIR Epidemic Model
# Time: 0.0 → 100.0 (1537 points)
# Status: success (23ms)
# Final state: S=59.13, I=1.55, R=939.32
# Steady state reached at t=78.5

# Generate plot
pflow plot results.json --output plot.svg

# Get detailed analysis
pflow analyze results.json
# Output:
# === Analysis: SIR Epidemic Model ===
#
# Peak infection:
#   - I peaks at 287.5 (t=23.4)
#   - Represents maximum disease prevalence
#
# Key transitions:
#   - S crosses I at t=15.2 (value=245.8)
#   - I crosses R at t=31.7 (value=203.4)
#
# Steady state:
#   - Reached at t=78.5
#   - Final: S=59.13, I≈0, R=940.87
#   - 94% of population recovered
#
# Conservation:
#   - Total population constant: 1000
```

### Comparison Workflow

```bash
# Simulate baseline
pflow simulate model.json --time 100 --output baseline.json

# Modify model (double infection rate)
pflow modify model.json --rate infection=0.0006 --output variant.json

# Simulate variant
pflow simulate variant.json --time 100 --output variant_results.json

# Compare
pflow compare baseline.json variant_results.json
# Output:
# === Comparison ===
#
# Peak infection:
#   Baseline: 287.5 at t=23.4
#   Variant:  456.2 at t=18.1
#   Change:   +58.6% earlier, +169 cases
#
# Final susceptible:
#   Baseline: 59.13
#   Variant:  12.45
#   Change:   -78.9% (more people infected)
#
# Recommendation: Doubling infection rate leads to larger,
# earlier epidemic peak. Intervention strategies should
# focus on reducing contact rate.
```

### Monitoring/Engine Workflow

```bash
# Run continuous simulation with monitoring
pflow engine model.json \
  --duration 3600 \
  --rule "queue_warning: Queue > 50" \
  --action "alert_ops" \
  --output monitoring.json

# View events timeline
pflow events monitoring.json
# Output:
# t=234.5  THRESHOLD: Queue exceeded 50 (value=50.2)
# t=235.0  ACTION:    Alert sent to ops@example.com
# t=1834.5 THRESHOLD: Queue exceeded 60 (value=67.8)
# t=1835.0 RATE:      Increased start_processing: 1.5 → 2.0
# t=2456.3 THRESHOLD: Queue returned below 50 (value=49.8)

# Analyze event patterns
pflow analyze monitoring.json --focus events
# Output:
# Event Analysis:
# - 3 threshold violations (2 exceeded, 1 below)
# - 1 rate adjustment (automatic response)
# - Peak queue length: 67.8 at t=1834.5
# - Rate adjustment effective: queue reduced 18 in 621s
```

## AI-Assisted Workflow Examples

### Example 1: Model Explanation

```
User: "What does this simulation show?"

Claude:
→ Reads results.json
→ Parses model, simulation, analysis sections
→ Response:

"This is an SIR epidemic model tracking disease spread through
a population of 1000 individuals. Key findings:

1. **Peak infection** occurs at day 23, with ~288 people infected
   simultaneously

2. **Critical transitions**:
   - Day 15: More infected than susceptible (tipping point)
   - Day 32: More recovered than infected (epidemic declining)

3. **Final outcome**: 94% of population eventually infected,
   with ~59 people never exposed

4. **Parameters**:
   - Infection rate: 0.0003 (moderate contagion)
   - Recovery rate: 0.1 (average ~10 day illness)

The epidemic naturally burns out by day 79 as susceptible
population becomes too small to sustain transmission."
```

### Example 2: Debugging

```
User: "Why is my model unstable?"

Claude:
→ Reads results.json
→ Checks status, conservation, steadyState
→ Response:

"I see several issues in your simulation:

1. **Conservation violated**:
   - Initial tokens: 1000
   - Final tokens: 1247
   - Tokens are being created (non-physical)

2. **No steady state reached**:
   - Simulation ran to t=100 but values still changing rapidly
   - Suggests runaway growth

3. **Looking at your rates**:
   - 'production' rate: 2.0
   - 'consumption' rate: 1.5
   - Production exceeds consumption → unbounded growth

**Fix**: Either:
- Increase consumption rate to ≥2.0, or
- Add capacity limits to prevent unbounded accumulation, or
- Add feedback (production rate decreases as queue fills)"
```

### Example 3: Optimization

```
User: "How can I reduce the peak infection below 200?"

Claude:
→ Reads baseline results
→ Identifies peak: I=287.5
→ Suggests parameter changes
→ Generates modified models
→ Simulates variants
→ Compares results
→ Response:

"I tested several interventions. Here's what works:

**Option 1: Reduce infection rate to 0.0002**
- Peak: 193.4 (33% reduction)
- Final susceptible: 147.2 (more people avoid infection)
- Implementation: Social distancing, masks

**Option 2: Increase recovery rate to 0.15**
- Peak: 221.3 (23% reduction)
- Epidemic duration shorter (ends at t=65)
- Implementation: Better treatment protocols

**Option 3: Combined (rate=0.00025, recovery=0.12)**
- Peak: 178.5 (38% reduction) ✓ Meets target
- Final susceptible: 125.8
- Balanced approach

I recommend Option 3 as it achieves your goal with moderate
changes to both parameters rather than extreme changes to one."
```

## JSON Schema Benefits

### For Humans
- **Self-documenting**: Results explain what was simulated
- **Portable**: Share results without sharing code
- **Archival**: Complete record of simulation parameters and outcomes

### For AI Assistants
- **Parseable**: Structured data, easy to extract information
- **Semantic**: Named sections (analysis.peaks) vs. raw arrays
- **Context-rich**: Model + parameters + results in one file

### For Tools
- **Composable**: Output of `simulate` → input to `plot`, `analyze`, `compare`
- **Extensible**: Add new analysis types without breaking old tools
- **Versionable**: Schema version allows evolution

## Implementation Notes

### Downsampling Strategy

For plotting and transmission efficiency, downsample to ~100-200 points:

```go
// Keep first, last, and evenly spaced interior points
func downsample(data []float64, targetPoints int) []float64 {
    if len(data) <= targetPoints {
        return data
    }

    result := make([]float64, targetPoints)
    result[0] = data[0]
    result[targetPoints-1] = data[len(data)-1]

    step := float64(len(data)-1) / float64(targetPoints-1)
    for i := 1; i < targetPoints-1; i++ {
        idx := int(float64(i) * step)
        result[i] = data[idx]
    }

    return result
}
```

For more sophisticated downsampling (preserving peaks), use Douglas-Peucker algorithm.

### Analysis Computation

#### Peaks Detection
```go
// Simple peak detection: point higher than both neighbors
func findPeaks(data []float64) []int {
    var peaks []int
    for i := 1; i < len(data)-1; i++ {
        if data[i] > data[i-1] && data[i] > data[i+1] {
            peaks = append(peaks, i)
        }
    }
    return peaks
}
```

#### Steady State Detection
```go
// Check if derivative is below threshold for sustained period
func detectSteadyState(t, y []float64, tol, duration float64) (bool, float64) {
    windowSize := int(duration / (t[1] - t[0]))

    for i := windowSize; i < len(y); i++ {
        maxChange := 0.0
        for j := i - windowSize; j < i; j++ {
            change := math.Abs((y[j+1] - y[j]) / (t[j+1] - t[j]))
            maxChange = math.Max(maxChange, change)
        }

        if maxChange < tol {
            return true, t[i]
        }
    }

    return false, 0
}
```

#### Crossings Detection
```go
// Find where two variables intersect
func findCrossings(t, y1, y2 []float64) []Crossing {
    var crossings []Crossing

    for i := 0; i < len(t)-1; i++ {
        // Check if signs differ (crossing occurred)
        diff1 := y1[i] - y2[i]
        diff2 := y1[i+1] - y2[i+1]

        if diff1*diff2 < 0 { // Sign change
            // Linear interpolation to find exact crossing time
            tCross := t[i] + (t[i+1]-t[i])*(-diff1)/(diff2-diff1)
            vCross := y1[i] + (y1[i+1]-y1[i])*(tCross-t[i])/(t[i+1]-t[i])

            crossings = append(crossings, Crossing{
                Time:  tCross,
                Value: vCross,
            })
        }
    }

    return crossings
}
```

## Extensions

### Future Analysis Types

```json
"analysis": {
  "fourier": {
    "dominant_frequency": 0.12,
    "period": 8.33,
    "harmonics": [...]
  },
  "stability": {
    "type": "stable_spiral",
    "eigenvalues": [-0.1, -0.05],
    "damping_ratio": 0.7
  },
  "bifurcation": {
    "parameter": "infection_rate",
    "critical_value": 0.00045,
    "behavior_change": "stable → oscillatory"
  }
}
```

### Custom Analysis Plugins

```bash
# Register custom analysis function
pflow plugin register my_analysis.so

# Use in simulation
pflow simulate model.json --analyze my_analysis --output results.json
```

## See Also

- `results.schema.json` - Full JSON schema definition
- `example_sir.json` - SIR epidemic model results
- `example_monitoring.json` - Monitoring/engine results with events
- `../docs/CLI.md` - CLI command reference
- `../docs/AI_WORKFLOWS.md` - AI-assisted workflow guide
