package composite

// Links, endpoints and the verdict on one link: pflow-rs's `bundle/rules.rs`
// (the part the three tools use), which is pflow-xyz's
// `mcp_model_rules.go`. A link is judged by metamodel.Bundle.Validate on a
// bundle of the two subnets it joins, plus the composition page's two rules
// (a link joins two different subnets; the same link is not added twice).

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func portKey(e metamodel.Endpoint) string { return e.Subnet + ":" + e.Port }

// LinkKey is "kind:subnet:port->subnet:port".
func LinkKey(l metamodel.Link) string {
	return string(l.Kind) + ":" + portKey(l.From) + "->" + portKey(l.To)
}

func endView(e metamodel.Endpoint) map[string]any {
	return map[string]any{"subnet": e.Subnet, "port": e.Port}
}

func linkView(l metamodel.Link) map[string]any {
	return map[string]any{"kind": string(l.Kind), "from": endView(l.From), "to": endView(l.To)}
}

var linkKinds = []metamodel.LinkKind{metamodel.TokenLink, metamodel.DataLink, metamodel.EventLink, metamodel.GuardLink}

func parseKind(s string) (metamodel.LinkKind, error) {
	for _, k := range linkKinds {
		if string(k) == s {
			return k, nil
		}
	}
	return "", fmt.Errorf("unknown link kind %q (token, data, event, guard)", s)
}

func parseEndpoint(v any) (metamodel.Endpoint, error) {
	switch x := v.(type) {
	case string:
		a, b, ok := strings.Cut(x, ":")
		if !ok || a == "" || b == "" {
			return metamodel.Endpoint{}, fmt.Errorf("endpoint %q is not \"subnet:port\"", x)
		}
		return metamodel.Endpoint{Subnet: a, Port: b}, nil
	case map[string]any:
		a, _ := x["subnet"].(string)
		b, _ := x["port"].(string)
		place, _ := x["place"].(string)
		trans, _ := x["transition"].(string)
		if a != "" && b == "" && (place != "" || trans != "") {
			return metamodel.Endpoint{}, fmt.Errorf("endpoint %s names a place or a transition: these tools link ports", jsonText(x))
		}
		if a == "" || b == "" {
			return metamodel.Endpoint{}, fmt.Errorf("endpoint %s needs \"subnet\" and one of \"port\", \"place\", \"transition\"", jsonText(x))
		}
		return metamodel.Endpoint{Subnet: a, Port: b}, nil
	}
	return metamodel.Endpoint{}, fmt.Errorf("an endpoint is {\"subnet\", \"port\"} or \"subnet:port\", not %s", jsonText(v))
}

func parseLink(v any) (metamodel.Link, error) {
	m, _ := v.(map[string]any)
	kind, ok := m["kind"].(string)
	if !ok {
		return metamodel.Link{}, fmt.Errorf("link %s has no \"kind\"", jsonText(v))
	}
	k, err := parseKind(kind)
	if err != nil {
		return metamodel.Link{}, err
	}
	from, err := parseEndpoint(m["from"])
	if err != nil {
		return metamodel.Link{}, err
	}
	to, err := parseEndpoint(m["to"])
	if err != nil {
		return metamodel.Link{}, err
	}
	return metamodel.Link{Kind: k, From: from, To: to}, nil
}

// issue is a refusal or warning, with whose rule it is.
type issue struct{ code, message, rule string }

func (i issue) view() map[string]any {
	return map[string]any{"code": i.code, "message": i.message, "rule": i.rule}
}

// verdict is the judgment on one link.
type verdict struct {
	ok       bool
	kind     string
	errors   []issue
	warnings []issue
}

func (v verdict) view() map[string]any {
	view := func(is []issue) []any {
		out := make([]any, len(is))
		for i, x := range is {
			out[i] = x.view()
		}
		return out
	}
	return map[string]any{"ok": v.ok, "kind": v.kind, "errors": view(v.errors), "warnings": view(v.warnings)}
}

// refusal is "[CODE] message; ...", the error a refused edit returns.
func (v verdict) refusal() string {
	parts := make([]string, len(v.errors))
	for i, e := range v.errors {
		parts[i] = "[" + e.code + "] " + e.message
	}
	return strings.Join(parts, "; ")
}

const linkLabel = "links[0]"

// validateLink judges l against the bundle's subnets: the same-subnet page
// rule, then Validate on a bundle of the subnets l names and l alone.
func validateLink(b *metamodel.Bundle, l metamodel.Link) verdict {
	v := verdict{ok: true, kind: string(l.Kind)}
	if l.From.Subnet == l.To.Subnet {
		v.ok = false
		v.errors = append(v.errors, issue{"PAGE_SAME_SUBNET",
			fmt.Sprintf("%s: both ends are on subnet %q; a link connects two subnets", linkLabel, l.From.Subnet), "page"})
		return v
	}
	one := metamodel.NewBundle("verdict")
	for i := range b.Subnets {
		if s := b.Subnets[i]; s.ID == l.From.Subnet || s.ID == l.To.Subnet {
			one.AddSubnet(s)
		}
	}
	one.AddLink(l)
	res := one.Validate()
	for _, e := range res.Errors {
		v.errors = append(v.errors, issue{e.Code, e.Message, "pflow-compose"})
	}
	for _, w := range res.Warnings {
		if w.Element == linkLabel {
			v.warnings = append(v.warnings, issue{w.Code, w.Message, "pflow-compose"})
		}
	}
	v.ok = len(v.errors) == 0
	return v
}

func hasLink(b *metamodel.Bundle, l metamodel.Link) bool {
	key := LinkKey(l)
	rev := LinkKey(metamodel.Link{Kind: l.Kind, From: l.To, To: l.From})
	for _, x := range b.Links {
		if k := LinkKey(x); k == key || (l.Kind == metamodel.EventLink && k == rev) {
			return true
		}
	}
	return false
}

func verdictIn(b *metamodel.Bundle, l metamodel.Link) verdict {
	v := validateLink(b, l)
	if v.ok && hasLink(b, l) {
		v.ok = false
		v.errors = append(v.errors, issue{"PAGE_DUPLICATE", "already linked", "page"})
	}
	return v
}

// portElement is the local id a port exposes.
func portElement(p *metamodel.Port) string {
	if p.IsTransition() {
		return p.Transition
	}
	return p.Place
}
