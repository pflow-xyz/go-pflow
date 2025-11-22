package templates

import (
	"github.com/pflow-xyz/go-pflow/petri"
)

// SIRTemplate implements the SIR epidemic model
type SIRTemplate struct{}

func (t *SIRTemplate) Name() string {
	return "sir"
}

func (t *SIRTemplate) Description() string {
	return "SIR epidemic model (Susceptible → Infected → Recovered)"
}

func (t *SIRTemplate) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "population",
			Description: "Total population size",
			Type:        "int",
			Default:     1000,
			Required:    false,
		},
		{
			Name:        "initial_infected",
			Description: "Initial number of infected individuals",
			Type:        "int",
			Default:     10,
			Required:    false,
		},
		{
			Name:        "infection_rate",
			Description: "Rate of infection (beta/N)",
			Type:        "float",
			Default:     0.0003,
			Required:    false,
		},
		{
			Name:        "recovery_rate",
			Description: "Rate of recovery (gamma)",
			Type:        "float",
			Default:     0.1,
			Required:    false,
		},
	}
}

func (t *SIRTemplate) Generate(params map[string]interface{}) (*petri.PetriNet, error) {
	// Extract parameters with defaults
	population := getIntParam(params, "population", 1000)
	initialInfected := getIntParam(params, "initial_infected", 10)
	initialSusceptible := population - initialInfected

	net := petri.NewPetriNet()

	// Add places
	net.AddPlace("S", float64(initialSusceptible), nil, 100, 100, strPtr("Susceptible"))
	net.AddPlace("I", float64(initialInfected), nil, 200, 100, strPtr("Infected"))
	net.AddPlace("R", 0.0, nil, 300, 100, strPtr("Recovered"))

	// Add transitions
	net.AddTransition("infection", "default", 150, 100, strPtr("Infection"))
	net.AddTransition("recovery", "default", 250, 100, strPtr("Recovery"))

	// Add arcs for infection: S + I → 2I
	net.AddArc("S", "infection", 1.0, false)
	net.AddArc("I", "infection", 1.0, false)
	net.AddArc("infection", "I", 2.0, false)

	// Add arcs for recovery: I → R
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	return net, nil
}

// SEIRTemplate implements the SEIR epidemic model with Exposed state
type SEIRTemplate struct{}

func (t *SEIRTemplate) Name() string {
	return "seir"
}

func (t *SEIRTemplate) Description() string {
	return "SEIR epidemic model (Susceptible → Exposed → Infected → Recovered)"
}

func (t *SEIRTemplate) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "population",
			Description: "Total population size",
			Type:        "int",
			Default:     1000,
			Required:    false,
		},
		{
			Name:        "initial_exposed",
			Description: "Initial number of exposed individuals",
			Type:        "int",
			Default:     5,
			Required:    false,
		},
		{
			Name:        "initial_infected",
			Description: "Initial number of infected individuals",
			Type:        "int",
			Default:     5,
			Required:    false,
		},
		{
			Name:        "exposure_rate",
			Description: "Rate of exposure (beta/N)",
			Type:        "float",
			Default:     0.0003,
			Required:    false,
		},
		{
			Name:        "incubation_rate",
			Description: "Rate of becoming infectious (sigma)",
			Type:        "float",
			Default:     0.2,
			Required:    false,
		},
		{
			Name:        "recovery_rate",
			Description: "Rate of recovery (gamma)",
			Type:        "float",
			Default:     0.1,
			Required:    false,
		},
	}
}

func (t *SEIRTemplate) Generate(params map[string]interface{}) (*petri.PetriNet, error) {
	population := getIntParam(params, "population", 1000)
	initialExposed := getIntParam(params, "initial_exposed", 5)
	initialInfected := getIntParam(params, "initial_infected", 5)
	initialSusceptible := population - initialExposed - initialInfected

	net := petri.NewPetriNet()

	// Add places
	net.AddPlace("S", float64(initialSusceptible), nil, 100, 100, strPtr("Susceptible"))
	net.AddPlace("E", float64(initialExposed), nil, 200, 100, strPtr("Exposed"))
	net.AddPlace("I", float64(initialInfected), nil, 300, 100, strPtr("Infected"))
	net.AddPlace("R", 0.0, nil, 400, 100, strPtr("Recovered"))

	// Add transitions
	net.AddTransition("exposure", "default", 150, 100, strPtr("Exposure"))
	net.AddTransition("incubation", "default", 250, 100, strPtr("Incubation"))
	net.AddTransition("recovery", "default", 350, 100, strPtr("Recovery"))

	// Add arcs for exposure: S + I → E + I
	net.AddArc("S", "exposure", 1.0, false)
	net.AddArc("I", "exposure", 1.0, false)
	net.AddArc("exposure", "E", 1.0, false)
	net.AddArc("exposure", "I", 1.0, false)

	// Add arcs for incubation: E → I
	net.AddArc("E", "incubation", 1.0, false)
	net.AddArc("incubation", "I", 1.0, false)

	// Add arcs for recovery: I → R
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	return net, nil
}

// Helper functions
func getIntParam(params map[string]interface{}, name string, defaultVal int) int {
	if val, ok := params[name]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return defaultVal
}

func getFloatParam(params map[string]interface{}, name string, defaultVal float64) float64 {
	if val, ok := params[name]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
	}
	return defaultVal
}

func getStringParam(params map[string]interface{}, name string, defaultVal string) string {
	if val, ok := params[name]; ok {
		if v, ok := val.(string); ok {
			return v
		}
	}
	return defaultVal
}

func strPtr(s string) *string {
	return &s
}
