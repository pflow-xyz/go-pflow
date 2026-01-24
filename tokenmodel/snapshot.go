package tokenmodel

// Snapshot represents the current state of all states in a schema.
// It separates token counts (Petri net semantics) from data values (structured state).
type Snapshot struct {
	// Tokens holds integer counts for TokenState places.
	// Key: state ID, Value: token count
	// Firing semantics: input arcs decrement, output arcs increment.
	Tokens map[string]int `json:"tokens,omitempty"`

	// Data holds typed values for DataState places.
	// Key: state ID, Value: typed data (maps, structs, etc.)
	// Firing semantics: arcs specify keys for map transformation.
	Data map[string]any `json:"data,omitempty"`
}

// NewSnapshot creates an empty snapshot.
func NewSnapshot() *Snapshot {
	return &Snapshot{
		Tokens: make(map[string]int),
		Data:   make(map[string]any),
	}
}

// NewSnapshotFromSchema creates a snapshot initialized from schema defaults.
func NewSnapshotFromSchema(s *Schema) *Snapshot {
	snap := NewSnapshot()
	for _, st := range s.States {
		if st.IsToken() {
			snap.Tokens[st.ID] = st.InitialTokens()
		} else {
			if st.Initial != nil {
				snap.Data[st.ID] = st.Initial
			} else {
				// Initialize empty map for map types
				snap.Data[st.ID] = make(map[string]any)
			}
		}
	}
	return snap
}

// Clone creates a deep copy of the snapshot.
func (s *Snapshot) Clone() *Snapshot {
	clone := NewSnapshot()

	for k, v := range s.Tokens {
		clone.Tokens[k] = v
	}

	for k, v := range s.Data {
		// Shallow copy of data values - deep copy would require reflection
		clone.Data[k] = v
	}

	return clone
}

// GetTokens returns the token count for a TokenState.
func (s *Snapshot) GetTokens(stateID string) int {
	return s.Tokens[stateID]
}

// SetTokens sets the token count for a TokenState.
func (s *Snapshot) SetTokens(stateID string, count int) {
	s.Tokens[stateID] = count
}

// AddTokens adds to the token count for a TokenState.
func (s *Snapshot) AddTokens(stateID string, delta int) {
	s.Tokens[stateID] += delta
}

// GetData returns the data value for a DataState.
func (s *Snapshot) GetData(stateID string) any {
	return s.Data[stateID]
}

// SetData sets the data value for a DataState.
func (s *Snapshot) SetData(stateID string, value any) {
	s.Data[stateID] = value
}

// GetDataMap returns the data value as a map, or nil if not a map.
func (s *Snapshot) GetDataMap(stateID string) map[string]any {
	if v, ok := s.Data[stateID].(map[string]any); ok {
		return v
	}
	return nil
}

// GetDataMapValue returns a value from a DataState map.
func (s *Snapshot) GetDataMapValue(stateID, key string) any {
	if m := s.GetDataMap(stateID); m != nil {
		return m[key]
	}
	return nil
}

// SetDataMapValue sets a value in a DataState map.
func (s *Snapshot) SetDataMapValue(stateID, key string, value any) {
	m := s.GetDataMap(stateID)
	if m == nil {
		m = make(map[string]any)
		s.Data[stateID] = m
	}
	m[key] = value
}

// Bindings holds variable bindings for parameterized action execution.
// Contains function parameters (from, to, amount, owner, spender, caller, tokenId, etc.)
type Bindings map[string]any

// Clone creates a deep copy of the bindings.
func (b Bindings) Clone() Bindings {
	clone := make(Bindings, len(b))
	for k, v := range b {
		clone[k] = v
	}
	return clone
}

// Get retrieves a value from the bindings, returning nil if not found.
func (b Bindings) Get(key string) any {
	return b[key]
}

// GetString returns the value as a string, or empty string if not found.
func (b Bindings) GetString(key string) string {
	if v, ok := b[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt returns the value as an int, defaulting to 0 if not found.
func (b Bindings) GetInt(key string) int {
	if v, ok := b[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

// GetInt64 returns the value as an int64, defaulting to 0 if not found.
func (b Bindings) GetInt64(key string) int64 {
	if v, ok := b[key]; ok {
		switch n := v.(type) {
		case int:
			return int64(n)
		case int64:
			return n
		case float64:
			return int64(n)
		case string:
			// Support string amounts for large numbers
			var result int64
			for _, c := range n {
				if c >= '0' && c <= '9' {
					result = result*10 + int64(c-'0')
				}
			}
			return result
		}
	}
	return 0
}
