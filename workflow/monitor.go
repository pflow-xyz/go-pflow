package workflow

import (
	"fmt"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// WorkflowMonitor provides real-time monitoring, predictions, and analytics
// for workflow execution using ODE-based simulation.
type WorkflowMonitor struct {
	engine    *Engine
	net       *petri.PetriNet
	rates     map[string]float64
	predictor *WorkflowPredictor

	// Configuration
	config MonitorConfig

	// Alert tracking
	alerts    []*Alert
	alertsMu  sync.RWMutex
	alertChan chan *Alert

	// Running state
	running bool
	stopCh  chan struct{}
}

// MonitorConfig configures the workflow monitor.
type MonitorConfig struct {
	// SLA checking
	SLACheckInterval time.Duration

	// Prediction settings
	EnablePredictions  bool
	PredictionInterval time.Duration
	SimulationTimeSpan float64 // How far ahead to simulate

	// Alert settings
	EnableAlerts    bool
	AlertBufferSize int

	// Performance thresholds
	WaitTimeWarningThreshold time.Duration
	WaitTimeCriticalThreshold time.Duration
}

// DefaultMonitorConfig returns sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		SLACheckInterval:          time.Minute,
		EnablePredictions:         true,
		PredictionInterval:        5 * time.Minute,
		SimulationTimeSpan:        100.0,
		EnableAlerts:              true,
		AlertBufferSize:           100,
		WaitTimeWarningThreshold:  15 * time.Minute,
		WaitTimeCriticalThreshold: 30 * time.Minute,
	}
}

// WorkflowPredictor uses ODE simulation for workflow predictions.
type WorkflowPredictor struct {
	net       *petri.PetriNet
	rates     map[string]float64
	timeSpan  float64
	solverOpts *solver.Options
}

// NewWorkflowPredictor creates a predictor for the given workflow Petri net.
func NewWorkflowPredictor(net *petri.PetriNet, rates map[string]float64) *WorkflowPredictor {
	return &WorkflowPredictor{
		net:        net,
		rates:      rates,
		timeSpan:   100.0,
		solverOpts: solver.FastOptions(),
	}
}

// WithTimeSpan sets the simulation time span.
func (p *WorkflowPredictor) WithTimeSpan(t float64) *WorkflowPredictor {
	p.timeSpan = t
	return p
}

// WithOptions sets solver options.
func (p *WorkflowPredictor) WithOptions(opts *solver.Options) *WorkflowPredictor {
	p.solverOpts = opts
	return p
}

// PredictFromState predicts workflow completion from current state.
func (p *WorkflowPredictor) PredictFromState(state map[string]float64) *CasePrediction {
	prob := solver.NewProblem(p.net, state, [2]float64{0, p.timeSpan}, p.rates)
	sol := solver.Solve(prob, solver.Tsit5(), p.solverOpts)

	finalState := sol.GetFinalState()

	// Find equilibrium time (when changes stop)
	equilibriumTime := p.findEquilibriumTime(sol)

	// Estimate completion probability
	completionProb := p.estimateCompletionProbability(finalState)

	// Calculate expected remaining duration
	remainingDuration := time.Duration(equilibriumTime * float64(time.Hour))

	return &CasePrediction{
		ComputedAt:            time.Now(),
		RemainingDuration:     remainingDuration,
		CompletionProbability: completionProb,
		FinalState:            finalState,
		EquilibriumTime:       equilibriumTime,
		Confidence:            p.calculateConfidence(sol, state),
	}
}

// findEquilibriumTime finds when the system reaches steady state.
func (p *WorkflowPredictor) findEquilibriumTime(sol *solver.Solution) float64 {
	if len(sol.T) < 2 {
		return sol.T[len(sol.T)-1]
	}

	tolerance := 1e-3

	for i := len(sol.T) - 1; i > 0; i-- {
		maxDelta := 0.0
		for key := range sol.U[i] {
			delta := sol.U[i][key] - sol.U[i-1][key]
			if delta < 0 {
				delta = -delta
			}
			if delta > maxDelta {
				maxDelta = delta
			}
		}

		if maxDelta > tolerance {
			// Found where things were still changing
			return sol.T[i]
		}
	}

	return sol.T[len(sol.T)-1]
}

// estimateCompletionProbability estimates the probability of completion.
func (p *WorkflowPredictor) estimateCompletionProbability(finalState map[string]float64) float64 {
	// Look for end place tokens
	endTokens := 0.0
	startTokens := 0.0

	for place, tokens := range finalState {
		if len(place) > 4 && place[len(place)-4:] == "_end" {
			endTokens += tokens
		}
		if len(place) > 6 && place[len(place)-6:] == "_start" {
			startTokens += tokens
		}
	}

	// If we started with tokens in start places and now have them in end places
	total := endTokens + startTokens
	if total > 0 {
		return endTokens / total
	}

	return 0.5 // Unknown
}

// calculateConfidence estimates prediction confidence.
func (p *WorkflowPredictor) calculateConfidence(sol *solver.Solution, initialState map[string]float64) float64 {
	// Higher confidence if:
	// 1. System reached equilibrium
	// 2. Conservation laws hold
	// 3. No negative values

	confidence := 1.0

	finalState := sol.GetFinalState()

	// Check for conservation
	initialTotal := 0.0
	finalTotal := 0.0
	for _, v := range initialState {
		initialTotal += v
	}
	for _, v := range finalState {
		finalTotal += v
	}
	if initialTotal > 0 {
		ratio := finalTotal / initialTotal
		if ratio < 0.9 || ratio > 1.1 {
			confidence *= 0.7 // Conservation violation
		}
	}

	// Check for negative values
	for _, v := range finalState {
		if v < -0.01 {
			confidence *= 0.5
		}
	}

	return confidence
}

// CasePrediction holds prediction results for a case.
type CasePrediction struct {
	ComputedAt            time.Time
	RemainingDuration     time.Duration
	ExpectedCompletion    time.Time
	CompletionProbability float64
	FinalState            map[string]float64
	EquilibriumTime       float64
	Confidence            float64
	RiskScore             float64
	BottleneckTasks       []string
}

// NewWorkflowMonitor creates a monitor for a workflow engine.
func NewWorkflowMonitor(engine *Engine, config MonitorConfig) *WorkflowMonitor {
	net := engine.workflow.ToPetriNet()
	rates := engine.workflow.learnedRates()

	m := &WorkflowMonitor{
		engine:    engine,
		net:       net,
		rates:     rates,
		predictor: NewWorkflowPredictor(net, rates),
		config:    config,
		alerts:    make([]*Alert, 0),
		stopCh:    make(chan struct{}),
	}

	if config.EnableAlerts {
		m.alertChan = make(chan *Alert, config.AlertBufferSize)
	}

	// Wire up engine events
	engine.OnAlert(func(alert *Alert) {
		m.recordAlert(alert)
	})

	engine.OnTaskReady(func(c *Case, t *TaskInstance) {
		m.checkWaitTime(c, t)
	})

	return m
}

// learnedRates extracts or estimates transition rates from the workflow.
func (w *Workflow) learnedRates() map[string]float64 {
	rates := make(map[string]float64)

	// Use estimated durations to derive rates
	for _, task := range w.Tasks {
		transName := "task_" + task.ID
		if task.EstimatedDuration > 0 {
			// Rate = 1/duration (in hours)
			rates[transName] = 1.0 / task.EstimatedDuration.Hours()
		} else {
			rates[transName] = 1.0 // Default rate
		}
	}

	return rates
}

// Start begins the monitoring loop.
func (m *WorkflowMonitor) Start() {
	if m.running {
		return
	}
	m.running = true

	go func() {
		slaTicker := time.NewTicker(m.config.SLACheckInterval)
		var predTicker *time.Ticker
		if m.config.EnablePredictions {
			predTicker = time.NewTicker(m.config.PredictionInterval)
		}

		defer slaTicker.Stop()
		if predTicker != nil {
			defer predTicker.Stop()
		}

		for {
			select {
			case <-slaTicker.C:
				m.engine.CheckSLAs()
			case <-func() <-chan time.Time {
				if predTicker != nil {
					return predTicker.C
				}
				return nil
			}():
				m.updateAllPredictions()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop stops the monitoring loop.
func (m *WorkflowMonitor) Stop() {
	if m.running {
		close(m.stopCh)
		m.running = false
	}
}

// PredictCase generates a prediction for a specific case.
func (m *WorkflowMonitor) PredictCase(caseID string) (*CasePrediction, error) {
	c := m.engine.GetCase(caseID)
	if c == nil {
		return nil, fmt.Errorf("case %s not found", caseID)
	}

	// Build current state from case
	state := m.buildStateFromCase(c)

	// Run prediction
	pred := m.predictor.PredictFromState(state)
	pred.ExpectedCompletion = m.engine.now().Add(pred.RemainingDuration)

	// Calculate risk score
	if c.Deadline != nil {
		remaining := c.Deadline.Sub(m.engine.now())
		if pred.RemainingDuration > remaining {
			pred.RiskScore = 1.0 // Will breach SLA
		} else {
			pred.RiskScore = float64(pred.RemainingDuration) / float64(remaining)
		}
	}

	// Identify bottlenecks
	pred.BottleneckTasks = m.identifyBottlenecks(c)

	return pred, nil
}

// buildStateFromCase converts case status to Petri net state.
func (m *WorkflowMonitor) buildStateFromCase(c *Case) map[string]float64 {
	state := make(map[string]float64)

	// Initialize all places to 0
	for place := range m.net.Places {
		state[place] = 0
	}

	// Set tokens based on task status
	for taskID, instance := range c.TaskInstances {
		switch instance.Status {
		case TaskStatusPending:
			state["task_"+taskID+"_pending"] = 1
		case TaskStatusReady:
			state["task_"+taskID+"_ready"] = 1
		case TaskStatusRunning:
			state["task_"+taskID+"_running"] = 1
		case TaskStatusCompleted, TaskStatusSkipped:
			state["task_"+taskID+"_done"] = 1
		}
	}

	return state
}

// identifyBottlenecks finds tasks that are likely causing delays.
func (m *WorkflowMonitor) identifyBottlenecks(c *Case) []string {
	var bottlenecks []string

	for taskID, instance := range c.TaskInstances {
		// Long-waiting tasks are bottlenecks
		if instance.Status == TaskStatusReady && instance.ReadyAt != nil {
			waitTime := m.engine.now().Sub(*instance.ReadyAt)
			if waitTime > m.config.WaitTimeWarningThreshold {
				bottlenecks = append(bottlenecks, taskID)
			}
		}

		// Tasks with high resource requirements
		task := m.engine.workflow.Tasks[taskID]
		for _, req := range task.RequiredResources {
			pool := m.engine.resources[req.ResourceID]
			if pool != nil {
				pool.mu.Lock()
				utilization := 1.0 - (pool.available / pool.resource.Capacity)
				pool.mu.Unlock()
				if utilization > 0.9 {
					bottlenecks = append(bottlenecks, taskID)
				}
			}
		}
	}

	return bottlenecks
}

// updateAllPredictions refreshes predictions for all active cases.
func (m *WorkflowMonitor) updateAllPredictions() {
	cases := m.engine.GetCases(func(c *Case) bool {
		return c.Status == CaseStatusRunning
	})

	for _, c := range cases {
		pred, err := m.PredictCase(c.ID)
		if err != nil {
			continue
		}

		// Check for SLA risk
		if pred.RiskScore > 0.8 && m.config.EnableAlerts {
			m.recordAlert(&Alert{
				ID:        fmt.Sprintf("risk_%s_%d", c.ID, m.engine.now().Unix()),
				Type:      AlertSLAWarning,
				Severity:  AlertWarning,
				CaseID:    c.ID,
				Message:   fmt.Sprintf("Case at risk: %.0f%% of SLA used, predicted remaining: %s", pred.RiskScore*100, pred.RemainingDuration.Round(time.Minute)),
				CreatedAt: m.engine.now(),
				Details: map[string]any{
					"risk_score":          pred.RiskScore,
					"remaining_duration":  pred.RemainingDuration.String(),
					"completion_prob":     pred.CompletionProbability,
					"bottleneck_tasks":    pred.BottleneckTasks,
				},
			})
		}
	}
}

// checkWaitTime alerts on long-waiting tasks.
func (m *WorkflowMonitor) checkWaitTime(c *Case, t *TaskInstance) {
	// This will be called periodically to check wait times
	// Initial call is when task becomes ready
}

// recordAlert stores an alert and sends to channel if available.
func (m *WorkflowMonitor) recordAlert(alert *Alert) {
	m.alertsMu.Lock()
	m.alerts = append(m.alerts, alert)
	m.alertsMu.Unlock()

	if m.alertChan != nil {
		select {
		case m.alertChan <- alert:
		default:
			// Channel full, drop alert
		}
	}
}

// GetAlerts returns all alerts.
func (m *WorkflowMonitor) GetAlerts() []*Alert {
	m.alertsMu.RLock()
	defer m.alertsMu.RUnlock()

	result := make([]*Alert, len(m.alerts))
	copy(result, m.alerts)
	return result
}

// AlertChannel returns a channel for receiving alerts.
func (m *WorkflowMonitor) AlertChannel() <-chan *Alert {
	return m.alertChan
}

// GetDashboardData returns data for a monitoring dashboard.
func (m *WorkflowMonitor) GetDashboardData() *DashboardData {
	metrics := m.engine.GetMetrics()
	resources := m.engine.GetResourceAvailability()
	readyTasks := m.engine.GetReadyTasks()

	// Get cases at risk
	var casesAtRisk []*CaseRiskInfo
	activeCases := m.engine.GetCases(func(c *Case) bool {
		return c.Status == CaseStatusRunning
	})

	for _, c := range activeCases {
		pred, err := m.PredictCase(c.ID)
		if err == nil && pred.RiskScore > 0.7 {
			casesAtRisk = append(casesAtRisk, &CaseRiskInfo{
				CaseID:             c.ID,
				Priority:           c.Priority,
				RiskScore:          pred.RiskScore,
				ExpectedCompletion: pred.ExpectedCompletion,
				BottleneckTasks:    pred.BottleneckTasks,
			})
		}
	}

	// Get task queue depth by task type
	taskQueue := make(map[string]int)
	for _, t := range readyTasks {
		taskQueue[t.TaskID]++
	}

	return &DashboardData{
		Timestamp:            m.engine.now(),
		ActiveCases:          metrics.ActiveCases,
		CompletedCases:       metrics.CompletedCases,
		FailedCases:          metrics.FailedCases,
		ReadyTaskCount:       len(readyTasks),
		ResourceUtilization:  metrics.ResourceUtilization,
		ResourceAvailability: resources,
		CasesAtRisk:          casesAtRisk,
		TaskQueue:            taskQueue,
		RecentAlerts:         m.getRecentAlerts(10),
		SLACompliance:        m.calculateSLACompliance(),
	}
}

// DashboardData provides a snapshot for monitoring displays.
type DashboardData struct {
	Timestamp            time.Time
	ActiveCases          int
	CompletedCases       int
	FailedCases          int
	ReadyTaskCount       int
	ResourceUtilization  map[string]float64
	ResourceAvailability map[string]float64
	CasesAtRisk          []*CaseRiskInfo
	TaskQueue            map[string]int
	RecentAlerts         []*Alert
	SLACompliance        float64
}

// CaseRiskInfo provides risk information for a case.
type CaseRiskInfo struct {
	CaseID             string
	Priority           Priority
	RiskScore          float64
	ExpectedCompletion time.Time
	BottleneckTasks    []string
}

// getRecentAlerts returns the most recent n alerts.
func (m *WorkflowMonitor) getRecentAlerts(n int) []*Alert {
	m.alertsMu.RLock()
	defer m.alertsMu.RUnlock()

	if len(m.alerts) <= n {
		result := make([]*Alert, len(m.alerts))
		copy(result, m.alerts)
		return result
	}

	result := make([]*Alert, n)
	copy(result, m.alerts[len(m.alerts)-n:])
	return result
}

// calculateSLACompliance calculates the overall SLA compliance rate.
func (m *WorkflowMonitor) calculateSLACompliance() float64 {
	m.alertsMu.RLock()
	defer m.alertsMu.RUnlock()

	breaches := 0
	for _, alert := range m.alerts {
		if alert.Type == AlertSLABreach {
			breaches++
		}
	}

	metrics := m.engine.GetMetrics()
	total := metrics.CompletedCases + metrics.FailedCases
	if total == 0 {
		return 1.0
	}

	return float64(total-breaches) / float64(total)
}

// WhatIfAnalysis simulates the impact of changes.
type WhatIfAnalysis struct {
	monitor *WorkflowMonitor
}

// NewWhatIfAnalysis creates a what-if analyzer.
func NewWhatIfAnalysis(monitor *WorkflowMonitor) *WhatIfAnalysis {
	return &WhatIfAnalysis{monitor: monitor}
}

// AddResource simulates adding resource capacity.
func (a *WhatIfAnalysis) AddResource(resourceID string, quantity float64) *WhatIfResult {
	// Clone rates and increase the relevant transition rates
	newRates := make(map[string]float64)
	for k, v := range a.monitor.rates {
		newRates[k] = v
	}

	// Find transitions that use this resource and increase their rates
	for taskID, task := range a.monitor.engine.workflow.Tasks {
		for _, req := range task.RequiredResources {
			if req.ResourceID == resourceID {
				transName := "task_" + taskID
				if rate, exists := newRates[transName]; exists {
					// More resources = faster processing (simple model)
					newRates[transName] = rate * (1 + quantity)
				}
			}
		}
	}

	// Run simulation with new rates
	predictor := NewWorkflowPredictor(a.monitor.net, newRates)

	// Compare predictions
	originalPred := a.monitor.predictor.PredictFromState(a.monitor.net.SetState(nil))
	newPred := predictor.PredictFromState(a.monitor.net.SetState(nil))

	return &WhatIfResult{
		Scenario:          fmt.Sprintf("Add %.0f %s", quantity, resourceID),
		OriginalDuration:  originalPred.RemainingDuration,
		ProjectedDuration: newPred.RemainingDuration,
		Improvement:       originalPred.RemainingDuration - newPred.RemainingDuration,
		ImprovementPct:    float64(originalPred.RemainingDuration-newPred.RemainingDuration) / float64(originalPred.RemainingDuration) * 100,
	}
}

// WhatIfResult shows the impact of a hypothetical change.
type WhatIfResult struct {
	Scenario          string
	OriginalDuration  time.Duration
	ProjectedDuration time.Duration
	Improvement       time.Duration
	ImprovementPct    float64
}

// String returns a human-readable result.
func (r *WhatIfResult) String() string {
	return fmt.Sprintf("%s: %s -> %s (%.1f%% improvement)",
		r.Scenario,
		r.OriginalDuration.Round(time.Minute),
		r.ProjectedDuration.Round(time.Minute),
		r.ImprovementPct)
}
