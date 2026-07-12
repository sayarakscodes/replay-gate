// Command replaygate is the registrations package for the examples worker:
// register the workflows under test, then hand off to gate.Main. The GitHub
// Action runs `replaygate replay --registrations ./examples/replaygate`
// against ./examples/corpus. Because OrderWorkflow here is unchanged, that
// replay is clean.
package main

import (
	"os"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/examples"
	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(examples.OrderWorkflow, workflow.RegisterOptions{Name: "OrderWorkflow"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
