# PNML vs. go-pflow: Feature Comparison Report

## Executive Summary

go-pflow's `metamodel` and `petri` packages cover roughly the structural core of ISO/IEC 15909-2 (places, transitions, arcs, initial markings, weights) solidly, plus a good chunk of the ISO/IEC 15909-3:2021 enrichments (inhibitor arcs, read/test arcs, capacity places) — but stop well short of PNML's two most architecturally distinctive ideas: (1) the **open, generic label mechanism** that lets any tool attach arbitrary typed content to any object without touching a core schema, expressed concretely via nested pages, reference nodes, and a hierarchy of Petri-net-type-definitions (PT-Net ⊂ Symmetric Net ⊂ HLPNG); and (2) the **algebraic high-level/colored-token type system** (declared sorts, partitions, multisets, terms over bound variables, the `all` broadcast operator). Where PNML defers entirely — continuous/stochastic semantics, timed transitions beyond an abstract concept, structured module composition, verification, event sourcing — go-pflow has built substantial, often more rigorous, machinery of its own.

**The single biggest structural difference in philosophy**: PNML is a **lowest-common-denominator XML interchange format**, designed so independent tools (TAPAAL, CPN Tools, PIPE, GreatSPN) can exchange nets by degrading gracefully — generic labels, opaque `toolspecific` escape hatches, and a formal PNTD hierarchy that lets a reader ignore what it doesn't understand while still parsing the core skeleton. go-pflow is the opposite: **one closed, statically-typed Go schema with no escape hatch** — every extension (delay, stages, schedule, guard, capacity, arc kind) is a named, versioned field validated at load time, and an unrecognized value (e.g., an unknown `ArcType`) is a **hard error** rather than something silently degraded or ignored. go-pflow trades PNML's tool-agnostic interoperability for internal consistency, content-addressable hashing, and a JSON-LD/schema.org serialization that is itself part of an ecosystem (petri-pilot, ODE/SSA solvers, event sourcing) rather than a neutral exchange point between competing tools.

---

## A. Core Structure & Framework

### XML structure / graph skeleton

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| `<pnml>` root, multi-`<net>` document | One document may contain several independent nets, possibly of different types | PARTIAL | `metamodel.Bundle{Subnets []Subnet}` (`metamodel/compose.go:208-231`) is the nearest analogue, but subnets are meant to be *linked/composed* via `Bundle.Links`/`Flatten`, not merely co-filed independent nets. A bare `Model` is always exactly one net. |
| `<net>` with `type` URI (PNTD selection) | Each net declares which grammar/semantics package governs its labels | PARTIAL | `metamodel.NetType` (`compose.go:42-56`: `WorkflowNet`, `ResourceNet`, `GameNet`, `ComputationNet`, `ClassificationNet`) classifies a `Subnet` for link-legality purposes (`compose_matrix.go:16`) only — it never changes the schema itself; every `Model` has the same fixed struct regardless of `NetType`. |
| `<page>` / arbitrary subpage hierarchy | Pages nest arbitrarily; a page is itself an object | ABSENT | No containment concept over one graph's places/transitions. `ViewDecl` (`schema.go:99-124`) projects a subset for presentation only (no containment semantics, doesn't affect firing); `Bundle`/`Subnet` composition is flat (one level) and boundary-based (`Port`s), not nested pages. |
| `<place>` | Place node with an ID | FULL | `metamodel.Place` (`schema.go:179-238`); `petri.Place` (`petri/net.go:13-20`). |
| `<transition>` | Transition node with an ID | FULL | `metamodel.Transition` (`schema.go:289+`); `petri.Transition` (`petri/net.go:53-61`). |
| `<arc>` (source/target IDREF, same-page, PT-net bipartite constraint) | Directed edge; PT-nets forbid place↔place/transition↔transition | PARTIAL | `Arc{From,To string}` (`schema.go:494-529`) / `petri.Arc{Source,Target string}` (`petri/net.go:78-84`) give the basic edge; bipartiteness is **not statically enforced on the type** — it emerges only behaviorally through `Inputs`/`Outputs`/`Tests` filtering (`firing.go:81-116`). Read-arc direction specifically *is* checked (`ValidateArcs`/`ErrReadArcDirection`, `validation.go:52-58`), but normal/inhibitor arcs place→place would pass unchecked. |
| `<referencePlace>`/`<referenceTransition>` | Semantics-free proxy nodes for cross-page arcs; flattening removes them losslessly | ABSENT | Nearest relative is `Link`/`Endpoint` (`compose.go:145-176`) for cross-*subnet* wiring, but it is semantically rich (four typed link kinds) rather than a semantics-free stand-in, and it fuses/gates elements on `Flatten` rather than losslessly preserving arc-drawing. |

### Labels

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Open/generic label mechanism | Core imposes no restriction on what labels appear | ABSENT (by design) | Closed, statically-typed Go schema (`metamodel/schema.go`) — a new "label" means a new Go field, not attached arbitrary XML. |
| `<name>` (display name distinct from ID) | Any object may carry a display name separate from its ID | PARTIAL | `petri.Place.LabelText`/`Transition.LabelText` (`petri/net.go:19,60`) are optional display overrides; `metamodel.Place`/`Transition` have no separate display-name field — only `ID` (doubles as name) and `Description` (`schema.go:180-181,290-291`). |
| `<text>` vs `<structure>` (concrete syntax vs. AST) | A label may carry a plain string and/or an abstract syntax tree, for tool interchange | PARTIAL | Guard/objective expressions (`Transition.Guard`, `Simulation.Objective`, `schema.go:292,136`) are always concrete-syntax strings; an internal AST exists post-parse (`stochastic/compiled.go`) but is never itself serialized as a second, exchangeable representation. |
| Annotation vs. attribute label kinds | Categorical distinction between text-rendering and shape-affecting labels | N/A | Not meaningful outside a generic-label framework; fields are already typed. |
| Global (net-level) labels | The net itself can carry labels | FULL | `Model.Description`, `.View`, `.Presentation`, `.Parameters`, `.AssertedClasses`, `.Constraints`, `.Events` (`schema.go:26-96`). |
| `<graphics>` (position/dimension/fill/line/font) | Rich visual metadata on nodes, arcs, annotations | PARTIAL | Only `X,Y int` on `Place`/`Transition` (`schema.go:236-237,392-393`; `petri/net.go:16-17,58-59`). No dimension/fill/line/font/bend-points anywhere; `Arc` has **no** position data at all. |
| `<toolspecific>` (arbitrary opaque tool metadata on any object) | Any tool attaches self-contained extension data to any object | PARTIAL | `ModelExtension`/`ExtendedModel` (`metamodel/extension.go:15-27,92-98`) is structurally similar (named, versioned, ignorable) but attaches only at whole-`Model` granularity, never per-place/per-transition/per-label. |

### IDs and referencing

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Unique `ID`/`IDRef` typing, document-scoped | XML `ID`/`IDREF` statically guarantee uniqueness and valid references | PARTIAL | `Place.ID`/`Transition.ID` are plain strings (`schema.go:180,290`); lookup is linear-scan first-match (`PlaceByID`, `TransitionByID`, `clone.go:3-20`) — **duplicate IDs on a bare `Model` are not rejected**, the second is silently unreachable. Arc endpoints are unchecked string refs (`ValidateArcs` checks arc-type legality and read-arc direction, not that `From`/`To` resolve). Uniqueness *is* checked one level up: `Bundle.Validate` flags `ErrDuplicateID`/`ErrDuplicateSubnet`/`ErrDuplicatePort` across subnets being flattened (`compose.go:440,484,505,546,552`). |

### Petri Net Type Definition (PNTD) genericity

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| PNTD hierarchy: PT-Net ⊂ Symmetric Net ⊂ HLPNG, each a machine-checkable extension | Different net "dialects" formalized as separate, checkable extensions of one Core Model | PARTIAL | `NetType` names a net's kind for composition-legality only; it never changes what fields/labels are legal on `Model` — one schema for every `NetType`, unlike PNML's genuinely different label sets per PNTD. |
| Distributable schema artifact per net type (RELAX NG `.pntd`) | Independent tools validate against a published grammar file | ABSENT | No distributable schema-description file; "the schema" is the Go struct definitions plus ad hoc `Validate*` functions, not a declarative artifact a third-party tool could check independently of the Go compiler. |
| PT-Net: `<initialMarking>` (default 0) + `<inscription>` (default 1) | The classic dialect | FULL | `Place.Initial int` (default zero, `schema.go:182`); `arcWeight()` defaults unset `Arc.Weight` to 1 (`firing.go:59-64`). |
| High-Level Core Structure: sorts/types, `<hlinitialMarking>`/`<hlinscription>` terms, `<condition>`, declared variables | Tokens are typed values; arcs carry terms; transitions carry boolean guard terms | PARTIAL | `petri.ExpandColors`/`ColorMap` (`petri/colors.go:1-304`) gives fixed-cardinality, named-vector colors — not an arbitrary sort/term/variable system. `Transition.Guard string` (`schema.go:292`) is the `<condition>` analogue, in go-pflow's own guard DSL rather than a term over declared sorts. No net-level `<declaration>`s of variables/user sorts. |
| Symmetric Nets (finite carrier sets, `all` operator) | Restricted high-level dialect | ABSENT | No such formalism. |
| HLPNGs (arbitrary user-declared sorts/operators) | General high-level dialect | ABSENT | No term/sort declaration system. |

### go-pflow-only capabilities noted in this section
`ReadArc` as a first-class base type with enforced canonical direction; `Place.Capacity` as a *post-firing* net-effect bound; `Arc.Kinetic` (mass-action vs. prerequisite-only arcs); `Delay`/`Stages`/`Schedule` timed-transition declarations; `Gating()` self-reporting what a continuous engine can't represent; JSON-LD/schema.org canonical serialization; event-sourcing fields (`Event`, `Emits`); a full guard expression language; typed `Bundle`/`Link` composition with a legality matrix; `Parameters`/`AssertedClass`; `Presentation`/`Views`; ODE/SSA/reachability/process-mining subsystems. (Full detail in the consolidated "go-pflow beyond PNML" section below.)

---

## B. Place/Transition Nets (ISO/IEC 15909-2)

go-pflow has **two** P/T-net representations: `petri.PetriNet` (float/vector-valued, ODE/SSA-oriented) and `metamodel.Model` (int-valued, application/analysis-oriented, JSON-LD serialized). Both are cited per row.

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Place initial marking (`<initialMarking>`) | Core; natural number M(p) | FULL | `petri.Place.Initial []float64` (`petri/net.go:16`); `metamodel.Place.Initial int` (`schema.go:182`), consumed by `InitialMarking()` (`firing.go:51-59`). Superset of PNML's single nonneg integer (supports color vectors). |
| Place capacity | Not in the ISO/IEC 15909-2 core standard at all | N/A vs. core (see §D for the ISO 15909-3:2021 extension) | `petri.Place.Capacity []float64`; `metamodel.Place.Capacity int`, a **post-firing** bound (0 = unbounded), enforced in `firing.go:182-195`. |
| Transition identity (`id`, `<name>`) | Generic PNML object attrs | FULL | `petri.Transition.Label`; `metamodel.Transition.ID`/`.Description` (`schema.go:290-291`). |
| Transition priority | Not core; extension-only, never shipped as a Part 2/3 PNTD | ABSENT | No priority field on `Transition`; `Enabled`/`Fire`/`EnabledTransitions` apply no priority filtering (`firing.go:137-225`). (See §D — every `Priority` symbol elsewhere in the repo belongs to unrelated actor-bus/SLA layers.) |
| Transition rate/timing (GSPN-style, not core PNML) | Not core; belongs to non-standard net classes | FULL, richer than the non-standard comparison point | `Transition.Rate` (mass-action); `.Schedule` (piecewise-constant); `.Stages` (Erlang-k); `.Delay` (deterministic) — executed by the Gillespie SSA in `stochastic/`. |
| Arc weight/inscription (`<inscription>`) | Core; W(f) ≥ 1 | FULL | `petri.Arc.Weight []float64` (default 1.0 via `GetWeightSum()`); `metamodel.Arc.Weight int` (default 1, `arcWeight()`). |
| Arc direction / bipartite P↔T constraint | Core; place↔transition alternation | PARTIAL | Both `petri.Arc` and `metamodel.Arc` are plain string-ID pairs with no compile-time P/T alternation; `metamodel.Validate`/`ValidateArcs` enforces direction semantically only for the arc kinds that need it (notably `ReadArc`, `schema.go:470-473`), not the general bipartite invariant. |
| Inhibitor arc | Extension PNTD (`inhibitorptnet.pntd`) — see §D for ISO 15909-3:2021 status | FULL | `petri.Arc.InhibitTransition bool`; `metamodel.ArcType = InhibitorArc` (`schema.go:458-464`), enforced in `enablement()` (`firing.go:159-164`). |
| Read/test arc | Ad hoc `specialarcs.rng` convention historically; see §D for ISO 15909-3:2021 | FULL at `metamodel` layer / PARTIAL at `petri` layer | `metamodel.ArcType = ReadArc` (`schema.go:466-477`), enforced by `Tests()`/`enablement()` (`firing.go:109-121,165-169`). `petri.Arc` has **no dedicated read flag** — per repo convention it is encoded as a reversed inhibitor arc (transition→place), a lossless but indirect encoding. |
| Reset arc (empties a place unconditionally on firing) | Extension PNTD (`resetptnet.pntd`); see §D | ABSENT | `arcTypes` registers only `NormalArc`, `InhibitorArc`, `ReadArc` (`schema.go:484-488`); an unrecognized type is a hard validation error (`IsKnownArcType`, `schema.go:490-491`); `readarc_test.go:91-107` explicitly asserts `ArcType("reset")` is rejected. |
| Reset+Inhibitor combined net | Extension PNTD (`resetinhibitorptnet.pntd`) | ABSENT | Follows directly from reset arcs being unsupported. |
| `<graphics>` / diagram layout | Core, purely presentational | N/A (analogue exists, not semantically comparable) | `X,Y` on `Place`/`Transition` in both packages. |
| `toolspecific` extension | Core, arbitrary opaque tool metadata | PARTIAL | `Place.Tags`/`Transition.Tags map[string]string` (`schema.go:206-233,395-416`) serve a similar annotation role but are **semantically active** (a `refine.*`-prefixed key drives colour-refinement analysis) rather than opaque — a narrower/different concept. |

### Summary of ABSENT items in this area
Transition priority; reset arcs; reset+inhibitor combined nets — all consistent with §D's fuller ISO 15909-3:2021 treatment below.

---

## C. High-Level / Symmetric Nets (colored tokens, guards, arc expressions)

go-pflow has **no formal high-level/symmetric-net type system**: no sorts, no declared variables, no terms, no partitions/enumerations-as-sorts, no `all` broadcast operator, no product sorts, no multiset algebra. What it has instead is a **positional, per-color vector unfolding** (`petri.ExpandColors`) achieving the practical effect of a small, fixed, uniform color scheme — closer to "manually enumerate the ground instances" than to "declare a sort and let the tool derive instances from bound variables." (Note: this is the same `ExpandColors` mechanism flagged in §B as a "go-pflow-only capability" from the P/T-net vantage point — from the high-level-net vantage point it is properly a *partial* analogue of PNML's colored-token machinery, not something wholly outside PNML's scope. Both framings are correct; they differ only in which PNML net class is the comparison point.)

| # | Feature | PNML Description | Verdict | Evidence |
|---|---|---|---|---|
| 1 | Three-tier hierarchy (P/T-Net ⊂ SN ⊂ HLPNG) | Nested type restriction; each class a syntactic subset of the general HLPNG | ABSENT | No net-class hierarchy or conformance levels; `metamodel.Model` and `petri.PetriNet` are each one flat schema used for everything. |
| 2 | Dot sort (trivial 1-element sort) | Encodes uncolored nets as degenerate HLPNGs | N/A | go-pflow's default scalar shapes *are* the uncolored case natively, not a declared restriction of a richer type. |
| 3 | Boolean sort | Mandatory sort of any guard/condition term | PARTIAL | Guard results must be boolean (`tokenmodel/guard/eval.go:411-426`, `guard.go:82-88`), but no declared Boolean sort usable for a place or arc-expression type exists. |
| 4 | Finite integer ranges | Sort spanning `1..n` | ABSENT | `Parameter.Min/Max` (`metamodel/parameters.go:32`) bounds a structural decision variable, not a token color's value range. |
| 5 | Finite enumerations | User-named finite ordered set | PARTIAL | `PetriNet.Token []string` (`petri/net.go:116`) names a fixed list of colors — closest analogue — but it's a single flat list shared net-wide, no per-place sort, no order relation exposed to guards/arcs. |
| 6 | Cyclic enumerations | Wraparound successor/predecessor | ABSENT | `Token` names are opaque vector-index labels only (`petri/colors.go:82-89`). |
| 7 | Partitions (the SN symmetry mechanism) | Enumeration split into named sub-ranges, each its own sort | ABSENT | Nearest relative, `Model.AssertedClasses` (`schema.go:65-76,243-249`), groups *model elements* for parameter-reduction, not color-domain elements for arc-expression symmetry. |
| 8 | Multisets (basis of arc inscriptions/markings) | The sort of multisets over any basis sort | PARTIAL | Per-color vectors (`Place.Initial []float64`, `Arc.Weight []float64`) are a positional/unfolded equivalent — index *i* holds the multiplicity of color *i* — but no manipulable multiset value exists (no union/difference/scalar-multiply, no literal syntax). |
| 9 | Product/tuple sorts | Cartesian-product sorts | ABSENT | Nothing constructs a compound color type; `Place.Type` (`schema.go:184`) names a Go/JSON data type for `DataKind` places — a different mechanism (arbitrary structured state, not a sort composed with others). |
| 10 | Integer/String/List sorts (HLPNG only) | Unrestricted arithmetic ints, strings, lists | PARTIAL | `Place.Type` (`"string"`, `"int64"`, `"map[...]"`, `schema.go:251-286`) and the guard grammar support these dynamically, but as Go's native types via a small expression grammar, not a formal declared sort algebra. |
| 11 | User-defined ("named") sorts | Modeller declares a new sort from existing ones | ABSENT | No sort-declaration mechanism; `Type` is a free-text string interpreted by codegen, not a declared/checked sort. |
| 12 | Terms (variables + operators, sort-derived) | Underlies arc inscriptions, guards, markings | PARTIAL | `tokenmodel/guard`'s expression language is a genuine term language (literals, ops, index/field access, calls) but has no sort-derivation/checking, no declared variables, and is used only for guards/objectives/invariants — never arc inscriptions. |
| 13 | Global variable declarations | Net/page-level labels shared across arcs/guards | ABSENT | Guard bindings (`Context.Bindings`) are supplied ad hoc per evaluation call; no declared, typed, model-level variable object. |
| 14 | Multiset expressions on arcs (term-derived token counts per firing mode) | General mechanism deriving counts from bound variables | PARTIAL | `ExpandColors` derives a fixed integer weight per color statically, not from a term evaluated against a firing-mode binding; no mode-selection step exists. |
| 15 | `all` (broadcast) operator | Built-in operator yielding one copy of every element | ABSENT | No such operator anywhere (confirmed by grep — only unrelated actor-bus/dataflow "broadcast" concepts exist). |
| 16 | Initial markings as ground multiset terms | No free variables; same sort as arc inscriptions | PARTIAL | `Place.Initial`/`petri.Place.Initial` are ground numeric data (matching "no free variables") but literal vectors, not evaluated terms of a multiset sort — no term language for markings exists. |
| 17 | Guards / transition conditions | Boolean term additionally restricting firing | FULL (enablement semantics) / PARTIAL (formality, and scope — see reconciliation below) | `Transition.Guard string` (`schema.go:292-309`) layers on top of the four structural firing rules; evaluated via `stochastic/markingguard.Eval`, but **only in engines that plug a `GuardFunc` in** — the core `Enabled`/`Fire` never evaluates `Guard` itself (see §D reconciliation). |
| 18 | P/T-Net guard restriction (`condition == true` always) | Degenerate trivial-guard case | N/A | An unset `Guard` (`""`) behaves as always-true but isn't framed as a sort-system restriction. |
| 19 | Place types/sorts (declared color domain per place) | Marking is a multiset over the place's declared sort | PARTIAL | `Place.Kind`/`Place.Type` gives a binary distinction (token-count vs. one opaque data blob), not an arbitrary declared finite sort; per-color vectors apply uniformly net-wide via a shared `Token` list, not per-place. |
| 20 | SN restriction: place sort ≠ multiset sort | OCL invariant against nested multisets | N/A | No sort system in which this nesting could be expressed. |
| 21 | P/T-Net restriction: every place sort = Dot | Forces plain-integer marking | N/A | go-pflow's scalar `Place.Initial int` is this shape by default, with no restriction mechanism needed to get there. |
| 22 | Pages/subpages (hierarchical containment) | Nested display/organization + scoping | ABSENT | No page/canvas nesting; `Model` is flat. |
| 23 | Reference places/transitions (proxy nodes, flattening semantics) | Lets an arc reach a node on another page; cyclic refs banned | PARTIAL (different mechanism, same goal) | `Bundle` `Port`/`Link` + `Flatten` achieve cross-boundary reference + flattening, but by equivalence-class fusion (typed by `NetType` legality) rather than untyped node-proxy substitution. |
| 24 | Module/interface concept (Part 3 territory, absent from Part 2 core) | No standardized module system in the core PNML this section targets | PARTIAL, arguably exceeds it | `Bundle`'s four typed `LinkKind`s + `NetType` legality matrix is a genuine typed composition/interface system that Part 2 explicitly lacks. |
| 25 | Inhibitor arcs (ISO 15909-3 enrichment) | See §D | FULL | `InhibitorArc` (`schema.go:461-464`), `firing.go:159-163`. |
| 26 | Reset arcs (ISO 15909-3 enrichment) | See §D | ABSENT | Not in `ArcType` (`schema.go:452-478`). |
| 27 | Read/test arcs (ISO 15909-3 enrichment) | See §D | FULL | `ReadArc` (`schema.go:466-478`), `firing.go:109-121,165-169`. |
| 28 | Capacity places (ISO 15909-3 enrichment) | See §D | PARTIAL (own semantics) | `Place.Capacity` — a **post-firing** net bound, not a marking cap (a capacity-2 place at 2 still admits a consume-1-produce-1 firing, `firing.go:130-133,173-195`) — a deliberate divergence from conventional pre-firing capacity checks. |

---

## D. Time, Stochastic, Priority, Reset/Inhibitor Extensions, Hierarchy, Tool Interop

### Reconciling the ISO 15909-3:2021 status of inhibitor/read/reset/capacity

Sections A–C characterize inhibitor/read/reset/capacity arcs as informal, vendor-specific PNML extensions (e.g. `inhibitorptnet.pntd`, an ad hoc `specialarcs.rng` convention). This section's research instead identifies **ISO/IEC 15909-3:2021** as the formal standardization point for exactly these four enrichments (§5.2.2 inhibitor, §5.2.3 reset, §5.2.4 read, §5.2.5 capacity). Both are accurate descriptions of the same underlying facts at different points in the standard's history — informal/tool-specific PNTDs existed first, ISO 15909-3:2021 later formalized the same four constructs as a generalized "enrichment process." This report adopts the ISO 15909-3:2021 framing as authoritative going forward. It does **not** change any go-pflow-side verdict: inhibitor and read arcs remain FULL, reset arcs remain ABSENT, and capacity remains a deliberate structural divergence (post-firing bound vs. the standard's pre-firing marking cap) — consistent across every section that touches them.

### Reconciling guard-evaluation scope

Section C notes the enablement semantics of `Transition.Guard` as FULL while flagging its formality as PARTIAL. This section adds the more important scope caveat: **the base firing rule (`Enabled`/`Fire`) never evaluates `Guard` itself** — guard evaluation is opt-in, supplied only by consumers that plug a `GuardFunc` in (e.g., `stochastic.Options{Guard: ...}`, `stochastic/markingguard`). A caller exploring reachability via the core `metamodel` API alone will not see guards enforced. This is not a contradiction of Section C's FULL/PARTIAL split — it is the reason for the split, made explicit: the *feature* (a boolean term gating firing) is real and correctly implemented, but it is not universal across every consumer of the base model the way PNML's spec would mandate for every conformant engine.

### 1. Time / stochastic nets

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Standardized timed-net PNTD | No published ISO PNTD for timed nets | N/A | — |
| Deterministic firing delay | Abstract "Petri net with time" concept only, no interchange schema | FULL | `Transition.Delay` (`schema.go:367-386`); validated by `ValidateDelays` (`metamodel/delay.go:11-23`); scheduled by the SSA engine (`stochastic/stochastic.go:1201-1204,1277,1297-1302,1572-1593`). |
| Phase-type / Erlang-k duration | No PNML equivalent | go-pflow-only | `Transition.Stages` (`schema.go:353-365`); `ExpandStages` (`metamodel/stages.go:58-200`). |
| Piecewise-constant scheduled rate | No PNML equivalent | go-pflow-only | `RateSegment`/`ScheduledRate` (`metamodel/schedule.go:1-81`). |
| Interval/age-based timed arcs (TAPN) | TAPAAL's proprietary PNML-extension dialect | ABSENT | No per-token age, no arc-level time window anywhere. |
| Descriptive SLA min/max duration | N/A (application layer) | PARTIAL/decorative | `Transition.MinDuration`/`MaxDuration` (`schema.go:332-335`) are opaque strings never read by the firing rule or ODE/SSA engines — workflow-SLA metadata only, consumed solely by fusion conflict-checks (`compose_fuse.go:178,349-362`). |
| Stochastic (CTMC/Gillespie) semantics | No standard PNTD | FULL, and more rigorous | Direct-method SSA with an explicit ODE-consistency contract (`stochastic/stochastic.go:1-30`); portable PRNG for cross-language parity (`stochastic/portable.go`) — a concern PNML never addresses. |
| Guard-gated firing | Not part of core PNML; not standardized as an open grammar | go-pflow-only | `Transition.Guard`/`GuardUnrepresentable` (`schema.go:292-309`); pluggable `GuardFunc` (`stochastic/guard.go:1-21`); default evaluator (`stochastic/markingguard/guard.go:1-40`). Scope caveat above applies. |
| ODE/mass-action continuous simulation | Not a PNML concept | go-pflow-only | `solver/ode.go`; `Gating()` (`firing.go:227-317`) enumerates every construct (reads, inhibitors, capacity, guards, delay, stages) a continuous solver cannot represent. |

### 2. Priority

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Transition priority affecting the enabling rule | Formalized in ISO 15909-1:2019 math; academic PNML extension proposed, never shipped as a Part 2/3 PNTD | ABSENT | No priority field on `Transition`; `enablement()`/`EnabledTransitions` treat every enabled transition identically — declaration order is used only for determinism, not priority (`firing.go:137-225,215-225`). |
| "Priority" symbols found in the repo | — | False positive / different concept | `actor/bus.go:58-73` (`SubscribeWithPriority`, message-bus dispatch order); `workflow/types.go:81-88`, `workflow/engine.go:113-185` (SLA/case priority) — neither touches the firing rule. |
| Priority composed with inhibitor arcs | Illustrative note in ISO 15909-3 §5.3 | N/A | Moot; priority itself is absent. |

### 3. Reset/inhibitor extensions

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Inhibitor arcs | ISO 15909-3:2021 §5.2.2 | FULL, and more general (arbitrary weight threshold, not just ≥1) | `InhibitorArc` (`schema.go:458-464`); `firing.go:158-164`; `petri/builder.go:75-76`. |
| Read/test arcs | ISO 15909-3:2021 §5.2.4 | FULL | `ReadArc` (`schema.go:466-477`), canonical place→transition direction validated (`validation.go:54`, `ErrReadArcDirection`). |
| Capacity places | ISO 15909-3:2021 §5.2.5 | FULL, with a deliberate semantic divergence (post-firing net-effect bound, not a raw marking ceiling) | `Place.Capacity` (`schema.go:191-204`); `firing.go:173-196`. |
| Reset arcs | ISO 15909-3:2021 §5.2.3; classical Reset nets are Turing-complete | ABSENT | `arcTypes` lists only `NormalArc`, `InhibitorArc`, `ReadArc` (`schema.go:451-491`); unknown types hard-error. No "empty this place" semantics anywhere. |
| Transfer arcs | Occasionally cited alongside reset arcs (`ArcNature` enumeration) | ABSENT | Same `arcTypes` map; grep confirms no `TransferArc` concept. |
| Unknown-arc-type safety | Not enforced at PNML's execution-engine level — an unrecognized label is a schema-validation failure, not a runtime guard | go-pflow-only design point | `IsKnownArcType` hard-fails (`schema.go:480-491`) — explicitly engineered against the failure mode `toolspecific` permits: "an older reader silently treats a type it has never heard of as a normal consuming arc, turning a constraint into token theft." |
| Non-kinetic input arcs (gate without scaling the rate law) | No PNML equivalent | go-pflow-only | `Arc.Kinetic *bool` (`schema.go:500-522`); carried through to rate engines (`firing.go:39-48`). |

### 4. Hierarchy / modularity

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| Page nesting (visual/organizational only, arcs same-page, flattens losslessly) | Core Model concept since Part 2 | ABSENT | No page/canvas nesting independent of semantics anywhere. |
| Reference nodes (`referencePlace`/`referenceTransition`, semantically inert IDREF stand-ins) | Part 2 mechanism | PARTIAL analogue | `Port`/`Endpoint` addressing serves a similar cross-boundary role, but a `Link` **actually changes firing/marking behavior** on flatten — not semantically inert like a PNML reference node. |
| Module concept (typed interface + implementation + instantiation algebra) | Only in ISO 15909-3:2021 §7; absent from Part 2 | FULL, independently convergent | `Subnet` (implementation) + `Port` (typed interface: in/out/inout/observe) + `Bundle.Links` (instantiation) + `Flatten` (the homomorphism) — plus a `NetType` type discipline PNML's module concept lacks. |
| Legality/type-checking of composition | Not part of ISO 15909-3's module algebra | go-pflow-only | Legality matrix in `compose_matrix.go`; `validateNetType`/`validateLink` (`compose.go:593-618`). |
| GuardLink lowering to structural arcs | Illustrated only, not given a lowering algorithm, in ISO 15909-3 §5.3 | go-pflow-only | `LoweringAuto/Expr/Structural/Inhibitor` (`compose.go:159-173`); `compose_guard.go:22-23`. |
| Arc-merge policy on fusion (`sum`/`max`) | Not addressed — PNML flattening never merges distinct arcs | go-pflow-only | `ArcMergePolicy`, `MergeSum`, `MergeMax` (`compose.go:195-205`). |

### 5. Tool interoperability / serialization

| Feature | PNML Description | go-pflow Verdict | Evidence |
|---|---|---|---|
| XML interchange with ISO-track standardization + multi-implementer conformance | pnml.org tools: ePNK, PNML Framework, ProM, Tina, Wolfgang, etc. | N/A — deliberate different design choice | — |
| JSON-LD/schema.org as the native interchange format | No PNML equivalent | go-pflow-only | Full `@context` mapping every field (`metamodel/jsonld.go:33-161`), including timed/stochastic extensions; dereferenceable vocabulary with reflection-enforced coverage (`jsonld.go:170-193`). |
| Content-hash/model-id stability as a formal interop contract | Not a PNML concern | go-pflow-only | Model IDs are SHA-256 of canonical JSON; petri-pilot hash-pins generated apps against it (`schema.go:229-233,412-416,508-511`). |
| `toolspecific`-style opaque escape hatch | Core PNML mechanism, but produces non-interoperable dialects in practice | ABSENT by design | Every extension is a named, typed, versioned field, validated rather than passed through opaquely; unknown `ArcType` hard-fails rather than degrading (`schema.go:480-491`) — the opposite of `toolspecific`'s silently-ignore contract. |
| `Gating()` (declares what a consumer class cannot represent) | No PNML equivalent — the absence of this is exactly why TAPAAL/GreatSPN/CPN-Tools PNML dialects are mutually unreadable | go-pflow-only | `firing.go:227-317`. |
| Event-sourcing (state as fold over an immutable log) | No PNML equivalent | go-pflow-only | `eventsource/event.go:1-30`; `State(t) = fold(apply, initialState, events[0..t])`. |

---

## Notable Gaps — Ranked by Interop Impact

PNML capabilities go-pflow has **no path to today**, ordered by how much each would matter if go-pflow ever built a PNML import/export layer targeting mainstream tools (TAPAAL, CPN Tools, PIPE, GreatSPN):

1. **High-level/Symmetric-Net sort & term algebra** (declared sorts, partitions, multisets-as-values, terms over bound variables, the `all` operator). This is the single largest blocker: any CPN Tools or Symmetric Net model imported today could at best be approximated by go-pflow's fixed per-color vector unfolding (`petri.ExpandColors`), which cannot represent user-declared sorts, product/tuple types, or variable-bound firing modes. Round-tripping such a model would be lossy in both directions.
2. **Pages/subpages with semantics-free reference nodes.** CPN Tools models are routinely hierarchical; PIPE and others also use pages for organization. go-pflow's `Bundle`/`Port`/`Link` composition is not a substitute because it is semantically active on flatten (fusion, guard-lowering, arc-merge policy) rather than a lossless visual convenience — importing a page-hierarchical PNML file would force either flattening (losing structure) or an ad hoc reinterpretation as a `Bundle` (changing behavior).
3. **Generic `toolspecific` / opaque-label escape hatch.** Without it, any PNML file carrying vendor metadata (tool-specific graphics, undocumented extension labels) either fails to round-trip or must be silently dropped — go-pflow's closed schema hard-fails on anything it doesn't recognize by design, which is safer internally but means an import pipeline cannot "pass through what it doesn't understand" the way every real PNML tool does.
4. **Reset arcs / reset+inhibitor nets.** Actively rejected today (`IsKnownArcType` hard-fails). A model using reset semantics (Reset nets, some TAPAAL extensions) cannot be imported without adding a new `ArcType` and firing-rule case.
5. **Transition priority.** GreatSPN- and TINA-style priority nets have no representation; importing one would require silently dropping the priority relation, changing which transition fires under conflict.
6. **Timed-arc intervals / per-token ages (TAPN).** TAPAAL-specific but TAPAAL is one of the more actively maintained PNML tools; no per-token clock concept exists to receive this.
7. **Rich `<graphics>` metadata** (dimension, fill, line, font, arc bend points). Cosmetic rather than semantic — round-tripping would lose visual fidelity but not net behavior. Lower priority than the above.
8. **Multi-`<net>` documents and a distributable RELAX NG schema artifact.** Format-level conveniences (bundling independent nets in one file; third-party schema validation) rather than semantic gaps — the lowest-impact items for a first import/export layer, since a single-net-per-file convention and Go-side validation can substitute in the short term.

---

## go-pflow Beyond PNML

Capabilities with no PNML standardization at all (core, ISO 15909-3:2021, or otherwise), consolidated across all four research areas:

- **`Arc.Kinetic`** — distinguishes prerequisite/gating arcs from arcs that scale a mass-action rate law; PNML has no rate-law dimension to even need this distinction. (`schema.go:500-522`, `firing.go:39-48`)
- **Timed-transition declarations beyond an abstract concept**: `Delay` (deterministic), `Stages` (Erlang-k/phase-type, mechanically expanded via `ExpandStages`), `Schedule` (piecewise-constant rate over model time). No PNML PNTD standardizes any of these. (`metamodel/delay.go`, `stages.go`, `schedule.go`)
- **Gillespie SSA stochastic simulation** with a documented consistency contract against the ODE engine, plus a portable PRNG for cross-language byte-parity — no standard PNTD covers stochastic semantics at all. (`stochastic/stochastic.go`, `stochastic/portable.go`)
- **ODE/mass-action continuous simulation**, and specifically **`Gating()`**, a machine-readable audit of exactly which constructs (reads, inhibitors, capacity, guards, delay, stages) a continuous solver cannot honor — arguably a direct structural answer to the exact interop fragmentation (mutually unreadable "timed PNML" dialects) that ISO 15909-3 discussion documents call out. (`solver/ode.go`, `firing.go:227-317`)
- **A full guard expression language** (`tokenmodel/guard`) with arithmetic, comparisons, aggregate functions (`sum`, `count`, `tokens`, `minOf`, `maxOf`), evaluated via a pluggable `GuardFunc` contract — richer and more operational than PNML's abstract, evaluator-agnostic `<condition>` term.
- **Typed `Bundle`/`Link` composition** — four link kinds (token/data/event/guard), a `NetType` legality matrix, and guard-lowering to structural read/inhibitor arcs. This is precisely the gap ISO/IEC 15909-3 §7 was created to fill, and go-pflow's version adds a type discipline and an executable lowering algorithm that the standard only illustrates informally. (`metamodel/compose.go`, `compose_matrix.go`, `compose_guard.go`)
- **`Model.Parameters`/`AssertedClasses`** — declared decision variables (with Min/Max) and modeller-asserted symmetry classes for parameter-sensitivity and colour-refinement tooling. No PNML term/sort concept addresses "this constant is a tunable parameter" or "these elements are asserted symmetric."
- **`Presentation`/`Views`** — machine-readable UI/application-generation intent riding the model, feeding actual app generation (petri-pilot). PNML's `<name>`/`<graphics>` labels are about diagram rendering only, not app generation.
- **JSON-LD/schema.org canonical serialization** — a dereferenceable `@context` and vocabulary glossary, content-addressed by SHA-256 hash — a wholesale alternative interchange philosophy (linked-data graph vs. XML/RELAX-NG document grammar), not a narrower version of PNML's approach.
- **Event-sourcing**: `Transition.Event`/`Emits`, `Model.Events` with typed `EventField`s, tying transition firings to a replayable event log (`State(t) = fold(apply, initialState, events[0..t])`). No PNML concept.
- **The `verify` package** — declarative property checking (proved/refuted/unknown + counterexample, structural/exhaustive/witness/partial methods). PNML defines no analysis or verification vocabulary; it is a pure interchange format.
- **`reachability.InvariantAnalyzer`** (Farkas P/T-invariants) and **Karp-Miller unboundedness witnesses**. Same — analysis is out of PNML's scope entirely.
- **`petri.ExpandColors`** as a literal, callable, reversible unfolding function (with `ColorMap.SumByBase`/`BaseName`/`Lookup` for reconstitution) — PNML never mandates an executable unfolding algorithm or a reverse-mapping API; unfolding symmetric nets to P/T nets is discussed informally in the literature but never standardized as a contract.
- **Data-kind places** (`StateKind`, `Place.Kind == DataKind`) holding structured application values with their own arc-binding vocabulary (`Keys`/`Value`) — an ad hoc "non-Petri" place kind bolted onto the P/T model, serving a role closer to a high-level/colored net feature but without any of PNML's algebraic sort machinery.
- **Unknown-construct hard-failure as a load-bearing design choice** (`IsKnownArcType`) — the structural inverse of PNML's `toolspecific`/generic-label tolerance, trading interoperability for the guarantee that an unrecognized construct is never silently misinterpreted as something weaker (e.g., a read arc misread as a normal consuming arc).
