package quirks

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// rawQuirksJSON is the embedded contents of /quirks/quirks.json — the single
// source of truth shared between the Go and TypeScript testkits.
//
//go:embed quirks.json
var rawQuirksJSON []byte

type rawFirmware struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type rawQuirk struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Severity    string       `json:"severity"`
	Description string       `json:"description"`
	Source      string       `json:"source"`
	Firmware    rawFirmware  `json:"firmware"`
	MatchRegex  string       `json:"match_regex"`
	Detect      []DetectRule `json:"detect,omitempty"`
}

type rawRegistry struct {
	SchemaVersion string     `json:"schema_version"`
	Quirks        []rawQuirk `json:"quirks"`
}

func init() {
	var reg rawRegistry
	if err := json.Unmarshal(rawQuirksJSON, &reg); err != nil {
		panic(fmt.Sprintf("quirks: invalid quirks.json: %v", err))
	}

	for _, raw := range reg.Quirks {
		q := Quirk{
			ID:          raw.ID,
			Name:        raw.Name,
			Category:    Category(raw.Category),
			Severity:    parseSeverity(raw.Severity),
			Description: raw.Description,
			Source:      raw.Source,
			Firmware:    FirmwareRange{Min: raw.Firmware.Min, Max: raw.Firmware.Max},
			Patterns:    raw.Detect,
		}
		attachCallbacks(&q)
		Register(q)
	}
}

func parseSeverity(s string) Severity {
	switch s {
	case "hint":
		return SeverityHint
	case "warning":
		return SeverityWarning
	case "critical":
		return SeverityCritical
	}
	panic("quirks: unknown severity " + s)
}
