package metamodel

// Presentation is the model's machine-readable theming: what a generic
// console needs to render this model as well as a hand-written one would.
//
// It is deliberately the smaller half of a pair. Per-element display —
// what to call a place, what unit its tokens are counted in — rides the
// existing Tags channel (`label`, `unit`), because that fact belongs on the
// element it describes and Tags is already the documented place for facts
// the net cannot express. Only `refine.`-prefixed tags participate in
// classification, so naming a place cannot shatter a parameter class. What
// is left over is genuinely about the model as a whole — its title, its
// accent, how its controls group, and which disruptions are worth offering
// as one click — and that is what lives here.
//
// The rule every consumer may rely on: **presentation only ever overrides a
// derived default, and never supplies one.** A console must render a model
// with no Presentation at all, deriving labels from ids, groups from the
// net's own roles, and disruptions from the shapes it already classifies.
// Anything a model can only say here is a feature the generic path is
// missing, not a reason to author configuration — that is the whole
// distinction between theming a console and going back to writing one per
// model.
//
// Disruptions are the one part that carries behaviour, and they carry it in
// exactly the vocabulary a scenario already speaks: a marking override, a
// rate override, a piecewise schedule. Nothing here is a new engine input,
// so a preset can express no run a hand-built request could not, and the
// engine needs no knowledge of presentation to honour one.
//
// omitempty keeps every existing model's marshalled bytes — and therefore
// its content id — unchanged.
type Presentation struct {
	// Title overrides the console heading. Empty means the model's Name.
	Title string `json:"title,omitempty"`

	// Accent is a CSS colour for the console's highlight, as `#rgb` or
	// `#rrggbb`. Validators reject anything else rather than passing an
	// arbitrary string into a stylesheet.
	Accent string `json:"accent,omitempty"`

	// Labels and Units give places and transitions human names and a noun
	// for what their tokens count — "Veterinarians", "walkouts". Keyed by
	// element id; an unnamed element falls back to its id, which is what
	// keeps every model renderable with no theming at all.
	//
	// These live here rather than on each element's Tags for a reason worth
	// recording: Tags is the natural home for a fact about an element, but
	// it is not carried by every representation the model has to survive.
	// Keeping all theming in one block means one thing to encode, one thing
	// to validate, and one thing to strip when a consumer wants the bare net.
	Labels map[string]string `json:"labels,omitempty"`
	Units  map[string]string `json:"units,omitempty"`

	// Groups order and caption the derived controls. A group names element
	// ids; a console renders the named ones together, in this order, under
	// this heading, and renders everything ungrouped afterwards.
	//
	// Grouping may not hide a control. A console that dropped an ungrouped
	// knob would let a model make one of its own parameters invisible, and
	// the standing example — an extra nurse worth -0.1 walkouts at two
	// providers and +8.2 at four — is exactly the knob a modeller would have
	// left out of the groups.
	Groups []ControlGroup `json:"groups,omitempty"`

	// Disruptions are named one-click hypotheticals: "no surgery today",
	// "x-ray machine down", "emergency wave". Each is a scenario fragment,
	// merged into whatever the operator has already dialled in.
	Disruptions []Disruption `json:"disruptions,omitempty"`
}

// ControlGroup captions a set of controls that belong together.
type ControlGroup struct {
	// ID names the group; unique within the presentation.
	ID string `json:"id"`

	// Title is the heading shown above the group.
	Title string `json:"title,omitempty"`

	// Description is optional help text for the group.
	Description string `json:"description,omitempty"`

	// Members are place and transition ids. Every one must exist in the
	// model — a group naming an element the net has not got is an error, not
	// a stale comment, which is what earns this field a place in the hash.
	Members []string `json:"members"`
}

// Disruption is a named scenario fragment: the thing an operator wants to
// ask about that is not a slider. Each field is an override the scenario API
// already accepts, so applying one is a merge and never a special case.
//
// A disruption may set any combination of the three, because the useful ones
// genuinely need more than one: "emergency wave" is a schedule, "x-ray
// machine down" is a marking, and "no surgery today" is a rate — but "the
// surgeon is out and the backlog is already at the door" is two of them.
type Disruption struct {
	// ID names the disruption; unique within the presentation.
	ID string `json:"id"`

	// Label is the control's caption — "x-ray machine down".
	Label string `json:"label,omitempty"`

	// Description says what it does to the run, for the operator who wants
	// to know what they just switched on.
	Description string `json:"description,omitempty"`

	// Marking sets place token counts at the start of the run. Place ids
	// must exist.
	Marking map[string]int `json:"marking,omitempty"`

	// Rates set transition rates for the whole run. Transition ids must
	// exist. A rate of 0 is how a gate is closed.
	Rates map[string]float64 `json:"rates,omitempty"`

	// Schedule sets piecewise rates over the run, keyed by transition id, in
	// the same RateSegment vocabulary a transition uses to declare its own
	// day shape. This is what makes a wave a wave rather than an average —
	// "two an hour between hour one and hour three" and "busier on average"
	// are different questions, and the second cannot tell you whether the
	// queue ever recovers.
	Schedule map[string][]RateSegment `json:"schedule,omitempty"`
}
