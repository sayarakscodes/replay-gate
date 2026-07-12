package gate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/pkg/gate"
)

// syntheticSimpleOrderHistory builds a history with the same shape as the
// SimpleOrder fixture (one ChargeCard activity), so matchingSimpleOrder
// (defined in gate_test.go) replays every entry clean.
func syntheticSimpleOrderHistory(seq int) *historypb.History {
	ts := func(offset int) *timestamppb.Timestamp {
		return timestamppb.New(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(offset) * time.Second))
	}
	payloads := func(s string) *commonpb.Payloads {
		return &commonpb.Payloads{Payloads: []*commonpb.Payload{{Data: []byte(s)}}}
	}
	tq := &taskqueuepb.TaskQueue{Name: "bench-tq"}

	return &historypb.History{Events: []*historypb.HistoryEvent{
		{EventId: 1, EventTime: ts(0), EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &commonpb.WorkflowType{Name: "SimpleOrder"}, TaskQueue: tq,
					Input: payloads(fmt.Sprintf(`{"orderId":"o-%d"}`, seq)),
				},
			}},
		{EventId: 2, EventTime: ts(1), EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED,
			Attributes: &historypb.HistoryEvent_WorkflowTaskScheduledEventAttributes{
				WorkflowTaskScheduledEventAttributes: &historypb.WorkflowTaskScheduledEventAttributes{TaskQueue: tq},
			}},
		{EventId: 3, EventTime: ts(2), EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_STARTED,
			Attributes: &historypb.HistoryEvent_WorkflowTaskStartedEventAttributes{
				WorkflowTaskStartedEventAttributes: &historypb.WorkflowTaskStartedEventAttributes{ScheduledEventId: 2},
			}},
		{EventId: 4, EventTime: ts(3), EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_COMPLETED,
			Attributes: &historypb.HistoryEvent_WorkflowTaskCompletedEventAttributes{
				WorkflowTaskCompletedEventAttributes: &historypb.WorkflowTaskCompletedEventAttributes{ScheduledEventId: 2, StartedEventId: 3},
			}},
		{EventId: 5, EventTime: ts(4), EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
			Attributes: &historypb.HistoryEvent_ActivityTaskScheduledEventAttributes{
				ActivityTaskScheduledEventAttributes: &historypb.ActivityTaskScheduledEventAttributes{
					ActivityId: "5", ActivityType: &commonpb.ActivityType{Name: "ChargeCard"}, TaskQueue: tq,
				},
			}},
		{EventId: 6, EventTime: ts(5), EventType: enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED,
			Attributes: &historypb.HistoryEvent_ActivityTaskCompletedEventAttributes{
				ActivityTaskCompletedEventAttributes: &historypb.ActivityTaskCompletedEventAttributes{ScheduledEventId: 5},
			}},
		{EventId: 7, EventTime: ts(6), EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED,
			Attributes: &historypb.HistoryEvent_WorkflowTaskScheduledEventAttributes{
				WorkflowTaskScheduledEventAttributes: &historypb.WorkflowTaskScheduledEventAttributes{TaskQueue: tq},
			}},
		{EventId: 8, EventTime: ts(7), EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_STARTED,
			Attributes: &historypb.HistoryEvent_WorkflowTaskStartedEventAttributes{
				WorkflowTaskStartedEventAttributes: &historypb.WorkflowTaskStartedEventAttributes{ScheduledEventId: 7},
			}},
		{EventId: 9, EventTime: ts(8), EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_COMPLETED,
			Attributes: &historypb.HistoryEvent_WorkflowTaskCompletedEventAttributes{
				WorkflowTaskCompletedEventAttributes: &historypb.WorkflowTaskCompletedEventAttributes{ScheduledEventId: 7, StartedEventId: 8},
			}},
		{EventId: 10, EventTime: ts(9), EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED,
			Attributes: &historypb.HistoryEvent_WorkflowExecutionCompletedEventAttributes{
				WorkflowExecutionCompletedEventAttributes: &historypb.WorkflowExecutionCompletedEventAttributes{WorkflowTaskCompletedEventId: 9},
			}},
	}}
}

// buildSyntheticCorpus writes n SimpleOrder-shaped histories to a fresh
// corpus directory, in the same manifest+protojson format internal/corpus
// expects, and returns its path.
func buildSyntheticCorpus(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	histDir := filepath.Join(dir, "histories", "SimpleOrder")
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		tb.Fatal(err)
	}

	var entries []corpus.Entry

	for i := range n {
		hist := syntheticSimpleOrderHistory(i)
		data, err := protojson.Marshal(hist)
		if err != nil {
			tb.Fatal(err)
		}
		relFile := filepath.Join("histories", "SimpleOrder", fmt.Sprintf("order-%d_run-%d.json", i, i))
		if err := os.WriteFile(filepath.Join(dir, relFile), data, 0o644); err != nil {
			tb.Fatal(err)
		}
		sum := sha256.Sum256(data)
		entries = append(entries, corpus.Entry{
			File: filepath.ToSlash(relFile), WorkflowType: "SimpleOrder",
			WorkflowID: "order-" + strconv.Itoa(i), RunID: "run-" + strconv.Itoa(i),
			Status: corpus.StatusCompleted, EventCount: len(hist.Events), SHA256: hex.EncodeToString(sum[:]),
		})
	}

	manifest := corpus.Manifest{
		CorpusVersion: corpus.ComputeCorpusVersion(entries),
		FormatVersion: corpus.FormatVersion,
		SampledAt:     time.Now().UTC(),
		Cluster:       corpus.ClusterInfo{Namespace: "bench"},
		Redaction:     corpus.RedactionInfo{Profile: "none"},
		Entries:       entries,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		tb.Fatal(err)
	}
	return dir
}

// BenchmarkReplayCorpus establishes the N1 throughput baseline (TRD §5.1,
// target >= 500 histories/sec on a standard CI runner) against a synthetic
// 1000-history corpus.
func BenchmarkReplayCorpus(b *testing.B) {
	const size = 1000
	dir := buildSyntheticCorpus(b, size)

	g := gate.New(gate.Config{CorpusDir: dir})
	g.RegisterWorkflowWithOptions(matchingSimpleOrder, workflow.RegisterOptions{Name: "SimpleOrder"})

	b.ResetTimer()
	for range b.N {
		rep, err := g.ReplayAll(gate.ReplayAllOptions{})
		if err != nil {
			b.Fatalf("ReplayAll: %v", err)
		}
		if rep.ExitCode(gate.FailOnOpen) != gate.ExitClean {
			b.Fatalf("expected a clean replay, got %d divergences", len(rep.Divergences()))
		}
	}
	b.StopTimer()

	histPerSec := float64(size) * float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(histPerSec, "histories/sec")
}
