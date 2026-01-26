// Package metamodel provides compatibility utilities for migrating from
// the legacy Model type to the modern generic PetriNet types.
package metamodel

// LegacyModel wraps a Model with methods for converting to modern types.
// Use this during migration to maintain backwards compatibility.
type LegacyModel struct {
	*Model
}

// WrapLegacy wraps a Model for compatibility operations.
func WrapLegacy(model *Model) *LegacyModel {
	return &LegacyModel{Model: model}
}

// ToGenericTokenNet converts the legacy model to a generic PetriNet
// with TokenState for token places. Data places are converted to
// token places with initial count 0.
func (m *LegacyModel) ToGenericTokenNet() *PetriNet[TokenState[string]] {
	net := NewPetriNet[TokenState[string]](m.Name)
	net.Version = m.Version
	net.Description = m.Description

	// Convert places
	for _, place := range m.Places {
		var initial TokenState[string]
		if place.IsToken() {
			initial = NewTokenState(place.Initial, place.ID)
		} else {
			initial = NewTokenState(0, place.ID)
		}

		gp := NewGenericPlace(place.ID, initial).
			WithPosition(float64(place.X), float64(place.Y)).
			WithCapacity(place.Capacity).
			WithDescription(place.Description)

		net.AddPlace(gp)
	}

	// Convert transitions
	for _, trans := range m.Transitions {
		gt := NewGenericTransition[TokenState[string], TokenState[string]](trans.ID).
			WithPosition(float64(trans.X), float64(trans.Y)).
			WithDescription(trans.Description)

		if trans.Guard != "" {
			gt.GuardExpr = trans.Guard
		}

		net.AddTransition(gt)
	}

	// Convert arcs
	for _, arc := range m.Arcs {
		ga := NewGenericArc[TokenState[string]](arc.From, arc.To).
			WithWeight(arc.Weight).
			WithKeys(arc.Keys...).
			WithValue(arc.Value)

		if arc.IsInhibitor() {
			ga = ga.AsInhibitor()
		}

		net.AddArc(ga)
	}

	// Convert constraints
	for _, c := range m.Constraints {
		net.AddConstraint(Constraint{ID: c.ID, Expr: c.Expr})
	}

	return net
}

// ToGenericDataNet converts the legacy model to a generic PetriNet
// with DataState for data places. Token places are converted with
// their count as the data value.
func (m *LegacyModel) ToGenericDataNet() *PetriNet[DataState[any]] {
	net := NewPetriNet[DataState[any]](m.Name)
	net.Version = m.Version
	net.Description = m.Description

	// Convert places
	for _, place := range m.Places {
		var initial DataState[any]
		if place.IsToken() {
			initial = NewDataState[any](place.Initial)
		} else {
			initial = NewDataState[any](place.InitialValue)
		}

		gp := NewGenericPlace(place.ID, initial).
			WithPosition(float64(place.X), float64(place.Y)).
			WithCapacity(place.Capacity).
			WithDescription(place.Description)

		net.AddPlace(gp)
	}

	// Convert transitions
	for _, trans := range m.Transitions {
		gt := NewGenericTransition[DataState[any], DataState[any]](trans.ID).
			WithPosition(float64(trans.X), float64(trans.Y)).
			WithDescription(trans.Description)

		if trans.Guard != "" {
			gt.GuardExpr = trans.Guard
		}

		net.AddTransition(gt)
	}

	// Convert arcs
	for _, arc := range m.Arcs {
		ga := NewGenericArc[DataState[any]](arc.From, arc.To).
			WithWeight(arc.Weight).
			WithKeys(arc.Keys...).
			WithValue(arc.Value)

		if arc.IsInhibitor() {
			ga = ga.AsInhibitor()
		}

		net.AddArc(ga)
	}

	// Convert constraints
	for _, c := range m.Constraints {
		net.AddConstraint(Constraint{ID: c.ID, Expr: c.Expr})
	}

	return net
}

// ToExtended wraps the legacy model in an ExtendedModel structure.
// Application-level constructs (roles, views, etc.) are preserved
// in the underlying Model.
func (m *LegacyModel) ToExtended() *ExtendedModel {
	return NewExtendedModel(m.Model)
}

// ModelFromGenericToken creates a legacy Model from a generic token net.
// This is useful for using generated code that expects the legacy format.
func ModelFromGenericToken[T any](net *PetriNet[TokenState[T]]) *Model {
	model := &Model{
		Name:        net.Name,
		Version:     net.Version,
		Description: net.Description,
	}

	// Convert places
	for _, gp := range net.Places {
		place := Place{
			ID:          gp.ID,
			Description: gp.Description,
			Initial:     gp.Initial.Count,
			Kind:        TokenKind,
			Capacity:    gp.Capacity,
			X:           int(gp.X),
			Y:           int(gp.Y),
		}
		model.Places = append(model.Places, place)
	}

	// Convert transitions
	for _, gt := range net.Transitions {
		trans := Transition{
			ID:          gt.ID,
			Description: gt.Description,
			Guard:       gt.GuardExpr,
			X:           int(gt.X),
			Y:           int(gt.Y),
		}
		model.Transitions = append(model.Transitions, trans)
	}

	// Convert arcs
	for _, ga := range net.Arcs {
		arc := Arc{
			From:   ga.From,
			To:     ga.To,
			Weight: ga.Weight,
			Keys:   ga.Keys,
			Value:  ga.Value,
		}
		if ga.Inhibitor {
			arc.Type = InhibitorArc
		}
		model.Arcs = append(model.Arcs, arc)
	}

	// Convert constraints
	model.Constraints = net.Constraints

	return model
}

// ModelFromGenericData creates a legacy Model from a generic data net.
// Data places are marked with DataKind.
func ModelFromGenericData[T any](net *PetriNet[DataState[T]]) *Model {
	model := &Model{
		Name:        net.Name,
		Version:     net.Version,
		Description: net.Description,
	}

	// Convert places
	for _, gp := range net.Places {
		place := Place{
			ID:           gp.ID,
			Description:  gp.Description,
			Kind:         DataKind,
			InitialValue: gp.Initial.Value,
			Capacity:     gp.Capacity,
			X:            int(gp.X),
			Y:            int(gp.Y),
		}
		model.Places = append(model.Places, place)
	}

	// Convert transitions
	for _, gt := range net.Transitions {
		trans := Transition{
			ID:          gt.ID,
			Description: gt.Description,
			Guard:       gt.GuardExpr,
			X:           int(gt.X),
			Y:           int(gt.Y),
		}
		model.Transitions = append(model.Transitions, trans)
	}

	// Convert arcs
	for _, ga := range net.Arcs {
		arc := Arc{
			From:   ga.From,
			To:     ga.To,
			Weight: ga.Weight,
			Keys:   ga.Keys,
			Value:  ga.Value,
		}
		if ga.Inhibitor {
			arc.Type = InhibitorArc
		}
		model.Arcs = append(model.Arcs, arc)
	}

	// Convert constraints
	model.Constraints = net.Constraints

	return model
}

// MigrateToModern converts a legacy Model to the modern format with extensions.
// Returns the core Petri net and a slice of extracted extensions.
//
// This function extracts application constructs (roles, views, etc.) into
// separate extension objects while preserving the core Petri net structure.
func (m *LegacyModel) MigrateToModern() (*PetriNet[TokenState[string]], []ModelExtension) {
	net := m.ToGenericTokenNet()
	var extensions []ModelExtension

	// Note: Extension types are defined in external packages (like petri-pilot).
	// This returns empty extensions - actual migration requires the external
	// extension types to be registered and used.
	//
	// Example migration in petri-pilot:
	//   legacy := metamodel.WrapLegacy(model)
	//   net := legacy.ToGenericTokenNet()
	//   entities := extensions.NewEntityExtension()
	//   // ... populate from model
	//   app := extensions.NewApplicationSpec(ModelFromGenericToken(net))
	//   app.WithEntities(entities)

	return net, extensions
}

// HasApplicationConstructs returns true if the model has any application-level
// constructs that should be migrated to extensions.
func (m *LegacyModel) HasApplicationConstructs() bool {
	return len(m.Roles) > 0 ||
		len(m.Access) > 0 ||
		len(m.Views) > 0 ||
		m.Navigation != nil ||
		m.Admin != nil ||
		len(m.Timers) > 0 ||
		len(m.Notifications) > 0 ||
		len(m.Relationships) > 0 ||
		len(m.Computed) > 0 ||
		len(m.Approvals) > 0 ||
		len(m.Templates) > 0 ||
		len(m.InboundWebhooks) > 0 ||
		len(m.Documents) > 0
}

// ApplicationConstructsSummary returns a summary of application constructs
// that need to be migrated.
func (m *LegacyModel) ApplicationConstructsSummary() map[string]int {
	summary := make(map[string]int)

	if len(m.Roles) > 0 {
		summary["roles"] = len(m.Roles)
	}
	if len(m.Access) > 0 {
		summary["access_rules"] = len(m.Access)
	}
	if len(m.Views) > 0 {
		summary["views"] = len(m.Views)
	}
	if m.Navigation != nil {
		summary["navigation"] = 1
	}
	if m.Admin != nil {
		summary["admin"] = 1
	}
	if len(m.Timers) > 0 {
		summary["timers"] = len(m.Timers)
	}
	if len(m.Notifications) > 0 {
		summary["notifications"] = len(m.Notifications)
	}
	if len(m.Relationships) > 0 {
		summary["relationships"] = len(m.Relationships)
	}
	if len(m.Computed) > 0 {
		summary["computed_fields"] = len(m.Computed)
	}
	if len(m.Approvals) > 0 {
		summary["approval_chains"] = len(m.Approvals)
	}
	if len(m.Templates) > 0 {
		summary["templates"] = len(m.Templates)
	}
	if len(m.InboundWebhooks) > 0 {
		summary["webhooks"] = len(m.InboundWebhooks)
	}
	if len(m.Documents) > 0 {
		summary["documents"] = len(m.Documents)
	}

	return summary
}
