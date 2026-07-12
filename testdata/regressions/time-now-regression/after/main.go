// Command after regresses "TimeNowRegression" by branching directly on
// time.Now() instead of the deterministic workflow.Now(ctx) — a classic
// injected non-deterministic construct. The cutoff
// is hardcoded in the past on purpose: it's already elapsed by the time this
// code can possibly run, so the branch flips to ProcessOrderV2 for every
// replay, deterministically, without depending on real wall-clock timing at
// test time. In production this is exactly how such bugs bite: a date-gated
// code path that quietly flips for every workflow already in flight.
package main

import (
	"os"
	"time"

	"go.temporal.io/sdk/workflow"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

func timeNowRegression(ctx workflow.Context) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second})
	cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if time.Now().Before(cutoff) {
		return workflow.ExecuteActivity(ctx, "ProcessOrder").Get(ctx, nil)
	}
	return workflow.ExecuteActivity(ctx, "ProcessOrderV2").Get(ctx, nil)
}

func main() {
	g := gate.New(gate.Config{})
	g.RegisterWorkflowWithOptions(timeNowRegression, workflow.RegisterOptions{Name: "TimeNowRegression"})
	os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
