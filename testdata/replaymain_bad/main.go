// Command replaymain_bad is a fixture Mode B registrations package with a
// deliberate regression: SimpleOrder drops its ChargeCard activity call, the
// "removed activity on an existing code path" case from the PRD's problem
// statement. Used by the CLI integration test to prove --registrations
// surfaces a real divergence with exit code 1.
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
	return nil // regression: the recorded history has a ChargeCard activity; this doesn't schedule one.
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
