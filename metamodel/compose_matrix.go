package metamodel

import "fmt"

// The net-type × link-kind legality matrix.
//
// This is what makes composition *typed* rather than ad hoc. Ch04 puts it
// plainly: "You can't accidentally link a workflow cursor to an inventory
// counter. The type system prevents semantic nonsense at the structural level."
//
// UntypedNet is legal with everything, so models written before net types
// existed keep composing; Validate warns rather than failing.

// linkLegal reports whether a link of the given kind may connect the two net
// types, and if not, why.
func linkLegal(kind LinkKind, from, to NetType) (bool, string) {
	if from == UntypedNet || to == UntypedNet {
		return true, ""
	}

	switch kind {
	case TokenLink:
		// Fusing places moves fungible tokens between the two nets. Only nets
		// whose places *count* things can take part. A WorkflowNet place is a
		// cursor slot: fusing it with an inventory counter destroys the
		// "exactly one token" mutex that makes it a workflow in the first place.
		if !countsTokens(from) {
			return false, fmt.Sprintf(
				"a token link cannot originate in a %s — its places are not fungible counters (use an event link to synchronise firing, or a guard link to gate on state)", from)
		}
		if !countsTokens(to) {
			return false, fmt.Sprintf(
				"a token link cannot terminate in a %s — its places are not fungible counters (use an event link to synchronise firing, or a guard link to gate on state)", to)
		}
		return true, ""

	case DataLink:
		// Read-only observation cannot invalidate an invariant on either side,
		// so it is universally legal.
		return true, ""

	case EventLink:
		// A rendezvous needs a firing instant to synchronise on. ComputationNet
		// transitions evolve continuously under rates, so there is none.
		if from == ComputationNet || to == ComputationNet {
			return false, fmt.Sprintf(
				"an event link cannot involve a %s — its transitions fire continuously under rates, so there is no discrete instant to synchronise (use a data link to observe its state)", ComputationNet)
		}
		return true, ""

	case GuardLink:
		// Gating one net on another's state is meaningful everywhere. It is
		// restrictive, which Validate warns about separately.
		return true, ""
	}
	return false, fmt.Sprintf("unknown link kind %q", kind)
}

// countsTokens reports whether a net type's places hold fungible, transferable
// tokens.
func countsTokens(t NetType) bool {
	switch t {
	case ResourceNet, GameNet:
		return true
	default:
		return false
	}
}
