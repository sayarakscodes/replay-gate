// Command before is the unmodified "RenamedActivity" workflow whose execution
// was recorded into ../corpus (see tools/gen-regressions). Replaying this
// package against that corpus must be clean.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func renamedActivity(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	return workflow.ExecuteActivity(ctx, "SendNotification").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(renamedActivity, workflow.RegisterOptions{Name: "RenamedActivity"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
