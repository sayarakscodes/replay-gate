// Command after regresses "ChangedTimer" by removing the timer entirely and
// calling the activity immediately — control flow that now skips a timer on
// an existing code path.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func changedTimer(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(changedTimer, workflow.RegisterOptions{Name: "ChangedTimer"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
