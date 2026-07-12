package report

import (
	"time"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/differ"
	"github.com/sayarakscodes/replay-gate/internal/patcher"
)

// Process exit codes per.
const (
	ExitClean = 0
	// ExitDivergence: a divergence exists in an open (RUNNING) workflow — or,
	// under FailOnAny, any divergence at all. Closed workflows never replay
	// in production, so this is the "will actually break something" signal.
	ExitDivergence = 1
	// ExitDivergenceWarn: divergences exist, but only in closed histories,
	// under the default FailOnOpen policy — worth surfacing, not worth
	// blocking a merge over.
	ExitDivergenceWarn   = 2
	ExitOperationalError = 3
)

// FailOn selects which divergences are "blocking" (ExitDivergence) versus
// "warn-only" (ExitDivergenceWarn) — see ExitCode.
const (
	FailOnOpen = "open"
	FailOnAny  = "any"
)

// EntryResult is the outcome of replaying one corpus entry.
type EntryResult struct {
	Ref      corpus.EntryRef
	Status   string // corpus status at sampling time (RUNNING, COMPLETED, ...)
	Err      error
	Skipped  bool // true when excluded via an on-unregistered=skip-warn policy
	Duration time.Duration
	// Divergence and Patch are populated whenever Err != nil and not Skipped
	// (F6/F4, ) — the differ's classification of Err, and the
	// patcher's suggested fix for it. Both nil for a passing or skipped entry.
	Divergence *differ.Divergence
	Patch      *patcher.Patch
}

func (r EntryResult) Passed() bool { return r.Err == nil }

// Report is the outcome of replaying an entire corpus, in stable corpus-manifest
// order regardless of which goroutine finished each entry first.
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

// OpenDivergences returns the divergences in a RUNNING (open) workflow — the
// subset that will actually break something already in flight in production.
func (r *Report) OpenDivergences() []EntryResult {
	var out []EntryResult
	for _, res := range r.Divergences() {
		if res.Status == corpus.StatusRunning {
			out = append(out, res)
		}
	}
	return out
}

// ExitCode maps the report to the process exit code contract in.
// failOn is FailOnOpen (default) or FailOnAny; an empty string is treated as
// FailOnOpen. An invalid value is treated the same as FailOnOpen — ExitCode
// has no error return, so callers that need to reject a bad flag value
// should validate it themselves before getting here.
func (r *Report) ExitCode(failOn string) int {
	divs := r.Divergences()
	if len(divs) == 0 {
		return ExitClean
	}
	if failOn == FailOnAny {
		return ExitDivergence
	}
	if len(r.OpenDivergences()) > 0 {
		return ExitDivergence
	}
	return ExitDivergenceWarn
}
