package monitoring

import (
	"fmt"
	"time"
)

// StartCase begins monitoring a new case.
func (m *Monitor) StartCase(caseID string, startTime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cases[caseID]; exists {
		return fmt.Errorf("case %s already exists", caseID)
	}

	// Initialize case with starting state (all tokens in start place)
	initialState := m.net.SetState(nil)

	c := &Case{
		ID:            caseID,
		StartTime:     startTime,
		LastEventTime: startTime,
		State:         initialState,
		History:       make([]Event, 0),
		Attributes:    make(map[string]interface{}),
	}

	m.cases[caseID] = c
	m.stats.TotalCases++

	return nil
}

// RecordEvent records a new event for a case and updates predictions.
func (m *Monitor) RecordEvent(caseID string, activity string, timestamp time.Time, resource string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.cases[caseID]
	if !exists {
		return fmt.Errorf("case %s not found", caseID)
	}

	// Create event
	event := Event{
		CaseID:     caseID,
		Activity:   activity,
		Timestamp:  timestamp,
		Resource:   resource,
		Attributes: make(map[string]interface{}),
	}

	// Add to history
	c.History = append(c.History, event)
	c.CurrentActivity = activity
	c.LastEventTime = timestamp

	// Update state estimate
	// In a real implementation, this would use the Petri net structure
	// to estimate which place we're in based on the activity sequence
	// For now, we'll keep it simple

	// Check for stuck cases
	if m.config.EnableAlerts {
		timeSinceLastEvent := time.Since(c.LastEventTime)
		if timeSinceLastEvent > m.config.StuckThreshold {
			m.triggerAlert(Alert{
				Timestamp: time.Now(),
				CaseID:    caseID,
				Type:      AlertTypeStuck,
				Severity:  SeverityWarning,
				Message:   fmt.Sprintf("Case has been inactive for %v", timeSinceLastEvent.Round(time.Minute)),
			})
		}
	}

	// Update predictions if enabled
	if m.config.EnablePredictions {
		m.updatePredictions(c)
	}

	return nil
}

// CompleteCase marks a case as completed and removes from active tracking.
func (m *Monitor) CompleteCase(caseID string, completionTime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.cases[caseID]
	if !exists {
		return fmt.Errorf("case %s not found", caseID)
	}

	// Update statistics
	m.stats.CompletedCases++

	// Check if prediction was accurate (if we had one)
	if c.Predictions != nil {
		predictedTime := c.Predictions.ExpectedCompletion
		actualTime := completionTime
		error := actualTime.Sub(predictedTime).Abs()

		// Update accuracy (within 10% of predicted time is "accurate")
		threshold := c.Predictions.RemainingTime / 10
		if error < threshold {
			// This would be updated properly with a moving average
			m.stats.PredictionAccuracy = 0.8 // Placeholder
		}
	}

	// Remove from active cases
	delete(m.cases, caseID)

	return nil
}

// updatePredictions updates predictions for a case using simulation.
func (m *Monitor) updatePredictions(c *Case) {
	// Use the Petri net model and learned rates to predict future
	// This is where the magic happens - we simulate forward from current state

	prediction := &Prediction{
		ComputedAt:     time.Now(),
		NextActivities: make([]NextActivity, 0),
	}

	// Use ODE-based prediction from current state
	remaining, confidence := PredictRemainingTime(c, m.predictor)
	prediction.RemainingTime = remaining
	prediction.ExpectedCompletion = time.Now().Add(remaining)
	prediction.Confidence = confidence

	// Predict next activities
	nextActivities := PredictNextActivity(c, m.predictor)
	prediction.NextActivities = nextActivities

	// Compute risk score
	if m.config.SLAThreshold > 0 {
		totalExpected := time.Since(c.StartTime) + remaining
		if totalExpected > m.config.SLAThreshold {
			prediction.RiskScore = 0.9 // High risk

			// Trigger SLA violation alert
			if m.config.EnableAlerts {
				m.triggerAlert(Alert{
					Timestamp: time.Now(),
					CaseID:    c.ID,
					Type:      AlertTypeSLAViolation,
					Severity:  SeverityCritical,
					Message: fmt.Sprintf("Predicted completion (%s) exceeds SLA threshold (%s)",
						totalExpected.Round(time.Minute), m.config.SLAThreshold),
					Prediction: prediction,
					Threshold:  m.config.SLAThreshold,
				})
			}
		} else {
			ratio := float64(totalExpected) / float64(m.config.SLAThreshold)
			prediction.RiskScore = ratio // 0-1 scale

			// Warning if getting close (>80%)
			if ratio > 0.8 && m.config.EnableAlerts {
				m.triggerAlert(Alert{
					Timestamp:  time.Now(),
					CaseID:     c.ID,
					Type:       AlertTypeDelayed,
					Severity:   SeverityWarning,
					Message:    fmt.Sprintf("Case at risk: %.0f%% of SLA threshold used", ratio*100),
					Prediction: prediction,
					Threshold:  m.config.SLAThreshold,
				})
			}
		}
	}

	c.Predictions = prediction
}

// PredictCompletion returns the latest prediction for a case.
// If predictions are stale, it will compute a new one.
func (m *Monitor) PredictCompletion(caseID string) (*Prediction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.cases[caseID]
	if !exists {
		return nil, fmt.Errorf("case %s not found", caseID)
	}

	// Update if stale or missing
	if c.Predictions == nil || time.Since(c.Predictions.ComputedAt) > m.config.PredictionInterval {
		m.updatePredictions(c)
	}

	return c.Predictions, nil
}

// Start begins the monitoring loop (for periodic updates).
func (m *Monitor) Start() {
	m.running = true

	go func() {
		ticker := time.NewTicker(m.config.PredictionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.periodicUpdate()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop stops the monitoring loop.
func (m *Monitor) Stop() {
	if m.running {
		close(m.stopCh)
		m.running = false
	}
}

// periodicUpdate updates predictions for all active cases.
func (m *Monitor) periodicUpdate() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.cases {
		if m.config.EnablePredictions {
			m.updatePredictions(c)
		}
	}
}

// PrintStatus prints a summary of current monitoring state.
func (m *Monitor) PrintStatus() {
	stats := m.GetStatistics()

	fmt.Println("=== Monitoring Status ===")
	fmt.Printf("Active cases: %d\n", stats.ActiveCases)
	fmt.Printf("Completed cases: %d\n", stats.CompletedCases)
	fmt.Printf("Total alerts: %d\n", stats.TotalAlerts)

	if len(stats.AlertsBySeverity) > 0 {
		fmt.Println("\nAlerts by severity:")
		for severity, count := range stats.AlertsBySeverity {
			fmt.Printf("  %s: %d\n", severity, count)
		}
	}

	if len(stats.AlertsByType) > 0 {
		fmt.Println("\nAlerts by type:")
		for alertType, count := range stats.AlertsByType {
			fmt.Printf("  %s: %d\n", alertType, count)
		}
	}

	fmt.Println("\nActive cases:")
	for _, c := range m.GetActiveCases() {
		fmt.Printf("  %s\n", c.String())
		if c.Predictions != nil {
			fmt.Printf("    Predicted completion: %s (in %s)\n",
				c.Predictions.ExpectedCompletion.Format("15:04:05"),
				c.Predictions.RemainingTime.Round(time.Minute))
			fmt.Printf("    Risk score: %.1f%%\n", c.Predictions.RiskScore*100)
		}
	}
}
