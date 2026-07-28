package eventlog

import (
	"strings"
	"testing"
)

// FuzzParseCSVReader feeds arbitrary text to the CSV event-log parser.
func FuzzParseCSVReader(f *testing.F) {
	seeds := []string{
		"",
		"case_id,activity,timestamp\n",
		"case_id,activity,timestamp\nc1,start,2026-01-01T10:00:00Z\n",
		"case_id,activity,timestamp\nc1,start,not-a-time\n",
		"just,three,words\nand,another,row",
		"\x00\x01\x02",
		"case_id,activity\nc1", // ragged row
		strings.Repeat("a,", 100) + "\n",
		`"unclosed quote,x,y`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		log, err := ParseCSVReader(strings.NewReader(src), DefaultCSVConfig())
		if err == nil && log == nil {
			t.Fatal("nil log with nil error")
		}
	})
}

// FuzzParseJSONLReader feeds arbitrary text to the JSONL event-log parser.
func FuzzParseJSONLReader(f *testing.F) {
	seeds := []string{
		"",
		"{}",
		`{"case_id":"c1","activity":"start","timestamp":"2026-01-01T10:00:00Z"}`,
		"{\"case_id\":\"c1\"}\n{\"activity\":\"x\"}\nnot json at all\n",
		"[1,2,3]",
		"null\nnull\n",
		`{"case_id":1,"activity":2,"timestamp":3}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		log, err := ParseJSONLReader(strings.NewReader(src), DefaultJSONLConfig())
		if err == nil && log == nil {
			t.Fatal("nil log with nil error")
		}
	})
}
