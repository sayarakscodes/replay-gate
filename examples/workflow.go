// Package examples is a minimal Temporal worker demonstrating Replay Gate
// integration. OrderWorkflow is the workflow under test; examples/corpus holds
// a real recorded history of it, and examples/replaygate is the registrations
// package the GitHub Action (action/action.yml) replays.
package examples

import (
	"time"

	"go.temporal.io/sdk/workflow"
)

// OrderWorkflow charges a card then ships the order — two sequential
// activities. examples/corpus records one real run of exactly this code, so
// replaying this package against that corpus is clean. examples/replaygate_regressed
// swaps the order to demonstrate a caught divergence.
func OrderWorkflow(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	if err := workflow.ExecuteActivity(ctx, "ChargeCard").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ShipOrder").Get(ctx, nil)
}
