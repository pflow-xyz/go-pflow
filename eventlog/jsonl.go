package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// JSONLConfig configures JSONL parsing behavior.
type JSONLConfig struct {
	CaseIDField      string   // JSON field for case ID (required)
	ActivityField    string   // JSON field for activity (required)
	TimestampField   string   // JSON field for timestamp (required)
	ResourceField    string   // JSON field for resource (optional)
	LifecycleField   string   // JSON field for lifecycle (optional)
	TimestampFormats []string // Date/time formats to try (optional)
}

// DefaultJSONLConfig returns a configuration with common defaults.
func DefaultJSONLConfig() JSONLConfig {
	return JSONLConfig{
		CaseIDField:    "case_id",
		ActivityField:  "activity",
		TimestampField: "timestamp",
		ResourceField:  "resource",
		LifecycleField: "lifecycle",
		TimestampFormats: []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05.000",
			"2006-01-02T15:04:05.000",
			"2006-01-02T15:04:05Z",
			"2006-01-02",
		},
	}
}

// ParseJSONL parses an event log from a JSONL (JSON Lines) file.
// Each line should be a valid JSON object with event data.
func ParseJSONL(filename string, config JSONLConfig) (*EventLog, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	return ParseJSONLReader(f, config)
}

// ParseJSONLReader parses an event log from a JSONL reader.
func ParseJSONLReader(r io.Reader, config JSONLConfig) (*EventLog, error) {
	// Validate required fields
	if config.CaseIDField == "" {
		return nil, fmt.Errorf("CaseIDField is required")
	}
	if config.ActivityField == "" {
		return nil, fmt.Errorf("ActivityField is required")
	}
	if config.TimestampField == "" {
		return nil, fmt.Errorf("TimestampField is required")
	}

	// Use default timestamp formats if none provided
	if len(config.TimestampFormats) == 0 {
		config.TimestampFormats = DefaultJSONLConfig().TimestampFormats
	}

	log := NewEventLog()
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse JSON object
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}

		// Extract required fields
		caseID, err := extractString(record, config.CaseIDField)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		activity, err := extractString(record, config.ActivityField)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		timestamp, err := extractTimestamp(record, config.TimestampField, config.TimestampFormats)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		// Create event
		event := Event{
			CaseID:     caseID,
			Activity:   activity,
			Timestamp:  timestamp,
			Attributes: make(map[string]interface{}),
		}

		// Extract optional fields
		if config.ResourceField != "" {
			if resource, err := extractString(record, config.ResourceField); err == nil {
				event.Resource = resource
			}
		}

		if config.LifecycleField != "" {
			if lifecycle, err := extractString(record, config.LifecycleField); err == nil {
				event.Lifecycle = lifecycle
			}
		}

		// Copy remaining fields as attributes
		for key, value := range record {
			if key == config.CaseIDField || key == config.ActivityField ||
				key == config.TimestampField || key == config.ResourceField ||
				key == config.LifecycleField {
				continue
			}
			event.Attributes[key] = value
		}

		log.AddEvent(event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Sort all traces by timestamp
	log.SortTraces()

	return log, nil
}

// extractString extracts a string value from a JSON record.
func extractString(record map[string]interface{}, field string) (string, error) {
	value, ok := record[field]
	if !ok {
		return "", fmt.Errorf("missing required field '%s'", field)
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("empty value for field '%s'", field)
		}
		return v, nil
	case float64:
		// Handle numeric case IDs
		return fmt.Sprintf("%.0f", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// extractTimestamp extracts and parses a timestamp from a JSON record.
func extractTimestamp(record map[string]interface{}, field string, formats []string) (time.Time, error) {
	value, ok := record[field]
	if !ok {
		return time.Time{}, fmt.Errorf("missing required field '%s'", field)
	}

	switch v := value.(type) {
	case string:
		return parseTimestamp(v, formats)
	case float64:
		// Unix timestamp (seconds or milliseconds)
		if v > 1e12 {
			// Milliseconds
			return time.Unix(int64(v/1000), int64(v)%1000*1e6), nil
		}
		// Seconds
		return time.Unix(int64(v), 0), nil
	case int64:
		if v > 1e12 {
			return time.Unix(v/1000, v%1000*1e6), nil
		}
		return time.Unix(v, 0), nil
	default:
		return time.Time{}, fmt.Errorf("invalid timestamp type for field '%s': %T", field, value)
	}
}

// ParseJSONLBytes parses an event log from JSONL bytes.
func ParseJSONLBytes(data []byte, config JSONLConfig) (*EventLog, error) {
	return ParseJSONLReader(
		bufio.NewReader(
			&byteReader{data: data},
		),
		config,
	)
}

// byteReader wraps bytes for io.Reader interface.
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
