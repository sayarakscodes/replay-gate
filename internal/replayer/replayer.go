// Package replayer wraps the Temporal Go SDK's worker.WorkflowReplayer to replay a
// single corpus history against a set of registered workflow functions (TRD §5.1).
package replayer

import (
	"fmt"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// Registration is a workflow function to register on a replayer, optionally
// under an explicit name (mirrors worker.RegisterWorkflow/RegisterWorkflowWithOptions).
type Registration struct {
	Fn   interface{}
	Name string
}

// Result is the outcome of replaying one history.
type Result struct {
	Err error
}

func (r Result) Passed() bool { return r.Err == nil }

// newReplayer builds a fresh WorkflowReplayer with the given registrations. A new
// instance is required per replay: the SDK's WorkflowReplayer is not safe for
// concurrent ReplayWorkflowHistory calls, so parallel replay (a later milestone)
// must call this once per goroutine rather than share one instance.
func newReplayer(registrations []Registration) worker.WorkflowReplayer {
	r := worker.NewWorkflowReplayer()
	for _, reg := range registrations {
		if reg.Name == "" {
			r.RegisterWorkflow(reg.Fn)
		} else {
			r.RegisterWorkflowWithOptions(reg.Fn, workflow.RegisterOptions{Name: reg.Name})
		}
	}
	return r
}

// ReplayOne replays a single history against a freshly built replayer. A panic
// inside user workflow code is recovered and reported as a failed result rather
// than crashing the caller — one bad history must not take down a replay run.
func ReplayOne(registrations []Registration, logger log.Logger, history *historypb.History) (result Result) {
	defer func() {
		if p := recover(); p != nil {
			result = Result{Err: fmt.Errorf("panic during replay: %v", p)}
		}
	}()

	r := newReplayer(registrations)
	err := r.ReplayWorkflowHistory(logger, history)
	return Result{Err: err}
}
