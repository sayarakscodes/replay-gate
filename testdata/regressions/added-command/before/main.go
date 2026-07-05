// Command before is the unmodified "AddedCommand" workflow whose execution
// was recorded into ../corpus (see tools/gen-regressions). Replaying this
// package against that corpus must be clean.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func addedCommand(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(addedCommand, workflow.RegisterOptions{Name: "AddedCommand"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
