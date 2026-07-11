package differ_test

import (
	"errors"
	"testing"

	"github.com/sayarakscodes/replay-gate/internal/differ"
)

func TestClassify_UnknownFallback(t *testing.T) {
	err := errors.New("[TMPRL1100] some future SDK message format nobody has a regex for yet")
	d := differ.Classify(err, nil, nil)
	if d.Class != differ.ClassUnknown {
		t.Errorf("expected ClassUnknown for an unrecognized message, got %q", d.Class)
	}
	if d.RawError != err.Error() {
		t.Error("RawError must be preserved verbatim even when classification fails")
	}
}

func TestClassify_UnrelatedErrorIsUnknown(t *testing.T) {
	err := errors.New("connection refused")
	d := differ.Classify(err, nil, nil)
	if d.Class != differ.ClassUnknown {
		t.Errorf("expected ClassUnknown for an error with no TMPRL1100 marker, got %q", d.Class)
	}
}

func TestClassify_PanicWrapper(t *testing.T) {
	err := errors.New("panic during replay: runtime error: index out of range [3] with length 2")
	d := differ.Classify(err, nil, nil)
	if d.Class != differ.ClassPanic {
		t.Errorf("expected ClassPanic, got %q", d.Class)
	}
	if d.RawError != err.Error() {
		t.Error("RawError must be preserved verbatim")
	}
}

func TestClassify_NeverPanicsOnNilHistoryOrFn(t *testing.T) {
	// Regression guard: Classify must degrade gracefully (not panic) when
	// hist/workflowFn are unavailable — real callers may not always have both.
	msgs := []string{
		"[TMPRL1100] nondeterministic workflow: history event is ActivityTaskScheduled: (ActivityId:5, ActivityType:(Name:Foo)), replay command is ScheduleActivityTask: (ActivityId:5, ActivityType:(Name:Bar))",
		"[TMPRL1100] nondeterministic workflow: missing replay command for ActivityTaskScheduled: (ActivityId:5, ActivityType:(Name:Foo))",
		"[TMPRL1100] nondeterministic workflow: extra replay command for ScheduleActivityTask: (ActivityId:5, ActivityType:(Name:Foo))",
		"[TMPRL1100] During replay, a matching Timer command was expected in history event position 5. However, the replayed code did not produce that.",
		"[TMPRL1100] lookup failed for scheduledEventID to activityID: scheduleEventID: 5, activityID: 5",
	}
	for _, msg := range msgs {
		d := differ.Classify(errors.New(msg), nil, nil)
		if d.RawError == "" {
			t.Errorf("RawError should always be set for message %q", msg)
		}
	}
}
