package metamodel

import "fmt"

// RateSegment is one piece of a piecewise-constant rate: Value firings per
// unit time until model time Until. The last segment of a schedule holds to
// whatever horizon a run uses.
type RateSegment struct {
	Until float64 `json:"until"`
	Value float64 `json:"value"`
}

// ScheduledRate reads the declared rate at model time t: the first segment
// whose Until exceeds t, or the last segment's value past the end. False
// when the transition declares no schedule.
func (t *Transition) ScheduledRate(at float64) (float64, bool) {
	if len(t.Schedule) == 0 {
		return 0, false
	}
	for _, seg := range t.Schedule {
		if at < seg.Until {
			return seg.Value, true
		}
	}
	return t.Schedule[len(t.Schedule)-1].Value, true
}

// ScheduleAverage is the duration-weighted mean of the declared segments —
// the one number to display where a single rate is expected. It weights each
// segment by its span, which covers [0, last Until); what the last segment
// contributes beyond that depends on a horizon this model does not know.
func (t *Transition) ScheduleAverage() (float64, bool) {
	if len(t.Schedule) == 0 {
		return 0, false
	}
	var sum, span, from float64
	for _, seg := range t.Schedule {
		sum += seg.Value * (seg.Until - from)
		span += seg.Until - from
		from = seg.Until
	}
	if span <= 0 {
		return t.Schedule[0].Value, true
	}
	return sum / span, true
}

// HasSchedules reports whether any transition declares a schedule.
func (m *Model) HasSchedules() bool {
	for i := range m.Transitions {
		if len(m.Transitions[i].Schedule) > 0 {
			return true
		}
	}
	return false
}

// ValidateSchedules reports every defect in the declared schedules at once:
// a negative rate, a segment boundary that does not advance, or a first
// boundary at or below zero. The rules are the same ones a scenario schedule
// is held to, because they describe the same shape.
func (m *Model) ValidateSchedules() []error {
	var errs []error
	for i := range m.Transitions {
		t := &m.Transitions[i]
		if len(t.Schedule) == 0 {
			continue
		}
		prev := 0.0
		for j, seg := range t.Schedule {
			if seg.Value < 0 {
				errs = append(errs, fmt.Errorf("schedule for %q: segment %d has a negative rate", t.ID, j))
			}
			if seg.Until <= prev {
				errs = append(errs, fmt.Errorf("schedule for %q: segment %d ends at %g, not after the previous segment's %g", t.ID, j, seg.Until, prev))
			}
			prev = seg.Until
		}
	}
	return errs
}
