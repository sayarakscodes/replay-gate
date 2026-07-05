// Command after regresses "AddedCommand" by appending a second activity call
// on an existing code path — the recorded history only has one activity
// scheduled, so the extra command has nothing to match against.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func addedCommand(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	if err := workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(addedCommand, workflow.RegisterOptions{Name: "AddedCommand"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
