package differ

import "strings"

// panicPrefix must match internal/replayer.ReplayOne's recover() wrapper
// exactly — that's our own message, not the SDK's, so unlike the regex
// templates in classify.go this one can't drift out from under us.
const panicPrefix = "panic during replay: "

// classifyPanic recognizes internal/replayer's own recovered-panic wrapper:
// a Go panic in user workflow code, for a reason other than a "[TMPRL1100]"
// command mismatch (those are also technically panics internally, but the
// SDK turns them into ordinary errors before ReplayWorkflowHistory returns —
// only a genuinely uncaught panic in user code reaches our own recover()).
func classifyPanic(msg string) (Divergence, bool) {
	if !strings.HasPrefix(msg, panicPrefix) {
		return Divergence{}, false
	}
	return Divergence{Class: ClassPanic, RawError: msg}, true
}
