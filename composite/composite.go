// Package composite is four tools over metamodel.Bundle that make a composition
// something the rest of the toolbox can read and its glue something that can be
// checked:
//
//   - bundle_net turns the flattened bundle into the editor's JSON-LD net, so
//     fire, simulate, reachability, invariants, verify, cid and export read a
//     composite exactly as they read any other net (what the editor's format
//     cannot say, a guard or a data place, is refused rather than dropped);
//   - bundle_prove judges what each link does to the nets it joins: per-link
//     behavioural consistency (by ablation: flatten without the link and
//     search again), the part laws lifted onto the composite and classified
//     preserved / extended / changed, composite claims (bounded, deadlock-free,
//     quasi-live; never proved by a search that did not finish), and a
//     machine-checkable record of type "CompositionProof";
//   - bundle_check_proof re-checks such a record against a bundle: certificate
//     arithmetic (y.C, z.C = 0, bounds, trace replay) and a recomputation of the
//     engine's own answers. A record written by another implementation checks
//     here, and the other way round;
//   - bundle_parts is a 13-part catalog (queue, resource-pool, saga, mutex,
//     token_bucket, circuit_breaker, batcher, approval, inventory, failover,
//     sir, kanban, traffic_light) and a way to build a composition from
//     {instances: [{id, part, params}], links}.
//
// The tools are pflow-rs's (crates/pflow-hosted/src/bundle: flatnet.rs,
// proof.rs, parts.rs), ported; pflow-rs is their reference and
// testdata/composite.json their golden (see testdata/README.md).
//
// Call runs one by name with JSON arguments, as a tool server would; Net,
// Prove, CheckProof and PartsCatalog/OpenParts are the same with typed
// arguments, for a host that has read the bundle itself.
package composite

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Tools are the tool names Call answers (a "pflow_" prefix is accepted).
var Tools = []string{"bundle_net", "bundle_prove", "bundle_check_proof", "bundle_parts"}

var (
	bundleOnlyKinds = map[string]argKind{"bundle": argAny}
	proveKinds      = map[string]argKind{"bundle": argAny, "max_states": argAny, "max_tokens": argAny, "ablate": argBool}
	checkKinds      = map[string]argKind{"bundle": argAny, "proof": argAny}
	partsKinds      = map[string]argKind{"instances": argAny, "links": argAny}
)

// Call runs one tool by name with JSON-decoded arguments. A refusal of the
// call is an error whose text is the hosted tool's.
func Call(tool string, args map[string]any) (any, error) {
	tool = strings.TrimPrefix(tool, "pflow_")
	hosted := "pflow_" + tool
	switch tool {
	case "bundle_net":
		b, err := readBundleArg(hosted, args, bundleOnlyKinds)
		if err != nil {
			return nil, err
		}
		return Net(b)
	case "bundle_prove":
		b, err := readBundleArg(hosted, args, proveKinds)
		if err != nil {
			return nil, err
		}
		o, err := OptsFromArgs(args, hosted)
		if err != nil {
			return nil, err
		}
		return Prove(b, o)
	case "bundle_check_proof":
		b, err := readBundleArg(hosted, args, checkKinds)
		if err != nil {
			return nil, err
		}
		rec, ok := args["proof"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: proof: the record pflow_bundle_prove returned is required", hosted)
		}
		return CheckProof(b, rec)
	case "bundle_parts":
		return parts(hosted, args)
	}
	return nil, fmt.Errorf("unknown tool %q", tool)
}

// Parts is bundle_parts: the catalog when no instances are given, else the
// composition built from `instances` and `links`. Both are the arguments as
// JSON decoded them (nil is absent).
func Parts(instances, links any) (map[string]any, error) {
	args := map[string]any{}
	if instances != nil {
		args["instances"] = instances
	}
	if links != nil {
		args["links"] = links
	}
	return parts("pflow_bundle_parts", args)
}

func parts(tool string, a map[string]any) (map[string]any, error) {
	if err := checkArgs(tool, a, partsKinds); err != nil {
		return nil, err
	}
	list := func(k string) ([]any, error) {
		switch x := a[k].(type) {
		case nil:
			return nil, nil
		case []any:
			return x, nil
		default:
			return nil, fmt.Errorf("%s: %s must be an array, got %s", tool, k, jsonText(x))
		}
	}
	instances, err := list("instances")
	if err != nil {
		return nil, err
	}
	links, err := list("links")
	if err != nil {
		return nil, err
	}
	if len(instances) > MaxSubnets {
		return nil, fmt.Errorf("%s: %d instances; at most %d", tool, len(instances), MaxSubnets)
	}
	if len(links) > MaxLinks {
		return nil, fmt.Errorf("%s: %d links; at most %d", tool, len(links), MaxLinks)
	}
	if len(instances) == 0 && a["instances"] == nil {
		return PartsCatalog(), nil
	}
	out, err := OpenParts(instances, links)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}
	return out, nil
}

var _ = metamodel.BundleType
