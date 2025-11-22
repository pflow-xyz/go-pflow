# Dataset Comparison for Learn Feature

This directory contains tests comparing different real-world and synthetic datasets with the `learn` package parameter fitting capabilities.

## Test Results Summary

### 1. Synthetic SIR Data ✅ **BEST FOR TESTING**

**Dataset:** Artificially generated SIR epidemic with known parameters and 5% Gaussian noise

**Results:**
- ✅ **Loss reduction: 98.66%** (excellent convergence)
- ✅ **Time: 809ms** (fast)
- ✅ **Parameter recovery:**
  - β (infection rate): 18.1% error
  - γ (recovery rate): 10.4% error
  - R0: 31.9% error
- ✅ **Data quality:** Clean signal with controlled noise

**Script:** `synthetic_sir.go`

**Why it's best:**
- Known ground truth for validation
- Clean data with appropriate noise levels
- Strong signal (epidemic grows and decays)
- Perfect for testing, debugging, and demonstrations

---

### 2. Texas Measles Outbreak (2025) ⚠️ **MODERATE**

**Dataset:** JHU CSSE weekly measles case counts for Texas (2025 outbreak)

**Results:**
- ⚠️ **Loss reduction: 99.82%** (appears good but...)
- ⚠️ **Time: 182ms** (very fast, suggests early convergence)
- ❌ **Parameter recovery:**
  - β (infection rate): **NEGATIVE** (-0.050) - physically impossible!
  - γ (recovery rate): 0.343
- ⚠️ **Data quality:** Weekly counts, outbreak in progress

**Script:** `measles_sir.go`

**Issues:**
- Negative infection rate indicates model mismatch
- Weekly case counts are *incident* cases, not *prevalent* cases
- Need to convert incident → prevalent for SIR fitting
- Outbreak still active (no clear decay phase)
- Small case counts (0-84) with weekly granularity

**Potential fixes:**
- Convert weekly incident cases to cumulative infected
- Use SEIR instead of SIR (accounts for reporting delays)
- Wait for complete outbreak curve (currently ongoing)
- Model multiple states simultaneously for more data

---

### 3. New Zealand COVID-19 ❌ **POOR**

**Dataset:** JHU CSSE COVID-19 confirmed cases and deaths for New Zealand

**Results:**
- ❌ **Loss reduction: 2.99%** (essentially no improvement)
- ❌ **Time: 77 seconds** (slow for poor results)
- ❌ **Parameter recovery:** No meaningful change from initial guess
- ❌ **Data quality:** 0-2 cases over 100 weeks

**Script:** `covid_seir.go`

**Issues:**
- Extremely low case counts (0-2) → no signal to fit
- New Zealand had effective COVID-zero strategy early in pandemic
- Cumulative data doesn't show epidemic curve
- Need to select different country/time period

**Better alternatives:**
- Use countries with clear epidemic waves (Italy, USA, Brazil)
- Focus on specific outbreak periods (March-June 2020)
- Use daily instead of weekly sampling for faster-moving outbreaks
- Consider using death data (more reliable reporting)

---

## Recommendations

### For Testing the Learn Feature

**✅ Use synthetic data** (`synthetic_sir.go`) because:
1. Known ground truth enables validation
2. Reproducible and fast
3. Can generate different scenarios (R0 values, noise levels, compartments)
4. Perfect for unit tests and documentation

### For Real-World Validation

**Primary dataset:** JHU Measles Time Series (with fixes)
- **URL:** `https://raw.githubusercontent.com/CSSEGISandData/measles_data/refs/heads/main/Top_states_time_series.csv`
- **Why:** Current, clean format, multiple states, interesting dynamics
- **Required preprocessing:**
  - Convert incident → cumulative cases
  - Model multiple states together
  - Consider SEIR instead of SIR
  - Wait for outbreak completion or use synthetic completion

**Backup dataset:** Historical COVID-19 (select appropriate region/period)
- **URL:** `https://raw.githubusercontent.com/CSSEGISandData/COVID-19/master/csse_covid_19_data/csse_covid_19_time_series/`
- **Recommended regions:**
  - Italy (March-June 2020) - clear first wave
  - New York State (March-May 2020) - strong signal
  - Wuhan, China (Jan-Mar 2020) - complete outbreak
- **Preprocessing:**
  - Use daily confirmed cases (not cumulative)
  - Compute daily new cases via differencing
  - Apply smoothing (7-day rolling average)
  - Extract single clear wave period

**Alternative:** Project Tycho Historical Measles (1888-2002)
- **URL:** https://www.tycho.pitt.edu/dataset/US.14189004/
- **Why:** Huge dataset, complete outbreaks, pre-vaccination era
- **Note:** Requires free account to download

---

## Dataset Quality Checklist

When evaluating datasets for the learn feature, check:

✅ **Signal strength:** Clear epidemic curve (growth + decay)
✅ **Data type:** Incident cases (new per day/week) or prevalent (active)
✅ **Completeness:** Full outbreak cycle (not ongoing)
✅ **Granularity:** Sufficient time points (≥20) with appropriate sampling
✅ **Magnitude:** Enough cases for signal vs noise (≥10 peak cases)
✅ **Preprocessing:** Understand data meaning (cumulative vs incident)
✅ **Ground truth:** Known or estimated parameter ranges for validation

---

## Usage

```bash
# Run from the examples/dataset_comparison directory
cd examples/dataset_comparison

# Run all tests
go run cmd/synthetic_sir/main.go       # Best for testing
go run cmd/measles_sir/main.go         # Real data (needs fixes)
go run cmd/measles_sir_fixed/main.go   # Fixed version
go run cmd/covid_seir/main.go          # Poor fit example

# View generated plots
open synthetic_sir_fit.svg
open measles_sir_fit.svg
open covid_seir_fit.svg
```

---

## Key Insights

### What Works Well
- ✅ Synthetic data with known parameters
- ✅ Complete outbreak curves (growth + decay)
- ✅ Strong signal-to-noise ratio
- ✅ Appropriate model complexity (SIR for simple, SEIR for latency)

### Common Pitfalls
- ❌ Using cumulative instead of incident cases
- ❌ Incomplete outbreak curves (ongoing epidemics)
- ❌ Very low case counts (≤5 peak)
- ❌ Model-data mismatch (incident vs prevalent compartments)
- ❌ Wrong time periods (before outbreak starts)

### Best Practices
1. **Start with synthetic data** to validate the fitting pipeline
2. **Preprocess real data carefully** (incident vs cumulative, smoothing)
3. **Select appropriate time windows** (single complete wave)
4. **Match model to data** (what does each compartment represent?)
5. **Validate parameter ranges** (check for physical plausibility)
6. **Compare multiple datasets** to understand robustness

---

## Future Work

Potential improvements for real-world dataset fitting:

1. **Preprocessing utilities:**
   - Convert cumulative → incident cases
   - Convert incident → prevalent (active) cases
   - Apply smoothing (moving averages)
   - Outlier detection and removal

2. **Model extensions:**
   - Time-varying parameters (lockdowns, interventions)
   - Multi-region coupling (spatial spread)
   - Age-structured models
   - Reporting delays and underreporting

3. **Additional datasets:**
   - Influenza (seasonal patterns)
   - Ebola outbreaks (complete cycles)
   - Synthetic benchmarks suite
   - Chemical kinetics reactions

4. **Validation tools:**
   - Parameter confidence intervals
   - Prediction intervals for trajectories
   - Residual analysis
   - Cross-validation (train/test splits)
