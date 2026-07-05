package gate

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/sayarakscodes/replay-gate/internal/replayer"
	"github.com/sayarakscodes/replay-gate/internal/report"
)

// Behavior for corpus entries whose workflow type has no matching registration
// (TRD §9): the default is a hard failure, since a silently skipped history is a
// coverage hole, not a clean result.
const (
	OnUnregisteredFail     = "fail"
	OnUnregisteredSkipWarn = "skip-warn"
)

// Process exit codes, re-exported from internal/report for callers of this
// package (TRD §5.6).
const (
	ExitClean            = report.ExitClean
	ExitDivergence       = report.ExitDivergence
	ExitOperationalError = report.ExitOperationalError
)

// Report and EntryResult are owned by internal/report (which also renders
// them); gate re-exports them by alias so callers don't need a second import.
type (
	Report      = report.Report
	EntryResult = report.EntryResult
)

type ReplayAllOptions struct {
	// Parallelism is the number of concurrent replay workers. 0 means GOMAXPROCS.
	Parallelism int
	// OnUnregistered controls how entries with an unregistered workflow type are
	// handled: OnUnregisteredFail (default) aborts the whole run, OnUnregisteredSkipWarn
	// excludes them from the report instead.
	OnUnregistered string
}

// ReplayAll replays every entry in the configured corpus in parallel, one
// worker.WorkflowReplayer per goroutine (the SDK type isn't safe to share across
// concurrent replays), and returns a report in deterministic corpus order.
func (g *Gate) ReplayAll(opts ReplayAllOptions) (*Report, error) {
	c, err := g.loadCorpus()
	if err != nil {
		return nil, err
	}

	parallelism := opts.Parallelism
	if parallelism <= 0 {
		parallelism = runtime.GOMAXPROCS(0)
	}

	onUnregistered := opts.OnUnregistered
	if onUnregistered == "" {
		onUnregistered = OnUnregisteredFail
	}
	if onUnregistered != OnUnregisteredFail && onUnregistered != OnUnregisteredSkipWarn {
		return nil, fmt.Errorf("invalid on-unregistered value %q (want %q or %q)", onUnregistered, OnUnregisteredFail, OnUnregisteredSkipWarn)
	}

	entries := c.Manifest.Entries
	results := make([]EntryResult, len(entries))

	jobs := make(chan int, len(entries))
	for i := range entries {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstUnregisteredErr error

	worker := func() {
		defer wg.Done()
		for i := range jobs {
			e := entries[i]
			ref := EntryRef{WorkflowType: e.WorkflowType, WorkflowID: e.WorkflowID, RunID: e.RunID}
			hist := c.Histories[ref]

			start := time.Now()
			res := replayer.ReplayOne(g.registrations, g.cfg.Logger, hist)
			dur := time.Since(start)

			if res.Err != nil && replayer.IsUnregisteredWorkflowType(res.Err) {
				if onUnregistered == OnUnregisteredSkipWarn {
					results[i] = EntryResult{Ref: ref, Status: e.Status, Skipped: true, Duration: dur}
					continue
				}
				mu.Lock()
				if firstUnregisteredErr == nil {
					firstUnregisteredErr = fmt.Errorf("entry %s: %w", ref, res.Err)
				}
				mu.Unlock()
			}

			results[i] = EntryResult{Ref: ref, Status: e.Status, Err: res.Err, Duration: dur}
		}
	}

	wg.Add(parallelism)
	for w := 0; w < parallelism; w++ {
		go worker()
	}
	wg.Wait()

	if firstUnregisteredErr != nil {
		return nil, firstUnregisteredErr
	}

	return &Report{CorpusDir: g.cfg.CorpusDir, CorpusVersion: c.Manifest.CorpusVersion, Results: results}, nil
}
