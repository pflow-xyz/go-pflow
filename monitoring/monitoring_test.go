package monitoring

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

// orderNet is a linear workflow using the place names this package's
// estimator and predictor key on: a case begins at "start" and is considered
// complete when token mass reaches "end".
func orderNet() (*petri.PetriNet, map[string]float64) {
	return petri.Build().
		Place("start", 1).Place("validated", 0).Place("end", 0).
		Transition("validate").Transition("ship").
		Arc("start", "validate", 1).Arc("validate", "validated", 1).
		Arc("validated", "ship", 1).Arc("ship", "end", 1).
		WithRates(1.0)
}

func quietConfig() MonitorConfig {
	cfg := DefaultMonitorConfig()
	cfg.EnableAlerts = false
	cfg.EnablePredictions = false
	return cfg
}

// --- case lifecycle -------------------------------------------------------

func TestCaseLifecycle(t *testing.T) {
	net, rates := orderNet()
	m := NewMonitor(net, rates, quietConfig())

	now := time.Now()
	if err := m.StartCase("c1", now); err != nil {
		t.Fatalf("StartCase: %v", err)
	}

	// Duplicate IDs are an error, not a silent overwrite.
	if err := m.StartCase("c1", now); err == nil {
		t.Error("duplicate StartCase should fail")
	}

	if err := m.RecordEvent("c1", "validate", now.Add(time.Minute), "alice"); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	if err := m.RecordEvent("ghost", "validate", now, ""); err == nil {
		t.Error("RecordEvent on unknown case should fail")
	}

	c, ok := m.GetCase("c1")
	if !ok {
		t.Fatal("GetCase lost the case")
	}
	if c.CurrentActivity != "validate" || len(c.History) != 1 {
		t.Errorf("case state: activity=%q history=%d, want validate/1", c.CurrentActivity, len(c.History))
	}

	if got := len(m.GetActiveCases()); got != 1 {
		t.Errorf("active cases = %d, want 1", got)
	}

	if err := m.CompleteCase("c1", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("CompleteCase: %v", err)
	}
	if err := m.CompleteCase("c1", now); err == nil {
		t.Error("completing a completed case should fail")
	}
	if got := len(m.GetActiveCases()); got != 0 {
		t.Errorf("active cases after completion = %d, want 0", got)
	}

	stats := m.GetStatistics()
	if stats.TotalCases != 1 || stats.CompletedCases != 1 {
		t.Errorf("stats = %+v, want 1 total / 1 completed", stats)
	}
}

// --- state estimation -----------------------------------------------------

func TestEstimateCurrentStateReplaysHistory(t *testing.T) {
	net, _ := orderNet()
	now := time.Now()

	c := &Case{ID: "c1", StartTime: now, History: []Event{
		{CaseID: "c1", Activity: "validate", Timestamp: now.Add(time.Minute)},
	}}

	state := EstimateCurrentState(c, net)

	// One firing of validate: the start token moved to validated.
	if state["start"] != 0 || state["validated"] != 1 || state["end"] != 0 {
		t.Errorf("state after validate = %v, want start:0 validated:1 end:0", state)
	}

	// Full history reaches the end.
	c.History = append(c.History, Event{CaseID: "c1", Activity: "ship", Timestamp: now.Add(2 * time.Minute)})
	state = EstimateCurrentState(c, net)
	if state["end"] != 1 {
		t.Errorf("state after full history = %v, want end:1", state)
	}
}

func TestEstimateCurrentStateSkipsUnknownActivities(t *testing.T) {
	net, _ := orderNet()
	now := time.Now()

	c := &Case{ID: "c1", StartTime: now, History: []Event{
		{CaseID: "c1", Activity: "not-in-model", Timestamp: now},
		{CaseID: "c1", Activity: "validate", Timestamp: now.Add(time.Minute)},
	}}

	state := EstimateCurrentState(c, net)
	if state["validated"] != 1 {
		t.Errorf("unknown activity should be skipped, not corrupt the replay: %v", state)
	}
}

// --- prediction -----------------------------------------------------------

func TestPredictNextActivity(t *testing.T) {
	net, rates := orderNet()
	p := NewPredictor(net, rates)
	now := time.Now()

	// After validate, the only enabled transition is ship.
	c := &Case{ID: "c1", StartTime: now, History: []Event{
		{CaseID: "c1", Activity: "validate", Timestamp: now},
	}}

	next := PredictNextActivity(c, p)
	if len(next) != 1 {
		t.Fatalf("next activities = %v, want exactly [ship]", next)
	}
	if next[0].Activity != "ship" {
		t.Errorf("next = %q, want ship", next[0].Activity)
	}
	if next[0].Probability < 0.99 {
		t.Errorf("sole enabled transition should have probability ~1, got %f", next[0].Probability)
	}
}

func TestPredictNextActivityProbabilitiesFollowRates(t *testing.T) {
	// A fork: fast (rate 9) and slow (rate 1) compete for the same token.
	net, rates := petri.Build().
		Place("start", 1).Place("a", 0).Place("b", 0).
		Transition("fast").Transition("slow").
		Arc("start", "fast", 1).Arc("fast", "a", 1).
		Arc("start", "slow", 1).Arc("slow", "b", 1).
		WithCustomRates(map[string]float64{"fast": 9, "slow": 1})

	p := NewPredictor(net, rates)
	c := &Case{ID: "c1", StartTime: time.Now(), History: nil}

	next := PredictNextActivity(c, p)
	if len(next) != 2 {
		t.Fatalf("expected 2 competing activities, got %v", next)
	}

	probs := map[string]float64{}
	total := 0.0
	for _, n := range next {
		probs[n.Activity] = n.Probability
		total += n.Probability
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("probabilities should sum to 1, got %f", total)
	}
	// Mass-action: fast should get 9/10 of the probability.
	if probs["fast"] < 0.85 || probs["fast"] > 0.95 {
		t.Errorf("fast probability = %f, want ~0.9", probs["fast"])
	}
	if probs["fast"] <= probs["slow"] {
		t.Errorf("rate 9 transition must be more likely than rate 1: %v", probs)
	}
}

func TestPredictFromStateReachesEnd(t *testing.T) {
	net, rates := orderNet()
	p := NewPredictor(net, rates)

	state := map[string]float64{"start": 1, "validated": 0, "end": 0}
	pred := p.PredictFromState(state, 0)

	if pred.PredictedEndTime <= pred.CurrentTime {
		t.Errorf("predicted end %f must be after current time %f", pred.PredictedEndTime, pred.CurrentTime)
	}
	// With rate-1 transitions the case completes in seconds, far below the
	// 24h horizon; hitting the horizon would mean completion was never seen.
	if pred.PredictedEndTime >= 86400 {
		t.Errorf("prediction hit the max horizon — end place never reached: %f", pred.PredictedEndTime)
	}
	if pred.Confidence <= 0 {
		t.Errorf("confidence = %f, want > 0 once token mass reaches end", pred.Confidence)
	}
}

func TestPredictCompletion(t *testing.T) {
	net, rates := orderNet()
	cfg := quietConfig()
	cfg.EnablePredictions = true
	m := NewMonitor(net, rates, cfg)

	if _, err := m.PredictCompletion("ghost"); err == nil {
		t.Error("PredictCompletion on unknown case should fail")
	}

	now := time.Now()
	if err := m.StartCase("c1", now); err != nil {
		t.Fatal(err)
	}
	if err := m.RecordEvent("c1", "validate", now, ""); err != nil {
		t.Fatal(err)
	}

	pred, err := m.PredictCompletion("c1")
	if err != nil {
		t.Fatalf("PredictCompletion: %v", err)
	}
	if pred == nil {
		t.Fatal("nil prediction")
	}
	if pred.RemainingTime < 0 {
		t.Errorf("remaining time = %v, want >= 0", pred.RemainingTime)
	}
}

// --- alerts ----------------------------------------------------------------

func TestSLAViolationAlert(t *testing.T) {
	net, rates := orderNet()
	cfg := DefaultMonitorConfig()
	cfg.EnableAlerts = true
	cfg.EnablePredictions = true
	cfg.SLAThreshold = time.Nanosecond // everything violates

	m := NewMonitor(net, rates, cfg)

	// Handlers are invoked asynchronously (triggerAlert spawns a goroutine per
	// handler), so collection must synchronize on delivery, not on RecordEvent
	// returning.
	got := make(chan Alert, 16)
	m.AddAlertHandler(func(a Alert) { got <- a })

	now := time.Now()
	if err := m.StartCase("c1", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := m.RecordEvent("c1", "validate", now, ""); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case a := <-got:
			if a.Type != AlertTypeSLAViolation {
				continue // e.g. a stuck-case warning; keep waiting
			}
			if a.Severity != SeverityCritical {
				t.Errorf("SLA violation severity = %q, want critical", a.Severity)
			}
			if a.CaseID != "c1" {
				t.Errorf("alert case = %q, want c1", a.CaseID)
			}
			stats := m.GetStatistics()
			if stats.TotalAlerts == 0 {
				t.Error("stats.TotalAlerts should count triggered alerts")
			}
			return
		case <-deadline:
			t.Fatal("no SLA violation alert within 5s")
		}
	}
}

func TestNoAlertsWhenDisabled(t *testing.T) {
	net, rates := orderNet()
	cfg := DefaultMonitorConfig()
	cfg.EnableAlerts = false
	cfg.EnablePredictions = true
	cfg.SLAThreshold = time.Nanosecond

	m := NewMonitor(net, rates, cfg)

	fired := make(chan struct{}, 1)
	m.AddAlertHandler(func(Alert) {
		select {
		case fired <- struct{}{}:
		default:
		}
	})

	now := time.Now()
	_ = m.StartCase("c1", now.Add(-time.Hour))
	_ = m.RecordEvent("c1", "validate", now, "")

	select {
	case <-fired:
		t.Error("alert handler fired with EnableAlerts=false")
	case <-time.After(200 * time.Millisecond):
		// no alert delivered: correct
	}
}

// --- concurrency ------------------------------------------------------------

// TestConcurrentAccess exercises the monitor from many goroutines; run with
// -race this is the package's only guard against locking regressions.
func TestConcurrentAccess(t *testing.T) {
	net, rates := orderNet()
	cfg := quietConfig()
	cfg.EnablePredictions = true
	m := NewMonitor(net, rates, cfg)

	var wg sync.WaitGroup
	now := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			caseID := string(rune('a' + id))
			if err := m.StartCase(caseID, now); err != nil {
				t.Errorf("StartCase %s: %v", caseID, err)
				return
			}
			_ = m.RecordEvent(caseID, "validate", now.Add(time.Second), "")
			_, _ = m.PredictCompletion(caseID)
			_ = m.GetActiveCases()
			_ = m.GetStatistics()
			_ = m.CompleteCase(caseID, now.Add(time.Minute))
		}(i)
	}
	wg.Wait()

	if got := m.GetStatistics().CompletedCases; got != 8 {
		t.Errorf("completed = %d, want 8", got)
	}
}

// TestCaseAndAlertString exercises the display helpers.
func TestCaseAndAlertString(t *testing.T) {
	c := &Case{ID: "c1", CurrentActivity: "validate", StartTime: time.Now().Add(-time.Hour)}
	if s := c.String(); !strings.Contains(s, "c1") {
		t.Errorf("Case.String() = %q, should mention the id", s)
	}
	a := &Alert{CaseID: "c1", Type: AlertTypeSLAViolation, Severity: SeverityCritical, Message: "late"}
	if s := a.String(); !strings.Contains(s, "c1") || !strings.Contains(s, "late") {
		t.Errorf("Alert.String() = %q, should mention id and message", s)
	}
}
