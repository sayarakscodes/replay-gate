package differ

import (
	"regexp"
	"strconv"

	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
)

// Every regex here is anchored to a "[TMPRL1100]" message template found in
// go.temporal.io/sdk's internal/internal_task_handlers.go and
// internal/internal_command_state_machine.go. They are inherently coupled to
// the SDK's exact wording (TRD §8 risk) — see this package's canary test,
// which fails loudly if a newer SDK changes any of this text.
//
// Verified version floor (TRD §14, corrected from an initial 1.25 assumption
// — see issue #7): the "[TMPRL1100]" marker itself was only added in SDK
// v1.26.0 (absent at v1.25.0, same wording otherwise); reExpectedAtPosition's
// template was only added in v1.34.0 (older versions hit a different,
// unhandled message for that case — the divergence is still *detected*,
// just classified as ClassUnknown instead of ClassRemoved on SDK < 1.34).
// All five templates below are confirmed byte-identical from v1.34.0 through
// the pinned v1.45.0.
var (
	// "nondeterministic workflow: history event is ActivityTaskScheduled: (...), replay command is ScheduleActivityTask: (...)"
	// The one template that carries both expected and generated in a single message. (>= v1.26.0)
	reMismatch = regexp.MustCompile(`^\[TMPRL1100\] nondeterministic workflow: history event is (\w+): \((.*)\), replay command is (\w+): \((.*)\)$`)

	// "nondeterministic workflow: missing replay command for ActivityTaskScheduled: (...)" (>= v1.26.0)
	reMissingCommand = regexp.MustCompile(`^\[TMPRL1100\] nondeterministic workflow: missing replay command for (\w+): \((.*)\)$`)

	// "nondeterministic workflow: extra replay command for ScheduleActivityTask: (...)" (>= v1.26.0)
	reExtraCommand = regexp.MustCompile(`^\[TMPRL1100\] nondeterministic workflow: extra replay command for (\w+): \((.*)\)$`)

	// "During replay, a matching Timer command was expected in history event position 5. However, ..." (>= v1.34.0)
	reExpectedAtPosition = regexp.MustCompile(`^\[TMPRL1100\] During replay, a matching (\w+) command was expected in history event position (\d+)\.`)

	// "lookup failed for scheduledEventID to activityID: scheduleEventID: 5, activityID: 5" (>= v1.26.0)
	reLookupFailed = regexp.MustCompile(`^\[TMPRL1100\] lookup failed for scheduledEventID to activityID: scheduleEventID: (\d+), activityID: (\d+)$`)

	// Sub-extractors for the attribute blobs the templates above capture —
	// Go's default %v struct formatting, e.g. "ActivityId:5, ActivityType:(Name:Foo), ...".
	reActivityID       = regexp.MustCompile(`ActivityId:([^,)]+)`)
	reActivityTypeName = regexp.MustCompile(`ActivityType:\(Name:([^)]+)\)`)
)

func classifyKnownTemplate(msg string, hist *historypb.History, workflowFn any) (Divergence, bool) {
	switch {
	case reMismatch.MatchString(msg):
		return classifyMismatch(msg, hist, workflowFn), true
	case reMissingCommand.MatchString(msg):
		return classifyMissingCommand(msg, hist), true
	case reExtraCommand.MatchString(msg):
		return classifyExtraCommand(msg), true
	case reExpectedAtPosition.MatchString(msg):
		return classifyExpectedAtPosition(msg), true
	case reLookupFailed.MatchString(msg):
		return classifyLookupFailed(msg, hist), true
	default:
		return Divergence{}, false
	}
}

func classifyMismatch(msg string, hist *historypb.History, workflowFn any) Divergence {
	m := reMismatch.FindStringSubmatch(msg)
	expectedType, expectedRaw := m[1], m[2]
	generatedType, generatedRaw := m[3], m[4]

	expected := &EventSummary{
		EventType:  expectedType,
		Name:       firstSubmatch(reActivityTypeName, expectedRaw),
		ActivityID: firstSubmatch(reActivityID, expectedRaw),
		Raw:        expectedRaw,
	}
	generated := &CommandSummary{
		CommandType: generatedType,
		Name:        firstSubmatch(reActivityTypeName, generatedRaw),
		ActivityID:  firstSubmatch(reActivityID, generatedRaw),
		Raw:         generatedRaw,
	}

	if eventID, ok := findActivityScheduledEventID(hist, expected.Name); ok {
		expected.EventID = eventID
	}

	// Reorder: the command the new code just generated matches some OTHER
	// recorded event elsewhere in history (just not at this position) — the
	// activity still happens, out of sequence, rather than being genuinely
	// renamed or newly conditional.
	if generated.Name != "" {
		if _, ok := findActivityScheduledEventID(hist, generated.Name); ok {
			return Divergence{
				Class:     ClassReorder,
				EventID:   expected.EventID,
				Expected:  expected,
				Generated: generated,
				RawError:  msg,
			}
		}
	}

	// Ambiguous: same-shape mismatch, but the generated activity type never
	// appears anywhere in the recorded history — could be a straight rename,
	// or a nondeterministic-construct-driven branch flip. Disambiguate with
	// the source-scan heuristic (construct.go); default to rename.
	class := ClassRename
	note := ""
	if hit, ok := scanForNondeterministicConstruct(workflowFn); ok {
		class = ClassNondeterministicConstruct
		note = hit
	}

	return Divergence{
		Class:     class,
		EventID:   expected.EventID,
		Expected:  expected,
		Generated: generated,
		RawError:  msg,
		Note:      note,
	}
}

func classifyMissingCommand(msg string, hist *historypb.History) Divergence {
	m := reMissingCommand.FindStringSubmatch(msg)
	eventType, raw := m[1], m[2]
	expected := &EventSummary{
		EventType:  eventType,
		Name:       firstSubmatch(reActivityTypeName, raw),
		ActivityID: firstSubmatch(reActivityID, raw),
		Raw:        raw,
	}
	if eventID, ok := findActivityScheduledEventID(hist, expected.Name); ok {
		expected.EventID = eventID
	}
	return Divergence{Class: ClassRemoved, EventID: expected.EventID, Expected: expected, RawError: msg}
}

func classifyExtraCommand(msg string) Divergence {
	m := reExtraCommand.FindStringSubmatch(msg)
	commandType, raw := m[1], m[2]
	generated := &CommandSummary{
		CommandType: commandType,
		Name:        firstSubmatch(reActivityTypeName, raw),
		ActivityID:  firstSubmatch(reActivityID, raw),
		Raw:         raw,
	}
	return Divergence{Class: ClassAdded, Generated: generated, RawError: msg}
}

func classifyExpectedAtPosition(msg string) Divergence {
	m := reExpectedAtPosition.FindStringSubmatch(msg)
	commandType, posStr := m[1], m[2]
	eventID, _ := strconv.ParseInt(posStr, 10, 64)
	return Divergence{
		Class:    ClassRemoved,
		EventID:  eventID,
		Expected: &EventSummary{EventID: eventID, EventType: commandType},
		RawError: msg,
	}
}

func classifyLookupFailed(msg string, hist *historypb.History) Divergence {
	m := reLookupFailed.FindStringSubmatch(msg)
	scheduledEventID, _ := strconv.ParseInt(m[1], 10, 64)

	expected := &EventSummary{EventID: scheduledEventID}
	if e := findEventByID(hist, scheduledEventID); e != nil {
		expected.EventType = e.GetEventType().String()
		if attrs := e.GetActivityTaskScheduledEventAttributes(); attrs != nil {
			expected.Name = attrs.GetActivityType().GetName()
			expected.ActivityID = attrs.GetActivityId()
		}
	}
	return Divergence{Class: ClassRemoved, EventID: scheduledEventID, Expected: expected, RawError: msg}
}

// findActivityScheduledEventID scans hist for an ActivityTaskScheduled event
// whose activity type name matches, returning its event ID.
func findActivityScheduledEventID(hist *historypb.History, activityTypeName string) (int64, bool) {
	if hist == nil || activityTypeName == "" {
		return 0, false
	}
	for _, e := range hist.Events {
		if e.GetEventType() != enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED {
			continue
		}
		if attrs := e.GetActivityTaskScheduledEventAttributes(); attrs != nil {
			if attrs.GetActivityType().GetName() == activityTypeName {
				return e.GetEventId(), true
			}
		}
	}
	return 0, false
}

func findEventByID(hist *historypb.History, id int64) *historypb.HistoryEvent {
	if hist == nil {
		return nil
	}
	for _, e := range hist.Events {
		if e.GetEventId() == id {
			return e
		}
	}
	return nil
}
