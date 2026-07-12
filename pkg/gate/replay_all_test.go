package gate_test

import (
	"testing"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func withActivityOpts(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Minute})
}

func matchingInvoiceFlow(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ValidateInvoice", nil).Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "SendInvoiceEmail", nil).Get(ctx, nil)
}

func matchingShipmentWorkflow(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "ReserveInventory", nil).Get(ctx, nil)
}

func registerAllMatching(g *gate.Gate) {
	g.RegisterWorkflowWithOptions(matchingSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})
	g.RegisterWorkflowWithOptions(matchingInvoiceFlow, workflow.RegisterOptions{Name: "InvoiceFlow"})
	g.RegisterWorkflowWithOptions(matchingShipmentWorkflow, workflow.RegisterOptions{Name: "ShipmentWorkflow"})
}

// expectedOrder mirrors testdata/corpus/manifest.json's entry order.
var expectedOrder = []string{"SimpleOrder", "InvoiceFlow", "ShipmentWorkflow"}

func TestReplayAll_AllMatching_Clean(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	registerAllMatching(g)

	rep, err := g.ReplayAll(gate.ReplayAllOptions{})
	if err != nil {
		t.Fatalf("ReplayAll: %v", err)
	}
	if rep.ExitCode(gate.FailOnOpen) != gate.ExitClean {
		t.Fatalf("expected ExitClean, got %d (divergences: %+v)", rep.ExitCode(gate.FailOnOpen), rep.Divergences())
	}
	if len(rep.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(rep.Results))
	}
	for i, want := range expectedOrder {
		if got := rep.Results[i].Ref.WorkflowType; got != want {
			t.Errorf("result[%d]: expected workflow type %s in manifest order, got %s", i, want, got)
		}
		if !rep.Results[i].Passed() {
			t.Errorf("result[%d] (%s): expected pass, got error: %v", i, want, rep.Results[i].Err)
		}
	}
}

func TestReplayAll_OrderingStableAcrossParallelism(t *testing.T) {
	for _, p := range []int{1, 2, 4, 8} {
		g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
		registerAllMatching(g)

		rep, err := g.ReplayAll(gate.ReplayAllOptions{Parallelism: p})
		if err != nil {
			t.Fatalf("parallelism=%d: ReplayAll: %v", p, err)
		}
		for i, want := range expectedOrder {
			if got := rep.Results[i].Ref.WorkflowType; got != want {
				t.Errorf("parallelism=%d: result[%d]: expected %s, got %s", p, i, want, got)
			}
		}
	}
}

func TestReplayAll_RegressedEntry_Divergence(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	g.RegisterWorkflowWithOptions(regressedSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})
	g.RegisterWorkflowWithOptions(matchingInvoiceFlow, workflow.RegisterOptions{Name: "InvoiceFlow"})
	g.RegisterWorkflowWithOptions(matchingShipmentWorkflow, workflow.RegisterOptions{Name: "ShipmentWorkflow"})

	rep, err := g.ReplayAll(gate.ReplayAllOptions{})
	if err != nil {
		t.Fatalf("ReplayAll: %v", err)
	}
	// SimpleOrder's fixture history is COMPLETED (closed) — under the default
	// fail-on=open this is a warning, not a blocker (PRD open question 2).
	if rep.ExitCode(gate.FailOnOpen) != gate.ExitDivergenceWarn {
		t.Fatalf("expected ExitDivergenceWarn for a closed-only divergence under fail-on=open, got %d", rep.ExitCode(gate.FailOnOpen))
	}
	if rep.ExitCode(gate.FailOnAny) != gate.ExitDivergence {
		t.Fatalf("expected ExitDivergence under fail-on=any, got %d", rep.ExitCode(gate.FailOnAny))
	}
	divs := rep.Divergences()
	if len(divs) != 1 || divs[0].Ref.WorkflowType != "SimpleOrder" {
		t.Fatalf("expected exactly one divergence on SimpleOrder, got %+v", divs)
	}
}

func TestReplayAll_UnregisteredType_FailsByDefault(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	g.RegisterWorkflowWithOptions(matchingSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})
	// InvoiceFlow and ShipmentWorkflow deliberately left unregistered.

	_, err := g.ReplayAll(gate.ReplayAllOptions{})
	if err == nil {
		t.Fatal("expected an error for unregistered workflow types with the default on-unregistered=fail policy")
	}
}

func TestReplayAll_UnregisteredType_SkipWarn(t *testing.T) {
	g := gate.New(gate.Config{CorpusDir: fixtureCorpus})
	g.RegisterWorkflowWithOptions(matchingSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})

	rep, err := g.ReplayAll(gate.ReplayAllOptions{OnUnregistered: gate.OnUnregisteredSkipWarn})
	if err != nil {
		t.Fatalf("ReplayAll: %v", err)
	}
	if rep.ExitCode(gate.FailOnOpen) != gate.ExitClean {
		t.Fatalf("skipped entries must not count as divergences, got exit code %d", rep.ExitCode(gate.FailOnOpen))
	}

	var skipped, passed int
	for _, r := range rep.Results {
		switch {
		case r.Skipped:
			skipped++
		case r.Passed():
			passed++
		}
	}
	if skipped != 2 || passed != 1 {
		t.Fatalf("expected 1 passed + 2 skipped, got %d passed, %d skipped", passed, skipped)
	}
}
