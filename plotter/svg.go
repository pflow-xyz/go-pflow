// Package plotter provides SVG visualization for ODE solutions.
package plotter

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Series represents a single data series to plot.
type Series struct {
	X     []float64
	Y     []float64
	Label string
	Color string
}

// PlotData contains metadata about the last rendered plot.
// This can be used for interactive features like crosshairs and tooltips.
type PlotData struct {
	PlotID     string
	Margin     map[string]float64
	PlotWidth  float64
	PlotHeight float64
	Xmin       float64
	Xmax       float64
	Ymin       float64
	Ymax       float64
	Series     []Series
}

// SVGPlotter creates SVG plots with customizable styling.
type SVGPlotter struct {
	Width      float64
	Height     float64
	Margin     map[string]float64
	PlotWidth  float64
	PlotHeight float64
	Title      string
	XLabel     string
	YLabel     string
	Series     []Series
	LastPlot   *PlotData
}

// NewSVGPlotter creates a new SVG plotter with the given dimensions.
func NewSVGPlotter(width, height float64) *SVGPlotter {
	margin := map[string]float64{"top": 40, "right": 30, "bottom": 50, "left": 60}
	pw := width - margin["left"] - margin["right"]
	ph := height - margin["top"] - margin["bottom"]
	return &SVGPlotter{
		Width:      width,
		Height:     height,
		Margin:     margin,
		PlotWidth:  pw,
		PlotHeight: ph,
		Title:      "",
		XLabel:     "Time",
		YLabel:     "Value",
		Series:     nil,
		LastPlot:   nil,
	}
}

// SetTitle sets the plot title.
func (p *SVGPlotter) SetTitle(t string) *SVGPlotter {
	p.Title = t
	return p
}

// SetXLabel sets the X-axis label.
func (p *SVGPlotter) SetXLabel(s string) *SVGPlotter {
	p.XLabel = s
	return p
}

// SetYLabel sets the Y-axis label.
func (p *SVGPlotter) SetYLabel(s string) *SVGPlotter {
	p.YLabel = s
	return p
}

// AddSeries adds a data series to the plot.
// If color is empty, a default color from a palette will be used.
func (p *SVGPlotter) AddSeries(x, y []float64, label, color string) *SVGPlotter {
	if color == "" {
		colors := []string{"#e41a1c", "#377eb8", "#4daf4a", "#984ea3", "#ff7f00", "#ffff33", "#a65628", "#f781bf"}
		color = colors[len(p.Series)%len(colors)]
	}
	p.Series = append(p.Series, Series{X: x, Y: y, Label: label, Color: color})
	return p
}

// Render generates the SVG string and stores metadata in LastPlot.
func (p *SVGPlotter) Render() string {
	// Compute data ranges
	xmin := math.Inf(1)
	xmax := math.Inf(-1)
	ymin := math.Inf(1)
	ymax := math.Inf(-1)

	for _, s := range p.Series {
		for i := range s.X {
			x := s.X[i]
			y := s.Y[i]
			if x < xmin {
				xmin = x
			}
			if x > xmax {
				xmax = x
			}
			if y < ymin {
				ymin = y
			}
			if y > ymax {
				ymax = y
			}
		}
	}

	// Handle edge cases
	if math.IsInf(xmin, 1) || math.IsInf(xmax, -1) {
		xmin = 0
		xmax = 1
	}
	if math.IsInf(ymin, 1) || math.IsInf(ymax, -1) {
		ymin = 0
		ymax = 1
	}

	xrange := xmax - xmin
	if xrange == 0 {
		xrange = 1
	}
	yrange := ymax - ymin
	if yrange == 0 {
		yrange = 1
	}

	// Add padding
	xmin -= xrange * 0.05
	xmax += xrange * 0.05
	ymin -= yrange * 0.1
	ymax += yrange * 0.1

	// Scaling functions
	sx := func(x float64) float64 {
		return p.Margin["left"] + ((x-xmin)/(xmax-xmin))*p.PlotWidth
	}
	sy := func(y float64) float64 {
		return p.Margin["top"] + p.PlotHeight - ((y-ymin)/(ymax-ymin))*p.PlotHeight
	}

	plotID := "plot_" + strconv.FormatInt(int64(math.Round(1000000*math.Abs(xmin+xmax+ymin+ymax))), 10)

	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" id="%s">`,
		int(p.Width), int(p.Height), plotID))

	// Background rectangle for visibility on dark themes
	sb.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#f8f9fa" rx="8"/>`,
		int(p.Width), int(p.Height)))

	// Title
	if p.Title != "" {
		sb.WriteString(fmt.Sprintf(`<text x="%f" y="25" text-anchor="middle" font-family="Arial, sans-serif" font-size="16" font-weight="bold">%s</text>`,
			p.Width/2, petri.Escape(p.Title)))
	}

	// Axes
	sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#333" stroke-width="2"/>`,
		p.Margin["left"], p.Margin["top"], p.Margin["left"], p.Margin["top"]+p.PlotHeight))
	sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#333" stroke-width="2"/>`,
		p.Margin["left"], p.Margin["top"]+p.PlotHeight, p.Margin["left"]+p.PlotWidth, p.Margin["top"]+p.PlotHeight))

	// Axis labels
	sb.WriteString(fmt.Sprintf(`<text x="%f" y="%f" text-anchor="middle" font-family="Arial, sans-serif" font-size="12">%s</text>`,
		p.Margin["left"]+p.PlotWidth/2, p.Height-10, petri.Escape(p.XLabel)))
	sb.WriteString(fmt.Sprintf(`<text x="15" y="%f" text-anchor="middle" font-family="Arial, sans-serif" font-size="12" transform="rotate(-90, 15, %f)">%s</text>`,
		p.Margin["top"]+p.PlotHeight/2, p.Margin["top"]+p.PlotHeight/2, petri.Escape(p.YLabel)))

	// Grid and ticks
	numXTicks := 5
	numYTicks := 5
	for i := 0; i <= numXTicks; i++ {
		x := xmin + (xmax-xmin)*float64(i)/float64(numXTicks)
		px := sx(x)
		sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#333" stroke-width="1"/>`,
			px, p.Margin["top"]+p.PlotHeight, px, p.Margin["top"]+p.PlotHeight+5))
		sb.WriteString(fmt.Sprintf(`<text x="%f" y="%f" text-anchor="middle" font-family="Arial, sans-serif" font-size="10">%.1f</text>`,
			px, p.Margin["top"]+p.PlotHeight+20, x))
		sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#ddd" stroke-width="0.5"/>`,
			px, p.Margin["top"], px, p.Margin["top"]+p.PlotHeight))
	}
	for i := 0; i <= numYTicks; i++ {
		y := ymin + (ymax-ymin)*float64(i)/float64(numYTicks)
		py := sy(y)
		sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#333" stroke-width="1"/>`,
			p.Margin["left"]-5, py, p.Margin["left"], py))
		sb.WriteString(fmt.Sprintf(`<text x="%f" y="%f" text-anchor="end" font-family="Arial, sans-serif" font-size="10">%.1f</text>`,
			p.Margin["left"]-10, py+4, y))
		sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="#ddd" stroke-width="0.5"/>`,
			p.Margin["left"], py, p.Margin["left"]+p.PlotWidth, py))
	}

	// Plot series
	for _, s := range p.Series {
		if len(s.X) == 0 {
			continue
		}
		path := strings.Builder{}
		for i := range s.X {
			px := sx(s.X[i])
			py := sy(s.Y[i])
			if i == 0 {
				path.WriteString(fmt.Sprintf("M%f,%f", px, py))
			} else {
				path.WriteString(fmt.Sprintf(" L%f,%f", px, py))
			}
		}
		sb.WriteString(fmt.Sprintf(`<path d="%s" stroke="%s" stroke-width="2" fill="none"/>`,
			path.String(), s.Color))
	}

	// Legend
	hasLabel := false
	for _, s := range p.Series {
		if s.Label != "" {
			hasLabel = true
			break
		}
	}
	if hasLabel {
		legendY := p.Margin["top"] + 10
		for _, s := range p.Series {
			if s.Label == "" {
				continue
			}
			x1 := p.Width - p.Margin["right"] - 50
			x2 := p.Width - p.Margin["right"] - 30
			sb.WriteString(fmt.Sprintf(`<line x1="%f" y1="%f" x2="%f" y2="%f" stroke="%s" stroke-width="2"/>`,
				x1, legendY, x2, legendY, s.Color))
			sb.WriteString(fmt.Sprintf(`<text x="%f" y="%f" font-family="Arial, sans-serif" font-size="10">%s</text>`,
				x2+5, legendY+4, petri.Escape(s.Label)))
			legendY += 20
		}
	}

	// Crosshair group for interactivity (hidden by default)
	sb.WriteString(fmt.Sprintf(`<g id="%s_crosshair" style="display:none;">`, plotID))
	sb.WriteString(fmt.Sprintf(`<line id="%s_line" x1="0" y1="%f" x2="0" y2="%f" stroke="#666" stroke-width="1" stroke-dasharray="4,4"/>`,
		plotID, p.Margin["top"], p.Margin["top"]+p.PlotHeight))
	sb.WriteString(`<rect id="tooltip_bg" x="0" y="0" rx="4" ry="4" fill="white" stroke="#666" stroke-width="1" opacity="0.95"/>`)
	sb.WriteString(`<text id="tooltip_text" x="0" y="0" font-family="Arial, sans-serif" font-size="11" fill="#333"></text>`)
	sb.WriteString(`</g>`)

	// Overlay rectangle for potential JS interactivity
	sb.WriteString(fmt.Sprintf(`<rect id="%s_overlay" x="%f" y="%f" width="%f" height="%f" fill="transparent" style="cursor:crosshair;"/>`,
		plotID, p.Margin["left"], p.Margin["top"], p.PlotWidth, p.PlotHeight))

	sb.WriteString(`</svg>`)

	// Store metadata
	p.LastPlot = &PlotData{
		PlotID:     plotID,
		Margin:     p.Margin,
		PlotWidth:  p.PlotWidth,
		PlotHeight: p.PlotHeight,
		Xmin:       xmin,
		Xmax:       xmax,
		Ymin:       ymin,
		Ymax:       ymax,
		Series:     p.Series,
	}

	return sb.String()
}

// PlotSolution is a convenience function to plot an ODE solution.
// If variables is nil, all state variables will be plotted.
func PlotSolution(sol *solver.Solution, variables []string, width, height float64, title, xlabel, ylabel string) (string, *PlotData) {
	plotter := NewSVGPlotter(width, height)
	if title != "" {
		plotter.SetTitle(title)
	}
	if xlabel != "" {
		plotter.SetXLabel(xlabel)
	}
	if ylabel != "" {
		plotter.SetYLabel(ylabel)
	}

	varsToPlot := variables
	if varsToPlot == nil {
		varsToPlot = sol.StateLabels
	}
	for _, vn := range varsToPlot {
		y := sol.GetVariable(vn)
		plotter.AddSeries(sol.T, y, vn, "")
	}

	svg := plotter.Render()
	return svg, plotter.LastPlot
}
