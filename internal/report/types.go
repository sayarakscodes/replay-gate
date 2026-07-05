package report

import (
	"time"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
)

// Process exit codes per TRD §5.6. The open/closed severity split (exit 2) is
// introduced in a later milestone; for now any divergence is exit 1.
const (
	ExitClean            = 0
	ExitDivergence       = 1
	ExitOperationalError = 3
)

// EntryResult is the outcome of replaying one corpus entry.
type EntryResult struct {
	Ref      corpus.EntryRef
	Status   string // corpus status at sampling time (RUNNING, COMPLETED, ...)
	Err      error
	Skipped  bool // true when excluded via an on-unregistered=skip-warn policy
	Duration time.Duration
}

func (r EntryResult) Passed() bool { return r.Err == nil }

// Report is the outcome of replaying an entire corpus, in stable corpus-manifest
// order regardless of which goroutine finished each entry first (N5).
type Report struct {
	CorpusDir     string
	CorpusVersion string
	Results       []EntryResult
}

// Divergences returns the entries that failed replay, excluding skipped entries.
func (r *Report) Divergences() []EntryResult {
	var out []EntryResult
	for _, res := range r.Results {
		if !res.Skipped && res.Err != nil {
			out = append(out, res)
		}
	}
	return out
}

// ExitCode maps the report to the process exit code contract in TRD §5.6.
func (r *Report) ExitCode() int {
	if len(r.Divergences()) > 0 {
		return ExitDivergence
	}
	return ExitClean
}
