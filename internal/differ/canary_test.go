package differ_test

import (
	"testing"

	"github.com/sayarakscodes/replay-gate/internal/differ"
	"github.com/sayarakscodes/replay-gate/internal/regressiontest"
	"github.com/sayarakscodes/replay-gate/internal/replayer"
)

// This is the SDK-drift canary: internal/regressiontest's "after"
// workflow functions are exact copies of testdata/regressions/*/after/main.go's
// logic, replayed here against the real recorded histories committed there.
// If a future SDK version changes the "[TMPRL1100]" message text,
// classify.go's regex table stops matching and this test fails loudly —
// which is the point: the differ's regex table is coupled to the pinned SDK
// range, and this test is what notices when that coupling
// breaks.
func TestCanary_RegressionBattery(t *testing.T) {
	wantClass := map[string]differ.Class{
		"reorder-activity":    differ.ClassReorder,
		"removed-activity":    differ.ClassRemoved,
		"changed-timer":       differ.ClassRemoved,
		"renamed-activity":    differ.ClassRename,
		"added-command":       differ.ClassAdded,
		"time-now-regression": differ.ClassNondeterministicConstruct,
	}

	for _, f := range regressiontest.Fixtures {
		t.Run(f.Class, func(t *testing.T) {
			hist := regressiontest.LoadHistory(t, "../..", f.Class, f.WorkflowType)

			result := replayer.ReplayOne(
				[]replayer.Registration{{Fn: f.AfterFn, Name: f.WorkflowType}},
				nil, hist,
			)
			if result.Passed() {
				t.Fatalf("expected the regressed workflow to diverge, but replay passed clean")
			}

			d := differ.Classify(result.Err, hist, f.AfterFn)
			if want := wantClass[f.Class]; d.Class != want {
				t.Errorf("expected class %q, got %q\nraw error: %s\nnote: %s", want, d.Class, d.RawError, d.Note)
			}
			if d.RawError == "" {
				t.Error("RawError must always be populated")
			}
		})
	}
}
