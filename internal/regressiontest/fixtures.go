// Package regressiontest centralizes the 6 regression classes' "after"
// workflow bodies and corpus-loading, shared by internal/differ's and
// internal/patcher's tests, so the fixtures behind the "all 6 classes caught"
// metric are defined in exactly one place. These must stay in exact lockstep
// with testdata/regressions/*/after/main.go — see that directory's README for
// how to add a new class.
package regressiontest

import (
	"path/filepath"
	"testing"
	"time"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
)

func withActivityOpts(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
}

func AfterReorderActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func AfterRemovedActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func AfterChangedTimer(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func AfterRenamedActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "SendNotificationV2").Get(ctx, nil)
}

func AfterAddedCommand(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil)
}

func AfterTimeNowRegression(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if time.Now().Before(cutoff) {
		return workflow.ExecuteActivity(ctx, "ProcessOrder").Get(ctx, nil)
	}
	return workflow.ExecuteActivity(ctx, "ProcessOrderV2").Get(ctx, nil)
}

// Fixture pairs a regression class with its "after" (regressed) workflow function.
type Fixture struct {
	Class        string // testdata/regressions/<Class>
	WorkflowType string
	AfterFn      any
}

var Fixtures = []Fixture{
	{"reorder-activity", "ReorderActivity", AfterReorderActivity},
	{"removed-activity", "RemovedActivity", AfterRemovedActivity},
	{"changed-timer", "ChangedTimer", AfterChangedTimer},
	{"renamed-activity", "RenamedActivity", AfterRenamedActivity},
	{"added-command", "AddedCommand", AfterAddedCommand},
	{"time-now-regression", "TimeNowRegression", AfterTimeNowRegression},
}

// LoadHistory loads the single recorded history for a fixture's corpus.
// dir is the path from the caller's package to the repo root (e.g. "../..").
func LoadHistory(t testing.TB, dir, class, workflowType string) *historypb.History {
	t.Helper()
	c, err := corpus.Load(filepath.Join(dir, "testdata", "regressions", class, "corpus"))
	if err != nil {
		t.Fatalf("loading corpus for %s: %v", class, err)
	}
	for ref, h := range c.Histories {
		if ref.WorkflowType == workflowType {
			return h
		}
	}
	t.Fatalf("no history for workflow type %s in %s's corpus", workflowType, class)
	return nil
}
