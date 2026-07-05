package gate_test

import (
	"testing"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

const fixtureCorpus = "../../testdata/corpus"

var simpleOrderRef = corpus.EntryRef{
	WorkflowType: "SimpleOrder",
	WorkflowID:   "order-1",
	RunID:        "run-a1",
}

// matchingSimpleOrder mirrors the recorded fixture history exactly: schedule
// one ChargeCard activity, then complete.
func matchingSimpleOrder(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
	})
	var result any
	return workflow.ExecuteActivity(ctx, "ChargeCard", nil).Get(ctx, &result)
}

// regressedSimpleOrder drops the activity call entirely — the classic "removed
// activity on an existing code path" regression from the PRD's problem statement.
func regressedSimpleOrder(ctx workflow.Context) error {
	return nil
}

func TestReplayOne_Pass(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	g.RegisterWorkflowWithOptions(matchingSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})

	result, err := g.ReplayOne(simpleOrderRef)
	if err != nil {
		t.Fatalf("ReplayOne: %v", err)
	}
	if !result.Passed() {
		t.Fatalf("expected matching workflow to replay clean, got error: %v", result.Err)
	}
}

func TestReplayOne_Fail(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	g.RegisterWorkflowWithOptions(regressedSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})

	result, err := g.ReplayOne(simpleOrderRef)
	if err != nil {
		t.Fatalf("ReplayOne: %v", err)
	}
	if result.Passed() {
		t.Fatal("expected regressed workflow (missing activity) to fail replay, but it passed")
	}
	t.Logf("divergence (expected): %v", result.Err)
}

func TestReplayOne_UnknownEntry(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	g.RegisterWorkflowWithOptions(matchingSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})

	_, err := g.ReplayOne(corpus.EntryRef{WorkflowType: "SimpleOrder", WorkflowID: "does-not-exist", RunID: "run-zz"})
	if err == nil {
		t.Fatal("expected error for an entry ref not present in the corpus")
	}
}
