// Package patcher maps a differ.Divergence to a suggested fix (F6, TRD §5.5).
// It never touches the user's source — v1 emits a rendered snippet or
// guidance text only (PRD non-goal: no code mutation).
package patcher

import (
	"fmt"
	"strings"

	"github.com/sayarakscodes/replay-gate/internal/differ"
)

// Patch is either a compilable GetVersion snippet (ChangeID + Snippet set) or
// remediation guidance (Guidance set) — never both, for classes GetVersion
// doesn't actually fix (panic, nondeterministic-construct, unknown).
type Patch struct {
	ChangeID string
	Snippet  string
	Guidance string
}

// Suggest maps a divergence to a patch. The bool reports whether a concrete
// GetVersion snippet was produced (true) versus guidance-only (false) — it
// does not mean "no suggestion at all": every class returns something.
//
// This is the pure, single-divergence form. Suggest's changeID is
// deterministic from d alone, so two divergences with the same class and
// name always render the same changeID — which is exactly right within one
// divergence, but needs deduplicating across a whole report of many
// divergences; use Patcher (state.go) for that.
func Suggest(d differ.Divergence) (Patch, bool) {
	switch d.Class {
	case differ.ClassRemoved:
		return suggestRemoved(d), true
	case differ.ClassAdded:
		return suggestAdded(d), true
	case differ.ClassRename:
		return suggestRename(d), true
	case differ.ClassReorder:
		return suggestReorder(d), true
	case differ.ClassPanic:
		return Patch{Guidance: panicGuidance(d)}, false
	case differ.ClassNondeterministicConstruct:
		return Patch{Guidance: constructGuidance(d)}, false
	default: // differ.ClassUnknown, and any future class we don't recognize yet
		return Patch{Guidance: unknownGuidance(d)}, false
	}
}

func isTimerEvent(eventType string) bool {
	return strings.Contains(strings.ToUpper(eventType), "TIMER")
}

func suggestRemoved(d differ.Divergence) Patch {
	name, eventType := "", ""
	if d.Expected != nil {
		name, eventType = d.Expected.Name, d.Expected.EventType
	}
	changeID := "removed-" + identifier(name, d.EventID)

	var body string
	if isTimerEvent(eventType) {
		body = "\tworkflow.NewTimer(ctx, /* original duration */).Get(ctx, nil)"
	} else {
		body = fmt.Sprintf("\tworkflow.ExecuteActivity(ctx, %s /* original input */).Get(ctx, nil)", quotedOr(name, "/* original activity type */"))
	}

	snippet := fmt.Sprintf(`v := workflow.GetVersion(ctx, %q, workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	// old path: keep the removed call so in-flight histories still match
%s
}`, changeID, body)
	return Patch{ChangeID: changeID, Snippet: snippet}
}

func suggestAdded(d differ.Divergence) Patch {
	name := ""
	if d.Generated != nil {
		name = d.Generated.Name
	}
	changeID := "added-" + identifier(name, d.EventID)

	snippet := fmt.Sprintf(`v := workflow.GetVersion(ctx, %q, workflow.DefaultVersion, 1)
if v != workflow.DefaultVersion {
	// new path: only run this for workflows started after this change
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
}`, changeID, quotedOr(name, "/* new activity type */"))
	return Patch{ChangeID: changeID, Snippet: snippet}
}

func suggestRename(d differ.Divergence) Patch {
	oldName, newName := "", ""
	if d.Expected != nil {
		oldName = d.Expected.Name
	}
	if d.Generated != nil {
		newName = d.Generated.Name
	}
	changeID := fmt.Sprintf("rename-%s-to-%s", identifier(oldName, 0), identifier(newName, 0))

	snippet := fmt.Sprintf(`v := workflow.GetVersion(ctx, %q, workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
} else {
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
}`, changeID, quotedOr(oldName, "/* old activity type */"), quotedOr(newName, "/* new activity type */"))
	return Patch{ChangeID: changeID, Snippet: snippet}
}

func suggestReorder(d differ.Divergence) Patch {
	expectedName, generatedName := "", ""
	if d.Expected != nil {
		expectedName = d.Expected.Name
	}
	if d.Generated != nil {
		generatedName = d.Generated.Name
	}
	changeID := fmt.Sprintf("reorder-%s-%s", identifier(expectedName, 0), identifier(generatedName, 0))
	expectedRef := quotedOr(expectedName, "/* originally-first activity */")
	generatedRef := quotedOr(generatedName, "/* originally-second activity */")

	snippet := fmt.Sprintf(`v := workflow.GetVersion(ctx, %q, workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
	// old order: in-flight histories recorded this sequence
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
} else {
	// new order
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
	workflow.ExecuteActivity(ctx, %s /* input */).Get(ctx, nil)
}`, changeID, expectedRef, generatedRef, generatedRef, expectedRef)
	return Patch{ChangeID: changeID, Snippet: snippet}
}

func panicGuidance(d differ.Divergence) string {
	return fmt.Sprintf("Replay panicked (%s) — this isn't a command-shape mismatch GetVersion can fix. "+
		"Investigate the panic directly; GetVersion only guards branch selection, not crashes.", d.RawError)
}

func constructGuidance(d differ.Divergence) string {
	detail := d.Note
	if detail == "" {
		detail = "a non-deterministic construct"
	}
	return fmt.Sprintf("Workflow code likely calls a non-deterministic construct directly (%s). "+
		"Replace it with the deterministic equivalent — workflow.Now(ctx) instead of time.Now(), "+
		"workflow.SideEffect for rand/uuid generation, avoid depending on map iteration order, "+
		"workflow.Go instead of a bare goroutine — then version-gate the change with GetVersion "+
		"if in-flight workflows must keep the old behavior.", detail)
}

func unknownGuidance(d differ.Divergence) string {
	return fmt.Sprintf("Could not classify this divergence from the replay error. "+
		"Raw error: %s. Manual investigation required.", d.RawError)
}
