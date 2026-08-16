package metamodel

import (
	"reflect"
	"strings"
	"testing"
)

// Every JSON field a marshalled Model can emit must have a term in
// ModelLDContext. This gate lives beside the schema so a new field cannot
// merge without its definition — an undefined term resolves through @vocab
// to an IRI nobody has described, which is linked data in shape but not in
// meaning.
func TestModelLDContextCoversTheSchema(t *testing.T) {
	tags := map[string]bool{}
	var walk func(rt reflect.Type, seen map[reflect.Type]bool)
	walk = func(rt reflect.Type, seen map[reflect.Type]bool) {
		for rt.Kind() == reflect.Ptr || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map || rt.Kind() == reflect.Array {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag != "" && tag != "-" {
				tags[tag] = true
			}
			walk(f.Type, seen)
		}
	}
	walk(reflect.TypeOf(Model{}), map[reflect.Type]bool{})

	var missing []string
	for tag := range tags {
		if _, ok := ModelLDContext[tag]; !ok {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("schema fields with no term in ModelLDContext (define them in metamodel/jsonld.go): %v", missing)
	}
}

// Glossary entries must name IRIs the context actually mints.
func TestModelGlossaryTermsAreMinted(t *testing.T) {
	minted := map[string]bool{}
	for _, def := range ModelLDContext {
		switch d := def.(type) {
		case string:
			minted[d] = true
		case map[string]any:
			if id, ok := d["@id"].(string); ok {
				minted[id] = true
			}
		}
	}
	for term := range ModelVocabularyTerms() {
		if !minted[term] {
			t.Errorf("glossary defines %q but no context term mints that IRI", term)
		}
	}
}
