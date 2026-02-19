package main

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/solver"
)

func benchmarkSudokuODE(b *testing.B, size, blockSize int) {
	net := CreateSudokuNet(size, blockSize, false, true)
	state := net.SetState(nil)
	rates := net.SetRates(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-4
		opts.Reltol = 1e-3
		opts.Dt = 0.2
		_ = solver.Solve(prob, solver.Tsit5(), opts)
	}
}

func BenchmarkSudoku4x4ODE(b *testing.B) {
	benchmarkSudokuODE(b, 4, 2)
}

func BenchmarkSudoku9x9ODE(b *testing.B) {
	benchmarkSudokuODE(b, 9, 3)
}

func BenchmarkSudoku16x16ODE(b *testing.B) {
	benchmarkSudokuODE(b, 16, 4)
}

