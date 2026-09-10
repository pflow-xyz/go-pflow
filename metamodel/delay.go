package metamodel

import "fmt"

// ValidateDelays checks every Delay declaration. A negative delay is
// meaningless, and a delayed transition with no consuming input would start
// again the instant it completed — an unbounded number of firings in zero
// model time — so both are refused here rather than discovered as a hung
// simulation. A delayed transition that also declares a rate is accepted:
// the rate is ignored, and engines say so in their caveats.
func (m *Model) ValidateDelays() []error {
	var errs []error
	for i := range m.Transitions {
		t := &m.Transitions[i]
		switch {
		case t.Delay < 0:
			errs = append(errs, fmt.Errorf("transition %s: delay %v is negative", t.ID, t.Delay))
		case t.Delay > 0 && len(m.Inputs(t.ID)) == 0:
			errs = append(errs, fmt.Errorf("transition %s: delay %v on a transition with no consuming input would restart forever in zero time", t.ID, t.Delay))
		}
	}
	return errs
}
