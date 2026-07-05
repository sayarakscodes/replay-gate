// Command after regresses "RenamedActivity" by calling the same logic under
// a renamed activity type — renaming an activity type is deterministic in
// isolation but breaks in-flight workflows whose recorded history references
// the old name (see PRD_Replay_Gate.md §1).
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func renamedActivity(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	return workflow.ExecuteActivity(ctx, "SendNotificationV2").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(renamedActivity, workflow.RegisterOptions{Name: "RenamedActivity"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
