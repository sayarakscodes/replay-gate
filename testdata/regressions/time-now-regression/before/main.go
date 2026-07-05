// Command before is the unmodified "TimeNowRegression" workflow whose
// execution was recorded into ../corpus (see tools/gen-regressions).
// Replaying this package against that corpus must be clean.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func timeNowRegression(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	return workflow.ExecuteActivity(ctx, "ProcessOrder").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(timeNowRegression, workflow.RegisterOptions{Name: "TimeNowRegression"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
