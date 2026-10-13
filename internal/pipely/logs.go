package pipely

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Azure may return a JSON envelope even when text/plain was requested.
func decodeLog(text string) string {
	var envelope struct {
		Value []string `json:"value"`
	}
	if json.Unmarshal([]byte(text), &envelope) == nil && envelope.Value != nil {
		return strings.Join(envelope.Value, "\n") + "\n"
	}
	var lines []string
	if json.Unmarshal([]byte(text), &lines) == nil && lines != nil {
		return strings.Join(lines, "\n") + "\n"
	}
	return text
}

// PrettyLog expands structured log records while preserving ordinary output.
func PrettyLog(text string) string {
	text = decodeLog(text)
	var whole bytes.Buffer
	if json.Indent(&whole, []byte(strings.TrimSpace(text)), "", "  ") == nil {
		return whole.String() + "\n"
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		start := strings.IndexAny(line, "{[")
		if start < 0 {
			continue
		}
		var formatted bytes.Buffer
		if json.Indent(&formatted, []byte(strings.TrimSpace(line[start:])), "", "  ") == nil {
			lines[i] = line[:start] + formatted.String()
		}
	}
	return strings.Join(lines, "\n")
}
