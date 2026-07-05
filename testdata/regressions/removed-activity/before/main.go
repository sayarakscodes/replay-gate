// Command before is the unmodified "RemovedActivity" workflow whose execution
// was recorded into ../corpus (see tools/gen-regressions). Replaying this
// package against that corpus must be clean.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func removedActivity(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	if err := workflow.ExecuteActivity(ctx, "ActivityA").Get(ctx, nil); err != nil {
		return err
	}
	return workflow.ExecuteActivity(ctx, "ActivityB").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(removedActivity, workflow.RegisterOptions{Name: "RemovedActivity"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
