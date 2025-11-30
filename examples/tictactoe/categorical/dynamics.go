package categorical

import (
	"fmt"
	"math"
)

// DynamicsType classifies the behavior of a lens over time
type DynamicsType int

const (
	Converged   DynamicsType = iota // Fixed point reached
	Oscillating                     // Limit cycle detected
	Diverging                       // Values increasing without bound
	Chaotic                         // Complex aperiodic behavior
)

// DynamicsAnalysis contains detailed analysis results
type DynamicsAnalysis struct {
	Type            DynamicsType
	Iterations      int
	ConvergenceIter int       // Iteration where convergence detected
	CyclePeriod     int       // Period of cycle if oscillating
	MoveHistory     []Move    // Full move history
	UtilityHistory  []Utility // Full utility history
	FinalUtility    Utility   // Final or average utility
	Variance        float64   // Variance in utility
}

// AnalyzeDynamics runs a lens for many iterations and classifies behavior
func AnalyzeDynamics(lens Lens, initialState GameState, iterations int) *DynamicsAnalysis {
	analysis := &DynamicsAnalysis{
		Iterations:     iterations,
		MoveHistory:    make([]Move, 0, iterations),
		UtilityHistory: make([]Utility, 0, iterations),
	}

	state := initialState
	gradient := Gradient{dUtility: make(map[int]float64)}

	for i := 0; i < iterations; i++ {
		// Forward: play move
		move := lens.Play(state)
		analysis.MoveHistory = append(analysis.MoveHistory, move)

		// Backward: learn utility
		utility := lens.Learn(state, gradient)
		analysis.UtilityHistory = append(analysis.UtilityHistory, utility)

		// Update state
		state = ApplyMove(state, move)

		// Reset if terminal
		if IsTerminal(state) {
			state = initialState
		}

		// Update gradient
		gradient = ComputeGradient(utility)

		// Check for early convergence
		if i >= 20 && analysis.Type == 0 {
			if hasConverged(analysis.UtilityHistory, i) {
				analysis.Type = Converged
				analysis.ConvergenceIter = i
			} else if period := detectCycle(analysis.MoveHistory, i); period > 0 {
				analysis.Type = Oscillating
				analysis.CyclePeriod = period
			}
		}
	}

	// Final classification if not yet determined
	if analysis.Type == 0 {
		if hasConverged(analysis.UtilityHistory, iterations-1) {
			analysis.Type = Converged
			analysis.ConvergenceIter = iterations - 1
		} else if period := detectCycle(analysis.MoveHistory, iterations-1); period > 0 {
			analysis.Type = Oscillating
			analysis.CyclePeriod = period
		} else if isDiverging(analysis.UtilityHistory) {
			analysis.Type = Diverging
		} else {
			analysis.Type = Chaotic
		}
	}

	// Compute statistics
	analysis.FinalUtility = analysis.UtilityHistory[len(analysis.UtilityHistory)-1]
	analysis.Variance = computeVariance(analysis.UtilityHistory)

	return analysis
}

// hasConverged checks if recent utilities have stabilized
func hasConverged(history []Utility, currentIdx int) bool {
	if currentIdx < 10 {
		return false
	}

	// Look at last 10 values
	start := currentIdx - 9
	recent := history[start : currentIdx+1]

	// Compute mean
	mean := 0.0
	for _, u := range recent {
		mean += u.WinProbX
	}
	mean /= float64(len(recent))

	// Compute variance
	variance := 0.0
	for _, u := range recent {
		diff := u.WinProbX - mean
		variance += diff * diff
	}
	variance /= float64(len(recent))

	// Converged if variance is very small
	return variance < 1e-6
}

// detectCycle finds the period of a repeating pattern in moves
func detectCycle(history []Move, currentIdx int) int {
	if currentIdx < 6 {
		return 0
	}

	// Check period-2 (most common: A B A B ...)
	if currentIdx >= 3 {
		if movesEqual(history[currentIdx], history[currentIdx-2]) &&
			movesEqual(history[currentIdx-1], history[currentIdx-3]) {
			return 2
		}
	}

	// Check period-3 (A B C A B C ...)
	if currentIdx >= 5 {
		if movesEqual(history[currentIdx], history[currentIdx-3]) &&
			movesEqual(history[currentIdx-1], history[currentIdx-4]) &&
			movesEqual(history[currentIdx-2], history[currentIdx-5]) {
			return 3
		}
	}

	// Check period-4 (A B C D A B C D ...)
	if currentIdx >= 7 {
		if movesEqual(history[currentIdx], history[currentIdx-4]) &&
			movesEqual(history[currentIdx-1], history[currentIdx-5]) &&
			movesEqual(history[currentIdx-2], history[currentIdx-6]) &&
			movesEqual(history[currentIdx-3], history[currentIdx-7]) {
			return 4
		}
	}

	return 0
}

// movesEqual checks if two moves are the same
func movesEqual(m1, m2 Move) bool {
	return m1.Position == m2.Position && m1.Player == m2.Player
}

// isDiverging checks if utilities are growing without bound
func isDiverging(history []Utility) bool {
	if len(history) < 20 {
		return false
	}

	// Check if trend is consistently increasing
	recent := history[len(history)-20:]

	// Linear regression to detect trend
	n := float64(len(recent))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i, u := range recent {
		x := float64(i)
		y := math.Abs(u.WinProbX) + math.Abs(u.WinProbO)

		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Slope of linear fit
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	// Diverging if slope is large and positive
	return slope > 0.1
}

// computeVariance calculates variance of utility over time
func computeVariance(history []Utility) float64 {
	if len(history) == 0 {
		return 0
	}

	// Compute mean
	mean := 0.0
	for _, u := range history {
		mean += u.WinProbX
	}
	mean /= float64(len(history))

	// Compute variance
	variance := 0.0
	for _, u := range history {
		diff := u.WinProbX - mean
		variance += diff * diff
	}
	variance /= float64(len(history))

	return variance
}

// String returns human-readable description
func (d DynamicsType) String() string {
	switch d {
	case Converged:
		return "CONVERGED"
	case Oscillating:
		return "OSCILLATING"
	case Diverging:
		return "DIVERGING"
	case Chaotic:
		return "CHAOTIC"
	default:
		return "UNKNOWN"
	}
}

// Report generates a detailed analysis report
func (a *DynamicsAnalysis) Report() string {
	report := fmt.Sprintf("=== Dynamics Analysis ===\n")
	report += fmt.Sprintf("Type: %s\n", a.Type)
	report += fmt.Sprintf("Iterations: %d\n", a.Iterations)

	switch a.Type {
	case Converged:
		report += fmt.Sprintf("Converged at iteration: %d\n", a.ConvergenceIter)
		report += fmt.Sprintf("Final utility: X=%.3f, O=%.3f, Draw=%.3f\n",
			a.FinalUtility.WinProbX,
			a.FinalUtility.WinProbO,
			a.FinalUtility.DrawProb)

	case Oscillating:
		report += fmt.Sprintf("Cycle period: %d\n", a.CyclePeriod)
		report += fmt.Sprintf("Average utility: X=%.3f, O=%.3f\n",
			a.FinalUtility.WinProbX,
			a.FinalUtility.WinProbO)

		// Show the cycle
		if a.CyclePeriod > 0 && len(a.MoveHistory) > a.CyclePeriod {
			report += fmt.Sprintf("Cycle pattern: ")
			start := len(a.MoveHistory) - a.CyclePeriod
			for i := 0; i < a.CyclePeriod; i++ {
				report += fmt.Sprintf("%d ", a.MoveHistory[start+i].Position)
			}
			report += "\n"
		}

	case Diverging:
		report += "Values diverging to infinity\n"

	case Chaotic:
		report += "Complex aperiodic behavior detected\n"
	}

	report += fmt.Sprintf("Variance: %.6f\n", a.Variance)

	return report
}

// PlotTrajectory generates ASCII art of utility over time
func (a *DynamicsAnalysis) PlotTrajectory() string {
	if len(a.UtilityHistory) == 0 {
		return "No data to plot\n"
	}

	height := 20
	width := 60

	plot := "Utility Trajectory\n"
	plot += "WinProb X\n"

	// Find min/max
	minVal := 0.0
	maxVal := 1.0

	// Plot
	for row := height; row >= 0; row-- {
		val := minVal + (maxVal-minVal)*float64(row)/float64(height)

		if row == height || row == 0 || row == height/2 {
			plot += fmt.Sprintf("%.2f |", val)
		} else {
			plot += "     |"
		}

		for col := 0; col < width; col++ {
			idx := int(float64(len(a.UtilityHistory)) * float64(col) / float64(width))
			if idx >= len(a.UtilityHistory) {
				idx = len(a.UtilityHistory) - 1
			}

			u := a.UtilityHistory[idx].WinProbX
			plotRow := int((u - minVal) / (maxVal - minVal) * float64(height))

			if plotRow == row {
				plot += "*"
			} else {
				plot += " "
			}
		}
		plot += "\n"
	}

	plot += "     +"
	for i := 0; i < width; i++ {
		plot += "-"
	}
	plot += "\n"
	plot += fmt.Sprintf("     0%sIterations%s%d\n",
		" ", " ", a.Iterations)

	return plot
}

// CompareStates compares two game states
func CompareStates(s1, s2 GameState) bool {
	if s1.Turn != s2.Turn {
		return false
	}

	for i := 0; i < 9; i++ {
		if s1.Board[i] != s2.Board[i] {
			return false
		}
	}

	return true
}
