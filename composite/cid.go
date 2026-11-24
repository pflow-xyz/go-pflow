package composite

// The CID of a net: CIDv1(dag-json, sha2-256, base58btc) over the
// @id-stripped, URDNA2015-canonicalised N-Quads of a pflow net's JSON-LD
// document. pflow-xyz's internal/seal does this with a general JSON-LD
// processor (json-gold); this is the small, dependency-free port of pflow-rs's
// canonical_cid.rs for the one context ("https://pflow.xyz/schema") and the
// one document shape a pflow net has, so the composite tools can name the net
// they are about without a JSON-LD library. A host that has the general
// sealer (pflow-xyz does) sets Sealer to it; the two agree on every net the
// editor writes (pflow-xyz tests that).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Sealer computes the CID of a net document, or says why it cannot. The
// default is SealNet.
var Sealer = SealNet

const (
	xsdString  = "http://www.w3.org/2001/XMLSchema#string"
	xsdBoolean = "http://www.w3.org/2001/XMLSchema#boolean"
	xsdInteger = "http://www.w3.org/2001/XMLSchema#integer"
	xsdDouble  = "http://www.w3.org/2001/XMLSchema#double"
	rdfType    = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"
	rdfFirst   = "http://www.w3.org/1999/02/22-rdf-syntax-ns#first"
	rdfRest    = "http://www.w3.org/1999/02/22-rdf-syntax-ns#rest"
	rdfNil     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#nil"
	pflowVocab = "https://pflow.xyz/schema#"
)

var listContainers = map[string]bool{"arcs": true, "token": true, "weight": true, "capacity": true, "initial": true, "parents": true}

type termKind int

const (
	termIRI termKind = iota
	termBNode
	termLiteral
)

type term struct {
	kind     termKind
	value    string
	datatype string
}

type quad [3]term

func iri(v string) term   { return term{kind: termIRI, value: v} }
func bnode(v string) term { return term{kind: termBNode, value: v} }

func nqEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

func nqTerm(t term) string {
	switch t.kind {
	case termIRI:
		return "<" + t.value + ">"
	case termBNode:
		return "_:" + t.value
	}
	if t.datatype == "" || t.datatype == xsdString {
		return "\"" + nqEscape(t.value) + "\""
	}
	return "\"" + nqEscape(t.value) + "\"^^<" + t.datatype + ">"
}

func nqLine(q quad) string { return nqTerm(q[0]) + " " + nqTerm(q[1]) + " " + nqTerm(q[2]) + " .\n" }

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---- identifier issuer

type issuer struct {
	prefix  string
	counter int
	order   []string
	ids     map[string]string
}

func newIssuer(prefix string) *issuer { return &issuer{prefix: prefix, ids: map[string]string{}} }

func (s *issuer) clone() *issuer {
	c := &issuer{prefix: s.prefix, counter: s.counter, order: append([]string(nil), s.order...), ids: make(map[string]string, len(s.ids))}
	for k, v := range s.ids {
		c.ids[k] = v
	}
	return c
}

func (s *issuer) issue(label string) string {
	if id, ok := s.ids[label]; ok {
		return id
	}
	id := s.prefix + strconv.Itoa(s.counter)
	s.counter++
	s.ids[label] = id
	s.order = append(s.order, label)
	return id
}

// ---- canonicalisation

type canon struct {
	bnodeQuads map[string][]quad
	issuer     *issuer
}

func blankLabels(quads []quad) []string {
	seen := map[string]bool{}
	var out []string
	for _, q := range quads {
		for _, t := range q {
			if t.kind == termBNode && !seen[t.value] {
				seen[t.value] = true
				out = append(out, t.value)
			}
		}
	}
	return out
}

func indexBNodeQuads(quads []quad) map[string][]quad {
	idx := map[string][]quad{}
	for _, q := range quads {
		for _, t := range []term{q[0], q[2]} {
			if t.kind == termBNode {
				idx[t.value] = append(idx[t.value], q)
			}
		}
	}
	return idx
}

func relabelForHash(t term, self string) term {
	if t.kind == termBNode {
		if t.value == self {
			return bnode("a")
		}
		return bnode("z")
	}
	return t
}

func (c *canon) hashFirstDegree(label string) string {
	qs := c.bnodeQuads[label]
	lines := make([]string, len(qs))
	for i, q := range qs {
		lines[i] = nqLine(quad{relabelForHash(q[0], label), q[1], relabelForHash(q[2], label)})
	}
	sort.Strings(lines)
	return sha256hex(strings.Join(lines, ""))
}

func (c *canon) hashRelated(related string, q quad, iss *issuer, position string) string {
	var id string
	if canonID, ok := c.issuer.ids[related]; ok {
		id = "_:" + canonID
	} else if temp, ok := iss.ids[related]; ok {
		id = "_:" + temp
	} else {
		id = c.hashFirstDegree(related)
	}
	pred := ""
	if q[1].kind == termIRI {
		pred = q[1].value
	}
	return sha256hex(position + "<" + pred + ">" + id)
}

// permutations of a small list, ascending-lex.
func permutations(v []string) ([][]string, error) {
	if len(v) == 0 {
		return [][]string{nil}, nil
	}
	a := append([]string(nil), v...)
	sort.Strings(a)
	if len(a) > 8 {
		return nil, fmt.Errorf("hash_n_degree_quads: %d same-hash related blank nodes exceeds the practical permutation limit (8! = 40320)", len(a))
	}
	result := [][]string{append([]string(nil), a...)}
	for {
		if len(a) < 2 {
			break
		}
		i := len(a) - 2
		for a[i] >= a[i+1] {
			if i == 0 {
				return result, nil
			}
			i--
		}
		j := len(a) - 1
		for a[j] <= a[i] {
			j--
		}
		a[i], a[j] = a[j], a[i]
		for l, r := i+1, len(a)-1; l < r; l, r = l+1, r-1 {
			a[l], a[r] = a[r], a[l]
		}
		result = append(result, append([]string(nil), a...))
	}
	return result, nil
}

func (c *canon) hashNDegree(label string, iss *issuer) (string, *issuer, error) {
	iss = iss.clone()
	hashToRelated := map[string][]string{}
	for _, q := range c.bnodeQuads[label] {
		for _, pt := range []struct {
			pos string
			t   term
		}{{"s", q[0]}, {"o", q[2]}} {
			if t := pt.t; t.kind == termBNode && t.value != label {
				h := c.hashRelated(t.value, q, iss, pt.pos)
				hashToRelated[h] = append(hashToRelated[h], t.value)
			}
		}
	}
	hashes := sortedKeys(hashToRelated)
	var data strings.Builder
	for _, h := range hashes {
		data.WriteString(h)
		related := append([]string(nil), hashToRelated[h]...)
		sort.Strings(related)
		related = dedup(related)
		var chosen string
		haveChosen := false
		chosenIssuer := iss.clone()
		perms, err := permutations(related)
		if err != nil {
			return "", nil, err
		}
		for _, perm := range perms {
			ic := iss.clone()
			var path strings.Builder
			var recursion []string
			for _, rl := range perm {
				if canonID, ok := c.issuer.ids[rl]; ok {
					path.WriteString("_:" + canonID)
				} else if id, ok := ic.ids[rl]; ok {
					path.WriteString("_:" + id)
				} else {
					id := ic.issue(rl)
					recursion = append(recursion, rl)
					path.WriteString("_:" + id)
				}
			}
			for _, rl := range recursion {
				rh, ri, err := c.hashNDegree(rl, ic)
				if err != nil {
					return "", nil, err
				}
				path.WriteString("_:" + ic.ids[rl])
				path.WriteString("<" + rh + ">")
				ic = ri
			}
			if p := path.String(); !haveChosen || p < chosen {
				chosen, haveChosen, chosenIssuer = p, true, ic
			}
		}
		data.WriteString(chosen)
		iss = chosenIssuer
	}
	return sha256hex(data.String()), iss, nil
}

func dedup(s []string) []string {
	var out []string
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}

func canonicalize(quads []quad) ([]quad, error) {
	c := &canon{bnodeQuads: indexBNodeQuads(quads), issuer: newIssuer("c14n")}
	labels := blankLabels(quads)
	hashToLabels := map[string][]string{}
	for _, l := range labels {
		h := c.hashFirstDegree(l)
		hashToLabels[h] = append(hashToLabels[h], l)
	}
	hashes := sortedKeys(hashToLabels)
	var deferred []string
	for _, h := range hashes {
		ls := hashToLabels[h]
		if len(ls) > 1 {
			deferred = append(deferred, h)
			continue
		}
		c.issuer.issue(ls[0])
	}
	for _, h := range deferred {
		type result struct {
			hash, label string
			iss         *issuer
		}
		var results []result
		for _, l := range hashToLabels[h] {
			if _, ok := c.issuer.ids[l]; ok {
				continue
			}
			temp := newIssuer("b")
			temp.issue(l)
			nd, ri, err := c.hashNDegree(l, temp)
			if err != nil {
				return nil, err
			}
			results = append(results, result{nd, l, ri})
		}
		sort.SliceStable(results, func(i, j int) bool { return results[i].hash < results[j].hash })
		for _, r := range results {
			if _, ok := c.issuer.ids[r.label]; ok {
				continue
			}
			for _, il := range r.iss.order {
				c.issuer.issue(il)
			}
		}
	}
	relabel := func(t term) term {
		if t.kind == termBNode {
			return bnode(c.issuer.ids[t.value])
		}
		return t
	}
	out := make([]quad, len(quads))
	for i, q := range quads {
		out[i] = quad{relabel(q[0]), q[1], relabel(q[2])}
	}
	return out, nil
}

// ---- the expander for the one shape a pflow net has

type expander struct {
	quads []quad
	n     int
}

func (e *expander) fresh() term {
	e.n++
	return bnode("j" + strconv.Itoa(e.n))
}

func literalFor(v any) (term, error) {
	switch x := v.(type) {
	case bool:
		if x {
			return term{kind: termLiteral, value: "true", datatype: xsdBoolean}, nil
		}
		return term{kind: termLiteral, value: "false", datatype: xsdBoolean}, nil
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 9.223372036854775e18 {
			return term{kind: termLiteral, value: strconv.FormatInt(int64(x), 10), datatype: xsdInteger}, nil
		}
		return term{kind: termLiteral, value: canonicalDouble(x), datatype: xsdDouble}, nil
	case int:
		return term{kind: termLiteral, value: strconv.Itoa(x), datatype: xsdInteger}, nil
	case string:
		return term{kind: termLiteral, value: x}, nil
	}
	return term{}, fmt.Errorf("cannot expand a literal of this JSON type: %v", v)
}

// canonicalDouble is the xsd:double canonical form json-gold writes: %1.15E
// with trailing mantissa zeros dropped to one digit and the exponent's sign
// and leading zeros dropped.
func canonicalDouble(f float64) string {
	s := strconv.FormatFloat(f, 'E', 15, 64)
	mant, exp, _ := strings.Cut(s, "E")
	mant = strings.TrimRight(mant, "0")
	if strings.HasSuffix(mant, ".") {
		mant += "0"
	}
	e, _ := strconv.Atoi(exp)
	return mant + "E" + strconv.Itoa(e)
}

func (e *expander) list(items []any) (term, error) {
	if len(items) == 0 {
		return iri(rdfNil), nil
	}
	cells := make([]term, len(items))
	for i := range items {
		cells[i] = e.fresh()
	}
	for i, item := range items {
		obj, err := e.value(item)
		if err != nil {
			return term{}, err
		}
		e.quads = append(e.quads, quad{cells[i], iri(rdfFirst), obj})
		rest := iri(rdfNil)
		if i+1 < len(items) {
			rest = cells[i+1]
		}
		e.quads = append(e.quads, quad{cells[i], iri(rdfRest), rest})
	}
	return cells[0], nil
}

func (e *expander) value(v any) (term, error) {
	if obj, ok := v.(map[string]any); ok {
		node := e.fresh()
		if err := e.object(node, obj); err != nil {
			return term{}, err
		}
		return node, nil
	}
	return literalFor(v)
}

func (e *expander) object(subject term, obj map[string]any) error {
	for _, k := range sortedKeys(obj) {
		v := obj[k]
		switch {
		case k == "@id" || k == "@context" || k == "@version":
			continue
		case k == "@type":
			if name, ok := v.(string); ok {
				e.quads = append(e.quads, quad{subject, iri(rdfType), iri(pflowVocab + name)})
			}
		case listContainers[k]:
			var items []any
			if a, ok := v.([]any); ok {
				items = a
			} else {
				items = []any{v}
			}
			var kept []any
			for _, x := range items {
				if x != nil {
					kept = append(kept, x)
				}
			}
			lt, err := e.list(kept)
			if err != nil {
				return err
			}
			e.quads = append(e.quads, quad{subject, iri(pflowVocab + k), lt})
		default:
			ot, err := e.value(v)
			if err != nil {
				return err
			}
			e.quads = append(e.quads, quad{subject, iri(pflowVocab + k), ot})
		}
	}
	return nil
}

// ---- multiformats

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(b []byte) string {
	digits := []byte{0}
	for _, x := range b {
		carry := int(x)
		for i := range digits {
			v := int(digits[i])*256 + carry
			digits[i] = byte(v % 58)
			carry = v / 58
		}
		for carry > 0 {
			digits = append(digits, byte(carry%58))
			carry /= 58
		}
	}
	out := make([]byte, len(digits))
	for i, d := range digits {
		out[len(digits)-1-i] = base58Alphabet[d]
	}
	s := string(out)
	if strings.HasPrefix(s, "1") && (len(b) == 0 || b[0] != 0) {
		s = s[1:]
	}
	lead := 0
	for lead < len(b) && b[lead] == 0 {
		lead++
	}
	return strings.Repeat("1", lead) + s
}

func uvarint(n uint64) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			out = append(out, b|0x80)
		} else {
			return append(out, b)
		}
	}
}

func cidDagJSONSha256(preimage []byte) string {
	d := sha256.Sum256(preimage)
	mh := append(uvarint(0x12), uvarint(uint64(len(d)))...)
	mh = append(mh, d[:]...)
	cid := append(uvarint(1), uvarint(0x0129)...)
	cid = append(cid, mh...)
	return "z" + base58Encode(cid)
}

// ---- the check a document must pass to be sealed here

// sealable drops the null-valued entries of obj (JSON-LD drops a null value),
// or says why the document would be expanded differently by a general
// processor.
func sealable(obj map[string]any, inNode bool) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range obj {
		if v == nil {
			continue
		}
		switch {
		case k == "@type":
			t, ok := v.(string)
			if !ok || t == "" || strings.Contains(t, ":") {
				return nil, fmt.Errorf("@type %s is not a plain type name", jsonText(v))
			}
		case k == "@version":
		case k == "@id" && inNode:
			return nil, fmt.Errorf("a nested @id")
		case strings.HasPrefix(k, "@"):
			return nil, fmt.Errorf("the JSON-LD keyword %s", k)
		case k == "" || strings.Contains(k, ":"):
			return nil, fmt.Errorf("the key %q, which JSON-LD reads as an IRI", k)
		}
		if listContainers[k] {
			switch x := v.(type) {
			case []any:
				kept := make([]any, 0, len(x))
				for _, item := range x {
					switch it := item.(type) {
					case []any:
						return nil, fmt.Errorf("a list inside the %s list", k)
					case map[string]any:
						m, err := sealable(it, true)
						if err != nil {
							return nil, err
						}
						kept = append(kept, m)
					default:
						kept = append(kept, item)
					}
				}
				out[k] = kept
			case map[string]any:
				m, err := sealable(x, true)
				if err != nil {
					return nil, err
				}
				out[k] = m
			default:
				out[k] = v
			}
			continue
		}
		switch x := v.(type) {
		case []any:
			return nil, fmt.Errorf("the array under %q, which JSON-LD reads as a set", k)
		case map[string]any:
			m, err := sealable(x, true)
			if err != nil {
				return nil, err
			}
			out[k] = m
		default:
			out[k] = v
		}
	}
	return out, nil
}

// SealNet is the CID of a net document in the editor's JSON-LD (the
// top-level @id dropped), or why it cannot be computed here.
func SealNet(doc map[string]any) (string, error) {
	// normalise through JSON so ints and floats are the numbers a parser sees
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	var norm map[string]any
	if err := json.Unmarshal(raw, &norm); err != nil {
		return "", err
	}
	switch c := norm["@context"].(type) {
	case string:
		if c != netContext {
			return "", fmt.Errorf("a @context other than %q", netContext)
		}
	case nil:
		return "", fmt.Errorf("a document without @context")
	default:
		return "", fmt.Errorf("a @context other than %q", netContext)
	}
	delete(norm, "@id")
	delete(norm, "@context")
	body, err := sealable(norm, false)
	if err != nil {
		return "", err
	}
	body["@context"] = netContext
	e := &expander{}
	root := e.fresh()
	if err := e.object(root, body); err != nil {
		return "", err
	}
	quads, err := canonicalize(e.quads)
	if err != nil {
		return "", err
	}
	lines := make([]string, len(quads))
	for i, q := range quads {
		lines[i] = nqLine(q)
	}
	sort.Strings(lines)
	return cidDagJSONSha256([]byte(strings.Join(lines, ""))), nil
}
