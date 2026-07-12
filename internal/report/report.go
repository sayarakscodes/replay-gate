// Package report renders a gate.Report in one of several formats (TRD §5.6):
// text and json for humans/machines, github for GitHub Actions annotations +
// job summary, and sarif for GitHub code scanning.
package report

import (
	"fmt"
	"io"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
)

// ReportVersion is the JSON schema version emitted in every "json" report, so
// consumers can detect breaking changes to the structure.
const ReportVersion = 1

const (
	FormatText   = "text"
	FormatJSON   = "json"
	FormatGitHub = "github"
	FormatSARIF  = "sarif"
)

// Write renders rep in the given format. An empty format defaults to text.
// failOn (FailOnOpen or FailOnAny) is used only by the github and sarif
// formats, to color each divergence by whether it actually blocks the build
// under that policy (an error if it blocks, a warning if it's warn-only) so
// the annotation severity matches the exit code; text/json ignore it.
//
// The github format writes GitHub Actions annotation commands to w, and, when
// the GITHUB_STEP_SUMMARY env var points at a file, also appends a Markdown
// job summary to it (that side-effect is intentional — a job summary can only
// be produced by writing that file, there's nowhere else for it to go).
func Write(w io.Writer, rep *Report, format, failOn string) error {
	switch format {
	case "", FormatText:
		return writeText(w, rep)
	case FormatJSON:
		return writeJSON(w, rep)
	case FormatGitHub:
		return writeGitHub(w, rep, failOn)
	case FormatSARIF:
		return writeSARIF(w, rep, failOn)
	default:
		return fmt.Errorf("unknown report format %q (want %q, %q, %q, or %q)", format, FormatText, FormatJSON, FormatGitHub, FormatSARIF)
	}
}

// blocks reports whether a divergence in a workflow of the given status would
// block the build (exit 1) under failOn, versus being warn-only (exit 2) —
// the same rule ExitCode uses, applied per-divergence for annotation severity.
func blocks(status, failOn string) bool {
	return failOn == FailOnAny || status == corpus.StatusRunning
}
