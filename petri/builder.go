package petri

// Builder provides a fluent API for constructing Petri nets.
// It simplifies net creation by chaining method calls and using sensible defaults.
//
// Example:
//
//	net := petri.Build().
//	    Place("S", 999).
//	    Place("I", 1).
//	    Place("R", 0).
//	    Transition("infect").
//	    Transition("recover").
//	    Arc("S", "infect", 1).
//	    Arc("I", "infect", 1).
//	    Arc("infect", "I", 2).
//	    Arc("I", "recover", 1).
//	    Arc("recover", "R", 1).
//	    Done()
type Builder struct {
	net    *PetriNet
	nextX  float64
	nextY  float64
	placeY float64
	transY float64
}

// Build creates a new Builder for constructing a Petri net.
func Build() *Builder {
	return &Builder{
		net:    NewPetriNet(),
		nextX:  100,
		nextY:  100,
		placeY: 100,
		transY: 200,
	}
}

// Place adds a place with the given label and initial token count.
// Uses auto-incrementing X coordinates for visualization.
func (b *Builder) Place(label string, initial float64) *Builder {
	b.net.AddPlace(label, initial, nil, b.nextX, b.placeY, nil)
	b.nextX += 100
	return b
}

// PlaceWithCapacity adds a place with initial tokens and capacity limit.
func (b *Builder) PlaceWithCapacity(label string, initial, capacity float64) *Builder {
	b.net.AddPlace(label, initial, capacity, b.nextX, b.placeY, nil)
	b.nextX += 100
	return b
}

// Transition adds a transition with the given label.
// Uses auto-incrementing X coordinates for visualization.
func (b *Builder) Transition(label string) *Builder {
	b.net.AddTransition(label, "default", b.nextX, b.transY, nil)
	b.nextX += 100
	return b
}

// TransitionWithRole adds a transition with a specific role.
func (b *Builder) TransitionWithRole(label, role string) *Builder {
	b.net.AddTransition(label, role, b.nextX, b.transY, nil)
	b.nextX += 100
	return b
}

// Arc adds an arc from source to target with the given weight.
func (b *Builder) Arc(source, target string, weight float64) *Builder {
	b.net.AddArc(source, target, weight, false)
	return b
}

// InhibitorArc adds an inhibitor arc from source to target.
func (b *Builder) InhibitorArc(source, target string, weight float64) *Builder {
	b.net.AddArc(source, target, weight, true)
	return b
}

// Flow adds bidirectional arcs for a simple flow pattern: place -> transition -> place.
// This is a convenience for the common pattern of consuming from one place
// and producing to another.
//
// Example:
//
//	builder.Flow("input", "process", "output", 1)
//	// Equivalent to:
//	// builder.Arc("input", "process", 1).Arc("process", "output", 1)
func (b *Builder) Flow(fromPlace, transition, toPlace string, weight float64) *Builder {
	b.net.AddArc(fromPlace, transition, weight, false)
	b.net.AddArc(transition, toPlace, weight, false)
	return b
}

// Chain creates a sequential chain of places connected by transitions.
// Useful for workflow/pipeline patterns.
//
// Example:
//
//	builder.Chain(1, "Received", "start", "Processing", "finish", "Complete")
//	// Creates: Received(1) -> start -> Processing(0) -> finish -> Complete(0)
func (b *Builder) Chain(initialTokens float64, elements ...string) *Builder {
	if len(elements) < 3 || len(elements)%2 == 0 {
		// Need odd number: place, trans, place, trans, place...
		return b
	}

	// First place gets initial tokens
	b.Place(elements[0], initialTokens)

	for i := 1; i < len(elements); i += 2 {
		trans := elements[i]
		nextPlace := elements[i+1]

		b.Transition(trans)
		b.Place(nextPlace, 0)
		b.Arc(elements[i-1], trans, 1)
		b.Arc(trans, nextPlace, 1)
	}

	return b
}

// SIR creates a standard SIR epidemic model.
// This is a convenience method for a common pattern.
func (b *Builder) SIR(susceptible, infected, recovered float64) *Builder {
	return b.
		Place("S", susceptible).
		Place("I", infected).
		Place("R", recovered).
		Transition("infect").
		Transition("recover").
		Arc("S", "infect", 1).
		Arc("I", "infect", 1).
		Arc("infect", "I", 2).
		Arc("I", "recover", 1).
		Arc("recover", "R", 1)
}

// Done returns the completed Petri net.
func (b *Builder) Done() *PetriNet {
	return b.net
}

// Net returns the Petri net being built (alias for Done).
func (b *Builder) Net() *PetriNet {
	return b.net
}

// WithRates returns the net and a rates map initialized to the given default rate.
// Useful for immediately setting up simulation.
//
// Example:
//
//	net, rates := petri.Build().
//	    Place("A", 10).Transition("t1").Arc("A", "t1", 1).
//	    WithRates(1.0)
func (b *Builder) WithRates(defaultRate float64) (*PetriNet, map[string]float64) {
	rates := make(map[string]float64)
	for label := range b.net.Transitions {
		rates[label] = defaultRate
	}
	return b.net, rates
}

// WithCustomRates returns the net and allows setting custom rates.
//
// Example:
//
//	net, rates := petri.Build().
//	    Place("S", 999).Place("I", 1).Place("R", 0).
//	    Transition("infect").Transition("recover").
//	    Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
//	    Arc("I", "recover", 1).Arc("recover", "R", 1).
//	    WithCustomRates(map[string]float64{"infect": 0.3, "recover": 0.1})
func (b *Builder) WithCustomRates(rates map[string]float64) (*PetriNet, map[string]float64) {
	return b.net, rates
}
