// Command replaymain_good is a fixture registrations package, used by
// the replaygate CLI's integration test (cmd/replaygate/replay_test.go). It
// mirrors the shape a real user's registrations package would take: register
// workflows, then hand off to gate.Main. All three workflows here match their
// recorded fixture history in testdata/corpus, so a replay against this
// package is expected to exit clean.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func withActivityOpts(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Minute})
}

func simpleOrder(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	var result any
	return workflow.ExecuteActivity(ctx, "ChargeCard", nil).Get(ctx, &result)
}

func invoiceFlow(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	if err := workflow.ExecuteActivity(ctx, "ValidateInvoice", nil).Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "SendInvoiceEmail", nil).Get(ctx, nil)
}

func shipmentWorkflow(ctx workflow.Context) error {
	ctx = withActivityOpts(ctx)
	return workflow.ExecuteActivity(ctx, "ReserveInventory", nil).Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(simpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})
	g.RegisterWorkflowWithOptions(invoiceFlow, workflow.RegisterOptions{Name: "InvoiceFlow"})
	g.RegisterWorkflowWithOptions(shipmentWorkflow, workflow.RegisterOptions{Name: "ShipmentWorkflow"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
