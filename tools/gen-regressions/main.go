// Command gen-regressions produces the fixture corpora under
// testdata/regressions/. Each class runs its "before" workflow for real
// against a local Temporal dev server, records the resulting history, and
// writes it out in the standard corpus format (internal/corpus) so it can be
// replayed via `replaygate replay` like any other corpus.
//
// Requires a local dev server:
//
//	brew install temporal && temporal server start-dev --headless
//
// Then, from the repo root:
//
//	go run ./tools/gen-regressions
//
// Re-run this after adding a new class to internal/regressions.go's registry
// (see testdata/regressions/README.md).
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
)

const outDir = "testdata/regressions"

// activities shared across classes; each returns a fixed value so runs are
// reproducible and the recorded results are self-explanatory when inspected.
type activities struct{}

func (activities) ActivityA(ctx context.Context) (string, error)        { return "A-done", nil }
func (activities) ActivityB(ctx context.Context) (string, error)        { return "B-done", nil }
func (activities) ProcessOrder(ctx context.Context) (string, error)     { return "processed-v1", nil }
func (activities) SendNotification(ctx context.Context) (string, error) { return "sent", nil }

func withActivityOpts(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
}

// beforeReorderActivity calls A then B; the after/ package (hand-written,
// not generated) calls B then A.
func beforeReorderActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil)
}

// beforeRemovedActivity calls A then B; the after/ package drops the call to B.
func beforeRemovedActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil)
}

// beforeChangedTimer starts a real timer then calls an activity; the after/
// package drops the timer entirely.
func beforeChangedTimer(ctx workflow.Context) error {
	if err := workflow.NewTimer(ctx, 2*time.Second).Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(withActivityOpts(ctx), "ActivityA").Get(ctx, nil)
}

// beforeRenamedActivity calls the "SendNotification" activity type; the
// after/ package calls the same underlying logic under "SendNotificationV2".
func beforeRenamedActivity(ctx workflow.Context) error {
	return workflow.ExecuteActivity(withActivityOpts(ctx), "SendNotification").Get(ctx, nil)
}

// beforeAddedCommand calls only A; the after/ package appends an extra call to B.
func beforeAddedCommand(ctx workflow.Context) error {
	return workflow.ExecuteActivity(withActivityOpts(ctx), "ActivityA").Get(ctx, nil)
}

// beforeTimeNowRegression calls ProcessOrder unconditionally; the after/
// package branches on a hardcoded past cutoff (always already elapsed by the
// time the code runs) and calls ProcessOrderV2 instead — the classic
// "date-gated code path that flips for every in-flight workflow" bug.
func beforeTimeNowRegression(ctx workflow.Context) error {
	return workflow.ExecuteActivity(withActivityOpts(ctx), "ProcessOrder").Get(ctx, nil)
}

type class struct {
	name           string // corpus workflow type name, matches before/after RegisterWorkflowWithOptions
	taskQueue      string
	beforeWorkflow any
}

var classes = []class{
	{name: "ReorderActivity", taskQueue: "regressions-reorder-activity", beforeWorkflow: beforeReorderActivity},
	{name: "RemovedActivity", taskQueue: "regressions-removed-activity", beforeWorkflow: beforeRemovedActivity},
	{name: "ChangedTimer", taskQueue: "regressions-changed-timer", beforeWorkflow: beforeChangedTimer},
	{name: "RenamedActivity", taskQueue: "regressions-renamed-activity", beforeWorkflow: beforeRenamedActivity},
	{name: "AddedCommand", taskQueue: "regressions-added-command", beforeWorkflow: beforeAddedCommand},
	{name: "TimeNowRegression", taskQueue: "regressions-time-now", beforeWorkflow: beforeTimeNowRegression},
}

func main() {
	c, err := client.Dial(client.Options{HostPort: "127.0.0.1:7233", Namespace: "default"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "dialing local dev server:", err)
		fmt.Fprintln(os.Stderr, "start one with: temporal server start-dev --headless")
		os.Exit(1)
	}
	defer c.Close()

	for _, cl := range classes {
		if err := recordClass(c, cl); err != nil {
			fmt.Fprintf(os.Stderr, "class %s: %v\n", cl.name, err)
			os.Exit(1)
		}
		fmt.Println("recorded", cl.name)
	}
}

func recordClass(c client.Client, cl class) error {
	w := worker.New(c, cl.taskQueue, worker.Options{})
	w.RegisterWorkflowWithOptions(cl.beforeWorkflow, workflow.RegisterOptions{Name: cl.name})
	var a activities
	w.RegisterActivityWithOptions(a.ActivityA, activity.RegisterOptions{Name: "ActivityA"})
	w.RegisterActivityWithOptions(a.ActivityB, activity.RegisterOptions{Name: "ActivityB"})
	w.RegisterActivityWithOptions(a.ProcessOrder, activity.RegisterOptions{Name: "ProcessOrder"})
	w.RegisterActivityWithOptions(a.SendNotification, activity.RegisterOptions{Name: "SendNotification"})

	if err := w.Start(); err != nil {
		return fmt.Errorf("starting worker: %w", err)
	}
	defer w.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        fmt.Sprintf("regression-%s", cl.taskQueue),
		TaskQueue: cl.taskQueue,
	}, cl.name)
	if err != nil {
		return fmt.Errorf("starting workflow: %w", err)
	}
	if err := run.Get(ctx, nil); err != nil {
		return fmt.Errorf("workflow did not complete cleanly: %w", err)
	}

	hist, err := fetchHistory(ctx, c, run.GetID(), run.GetRunID())
	if err != nil {
		return fmt.Errorf("fetching history: %w", err)
	}

	return writeCorpus(cl, run.GetID(), run.GetRunID(), hist)
}

func fetchHistory(ctx context.Context, c client.Client, workflowID, runID string) (*historypb.History, error) {
	iter := c.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	var events []*historypb.HistoryEvent
	for iter.HasNext() {
		e, err := iter.Next()
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return &historypb.History{Events: events}, nil
}

func writeCorpus(cl class, workflowID, runID string, hist *historypb.History) error {
	classDir := filepath.Join(outDir, kebab(cl.name))
	corpusDir := filepath.Join(classDir, "corpus")
	histDir := filepath.Join(corpusDir, "histories", cl.name)
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		return err
	}

	data, err := protojson.MarshalOptions{Indent: "  "}.Marshal(hist)
	if err != nil {
		return err
	}
	relFile := filepath.Join("histories", cl.name, fmt.Sprintf("%s_%s.json", workflowID, runID))
	if err := os.WriteFile(filepath.Join(corpusDir, relFile), data, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(data)

	entries := []corpus.Entry{{
		File:         filepath.ToSlash(relFile),
		WorkflowType: cl.name,
		WorkflowID:   workflowID,
		RunID:        runID,
		Status:       corpus.StatusCompleted,
		EventCount:   len(hist.Events),
		SHA256:       hex.EncodeToString(sum[:]),
	}}

	manifest := corpus.Manifest{
		CorpusVersion: corpus.ComputeCorpusVersion(entries),
		FormatVersion: corpus.FormatVersion,
		SampledAt:     time.Now().UTC(),
		Cluster:       corpus.ClusterInfo{Namespace: "default", Endpoint: "127.0.0.1:7233 (local dev server)"},
		Redaction:     corpus.RedactionInfo{Profile: "none"},
		Entries:       entries,
	}
	mdata, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(corpusDir, "manifest.json"), mdata, 0o644)
}

func kebab(s string) string {
	var out []rune
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out = append(out, '-')
			}
			out = append(out, r-'A'+'a')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
