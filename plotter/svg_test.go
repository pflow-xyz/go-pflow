package plotter

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

func TestNewSVGPlotter(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)

	if plotter.Width != 800 {
		t.Errorf("Expected width 800, got %f", plotter.Width)
	}
	if plotter.Height != 600 {
		t.Errorf("Expected height 600, got %f", plotter.Height)
	}
	if plotter.XLabel != "Time" {
		t.Errorf("Expected default XLabel 'Time', got '%s'", plotter.XLabel)
	}
	if plotter.YLabel != "Value" {
		t.Errorf("Expected default YLabel 'Value', got '%s'", plotter.YLabel)
	}
	if plotter.Series != nil {
		t.Error("Expected Series to be nil initially")
	}
}

func TestSetTitle(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.SetTitle("Test Plot")

	if plotter.Title != "Test Plot" {
		t.Errorf("Expected title 'Test Plot', got '%s'", plotter.Title)
	}

	// Test chaining
	result := plotter.SetTitle("Another Title")
	if result != plotter {
		t.Error("SetTitle should return the plotter for chaining")
	}
}

func TestSetLabels(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.SetXLabel("X Axis").SetYLabel("Y Axis")

	if plotter.XLabel != "X Axis" {
		t.Errorf("Expected XLabel 'X Axis', got '%s'", plotter.XLabel)
	}
	if plotter.YLabel != "Y Axis" {
		t.Errorf("Expected YLabel 'Y Axis', got '%s'", plotter.YLabel)
	}
}

func TestAddSeries(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	x := []float64{0, 1, 2, 3}
	y := []float64{0, 1, 4, 9}

	plotter.AddSeries(x, y, "Data", "#ff0000")

	if len(plotter.Series) != 1 {
		t.Fatalf("Expected 1 series, got %d", len(plotter.Series))
	}

	series := plotter.Series[0]
	if series.Label != "Data" {
		t.Errorf("Expected label 'Data', got '%s'", series.Label)
	}
	if series.Color != "#ff0000" {
		t.Errorf("Expected color '#ff0000', got '%s'", series.Color)
	}
	if len(series.X) != 4 || len(series.Y) != 4 {
		t.Errorf("Expected 4 data points, got X=%d, Y=%d", len(series.X), len(series.Y))
	}
}

func TestAddSeriesDefaultColor(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.AddSeries([]float64{0, 1}, []float64{0, 1}, "Series1", "")
	plotter.AddSeries([]float64{0, 1}, []float64{0, 2}, "Series2", "")

	// Should use default color palette
	if plotter.Series[0].Color == "" {
		t.Error("First series should have a default color")
	}
	if plotter.Series[1].Color == "" {
		t.Error("Second series should have a default color")
	}
	if plotter.Series[0].Color == plotter.Series[1].Color {
		t.Error("Different series should have different default colors")
	}
}

func TestRenderBasic(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.SetTitle("Test Plot")
	plotter.AddSeries([]float64{0, 1, 2}, []float64{0, 1, 4}, "y=x²", "#0000ff")

	svg := plotter.Render()

	// Check that it produces valid SVG
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("SVG should start with <svg tag")
	}
	if !strings.HasSuffix(svg, "</svg>") {
		t.Error("SVG should end with </svg> tag")
	}

	// Check for key elements
	if !strings.Contains(svg, "Test Plot") {
		t.Error("SVG should contain the title")
	}
	if !strings.Contains(svg, "y=x²") {
		t.Error("SVG should contain the series label")
	}
	if !strings.Contains(svg, "#0000ff") {
		t.Error("SVG should contain the series color")
	}
	if !strings.Contains(svg, "<path") {
		t.Error("SVG should contain a path element for the data")
	}
}

func TestRenderEmptySeries(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	svg := plotter.Render()

	// Should produce valid SVG even with no data
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("Empty plot should still produce valid SVG")
	}
}

func TestRenderStoresPlotData(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.AddSeries([]float64{0, 1, 2}, []float64{0, 1, 4}, "data", "")

	if plotter.LastPlot != nil {
		t.Error("LastPlot should be nil before rendering")
	}

	plotter.Render()

	if plotter.LastPlot == nil {
		t.Fatal("LastPlot should be set after rendering")
	}

	// Check metadata
	if plotter.LastPlot.PlotID == "" {
		t.Error("PlotID should be set")
	}
	if len(plotter.LastPlot.Series) != 1 {
		t.Errorf("Expected 1 series in LastPlot, got %d", len(plotter.LastPlot.Series))
	}
	if plotter.LastPlot.PlotWidth <= 0 || plotter.LastPlot.PlotHeight <= 0 {
		t.Error("Plot dimensions should be positive")
	}
}

func TestRenderWithHTMLEscaping(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.SetTitle("<script>alert('xss')</script>")
	plotter.AddSeries([]float64{0, 1}, []float64{0, 1}, "<tag>", "")

	svg := plotter.Render()

	// Check that HTML is escaped
	if strings.Contains(svg, "<script>") {
		t.Error("HTML in title should be escaped")
	}
	if !strings.Contains(svg, "&lt;") {
		t.Error("< should be escaped to &lt;")
	}
	if !strings.Contains(svg, "&gt;") {
		t.Error("> should be escaped to &gt;")
	}
}

func TestPlotSolution(t *testing.T) {
	// Create a simple solution
	sol := &solver.Solution{
		T: []float64{0, 1, 2, 3, 4},
		U: []map[string]float64{
			{"A": 10.0, "B": 0.0},
			{"A": 7.5, "B": 2.5},
			{"A": 5.0, "B": 5.0},
			{"A": 2.5, "B": 7.5},
			{"A": 0.0, "B": 10.0},
		},
		StateLabels: []string{"A", "B"},
	}

	svg, plotData := PlotSolution(sol, nil, 800, 600, "Test Solution", "Time (s)", "Concentration")

	// Check SVG
	if !strings.Contains(svg, "Test Solution") {
		t.Error("Plot should contain the title")
	}
	if !strings.Contains(svg, "Time (s)") {
		t.Error("Plot should contain X label")
	}
	if !strings.Contains(svg, "Concentration") {
		t.Error("Plot should contain Y label")
	}
	if !strings.Contains(svg, "A") {
		t.Error("Plot should contain series A")
	}
	if !strings.Contains(svg, "B") {
		t.Error("Plot should contain series B")
	}

	// Check plot data
	if plotData == nil {
		t.Fatal("PlotData should not be nil")
	}
	if len(plotData.Series) != 2 {
		t.Errorf("Expected 2 series, got %d", len(plotData.Series))
	}
}

func TestPlotSolutionSelectedVariables(t *testing.T) {
	sol := &solver.Solution{
		T: []float64{0, 1, 2},
		U: []map[string]float64{
			{"A": 1.0, "B": 2.0, "C": 3.0},
			{"A": 1.5, "B": 2.5, "C": 3.5},
			{"A": 2.0, "B": 3.0, "C": 4.0},
		},
		StateLabels: []string{"A", "B", "C"},
	}

	// Plot only A and C
	_, plotData := PlotSolution(sol, []string{"A", "C"}, 800, 600, "", "", "")

	if plotData == nil {
		t.Fatal("PlotData should not be nil")
	}
	if len(plotData.Series) != 2 {
		t.Errorf("Expected 2 series (A and C), got %d", len(plotData.Series))
	}

	// Check that B is not included
	for _, series := range plotData.Series {
		if series.Label == "B" {
			t.Error("Series B should not be included")
		}
	}
}

func TestRenderWithCrosshair(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.AddSeries([]float64{0, 1}, []float64{0, 1}, "data", "")
	svg := plotter.Render()

	// Check for crosshair elements
	if !strings.Contains(svg, "_crosshair") {
		t.Error("SVG should contain crosshair group")
	}
	if !strings.Contains(svg, "_overlay") {
		t.Error("SVG should contain overlay for interactivity")
	}
}

func TestRenderWithLegend(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	plotter.AddSeries([]float64{0, 1}, []float64{0, 1}, "Series 1", "#ff0000")
	plotter.AddSeries([]float64{0, 1}, []float64{0, 2}, "Series 2", "#00ff00")
	svg := plotter.Render()

	// Check that both series appear in legend
	if !strings.Contains(svg, "Series 1") {
		t.Error("Legend should contain Series 1")
	}
	if !strings.Contains(svg, "Series 2") {
		t.Error("Legend should contain Series 2")
	}
}

func TestRenderWithoutLegend(t *testing.T) {
	plotter := NewSVGPlotter(800, 600)
	// Add series without labels
	plotter.AddSeries([]float64{0, 1}, []float64{0, 1}, "", "#ff0000")
	svg := plotter.Render()

	// Should still render, just without legend entries
	if !strings.Contains(svg, "<svg") {
		t.Error("Should produce valid SVG even without labels")
	}
}

func TestIntegrationWithPetriNet(t *testing.T) {
	// Full integration test: create net, solve, plot
	net := petri.NewPetriNet()
	net.AddPlace("A", 10.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	initialState := map[string]float64{"A": 10.0, "B": 0.0}
	rates := map[string]float64{"convert": 0.1}
	prob := solver.NewProblem(net, initialState, [2]float64{0, 10}, rates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	svg, plotData := PlotSolution(sol, nil, 800, 600, "A→B Conversion", "Time", "Amount")

	// Verify the plot contains expected elements
	if !strings.Contains(svg, "A→B Conversion") {
		t.Error("Plot should contain title")
	}
	if plotData == nil {
		t.Fatal("PlotData should not be nil")
	}
	if len(plotData.Series) != 2 {
		t.Errorf("Expected 2 series (A and B), got %d", len(plotData.Series))
	}

	// Check that series have data points
	for _, series := range plotData.Series {
		if len(series.X) == 0 || len(series.Y) == 0 {
			t.Errorf("Series %s has no data points", series.Label)
		}
	}
}
