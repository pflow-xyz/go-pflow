package metamodel

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

// ToTokenModel converts a metamodel Model to a tokenmodel Schema.
func ToTokenModel(model *Model) *tokenmodel.Schema {
	s := tokenmodel.NewSchema(model.Name)
	s.Version = model.Version
	if s.Version == "" {
		s.Version = "1.0.0"
	}

	// Convert places to states
	for _, place := range model.Places {
		state := tokenmodel.State{
			ID:       place.ID,
			Exported: place.Exported,
		}

		if place.IsData() {
			state.Kind = tokenmodel.DataState
			state.Type = place.Type
			state.Initial = nil // Data states start empty unless specified
		} else {
			state.Kind = tokenmodel.TokenState
			state.Type = "int"
			state.Initial = place.Initial
		}

		s.AddState(state)
	}

	// Convert transitions to actions
	for _, transition := range model.Transitions {
		action := tokenmodel.Action{
			ID:            transition.ID,
			Guard:         transition.Guard,
			EventID:       transition.EventType,
			EventBindings: bindingsToMap(transition.Bindings),
		}
		s.AddAction(action)
	}

	// Convert arcs
	for _, arc := range model.Arcs {
		metaArc := tokenmodel.Arc{
			Source: arc.From,
			Target: arc.To,
			Keys:   arc.Keys,
			Value:  arc.Value,
		}

		// Default value binding for data arcs
		if metaArc.Value == "" && len(metaArc.Keys) > 0 {
			metaArc.Value = "amount"
		}

		s.AddArc(metaArc)
	}

	// Convert constraints
	for _, constraint := range model.Constraints {
		s.AddConstraint(tokenmodel.Constraint{
			ID:   constraint.ID,
			Expr: constraint.Expr,
		})
	}

	return s
}

// FromTokenModel converts a tokenmodel Schema to a metamodel Model.
func FromTokenModel(s *tokenmodel.Schema) *Model {
	model := &Model{
		Name:    s.Name,
		Version: s.Version,
	}

	// Convert states to places
	for _, state := range s.States {
		place := Place{
			ID:       state.ID,
			Type:     state.Type,
			Exported: state.Exported,
		}

		if state.IsToken() {
			place.Kind = TokenKind
			place.Initial = state.InitialTokens()
		} else {
			place.Kind = DataKind
			place.Initial = 0
		}

		model.Places = append(model.Places, place)
	}

	// Convert actions to transitions
	for _, action := range s.Actions {
		transition := Transition{
			ID:        action.ID,
			Guard:     action.Guard,
			EventType: action.EventID,
			Bindings:  mapToBindings(action.EventBindings),
		}
		model.Transitions = append(model.Transitions, transition)
	}

	// Convert arcs
	for _, arc := range s.Arcs {
		schemaArc := Arc{
			From:   arc.Source,
			To:     arc.Target,
			Weight: 1, // Default weight
			Keys:   arc.Keys,
			Value:  arc.Value,
		}
		model.Arcs = append(model.Arcs, schemaArc)
	}

	// Convert constraints
	for _, constraint := range s.Constraints {
		model.Constraints = append(model.Constraints, Constraint{
			ID:   constraint.ID,
			Expr: constraint.Expr,
		})
	}

	return model
}

// EnrichModel adds default values and infers missing fields.
func EnrichModel(model *Model) *Model {
	enriched := *model // shallow copy

	// Ensure places have default kind
	for i := range enriched.Places {
		if enriched.Places[i].Kind == "" {
			enriched.Places[i].Kind = TokenKind
		}
	}

	// Infer event types from transition IDs
	for i := range enriched.Transitions {
		t := &enriched.Transitions[i]
		if t.EventType == "" {
			if t.Event != "" {
				t.EventType = toEventTypeFromEvent(t.Event, t.ID)
			} else {
				t.EventType = toEventType(t.ID)
			}
		}
		if t.HTTPPath == "" {
			t.HTTPPath = "/api/" + t.ID
		}
		if t.HTTPMethod == "" {
			t.HTTPMethod = "POST"
		}
	}

	return &enriched
}

// ValidateForCodegen checks if a model is ready for code generation.
func ValidateForCodegen(model *Model) []string {
	var issues []string

	if model.Name == "" {
		issues = append(issues, "model name is required")
	}

	if len(model.Places) == 0 {
		issues = append(issues, "model has no places (states)")
	}

	if len(model.Transitions) == 0 {
		issues = append(issues, "model has no transitions (actions)")
	}

	// Check for unconnected elements
	connected := make(map[string]bool)
	for _, arc := range model.Arcs {
		connected[arc.From] = true
		connected[arc.To] = true
	}

	for _, p := range model.Places {
		if !connected[p.ID] {
			issues = append(issues, "place '"+p.ID+"' has no connections")
		}
	}

	for _, t := range model.Transitions {
		if !connected[t.ID] {
			issues = append(issues, "transition '"+t.ID+"' has no connections")
		}
	}

	// Check data places have types
	for _, p := range model.Places {
		if p.IsData() && p.Type == "" {
			issues = append(issues, "data place '"+p.ID+"' needs a type")
		}
	}

	return issues
}

// APIRoute represents an inferred API endpoint.
type APIRoute struct {
	TransitionID string
	Method       string
	Path         string
	Description  string
	EventType    string
}

// InferAPIRoutes generates API route information from transitions.
func InferAPIRoutes(model *Model) []APIRoute {
	var routes []APIRoute

	for _, t := range model.Transitions {
		route := APIRoute{
			TransitionID: t.ID,
			Method:       t.HTTPMethod,
			Path:         t.HTTPPath,
			Description:  t.Description,
			EventType:    t.EventType,
		}

		if route.Method == "" {
			route.Method = "POST"
		}
		if route.Path == "" {
			route.Path = "/api/" + t.ID
		}
		if route.EventType == "" {
			route.EventType = toEventType(t.ID)
		}

		routes = append(routes, route)
	}

	return routes
}

// EventDef represents an inferred event definition.
type EventDef struct {
	Type         string
	TransitionID string
	Fields       []InferredEventField
}

// InferredEventField represents a field in an inferred event.
type InferredEventField struct {
	Name string
	Type string
}

// InferEvents generates event definitions from transitions.
func InferEvents(model *Model) []EventDef {
	// If explicit events are defined, use them
	if len(model.Events) > 0 {
		return buildEventsFromSchema(model)
	}

	// Fallback: infer events from transitions
	return inferEventsFromTransitions(model)
}

// StateField represents a field in aggregate state.
type StateField struct {
	Name      string
	Type      string
	IsToken   bool
	Persisted bool
}

// InferAggregateState generates aggregate state fields from places.
func InferAggregateState(model *Model) []StateField {
	var fields []StateField

	for _, p := range model.Places {
		field := StateField{
			Name:      p.ID,
			Type:      "int",
			IsToken:   p.IsToken(),
			Persisted: p.Persisted,
		}

		if p.IsData() {
			field.Type = p.Type
			if field.Type == "" {
				field.Type = "any"
			}
		}

		fields = append(fields, field)
	}

	return fields
}

// Helper functions

func bindingsToMap(bindings []Binding) map[string]string {
	if len(bindings) == 0 {
		return nil
	}
	result := make(map[string]string)
	for _, b := range bindings {
		result[b.Name] = b.Type
	}
	return result
}

func mapToBindings(m map[string]string) []Binding {
	if len(m) == 0 {
		return nil
	}
	var result []Binding
	for name, typ := range m {
		result = append(result, Binding{
			Name: name,
			Type: typ,
		})
	}
	return result
}

func toEventTypeFromEvent(eventName, transitionID string) string {
	suffix := extractNumericSuffix(transitionID)
	return eventName + suffix
}

func extractNumericSuffix(s string) string {
	suffixStart := len(s)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] >= '0' && s[i] <= '9' {
			suffixStart = i
		} else {
			break
		}
	}
	return s[suffixStart:]
}

func toEventType(id string) string {
	if len(id) == 0 {
		return "Event"
	}

	result := ""
	capitalizeNext := true
	for _, c := range id {
		if c == '_' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			result += string(toUpper(c))
			capitalizeNext = false
		} else {
			result += string(c)
		}
	}

	// Add past tense suffix if not already present
	if len(result) > 2 && result[len(result)-2:] != "ed" {
		result += "ed"
	}

	return result
}

func toUpper(c rune) rune {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

func eventIDToType(id string) string {
	if len(id) == 0 {
		return "Event"
	}

	result := ""
	capitalizeNext := true
	for _, c := range id {
		if c == '_' || c == '-' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			result += string(toUpper(c))
			capitalizeNext = false
		} else {
			result += string(c)
		}
	}

	return result
}

func buildEventsFromSchema(model *Model) []EventDef {
	// Build event lookup map
	eventMap := make(map[string]*Event)
	for i := range model.Events {
		eventMap[model.Events[i].ID] = &model.Events[i]
	}

	// Track seen event types to avoid duplicates
	seen := make(map[string]bool)
	var events []EventDef

	for _, t := range model.Transitions {
		var eventType string
		var fields []InferredEventField

		if t.Event != "" {
			if schemaEvent, ok := eventMap[t.Event]; ok {
				eventType = eventIDToType(schemaEvent.ID)
				fields = convertEventFields(schemaEvent.Fields)
			} else {
				eventType = toEventType(t.ID)
				fields = inferEventFields(model, t)
			}
		} else if t.EventType != "" {
			eventType = t.EventType
			fields = inferEventFields(model, t)
		} else {
			eventType = toEventType(t.ID)
			fields = inferEventFields(model, t)
		}

		if !seen[eventType] {
			seen[eventType] = true
			events = append(events, EventDef{
				Type:         eventType,
				TransitionID: t.ID,
				Fields:       fields,
			})
		}
	}

	return events
}

func convertEventFields(fields []EventField) []InferredEventField {
	result := []InferredEventField{
		{Name: "aggregate_id", Type: "string"},
		{Name: "timestamp", Type: "time.Time"},
	}

	for _, f := range fields {
		result = append(result, InferredEventField{
			Name: f.Name,
			Type: schemaTypeToGo(f.Type, f.Of),
		})
	}

	return result
}

func schemaTypeToGo(typ, of string) string {
	switch typ {
	case "string":
		return "string"
	case "number":
		return "float64"
	case "integer":
		return "int"
	case "boolean":
		return "bool"
	case "time":
		return "time.Time"
	case "array":
		if of != "" {
			return "[]" + schemaTypeToGo(of, "")
		}
		return "[]any"
	case "object":
		if of != "" {
			return "map[string]" + schemaTypeToGo(of, "")
		}
		return "map[string]any"
	default:
		return typ
	}
}

func inferEventsFromTransitions(model *Model) []EventDef {
	var events []EventDef

	for _, t := range model.Transitions {
		eventType := t.EventType
		if eventType == "" {
			eventType = toEventType(t.ID)
		}

		event := EventDef{
			Type:         eventType,
			TransitionID: t.ID,
			Fields:       inferEventFields(model, t),
		}

		events = append(events, event)
	}

	return events
}

func inferEventFields(model *Model, t Transition) []InferredEventField {
	fields := []InferredEventField{
		{Name: "aggregate_id", Type: "string"},
		{Name: "timestamp", Type: "time.Time"},
	}

	// Add fields from arc bindings
	seen := make(map[string]bool)
	seen["aggregate_id"] = true
	seen["timestamp"] = true

	for _, arc := range model.Arcs {
		if arc.From == t.ID || arc.To == t.ID {
			for _, key := range arc.Keys {
				if !seen[key] {
					fields = append(fields, InferredEventField{Name: key, Type: "string"})
					seen[key] = true
				}
			}
			if arc.Value != "" && !seen[arc.Value] {
				fields = append(fields, InferredEventField{Name: arc.Value, Type: "int"})
				seen[arc.Value] = true
			}
		}
	}

	return fields
}
