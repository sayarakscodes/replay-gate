package patcher_test

import (
	"strings"
	"testing"

	"github.com/sayarakscodes/replay-gate/internal/differ"
	"github.com/sayarakscodes/replay-gate/internal/patcher"
	"github.com/sayarakscodes/replay-gate/internal/regressiontest"
	"github.com/sayarakscodes/replay-gate/internal/replayer"
)

// classifyFixture replays one regression-battery fixture and classifies the
// resulting divergence — the same real Divergence values #7's canary
// produces, not synthetic ones, so these golden snapshots reflect what the
// patcher actually renders in practice.
func classifyFixture(t *testing.T, f regressiontest.Fixture) differ.Divergence {
	t.Helper()
	hist := regressiontest.LoadHistory(t, "../..", f.Class, f.WorkflowType)
	result := replayer.ReplayOne([]replayer.Registration{{Fn: f.AfterFn, Name: f.WorkflowType}}, nil, hist)
	if result.Passed() {
		t.Fatalf("expected %s's regressed workflow to diverge", f.Class)
	}
	return differ.Classify(result.Err, hist, f.AfterFn)
}

func fixtureByClass(t *testing.T, class string) regressiontest.Fixture {
	t.Helper()
	for _, f := range regressiontest.Fixtures {
		if f.Class == class {
			return f
		}
	}
	t.Fatalf("no fixture named %s", class)
	return regressiontest.Fixture{}
}

// Golden snapshots: one per class in the regression battery (#7). Failures
// here mean the rendered patch text changed — review the diff and update
// deliberately, don't just paste the new output back in.
var goldenSnippets = map[string]string{
	"reorder-activity": `v := workflow.GetVersion(ctx, "reorder-activity-a-activity-b", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	// old order: in-flight histories recorded this sequence
	workflow.ExecuteActivity(ctx, "ActivityA" /* input */).Get(ctx, nil)
	workflow.ExecuteActivity(ctx, "ActivityB" /* input */).Get(ctx, nil)
} else {
	// new order
	workflow.ExecuteActivity(ctx, "ActivityB" /* input */).Get(ctx, nil)
	workflow.ExecuteActivity(ctx, "ActivityA" /* input */).Get(ctx, nil)
}`,
	"removed-activity": `v := workflow.GetVersion(ctx, "removed-activity-b", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	// old path: keep the removed call so in-flight histories still match
	workflow.ExecuteActivity(ctx, "ActivityB" /* original input */).Get(ctx, nil)
}`,
	"changed-timer": `v := workflow.GetVersion(ctx, "removed-event-5", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	// old path: keep the removed call so in-flight histories still match
	workflow.NewTimer(ctx, /* original duration */).Get(ctx, nil)
}`,
	"renamed-activity": `v := workflow.GetVersion(ctx, "rename-send-notification-to-send-notification-v2", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	workflow.ExecuteActivity(ctx, "SendNotification" /* input */).Get(ctx, nil)
} else {
	workflow.ExecuteActivity(ctx, "SendNotificationV2" /* input */).Get(ctx, nil)
}`,
	"added-command": `v := workflow.GetVersion(ctx, "added-activity-b", workflow.DefaultVersion, 1)
if v != workflow.DefaultVersion {
	// new path: only run this for workflows started after this change
	workflow.ExecuteActivity(ctx, "ActivityB" /* input */).Get(ctx, nil)
}`,
}

func TestSuggest_GoldenSnippets(t *testing.T) {
	for class, want := range goldenSnippets {
		t.Run(class, func(t *testing.T) {
			d := classifyFixture(t, fixtureByClass(t, class))
			p, ok := patcher.Suggest(d)
			if !ok {
				t.Fatalf("expected a GetVersion snippet for class %q, got guidance-only: %s", d.Class, p.Guidance)
			}
			if p.Snippet != want {
				t.Errorf("snippet mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", class, p.Snippet, want)
			}
			if p.ChangeID == "" {
				t.Error("expected a non-empty ChangeID")
			}
		})
	}
}

func TestSuggest_TimeNowRegression_GuidanceOnly(t *testing.T) {
	d := classifyFixture(t, fixtureByClass(t, "time-now-regression"))
	if d.Class != differ.ClassNondeterministicConstruct {
		t.Fatalf("expected the fixture to classify as nondeterministic-construct, got %q", d.Class)
	}
	p, ok := patcher.Suggest(d)
	if ok {
		t.Fatalf("expected guidance-only (no GetVersion snippet) for nondeterministic-construct, got snippet: %s", p.Snippet)
	}
	if p.Snippet != "" {
		t.Error("guidance-only patches must not also set Snippet")
	}
	if !strings.Contains(p.Guidance, "time.Now") {
		t.Errorf("expected guidance to mention the detected construct, got: %s", p.Guidance)
	}
}

func TestSuggest_Panic_GuidanceOnly(t *testing.T) {
	d := differ.Divergence{Class: differ.ClassPanic, RawError: "panic during replay: runtime error: index out of range [3] with length 2"}
	p, ok := patcher.Suggest(d)
	if ok {
		t.Fatal("expected guidance-only for ClassPanic")
	}
	if !strings.Contains(p.Guidance, "index out of range") {
		t.Errorf("expected guidance to include the raw error, got: %s", p.Guidance)
	}
}

func TestSuggest_Unknown_GuidanceOnly(t *testing.T) {
	d := differ.Divergence{Class: differ.ClassUnknown, RawError: "[TMPRL1100] some future message"}
	p, ok := patcher.Suggest(d)
	if ok {
		t.Fatal("expected guidance-only for ClassUnknown")
	}
	if !strings.Contains(p.Guidance, d.RawError) {
		t.Errorf("expected guidance to include the raw error, got: %s", p.Guidance)
	}
}

func TestPatcher_SuffixesOnCollision(t *testing.T) {
	d := differ.Divergence{
		Class:     differ.ClassAdded,
		Generated: &differ.CommandSummary{Name: "ActivityB"},
	}

	p := patcher.New()
	first, _ := p.Suggest(d)
	second, _ := p.Suggest(d)
	third, _ := p.Suggest(d)

	if first.ChangeID != "added-activity-b" {
		t.Errorf("expected the first occurrence to use the base changeID, got %q", first.ChangeID)
	}
	if second.ChangeID == first.ChangeID {
		t.Error("expected the second occurrence to get a suffixed changeID")
	}
	if third.ChangeID == second.ChangeID {
		t.Error("expected the third occurrence to get a different suffix than the second")
	}

	for _, patch := range []patcher.Patch{first, second, third} {
		if !strings.Contains(patch.Snippet, patch.ChangeID) {
			t.Errorf("expected snippet to reference its own changeID %q:\n%s", patch.ChangeID, patch.Snippet)
		}
	}
}

func TestPatcher_IndependentInstancesDontShareState(t *testing.T) {
	d := differ.Divergence{Class: differ.ClassAdded, Generated: &differ.CommandSummary{Name: "ActivityB"}}
	p1, p2 := patcher.New(), patcher.New()
	a, _ := p1.Suggest(d)
	b, _ := p2.Suggest(d)
	if a.ChangeID != b.ChangeID {
		t.Errorf("expected independent Patcher instances to both produce the base changeID, got %q vs %q", a.ChangeID, b.ChangeID)
	}
}

func TestSuggest_NeverPanicsOnEmptyDivergence(t *testing.T) {
	for _, class := range []differ.Class{
		differ.ClassReorder, differ.ClassRemoved, differ.ClassAdded, differ.ClassRename,
		differ.ClassPanic, differ.ClassNondeterministicConstruct, differ.ClassUnknown, differ.Class("future-class"),
	} {
		_, _ = patcher.Suggest(differ.Divergence{Class: class})
	}
}
