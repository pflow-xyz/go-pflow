// Package-level JSON-LD vocabulary for the model schema.
//
// The definitions live HERE, beside the structs they describe, so a field
// and its meaning change in one commit in the public repo — that is the
// difference between an open vocabulary as a fact and as a claim. Serving
// is a consumer concern: sim.pflow.xyz mounts ModelContextDocument at
// ModelContextURL and merges ModelVocabularyTerms into its glossary; any
// other consumer may do the same.

package metamodel

// ModelVocabIRI is the namespace every context term resolves into. It is
// dereferenceable: the host serves a glossary there.
const ModelVocabIRI = "https://sim.pflow.xyz/ns/v1#"

// ModelContextURL is where the model-schema context lives. It is separate
// from the classification context because the two documents describe
// different things: this one defines the VOCABULARY — every term a stored
// model can emit — while the classification context defines the derived
// report. The version lives in the path for the same reason it does there.
const ModelContextURL = "/context/model/v1"

// ModelLDContext defines every term a marshalled Model can emit.
// TestModelContextCoversTheSchema walks the struct tags by reflection and
// fails if a field ships without a definition here — the exact failure the
// classification context once had (terms resolving through @vocab to IRIs
// nobody described, linked data in shape but not in meaning).
//
// Element ids become node IRIs ("id": "@id"), and arc endpoints, view
// projections and player references are @id-coerced, so an expanded model
// is a graph whose arcs point at its places and transitions rather than a
// tree of strings that happen to match.
var ModelLDContext = map[string]any{
	"@vocab": ModelVocabIRI,
	"schema": "https://schema.org/",

	"id":   "@id",
	"name": "schema:name",

	// The net.
	"places":      map[string]any{"@id": "hasPlace"},
	"transitions": map[string]any{"@id": "hasTransition", "@type": "@id"},
	"arcs":        map[string]any{"@id": "hasArc"},
	"from":        map[string]any{"@id": "arcFrom", "@type": "@id"},
	"to":          map[string]any{"@id": "arcTo", "@type": "@id"},
	"weight":      map[string]any{"@id": "arcWeight", "@type": "schema:Integer"},
	// Arc.Type ("inhibitor" | "read") and Place.Type (a data type name)
	// share the JSON key; it maps to a plain property, NOT the @type
	// keyword — an inhibitor arc is not an rdf:type assertion.
	"type": "elementType",
	// Kinetic is the prerequisite-not-accelerant flag: the arc still gates
	// and still consumes, it just leaves the mass-action rate product.
	"kinetic": map[string]any{"@id": "isKinetic", "@type": "schema:Boolean"},

	// Places.
	"initial":  map[string]any{"@id": "initialMarking", "@type": "schema:Integer"},
	"capacity": map[string]any{"@id": "capacity", "@type": "schema:Integer"},
	"kind":     "stateKind",
	"x":        map[string]any{"@id": "displayX", "@type": "schema:Integer"},
	"y":        map[string]any{"@id": "displayY", "@type": "schema:Integer"},
	"exported": map[string]any{"@id": "isExported", "@type": "schema:Boolean"},
	"persisted": map[string]any{
		"@id": "isPersisted", "@type": "schema:Boolean"},
	"resource":      map[string]any{"@id": "isResourcePool", "@type": "schema:Boolean"},
	"initial_value": "initialValue",
	"tags":          map[string]any{"@id": "hasTag"},

	// Transitions.
	"rate":                 map[string]any{"@id": "ratePerHour", "@type": "schema:Number"},
	"guard":                "guardExpression",
	"guardUnrepresentable": map[string]any{"@id": "guardUnrepresentable", "@type": "schema:Boolean"},
	"bindings":             map[string]any{"@id": "hasBinding"},
	"fields":               map[string]any{"@id": "hasField"},
	"emits":                map[string]any{"@id": "emitsEvent"},
	"legacy_bindings":      "legacyBindings",
	"keys":                 map[string]any{"@id": "bindingKeys"},
	"value":                "bindingValue",
	"place":                map[string]any{"@id": "bindsPlace", "@type": "@id"},
	"event":                "eventName",
	"event_type":           "eventType",
	"http_method":          "httpMethod",
	"http_path":            "httpPath",
	"clearsHistory":        map[string]any{"@id": "clearsHistory", "@type": "schema:Boolean"},

	// Transition form fields.
	"label":       "schema:name",
	"placeholder": "fieldPlaceholder",
	"required":    map[string]any{"@id": "isRequired", "@type": "schema:Boolean"},
	"default":     "defaultValue",
	"options":     map[string]any{"@id": "fieldOption"},
	"autoFill":    "autoFillFrom",

	// Events and constraints.
	"events":      map[string]any{"@id": "hasEvent"},
	"constraints": map[string]any{"@id": "hasConstraint"},
	"expr":        "constraintExpression",
	"of":          map[string]any{"@id": "constraintOver", "@type": "@id"},
	"duration":    "duration",
	"minDuration": "minDuration",
	"maxDuration": "maxDuration",

	// Presentation: the prose intent and the screens.
	"view":  "presentationIntent",
	"views": map[string]any{"@id": "hasView"},
	"title": "schema:name",
	"role":  "viewRole",
	// A view's prompt is its share of the presentation intent, addressed
	// to whoever renders the screen.
	"prompt": "viewPrompt",
	"links":  map[string]any{"@id": "linksToView", "@type": "@id"},

	// Theming: the machine-readable half of the presentation intent. Groups
	// caption derived controls and disruptions name one-click hypotheticals;
	// both are overrides of something a console already derives, which is why
	// none of these terms describes behaviour the engine needs to know about.
	"presentation": map[string]any{"@id": "hasPresentation"},
	"accent":       map[string]any{"@id": "accentColor"},
	"groups":       map[string]any{"@id": "hasControlGroup"},
	"disruptions":  map[string]any{"@id": "hasDisruption"},
	"marking":      map[string]any{"@id": "markingOverride"},
	"labels":       map[string]any{"@id": "displayLabel"},
	"units":        map[string]any{"@id": "displayUnit"},

	// The decision layer: objective, players, solver.
	"simulation": map[string]any{"@id": "hasSimulation"},
	"objective":  "objectiveExpression",
	"players":    map[string]any{"@id": "hasPlayer"},
	"maximizes":  map[string]any{"@id": "maximizesObjective", "@type": "schema:Boolean"},
	"turnPlace":  map[string]any{"@id": "turnPlace", "@type": "@id"},
	"solver":     map[string]any{"@id": "hasSolverConfig"},
	"rates":      map[string]any{"@id": "rateOverrides"},
	"tspan":      map[string]any{"@id": "timeSpan", "@container": "@list"},
	"dt":         map[string]any{"@id": "timeStep", "@type": "schema:Number"},

	// Phase-type durations: Erlang-k service declared on a transition.
	"stages": map[string]any{"@id": "erlangStages", "@type": "schema:Integer"},

	// Declared demand pattern: piecewise-constant rate over model time.
	"schedule": map[string]any{"@id": "rateSchedule", "@container": "@list"},
	"until":    map[string]any{"@id": "segmentUntil", "@type": "schema:Number"},

	// Declared parameters: structural decision variables (arc weights,
	// capacities) the model names as choices rather than facts.
	"parameters": map[string]any{"@id": "declaresParameter"},
	"min":        map[string]any{"@id": "parameterMin", "@type": "schema:Integer"},
	"max":        map[string]any{"@id": "parameterMax", "@type": "schema:Integer"},

	// Asserted classes: modeller simplifications, re-checked not adopted.
	"assertedClasses": map[string]any{"@id": "assertsClass"},
	"members":         map[string]any{"@id": "hasMember", "@type": "@id"},
	"note":            "schema:description",

	// Display metadata.
	"description": "schema:description",
	"version":     "schema:version",
	"decimals":    map[string]any{"@id": "tokenDecimals", "@type": "schema:Integer"},
	"unit":        "schema:unitText",
}

// ModelContextDocument is what ModelContextURL serves.
func ModelContextDocument() map[string]any {
	return map[string]any{"@context": ModelLDContext}
}

// ModelVocabularyTerms extends the glossary with the model-schema terms
// whose meaning is load-bearing — the ones a consumer must not guess at.
func ModelVocabularyTerms() map[string]string {
	return map[string]string{
		"initialMarking":      "tokens the place holds before anything fires",
		"capacity":            "a post-firing upper bound, netting out what the same firing consumes; zero means unbounded, not a bound of zero",
		"arcWeight":           "tokens the arc moves (or, on an inhibitor, the count at which it blocks; on a read arc, the count it requires)",
		"elementType":         "on an arc: 'inhibitor' blocks at >= weight, 'read' requires >= weight and consumes nothing; on a place: the data type held",
		"isKinetic":           "false marks a prerequisite that is not an accelerant: the arc still gates enablement and is still consumed, but leaves the mass-action rate product",
		"ratePerHour":         "mean firings per hour when continuously enabled; rate 0 declares a decision fired only by choice; rate >= 100 is the instant-pickup idiom calibration never overwrites",
		"hasTag":              "declared key/values; 'outcome: loss|served' overrides outcome inference on a terminal place, 'refine.*' keys seed parameter-class refinement",
		"presentationIntent":  "what an application rendering this model should be and do, as prose to whoever generates it",
		"hasView":             "one screen of the application: a validated projection of places and transitions, with a role, a prompt, and links to the views it can navigate to",
		"viewRole":            "board (act on the state), dashboard (watch it), controls (turn the knobs), advisor (derived recommendations), trust (invariants and anchors), log (event history)",
		"linksToView":         "the navigation graph: views as nodes, links as edges",
		"objectiveExpression": "a numeric expression over the marking that players maximize or minimize and the advisor prices decisions against",
		"maximizesObjective":  "true: this player maximizes the objective; false: minimizes",
		"turnPlace":           "the place whose token marks this player's turn",
		"rateOverrides":       "per-transition rate overrides in the solver map — where calibration writes learned rates, leaving declared rates untouched",
		"assertsClass":        "a modeller's claim that members share one parameter; tools re-check it and report what the simplification costs",
		"declaresParameter":   "a structural decision variable: the arc weights (moved together) or place capacity it binds are a choice, not a fact; the bound element's current value is the base",
		"erlangStages":        "the transition's duration is the sum of this many exponential stages at stages x rate: same mean, variance divided by stages; 0 and 1 both mean plain exponential",
		"rateSchedule":        "the transition's rate as piecewise-constant segments over model time — the declared day shape; the last segment holds to the horizon, and a scenario's overrides still win",
	}
}
