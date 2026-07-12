// Command replaygate_regressed is a deliberately-broken copy of
// examples/replaygate: OrderWorkflow charges the card and ships in the WRONG
// order (ShipOrder before ChargeCard), incompatible with the recorded history
// in examples/corpus. It exists so the e2e workflow (.github/workflows) can
// prove the Action catches a real regression — it is not part of the demo a
// real user would ship.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func regressedOrderWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	if err := workflow.ExecuteActivity(ctx, "ShipOrder").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ChargeCard").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(regressedOrderWorkflow, workflow.RegisterOptions{Name: "OrderWorkflow"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
