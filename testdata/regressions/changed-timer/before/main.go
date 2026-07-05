// Command before is the unmodified "ChangedTimer" workflow whose execution
// was recorded into ../corpus (see tools/gen-regressions). Replaying this
// package against that corpus must be clean.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func changedTimer(ctx workflow.Context) error {
	if err := workflow.NewTimer(ctx, 2*time.Second).Get(ctx, nil); err != nil {
		return err
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	return workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(changedTimer, workflow.RegisterOptions{Name: "ChangedTimer"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
