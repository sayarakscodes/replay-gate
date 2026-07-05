// Package report renders a gate.Report as text or JSON output (TRD §5.6).
package report

import (
	"fmt"
	"io"
)

// ReportVersion is the JSON schema version emitted in every "json" report, so
// consumers can detect breaking changes to the structure.
const ReportVersion = 1

const (
	FormatText = "text"
	FormatJSON = "json"
)

// Write renders rep in the given format. An empty format defaults to text.
func Write(w io.Writer, rep *Report, format string) error {
	switch format {
	case "", FormatText:
		return writeText(w, rep)
	case FormatJSON:
		return writeJSON(w, rep)
	default:
		return fmt.Errorf("unknown report format %q (want %q or %q)", format, FormatText, FormatJSON)
	}
}
