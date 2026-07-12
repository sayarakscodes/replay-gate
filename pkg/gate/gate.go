// Package gate is the public embedding API for Replay Gate:
// a caller registers their workflow functions and replays corpus histories
// against them, from a Go test in their own repo.
package gate

import (
	"fmt"
	"log/slog"
	"os"

	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/replayer"
)

// EntryRef identifies a single workflow execution's history within a corpus.
type EntryRef = corpus.EntryRef

type Config struct {
	// CorpusDir is the path to a corpus directory as produced by `replaygate sample`
	// or hand-built per.
	CorpusDir string
	// Logger is optional; defaults to a stderr logger if unset. This matters
	// beyond cosmetics: the SDK's own default logger writes to os.Stdout,
	// which would corrupt a "json" report also written to stdout.
	Logger log.Logger
}

// Gate replays corpus histories against a set of registered workflow functions.
type Gate struct {
	cfg           Config
	registrations []replayer.Registration
	corpus        *corpus.Corpus
}

func New(cfg Config) *Gate {
	if cfg.Logger == nil {
		cfg.Logger = log.NewStructuredLogger(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}
	return &Gate{cfg: cfg}
}

// RegisterWorkflow registers a workflow function under its default (inferred) name.
func (g *Gate) RegisterWorkflow(fn any) {
	g.registrations = append(g.registrations, replayer.Registration{Fn: fn})
}

// RegisterWorkflowWithOptions registers a workflow function under an explicit name.
func (g *Gate) RegisterWorkflowWithOptions(fn any, options workflow.RegisterOptions) {
	g.registrations = append(g.registrations, replayer.Registration{Fn: fn, Name: options.Name})
}

// HistoryResult is the outcome of replaying one corpus entry.
type HistoryResult struct {
	Ref EntryRef
	Err error
}

func (r HistoryResult) Passed() bool { return r.Err == nil }

// loadCorpus loads and caches the corpus on first use; later calls reuse it.
func (g *Gate) loadCorpus() (*corpus.Corpus, error) {
	if g.corpus != nil {
		return g.corpus, nil
	}
	c, err := corpus.Load(g.cfg.CorpusDir)
	if err != nil {
		return nil, fmt.Errorf("loading corpus %s: %w", g.cfg.CorpusDir, err)
	}
	g.corpus = c
	return c, nil
}

// ReplayOne replays a single named history from the configured corpus against
// the registered workflows and returns its pass/fail result. It's the
// single-history mechanism that ReplayAll (parallel replay of a whole corpus)
// builds on.
func (g *Gate) ReplayOne(ref EntryRef) (HistoryResult, error) {
	c, err := g.loadCorpus()
	if err != nil {
		return HistoryResult{}, err
	}

	hist, ok := c.Histories[ref]
	if !ok {
		return HistoryResult{}, fmt.Errorf("no history for %s in corpus %s", ref, g.cfg.CorpusDir)
	}

	result := replayer.ReplayOne(g.registrations, g.cfg.Logger, hist)
	return HistoryResult{Ref: ref, Err: result.Err}, nil
}
