// Command after regresses "ReorderActivity" by swapping the order of the two
// activity calls — a change that's perfectly deterministic in isolation but
// incompatible with the history recorded in ../corpus, since the recorded
// stream has ActivityA scheduled before ActivityB.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func reorderActivity(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	if err := workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(reorderActivity, workflow.RegisterOptions{Name: "ReorderActivity"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
