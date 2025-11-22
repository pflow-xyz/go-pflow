// Package monitoring provides real-time predictive process monitoring.
// Tracks active cases, predicts completion times, and detects SLA violations.
package monitoring

import (
	"fmt"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Case represents an active process instance being monitored.
type Case struct {
	ID              string                 // Unique case identifier
	StartTime       time.Time              // When the case started
	CurrentActivity string                 // Current activity (last observed)
	LastEventTime   time.Time              // Time of last event
	State           map[string]float64     // Current Petri net state estimate
	History         []Event                // All events for this case
	Attributes      map[string]interface{} // Case attributes
	Predictions     *Prediction            // Latest predictions
}

// Event represents a single event in a case.
type Event struct {
	CaseID     string
	Activity   string
	Timestamp  time.Time
	Resource   string
	Attributes map[string]interface{}
}

// Prediction contains predicted outcomes for a case.
type Prediction struct {
	ComputedAt         time.Time      // When prediction was made
	ExpectedCompletion time.Time      // Predicted completion time
	RemainingTime      time.Duration  // Time until completion
	Confidence         float64        // Confidence score (0-1)
	NextActivities     []NextActivity // Likely next activities
	RiskScore          float64        // Risk of SLA violation (0-1)
}

// NextActivity represents a predicted next activity.
type NextActivity struct {
	Activity     string
	Probability  float64
	ExpectedTime time.Duration // Time until this activity
}

// Alert represents a triggered alert condition.
type Alert struct {
	Timestamp  time.Time
	CaseID     string
	Type       AlertType
	Severity   AlertSeverity
	Message    string
	Prediction *Prediction
	Threshold  interface{} // The threshold that was violated
}

// AlertType categorizes alerts.
type AlertType string

const (
	AlertTypeSLAViolation   AlertType = "sla_violation"
	AlertTypeDelayed        AlertType = "delayed"
	AlertTypeStuck          AlertType = "stuck"
	AlertTypeUnexpectedPath AlertType = "unexpected_path"
	AlertTypeResourceIssue  AlertType = "resource_issue"
)

// AlertSeverity indicates alert importance.
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// AlertHandler is called when an alert is triggered.
type AlertHandler func(alert Alert)

// MonitorConfig configures the monitoring system.
type MonitorConfig struct {
	PredictionInterval time.Duration // How often to update predictions
	SLAThreshold       time.Duration // SLA deadline
	StuckThreshold     time.Duration // Time without activity = stuck
	ConfidenceLevel    float64       // Minimum confidence for predictions
	EnablePredictions  bool          // Enable/disable predictions
	EnableAlerts       bool          // Enable/disable alerting
}

// DefaultMonitorConfig returns sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		PredictionInterval: 1 * time.Minute,
		SLAThreshold:       4 * time.Hour, // Common ER SLA
		StuckThreshold:     30 * time.Minute,
		ConfidenceLevel:    0.7,
		EnablePredictions:  true,
		EnableAlerts:       true,
	}
}

// Statistics tracks monitoring performance metrics.
type Statistics struct {
	TotalCases            int
	ActiveCases           int
	CompletedCases        int
	TotalAlerts           int
	AlertsBySeverity      map[AlertSeverity]int
	AlertsByType          map[AlertType]int
	AveragePredictionTime time.Duration
	PredictionAccuracy    float64 // % of predictions within threshold
}

// Monitor is the main process monitoring engine.
type Monitor struct {
	net       *petri.PetriNet
	rates     map[string]float64
	config    MonitorConfig
	predictor *Predictor // ODE-based predictor

	cases map[string]*Case // Active cases
	mu    sync.RWMutex     // Protects cases map

	handlers []AlertHandler
	stats    Statistics

	running bool
	stopCh  chan struct{}
}

// NewMonitor creates a new process monitor with learned model parameters.
func NewMonitor(net *petri.PetriNet, rates map[string]float64, config MonitorConfig) *Monitor {
	predictor := NewPredictor(net, rates)
	return &Monitor{
		net:       net,
		rates:     rates,
		config:    config,
		predictor: predictor,
		cases:     make(map[string]*Case),
		handlers:  make([]AlertHandler, 0),
		stats: Statistics{
			AlertsBySeverity: make(map[AlertSeverity]int),
			AlertsByType:     make(map[AlertType]int),
		},
		stopCh: make(chan struct{}),
	}
}

// AddAlertHandler registers a function to be called on alerts.
func (m *Monitor) AddAlertHandler(handler AlertHandler) {
	m.handlers = append(m.handlers, handler)
}

// GetCase retrieves a case by ID.
func (m *Monitor) GetCase(caseID string) (*Case, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, exists := m.cases[caseID]
	return c, exists
}

// GetActiveCases returns all currently active cases.
func (m *Monitor) GetActiveCases() []*Case {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cases := make([]*Case, 0, len(m.cases))
	for _, c := range m.cases {
		cases = append(cases, c)
	}
	return cases
}

// GetStatistics returns current monitoring statistics.
func (m *Monitor) GetStatistics() Statistics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := m.stats
	stats.ActiveCases = len(m.cases)
	return stats
}

// triggerAlert sends an alert to all registered handlers.
func (m *Monitor) triggerAlert(alert Alert) {
	// Update statistics
	m.stats.TotalAlerts++
	m.stats.AlertsBySeverity[alert.Severity]++
	m.stats.AlertsByType[alert.Type]++

	// Call handlers
	for _, handler := range m.handlers {
		go handler(alert) // Non-blocking
	}
}

// String returns a human-readable representation of a case.
func (c *Case) String() string {
	elapsed := time.Since(c.StartTime)
	return fmt.Sprintf("Case %s: %s (started %s ago, last: %s)",
		c.ID, c.CurrentActivity, elapsed.Round(time.Second), c.LastEventTime.Format("15:04:05"))
}

// String returns a human-readable representation of an alert.
func (a *Alert) String() string {
	return fmt.Sprintf("[%s] %s - Case %s: %s",
		a.Severity, a.Type, a.CaseID, a.Message)
}
