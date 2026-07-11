package differ_test

import (
	"testing"
	"time"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/differ"
	"github.com/sayarakscodes/replay-gate/internal/replayer"
)

// This is the SDK-drift canary (TRD §5.4): the "after" workflow functions
// below are exact copies of testdata/regressions/*/after/main.go's logic,
// replayed against the real recorded histories committed there. If a future
// SDK version changes the "[TMPRL1100]" message text, classify.go's regex
// table stops matching and this test fails loudly — which is the point: the
// differ's regex table is coupled to the pinned SDK range (TRD §2, §8 risk),
// and this test is what notices when that coupling breaks.
//
// Duplicating the workflow bodies here (rather than importing the `after`
// packages, which are separate `package main` directories and thus not
// importable) is deliberate: it also lets the source-scan heuristic
// (construct.go) run against this file's own source, so the time.Now() case
// exercises the real heuristic, not a stub.

func withActivityOpts(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
}

func afterReorderActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func afterRemovedActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func afterChangedTimer(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func afterRenamedActivity(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "SendNotificationV2").Get(ctx, nil)
}

func afterAddedCommand(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil)
}

func afterTimeNowRegression(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if time.Now().Before(cutoff) {
		return workflow.ExecuteActivity(ctx, "ProcessOrder").Get(ctx, nil)
	}
	return workflow.ExecuteActivity(ctx, "ProcessOrderV2").Get(ctx, nil)
}

func TestCanary_RegressionBattery(t *testing.T) {
	tests := []struct {
		class        string
		workflowType string
		fn           any
		wantClass    differ.Class
	}{
		{"reorder-activity", "ReorderActivity", afterReorderActivity, differ.ClassReorder},
		{"removed-activity", "RemovedActivity", afterRemovedActivity, differ.ClassRemoved},
		{"changed-timer", "ChangedTimer", afterChangedTimer, differ.ClassRemoved},
		{"renamed-activity", "RenamedActivity", afterRenamedActivity, differ.ClassRename},
		{"added-command", "AddedCommand", afterAddedCommand, differ.ClassAdded},
		{"time-now-regression", "TimeNowRegression", afterTimeNowRegression, differ.ClassNondeterministicConstruct},
	}

	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			c, err := corpus.Load("../../testdata/regressions/" + tt.class + "/corpus")
			if err != nil {
				t.Fatalf("loading corpus: %v", err)
			}

			var hist *historypb.History
			for ref, h := range c.Histories {
				if ref.WorkflowType == tt.workflowType {
					hist = h
					break
				}
			}
			if hist == nil {
				t.Fatalf("no history for workflow type %s in corpus", tt.workflowType)
			}

			result := replayer.ReplayOne(
				[]replayer.Registration{{Fn: tt.fn, Name: tt.workflowType}},
				nil, hist,
			)
			if result.Passed() {
				t.Fatalf("expected the regressed workflow to diverge, but replay passed clean")
			}

			d := differ.Classify(result.Err, hist, tt.fn)
			if d.Class != tt.wantClass {
				t.Errorf("expected class %q, got %q\nraw error: %s\nnote: %s", tt.wantClass, d.Class, d.RawError, d.Note)
			}
			if d.RawError == "" {
				t.Error("RawError must always be populated")
			}
		})
	}
}
