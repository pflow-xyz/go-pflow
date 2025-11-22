package eventlog

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// CSVConfig configures CSV parsing behavior.
type CSVConfig struct {
	CaseIDColumn     string   // Column name for case ID (required)
	ActivityColumn   string   // Column name for activity (required)
	TimestampColumn  string   // Column name for timestamp (required)
	ResourceColumn   string   // Column name for resource (optional)
	LifecycleColumn  string   // Column name for lifecycle (optional)
	TimestampFormats []string // Date/time formats to try (optional)
	Delimiter        rune     // CSV delimiter (default: comma)
	SkipRows         int      // Number of header rows to skip
}

// DefaultCSVConfig returns a configuration with common defaults.
func DefaultCSVConfig() CSVConfig {
	return CSVConfig{
		CaseIDColumn:    "case_id",
		ActivityColumn:  "activity",
		TimestampColumn: "timestamp",
		ResourceColumn:  "resource",
		LifecycleColumn: "lifecycle",
		TimestampFormats: []string{
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05.000",
			"2006-01-02T15:04:05.000",
			"2006-01-02",
			"01/02/2006 15:04:05",
			"01/02/2006",
		},
		Delimiter: ',',
		SkipRows:  0,
	}
}

// ParseCSV parses an event log from a CSV file.
func ParseCSV(filename string, config CSVConfig) (*EventLog, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	return ParseCSVReader(f, config)
}

// ParseCSVReader parses an event log from a CSV reader.
func ParseCSVReader(r io.Reader, config CSVConfig) (*EventLog, error) {
	// Validate required fields
	if config.CaseIDColumn == "" {
		return nil, fmt.Errorf("CaseIDColumn is required")
	}
	if config.ActivityColumn == "" {
		return nil, fmt.Errorf("ActivityColumn is required")
	}
	if config.TimestampColumn == "" {
		return nil, fmt.Errorf("TimestampColumn is required")
	}

	reader := csv.NewReader(r)
	if config.Delimiter != 0 {
		reader.Comma = config.Delimiter
	}

	// Skip initial rows if configured
	for i := 0; i < config.SkipRows; i++ {
		if _, err := reader.Read(); err != nil {
			return nil, fmt.Errorf("skipping row %d: %w", i, err)
		}
	}

	// Read header row
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	// Build column index map
	colIndex := make(map[string]int)
	for i, col := range header {
		colIndex[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Find required columns
	caseIdx, ok := colIndex[strings.ToLower(config.CaseIDColumn)]
	if !ok {
		return nil, fmt.Errorf("case ID column '%s' not found in header: %v", config.CaseIDColumn, header)
	}

	activityIdx, ok := colIndex[strings.ToLower(config.ActivityColumn)]
	if !ok {
		return nil, fmt.Errorf("activity column '%s' not found in header: %v", config.ActivityColumn, header)
	}

	timestampIdx, ok := colIndex[strings.ToLower(config.TimestampColumn)]
	if !ok {
		return nil, fmt.Errorf("timestamp column '%s' not found in header: %v", config.TimestampColumn, header)
	}

	// Find optional columns
	resourceIdx := -1
	if config.ResourceColumn != "" {
		if idx, ok := colIndex[strings.ToLower(config.ResourceColumn)]; ok {
			resourceIdx = idx
		}
	}

	lifecycleIdx := -1
	if config.LifecycleColumn != "" {
		if idx, ok := colIndex[strings.ToLower(config.LifecycleColumn)]; ok {
			lifecycleIdx = idx
		}
	}

	// Parse events
	log := NewEventLog()
	lineNum := config.SkipRows + 2 // +1 for header, +1 for 1-based line numbers

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading line %d: %w", lineNum, err)
		}

		// Check minimum number of columns
		if len(record) <= caseIdx || len(record) <= activityIdx || len(record) <= timestampIdx {
			return nil, fmt.Errorf("line %d: insufficient columns", lineNum)
		}

		// Parse required fields
		caseID := strings.TrimSpace(record[caseIdx])
		activity := strings.TrimSpace(record[activityIdx])
		timestampStr := strings.TrimSpace(record[timestampIdx])

		if caseID == "" {
			return nil, fmt.Errorf("line %d: empty case ID", lineNum)
		}
		if activity == "" {
			return nil, fmt.Errorf("line %d: empty activity", lineNum)
		}

		// Parse timestamp
		timestamp, err := parseTimestamp(timestampStr, config.TimestampFormats)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid timestamp '%s': %w", lineNum, timestampStr, err)
		}

		// Create event
		event := Event{
			CaseID:     caseID,
			Activity:   activity,
			Timestamp:  timestamp,
			Attributes: make(map[string]interface{}),
		}

		// Parse optional fields
		if resourceIdx >= 0 && len(record) > resourceIdx {
			event.Resource = strings.TrimSpace(record[resourceIdx])
		}

		if lifecycleIdx >= 0 && len(record) > lifecycleIdx {
			event.Lifecycle = strings.TrimSpace(record[lifecycleIdx])
		}

		// Parse additional attributes (all columns not already used)
		for i, value := range record {
			// Skip columns we've already processed
			if i == caseIdx || i == activityIdx || i == timestampIdx ||
				i == resourceIdx || i == lifecycleIdx {
				continue
			}

			colName := header[i]
			if colName == "" {
				continue
			}

			// Try to parse as number, otherwise keep as string
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}

			if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
				event.Attributes[colName] = num
			} else {
				event.Attributes[colName] = trimmed
			}
		}

		log.AddEvent(event)
		lineNum++
	}

	// Sort all traces by timestamp
	log.SortTraces()

	return log, nil
}

// parseTimestamp tries multiple date formats to parse a timestamp string.
func parseTimestamp(s string, formats []string) (time.Time, error) {
	// Try each format
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	// If all formats fail, return error with the formats tried
	return time.Time{}, fmt.Errorf("could not parse timestamp with any of the configured formats")
}
