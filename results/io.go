package results

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteJSON writes results to a JSON file
func WriteJSON(results *Results, filename string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// ReadJSON reads results from a JSON file
func ReadJSON(filename string) (*Results, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var results Results
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("unmarshal results: %w", err)
	}

	return &results, nil
}

// ToJSON converts results to JSON string
func ToJSON(results *Results) (string, error) {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON parses results from JSON string
func FromJSON(jsonStr string) (*Results, error) {
	var results Results
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil, err
	}
	return &results, nil
}
