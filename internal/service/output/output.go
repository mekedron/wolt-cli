package output

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Format represents command output encoding.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// ParseFormat validates format values.
func ParseFormat(v string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(v))) {
	case "", FormatTable:
		return FormatTable, nil
	case FormatJSON:
		return FormatJSON, nil
	case FormatYAML:
		return FormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported format %q", v)
	}
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(buf)
}

// Envelope is the machine-output payload.
type Envelope struct {
	Meta     map[string]any `json:"meta" yaml:"meta"`
	Data     any            `json:"data" yaml:"data"`
	Warnings []string       `json:"warnings" yaml:"warnings"`
	Error    map[string]any `json:"error,omitempty" yaml:"error,omitempty"`
}

// BuildEnvelope constructs a response envelope.
func BuildEnvelope(profile, locale string, data any, warnings []string, errPayload map[string]any) Envelope {
	env := Envelope{
		Meta: map[string]any{
			"request_id":   newRequestID(),
			"generated_at": time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
			"profile":      profile,
			"locale":       locale,
		},
		Data:     data,
		Warnings: warnings,
		Error:    errPayload,
	}
	if env.Warnings == nil {
		env.Warnings = []string{}
	}
	return env
}

// RenderPayload renders payload in json/yaml format.
func RenderPayload(payload Envelope, format Format) (string, error) {
	switch format {
	case FormatJSON:
		bytes, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal json: %w", err)
		}
		return string(bytes), nil
	case FormatYAML:
		bytes, err := yaml.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("marshal yaml: %w", err)
		}
		return string(bytes), nil
	default:
		return "", fmt.Errorf("render payload only supports json/yaml")
	}
}

// WriteOutput writes output to the provided writer and optional file.
func WriteOutput(w io.Writer, text string, outputPath string) error {
	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(text), 0o644); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
	}
	if _, err := fmt.Fprintln(w, text); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// RenderTable renders plain text tables.
func RenderTable(title string, headers []string, rows [][]string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title)
		b.WriteByte('\n')
	}
	if len(headers) > 0 {
		b.WriteString(strings.Join(headers, "\t"))
		b.WriteByte('\n')
	}
	for _, row := range rows {
		b.WriteString(strings.Join(row, "\t"))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
