package composite

// Reading a tool's arguments and the bundle it carries, and the limits a
// bundle is held to before any engine sees it. This is pflow-rs's
// `bundle/read.rs`, which is pflow-xyz's `mcp_model_read.go` in Rust.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Limits a bundle and a call are held to (the hosted server's).
const (
	MaxSubnets     = 32
	MaxPlaces      = 1000 // across the bundle
	MaxTransitions = 1000
	MaxArcs        = 4000
	MaxPorts       = 512
	MaxLinks       = 200
	MaxConstraints = 200
	MaxIDBytes     = 128 // a subnet, place, transition, port, link or constraint id, an arc end, a schema

	// MaxAnswerBytes is what a flat net may weigh in an answer.
	MaxAnswerBytes = 2 << 20

	// ExactInt: JSON-RPC numbers are doubles; 2^53 and up cannot be told from
	// their neighbours.
	ExactInt = float64(1 << 53)
)

// argKind is what a tool argument must be.
type argKind int

const (
	argAny argKind = iota
	argStr
	argBool
	argStrList
)

var argKindName = map[argKind]string{argStr: "a string", argBool: "a boolean", argStrList: "a list of strings", argAny: "anything"}

// jsonText is a value as a message prints it: compact JSON.
func jsonText(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// quoteList is ["a", "b"], the way Rust's {:?} of a Vec<&str> prints.
func quoteList(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// checkArgs refuses an argument the tool does not take, or one of the wrong
// type, before anything runs (a null is as good as absent).
func checkArgs(tool string, a map[string]any, kinds map[string]argKind) error {
	for _, k := range sortedKeys(a) {
		kind, ok := kinds[k]
		if !ok {
			return fmt.Errorf("%s: unknown argument %q; takes %s", tool, k, quoteList(sortedKeys(kinds)))
		}
		v := a[k]
		if v == nil || kind == argAny {
			continue
		}
		good := false
		switch kind {
		case argStr:
			_, good = v.(string)
		case argBool:
			_, good = v.(bool)
		case argStrList:
			if l, isList := v.([]any); isList {
				good = true
				for _, x := range l {
					if _, s := x.(string); !s {
						good = false
					}
				}
			}
		}
		if !good {
			return fmt.Errorf("%s: argument %q should be %s, got %s", tool, k, argKindName[kind], jsonText(v))
		}
	}
	return nil
}

// ReadBundle checks the arguments against `kinds` (the names a tool takes; each
// "anything" except where a tool says otherwise) and reads the bundle,
// refusing one past the limits before any engine sees it. Subnets that declare
// no ports get one inout port per exported place.
func readBundleArg(tool string, args map[string]any, kinds map[string]argKind) (*metamodel.Bundle, error) {
	if err := checkArgs(tool, args, kinds); err != nil {
		return nil, err
	}
	raw, ok := args["bundle"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: bundle: a PetriNetBundle (a JSON object with subnets and links) is required; pflow_bundle_example returns one", tool)
	}
	if t, ok := raw["@type"].(string); ok && t != "" && t != metamodel.BundleType {
		return nil, fmt.Errorf("%s: bundle: @type is %q, not %q", tool, t, metamodel.BundleType)
	}
	subnets, ok := raw["subnets"].([]any)
	if !ok || len(subnets) == 0 {
		return nil, fmt.Errorf("%s: bundle: needs a non-empty \"subnets\" array", tool)
	}
	if len(subnets) > MaxSubnets {
		return nil, fmt.Errorf("%s: bundle: %d subnets; at most %d", tool, len(subnets), MaxSubnets)
	}
	if links, ok := raw["links"].([]any); ok && len(links) > MaxLinks {
		return nil, fmt.Errorf("%s: bundle: %d links; at most %d", tool, len(links), MaxLinks)
	}
	if err := checkNumbers(tool, raw); err != nil {
		return nil, err
	}
	if err := checkEnums(tool, raw); err != nil {
		return nil, err
	}
	text, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: bundle: not JSON: %v", tool, err)
	}
	b := &metamodel.Bundle{}
	if err := json.Unmarshal(text, b); err != nil {
		return nil, fmt.Errorf("%s: bundle: %v", tool, err)
	}
	if err := checkSize(tool, b); err != nil {
		return nil, err
	}
	DerivePorts(b)
	return b, nil
}

// CheckBundle refuses a bundle past the counts and names the hosted tools hold
// every bundle to. A caller that has read the bundle itself (pflow-xyz does)
// has done this already.
func CheckBundle(tool string, b *metamodel.Bundle) error { return checkSize(tool, b) }

func checkNumbers(tool string, raw map[string]any) error {
	var path []string
	var walk func(v any) error
	walk = func(v any) error {
		switch x := v.(type) {
		case float64:
			if math.Abs(x) >= ExactInt {
				return fmt.Errorf("%s: bundle: %s is %s; a count or weight is at most 9007199254740991 (2^53 - 1), the largest integer JSON-RPC carries exactly (a bigger one would arrive rounded)",
					tool, strings.TrimPrefix(strings.Join(path, ""), "."), numberText(x))
			}
		case map[string]any:
			for _, k := range sortedKeys(x) {
				path = append(path, "."+k)
				if err := walk(x[k]); err != nil {
					return err
				}
				path = path[:len(path)-1]
			}
		case []any:
			for i, e := range x {
				path = append(path, fmt.Sprintf("[%d]", i))
				if err := walk(e); err != nil {
					return err
				}
				path = path[:len(path)-1]
			}
		}
		return nil
	}
	return walk(raw)
}

func numberText(x float64) string {
	if math.Abs(x) < 1e19 {
		return strconv.FormatFloat(x, 'f', 0, 64)
	}
	return strconv.FormatFloat(x, 'g', -1, 64)
}

func checkEnum(tool, path string, m map[string]any, key string, allowed ...string) error {
	v, present := m[key]
	if !present || v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		for _, a := range allowed {
			if s == a {
				return nil
			}
		}
	}
	quoted := make([]string, len(allowed))
	for i, a := range allowed {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return fmt.Errorf("%s: bundle: %s.%s is %s; it is one of %s", tool, path, key, jsonText(v), strings.Join(quoted, ", "))
}

func objects(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, len(list))
	for i, e := range list {
		out[i], _ = e.(map[string]any)
	}
	return out
}

func checkEnums(tool string, raw map[string]any) error {
	if err := checkEnum(tool, "bundle", raw, "arc_merge", "sum", "max"); err != nil {
		return err
	}
	for i, s := range objects(raw["subnets"]) {
		path := fmt.Sprintf("subnets[%d]", i)
		if err := checkEnum(tool, path, s, "net_type", "", "WorkflowNet", "ResourceNet", "GameNet", "ComputationNet", "ClassificationNet"); err != nil {
			return err
		}
		model, _ := s["model"].(map[string]any)
		for j, p := range objects(model["places"]) {
			if err := checkEnum(tool, fmt.Sprintf("%s.model.places[%d]", path, j), p, "kind", "token", "data"); err != nil {
				return err
			}
		}
		for j, p := range objects(s["ports"]) {
			pp := fmt.Sprintf("%s.ports[%d]", path, j)
			if err := checkEnum(tool, pp, p, "kind", "in", "out", "inout", "observe"); err != nil {
				return err
			}
			if err := checkEnum(tool, pp, p, "target", "place", "transition"); err != nil {
				return err
			}
		}
	}
	for i, l := range objects(raw["links"]) {
		if err := checkEnum(tool, fmt.Sprintf("links[%d]", i), l, "kind", "token", "data", "event", "guard"); err != nil {
			return err
		}
	}
	return nil
}

func checkSize(tool string, b *metamodel.Bundle) error {
	var places, transitions, arcs, ports, constraints int
	long := func(path, id string) error {
		if len(id) > MaxIDBytes {
			return fmt.Errorf("%s: bundle: %s is %d bytes; names are at most %d bytes", tool, path, len(id), MaxIDBytes)
		}
		return nil
	}
	check := func(path string, ids ...string) error {
		for i := 0; i+1 < len(ids); i += 2 {
			if err := long(path+ids[i], ids[i+1]); err != nil {
				return err
			}
		}
		return nil
	}
	for i := range b.Subnets {
		s := &b.Subnets[i]
		sp := fmt.Sprintf("subnets[%d]", i)
		if err := check(sp, ".id", s.ID); err != nil {
			return err
		}
		ports += len(s.Ports)
		for j := range s.Ports {
			p := &s.Ports[j]
			if err := check(fmt.Sprintf("%s.ports[%d]", sp, j), ".id", p.ID, ".place", p.Place, ".transition", p.Transition, ".schema", p.Schema); err != nil {
				return err
			}
		}
		if s.Model == nil {
			continue
		}
		places += len(s.Model.Places)
		transitions += len(s.Model.Transitions)
		arcs += len(s.Model.Arcs)
		constraints += len(s.Model.Constraints)
		for j := range s.Model.Places {
			p := &s.Model.Places[j]
			if err := check(fmt.Sprintf("%s.model.places[%d]", sp, j), ".id", p.ID, ".type", p.Type); err != nil {
				return err
			}
		}
		for j := range s.Model.Transitions {
			if err := check(fmt.Sprintf("%s.model.transitions[%d]", sp, j), ".id", s.Model.Transitions[j].ID); err != nil {
				return err
			}
		}
		for j := range s.Model.Arcs {
			a := &s.Model.Arcs[j]
			if err := check(fmt.Sprintf("%s.model.arcs[%d]", sp, j), ".from", a.From, ".to", a.To); err != nil {
				return err
			}
		}
		for j := range s.Model.Constraints {
			if err := check(fmt.Sprintf("%s.model.constraints[%d]", sp, j), ".id", s.Model.Constraints[j].ID); err != nil {
				return err
			}
		}
	}
	constraints += len(b.Constraints)
	for j := range b.Constraints {
		if err := check(fmt.Sprintf("constraints[%d]", j), ".id", b.Constraints[j].ID); err != nil {
			return err
		}
	}
	for i := range b.Links {
		l := &b.Links[i]
		lp := fmt.Sprintf("links[%d]", i)
		if err := check(lp, ".id", l.ID,
			".from.subnet", l.From.Subnet, ".from.port", l.From.Port, ".from.place", l.From.Place, ".from.transition", l.From.Transition,
			".to.subnet", l.To.Subnet, ".to.port", l.To.Port, ".to.place", l.To.Place, ".to.transition", l.To.Transition); err != nil {
			return err
		}
	}
	for _, c := range []struct {
		what     string
		n, limit int
	}{
		{"places", places, MaxPlaces}, {"transitions", transitions, MaxTransitions}, {"arcs", arcs, MaxArcs},
		{"ports", ports, MaxPorts}, {"constraints", constraints, MaxConstraints},
	} {
		if c.n > c.limit {
			return fmt.Errorf("%s: bundle: %d %s; at most %d", tool, c.n, c.what, c.limit)
		}
	}
	return nil
}

// DerivePorts gives every subnet that declares no ports one inout port per
// exported place, named for the place (a place id that repeats gives one
// port, and only when its first place is exported).
func DerivePorts(b *metamodel.Bundle) {
	for i := range b.Subnets {
		s := &b.Subnets[i]
		if len(s.Ports) > 0 || s.Model == nil {
			continue
		}
		seen := map[string]bool{}
		for _, p := range s.Model.Places {
			if first := s.Model.PlaceByID(p.ID); p.Exported && first != nil && first.Exported && !seen[p.ID] {
				seen[p.ID] = true
				s.Ports = append(s.Ports, metamodel.Port{ID: p.ID, Kind: metamodel.PortInOut, Place: p.ID, Schema: first.Type})
			}
		}
	}
}
