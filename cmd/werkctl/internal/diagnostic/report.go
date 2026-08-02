// Package diagnostic provides the stable, machine-readable result contract
// shared by read-only werkctl inspection commands.
package diagnostic

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	ExitHealthy = 0
	ExitWarning = 1
	ExitFailure = 2
	SchemaV1    = "v1"
)

// Status is the severity of a single check or a complete report.
type Status string

const (
	Pass Status = "PASS"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

// Check is deliberately small so its JSON representation remains a stable
// automation contract. Diagnosis and remediation are omitted for healthy
// checks unless they add operator value.
type Check struct {
	ID          string `json:"id"`
	Status      Status `json:"status"`
	Summary     string `json:"summary"`
	Diagnosis   string `json:"diagnosis,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// Counts summarizes a report without requiring consumers to recalculate it.
type Counts struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

// Report is the version-one output shape for werkctl diagnostics.
type Report struct {
	SchemaVersion string  `json:"schema_version"`
	Command       string  `json:"command"`
	Status        Status  `json:"status"`
	Summary       Counts  `json:"summary"`
	Checks        []Check `json:"checks"`
}

// NewReport calculates the aggregate status while preserving check order.
func NewReport(command string, checks []Check) Report {
	normalized := append([]Check(nil), checks...)
	if len(normalized) == 0 {
		normalized = []Check{{
			ID: "diagnostic.empty-report", Status: Fail,
			Summary:     "Diagnoselauf hat keine Prüfresultate erzeugt",
			Diagnosis:   "Der interne Diagnosevertrag wurde nicht erfüllt.",
			Remediation: "werkctl-Version und Command-Implementierung prüfen.",
		}}
	}
	for index, check := range normalized {
		if check.ID != "" && check.Summary != "" && validStatus(check.Status) {
			continue
		}
		normalized[index] = Check{
			ID: fmt.Sprintf("diagnostic.invalid-check.%d", index+1), Status: Fail,
			Summary:     "Diagnoseprüfung verletzt den internen Ergebnisvertrag",
			Diagnosis:   "ID, Status oder Zusammenfassung fehlen beziehungsweise sind ungültig.",
			Remediation: "werkctl-Version und Command-Implementierung prüfen.",
		}
	}
	report := Report{
		SchemaVersion: SchemaV1,
		Command:       command,
		Status:        Pass,
		Checks:        normalized,
	}
	for _, check := range normalized {
		switch check.Status {
		case Pass:
			report.Summary.Pass++
		case Warn:
			report.Summary.Warn++
			if report.Status == Pass {
				report.Status = Warn
			}
		case Fail:
			report.Summary.Fail++
			report.Status = Fail
		}
	}
	return report
}

func validStatus(status Status) bool {
	return status == Pass || status == Warn || status == Fail
}

// ExitCode maps reports to the documented process contract.
func (report Report) ExitCode() int {
	switch report.Status {
	case Pass:
		return ExitHealthy
	case Warn:
		return ExitWarning
	default:
		return ExitFailure
	}
}

// Write renders either deterministic human output or the JSON contract.
func (report Report) Write(writer io.Writer, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(report)
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(writer, "%s %-28s %s\n", check.Status, check.ID, check.Summary); err != nil {
			return err
		}
		if check.Diagnosis != "" {
			if _, err := fmt.Fprintf(writer, "     Diagnose: %s\n", check.Diagnosis); err != nil {
				return err
			}
		}
		if check.Remediation != "" {
			if _, err := fmt.Fprintf(writer, "     Abhilfe: %s\n", check.Remediation); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(
		writer,
		"RESULT %s (%d PASS, %d WARN, %d FAIL)\n",
		report.Status,
		report.Summary.Pass,
		report.Summary.Warn,
		report.Summary.Fail,
	)
	return err
}
