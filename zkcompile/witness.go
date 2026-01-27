package zkcompile

import "fmt"

// WitnessSource identifies where a witness value comes from.
type WitnessSource int

const (
	// FromBinding: value comes from transaction input (e.g., amount, to, from)
	FromBinding WitnessSource = iota
	// FromState: value comes from Merkle proof of state (e.g., balances[from])
	FromState
	// Computed: intermediate value computed during proving
	Computed
	// Constant: known constant value (e.g., address(0))
	Constant
)

func (s WitnessSource) String() string {
	switch s {
	case FromBinding:
		return "binding"
	case FromState:
		return "state"
	case Computed:
		return "computed"
	case Constant:
		return "constant"
	default:
		return "?"
	}
}

// WitnessVar represents a variable in the circuit.
type WitnessVar struct {
	Name     string        // Unique variable name
	Source   WitnessSource // Where the value comes from
	PlaceID  string        // For state access: which place (e.g., "balances")
	Keys     []string      // For state access: key bindings (e.g., ["from"])
	ConstVal string        // For constants: the value
}

func (w *WitnessVar) String() string {
	switch w.Source {
	case FromState:
		return fmt.Sprintf("%s[%v]", w.PlaceID, w.Keys)
	case FromBinding:
		return fmt.Sprintf("binding(%s)", w.Name)
	case Constant:
		return fmt.Sprintf("const(%s)", w.ConstVal)
	default:
		return w.Name
	}
}

// StateAccess represents a state read that requires a Merkle proof.
type StateAccess struct {
	WitnessName string   // Name of witness variable holding the value
	PlaceID     string   // Place being accessed (e.g., "balances")
	KeyBindings []string // Binding names for keys (e.g., ["from"])
	IsNested    bool     // True if nested map (e.g., allowances[owner][spender])
}

func (s *StateAccess) String() string {
	if s.IsNested && len(s.KeyBindings) >= 2 {
		return fmt.Sprintf("%s[%s][%s]", s.PlaceID, s.KeyBindings[0], s.KeyBindings[1])
	}
	if len(s.KeyBindings) > 0 {
		return fmt.Sprintf("%s[%s]", s.PlaceID, s.KeyBindings[0])
	}
	return s.PlaceID
}

// WitnessTable tracks all witness variables and state accesses.
type WitnessTable struct {
	Variables    map[string]*WitnessVar
	StateReads   []*StateAccess
	nextTempID   int
}

// NewWitnessTable creates a new witness table.
func NewWitnessTable() *WitnessTable {
	return &WitnessTable{
		Variables:  make(map[string]*WitnessVar),
		StateReads: make([]*StateAccess, 0),
	}
}

// AddBinding registers a witness from transaction bindings.
func (w *WitnessTable) AddBinding(name string) *WitnessVar {
	if v, ok := w.Variables[name]; ok {
		return v
	}
	v := &WitnessVar{
		Name:   name,
		Source: FromBinding,
	}
	w.Variables[name] = v
	return v
}

// AddStateRead registers a state read with Merkle proof.
func (w *WitnessTable) AddStateRead(placeID string, keyBindings []string) *WitnessVar {
	// Generate witness name from place and keys
	name := placeID
	for _, k := range keyBindings {
		name += "_" + k
	}

	if v, ok := w.Variables[name]; ok {
		return v
	}

	v := &WitnessVar{
		Name:    name,
		Source:  FromState,
		PlaceID: placeID,
		Keys:    keyBindings,
	}
	w.Variables[name] = v

	access := &StateAccess{
		WitnessName: name,
		PlaceID:     placeID,
		KeyBindings: keyBindings,
		IsNested:    len(keyBindings) > 1,
	}
	w.StateReads = append(w.StateReads, access)

	return v
}

// AddConstant registers a constant value.
func (w *WitnessTable) AddConstant(value string) *WitnessVar {
	name := "const_" + value
	if v, ok := w.Variables[name]; ok {
		return v
	}
	v := &WitnessVar{
		Name:     name,
		Source:   Constant,
		ConstVal: value,
	}
	w.Variables[name] = v
	return v
}

// AddComputed registers an intermediate computed value.
func (w *WitnessTable) AddComputed(prefix string) *WitnessVar {
	name := fmt.Sprintf("%s_%d", prefix, w.nextTempID)
	w.nextTempID++
	v := &WitnessVar{
		Name:   name,
		Source: Computed,
	}
	w.Variables[name] = v
	return v
}

// Get retrieves a witness variable by name.
func (w *WitnessTable) Get(name string) (*WitnessVar, bool) {
	v, ok := w.Variables[name]
	return v, ok
}
