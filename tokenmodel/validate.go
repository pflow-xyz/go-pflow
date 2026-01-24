package tokenmodel

// Validate checks the schema for structural correctness.
func (s *Schema) Validate() error {
	stateIDs := make(map[string]bool)
	actionIDs := make(map[string]bool)

	for _, st := range s.States {
		if st.ID == "" {
			return ErrEmptyID
		}
		if stateIDs[st.ID] {
			return ErrDuplicateID
		}
		stateIDs[st.ID] = true
	}

	for _, a := range s.Actions {
		if a.ID == "" {
			return ErrEmptyID
		}
		if actionIDs[a.ID] {
			return ErrDuplicateID
		}
		actionIDs[a.ID] = true
	}

	for _, arc := range s.Arcs {
		sourceIsState := stateIDs[arc.Source]
		sourceIsAction := actionIDs[arc.Source]
		targetIsState := stateIDs[arc.Target]
		targetIsAction := actionIDs[arc.Target]

		if !sourceIsState && !sourceIsAction {
			return ErrInvalidArcSource
		}
		if !targetIsState && !targetIsAction {
			return ErrInvalidArcTarget
		}
		// Arcs must connect states to actions or vice versa (bipartite)
		if sourceIsState && targetIsState {
			return ErrInvalidArcConnection
		}
		if sourceIsAction && targetIsAction {
			return ErrInvalidArcConnection
		}
	}

	return nil
}
