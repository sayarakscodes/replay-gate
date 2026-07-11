package redact_test

import (
	"encoding/base64"
	"strings"
	"testing"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sayarakscodes/replay-gate/internal/redact"
)

const secret = "SECRET-PAYLOAD-CONTENT-DO-NOT-LEAK"

// Payload.Data is a proto `bytes` field, so protojson always base64-encodes
// it — the plaintext secret never appears literally in marshaled output
// regardless of redaction. Property tests must look for this form instead.
func base64Contains(haystack, plaintext string) bool {
	return strings.Contains(haystack, base64.StdEncoding.EncodeToString([]byte(plaintext)))
}

func payloads(s string) *commonpb.Payloads {
	return &commonpb.Payloads{Payloads: []*commonpb.Payload{
		{Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: []byte(s)},
	}}
}

// secretHistory exercises every shape a payload can appear in: a Payloads
// field (Started/Scheduled/Completed), and a bare map[string]*Payload
// (Header.Fields) — the two structural cases RedactHistory's generic walker
// has to handle correctly.
func secretHistory() *historypb.History {
	return &historypb.History{Events: []*historypb.HistoryEvent{
		{EventId: 1, EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &commonpb.WorkflowType{Name: "Secret"},
					Input:        payloads(secret),
					Header:       &commonpb.Header{Fields: map[string]*commonpb.Payload{"trace-id": {Data: []byte(secret)}}},
				},
			}},
		{EventId: 2, EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
			Attributes: &historypb.HistoryEvent_ActivityTaskScheduledEventAttributes{
				ActivityTaskScheduledEventAttributes: &historypb.ActivityTaskScheduledEventAttributes{
					ActivityId: "1", ActivityType: &commonpb.ActivityType{Name: "DoThing"},
					Input: payloads(secret),
				},
			}},
		{EventId: 3, EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED,
			Attributes: &historypb.HistoryEvent_ActivityTaskCompletedEventAttributes{
				ActivityTaskCompletedEventAttributes: &historypb.ActivityTaskCompletedEventAttributes{
					Result: payloads(secret),
				},
			}},
		{EventId: 4, EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
				WorkflowExecutionSignaledEventAttributes: &historypb.WorkflowExecutionSignaledEventAttributes{
					SignalName: "Sig",
					Input:      payloads(secret),
				},
			}},
		{EventId: 5, EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionCompletedEventAttributes{
				WorkflowExecutionCompletedEventAttributes: &historypb.WorkflowExecutionCompletedEventAttributes{
					Result: payloads(secret),
				},
			}},
	}}
}

func marshal(t *testing.T, hist *historypb.History) string {
	t.Helper()
	data, err := protojson.Marshal(hist)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

func TestRedactHistory_NoneLeavesSecretIntact(t *testing.T) {
	hist := secretHistory()
	redact.RedactHistory(hist, redact.NoneScrubber{})
	out := marshal(t, hist)
	if !base64Contains(out, secret) {
		t.Fatal("sanity check failed: NoneScrubber should leave the secret intact, but it's gone")
	}
}

func TestRedactHistory_DefaultScrubsEverySecretOccurrence(t *testing.T) {
	hist := secretHistory()
	redact.RedactHistory(hist, redact.DefaultScrubber{})
	out := marshal(t, hist)
	if base64Contains(out, secret) {
		t.Fatalf("default profile leaked the secret:\n%s", out)
	}
}

func TestRedactHistory_HashScrubsEverySecretOccurrence(t *testing.T) {
	hist := secretHistory()
	redact.RedactHistory(hist, redact.HashScrubber{Key: []byte("test-key")})
	out := marshal(t, hist)
	if base64Contains(out, secret) {
		t.Fatalf("hash profile leaked the secret:\n%s", out)
	}
}

func TestRedactHistory_HashPreservesEquality(t *testing.T) {
	a := payloads(secret)
	b := payloads(secret)
	c := payloads(secret + "-different")

	scrubber := redact.HashScrubber{Key: []byte("test-key")}
	sa := scrubber.Scrub(a.Payloads[0])
	sb := scrubber.Scrub(b.Payloads[0])
	sc := scrubber.Scrub(c.Payloads[0])

	if string(sa.Data) != string(sb.Data) {
		t.Error("hash profile should produce identical output for identical input")
	}
	if string(sa.Data) == string(sc.Data) {
		t.Error("hash profile should produce different output for different input")
	}
}

func TestRedactHistory_DefaultPreservesSize(t *testing.T) {
	p := &commonpb.Payload{Data: []byte("twelve bytes")}
	scrubbed := redact.DefaultScrubber{}.Scrub(p)
	if len(scrubbed.Data) != len(p.Data) {
		t.Errorf("expected scrubbed data length %d to match original %d", len(scrubbed.Data), len(p.Data))
	}
	for _, b := range scrubbed.Data {
		if b != 0 {
			t.Fatal("expected scrubbed data to be all zero bytes")
		}
	}
}
