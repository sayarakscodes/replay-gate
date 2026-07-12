// Package differ turns the opaque error from ReplayWorkflowHistory into a
// structured Divergence. The raw SDK error is nearly unreadable
// ; this is the highest-signal engineering in the whole project.
//
// Extraction is regex-only, not typed (errors.As): the concrete error type
// the SDK returns for non-determinism panics is *internal.workflowPanicError,
// an unexported type deliberately not aliased publicly (unlike
// temporal.PanicError, which is a different type for a different purpose —
// user code panics, not replay divergences). errors.As can never match an
// unexported type from outside its package, so typed extraction is not an
// available tier here, confirmed empirically against go.temporal.io/sdk
// v1.45.0 (see testdata/regressions and this package's canary test).
//
// Generated is nullable: only one of the SDK's ~10 "[TMPRL1100]"-marked
// message templates carries both the expected (recorded) and generated
// (produced-by-the-new-code) command in one message. Others carry only one
// side, or neither — see classify.go's regex table.
package differ

import (
	historypb "go.temporal.io/api/history/v1"
)

// Class is deliberately a small, closed set of the named regression classes
// plus a fallback. Anything the regex table doesn't recognize is Unknown —
// never guessed — to keep the false-positive guarantee meaningful: a report
// line always corresponds to a real
// replay failure, but its Class is only ever asserted when we have real
// evidence for it.
type Class string

const (
	// ClassReorder: the code produced a command matching some other point in
	// the recorded history, out of sequence.
	ClassReorder Class = "reorder"
	// ClassRemoved: a recorded event (activity, timer, ...) has no matching
	// generated command at all.
	ClassRemoved Class = "removed"
	// ClassAdded: a generated command has no corresponding recorded event.
	ClassAdded Class = "added"
	// ClassRename: the code produced a differently-named command (activity
	// type, etc.) in the same slot a differently-named one was recorded.
	// This is also the label used when the true cause is ambiguous between a
	// straight rename and a nondeterministic-construct-driven branch flip —
	// see the source-scan heuristic in construct.go.
	ClassRename Class = "rename"
	// ClassNondeterministicConstruct: same error shape as ClassRename, but the
	// registered workflow function's own source contains a known
	// non-deterministic construct (time.Now(), rand, goroutines) — a real,
	// if best-effort, signal that this is the more likely explanation.
	ClassNondeterministicConstruct Class = "nondeterministic-construct"
	// ClassPanic: user workflow code panicked during replay (recovered by
	// internal/replayer), for a reason unrelated to a "[TMPRL1100]" command
	// mismatch — e.g. a nil pointer bug incidentally triggered by replay.
	ClassPanic Class = "panic"
	// ClassUnknown: a real replay failure whose message didn't match any
	// known template. Always carries RawError; never dropped.
	ClassUnknown Class = "unknown"
)

// EventSummary describes the recorded history event a divergence expected,
// to whatever extent it was recoverable from the SDK error and history.
type EventSummary struct {
	EventID    int64  // 0 if not recovered
	EventType  string // e.g. "ActivityTaskScheduled", "TimerStarted"
	Name       string // activity type name / signal name / etc, best-effort
	ActivityID string // best-effort, empty if not applicable
	Raw        string // the raw attribute text from the SDK message, verbatim
}

// CommandSummary describes the command the build under test generated
// instead, to whatever extent it was recoverable.
type CommandSummary struct {
	CommandType string
	Name        string
	ActivityID  string
	Raw         string
}

// Divergence is the structured result of classifying one replay failure.
// Expected and Generated are nullable — see the package doc — RawError is
// always populated.
type Divergence struct {
	Class     Class
	EventID   int64 // best-effort; 0 if not recovered from the error text
	Expected  *EventSummary
	Generated *CommandSummary
	RawError  string
	// Note carries extra, human-readable context the classifier discovered
	// but that doesn't fit Expected/Generated — e.g. which non-deterministic
	// construct the source-scan heuristic found, or why a template matched
	// but classification still couldn't be more specific.
	Note string
}

// Classify turns a replay error into a Divergence. hist is the full corpus
// history being replayed (used to cross-reference event IDs and detect
// reorders); workflowFn is the registered workflow function (used for the
// source-scan heuristic in construct.go). Both may be nil — Classify
// degrades gracefully, just with less specific classification.
//
// err must be non-nil; a nil replay error means there's no divergence to
// classify, which is the caller's job to check.
func Classify(err error, hist *historypb.History, workflowFn any) Divergence {
	msg := err.Error()

	if d, ok := classifyPanic(msg); ok {
		return d
	}
	if d, ok := classifyKnownTemplate(msg, hist, workflowFn); ok {
		return d
	}
	return Divergence{Class: ClassUnknown, RawError: msg}
}
