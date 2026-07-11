package sampler_test

import (
	"context"
	"os"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/sampler"
)

const devServerAddr = "127.0.0.1:7233"

// dialOrSkip connects to a local dev server, skipping the test (not failing
// it) when one isn't running — this test needs a live cluster by nature, so
// it degrades gracefully in environments without one (see testdata/regressions
// and README.md for how to start one: `temporal server start-dev --headless`).
func dialOrSkip(t *testing.T) client.Client {
	t.Helper()
	c, err := client.Dial(client.Options{HostPort: devServerAddr, Namespace: "default"})
	if err != nil {
		t.Skipf("no local Temporal dev server reachable at %s (start one with `temporal server start-dev --headless`): %v", devServerAddr, err)
	}
	// Dial succeeds even if nothing is listening yet in some SDK versions;
	// force a round trip to confirm the server actually answers.
	if _, err := c.WorkflowService().GetSystemInfo(context.Background(), nil); err != nil {
		c.Close()
		t.Skipf("no local Temporal dev server reachable at %s: %v", devServerAddr, err)
	}
	return c
}

func roundTripWorkflow(ctx workflow.Context) (string, error) {
	return "round-trip-ok", nil
}

func TestSampler_RoundTrip(t *testing.T) {
	c := dialOrSkip(t)
	defer c.Close()

	const taskQueue = "sampler-roundtrip-tq"
	w := worker.New(c, taskQueue, worker.Options{})
	w.RegisterWorkflowWithOptions(roundTripWorkflow, workflow.RegisterOptions{Name: "SamplerRoundTrip"})
	if err := w.Start(); err != nil {
		t.Fatalf("starting worker: %v", err)
	}
	defer w.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "sampler-roundtrip-" + time.Now().Format("150405.000000000"),
		TaskQueue: taskQueue,
	}, "SamplerRoundTrip")
	if err != nil {
		t.Fatalf("starting workflow: %v", err)
	}
	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("workflow did not complete: %v", err)
	}

	cfg := sampler.Config{
		Cap:             10,
		OpenClosedSplit: 0.7,
		ClosedWindow:    time.Hour,
		MaxEvents:       10000,
		RateLimit:       sampler.RateLimit{VisibilityRPS: 50, HistoryRPS: 50},
		TypeScanLimit:   1000,
	}
	s := sampler.New(c, "default", cfg, nil)

	dir := t.TempDir()
	result, err := s.Run(ctx, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Written == 0 {
		t.Fatal("expected at least one history to be sampled")
	}

	if err := corpus.Verify(dir); err != nil {
		t.Fatalf("sampled corpus failed verification: %v", err)
	}

	loaded, err := corpus.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ref := corpus.EntryRef{WorkflowType: "SamplerRoundTrip", WorkflowID: run.GetID(), RunID: run.GetRunID()}
	hist, ok := loaded.Histories[ref]
	if !ok {
		t.Fatalf("sampled corpus does not contain the workflow this test just ran (%s); got %d entries", ref, len(loaded.Manifest.Entries))
	}

	// Byte-identical round trip: re-marshal what Load decoded and compare
	// against the exact bytes written to disk.
	var entryFile string
	for _, e := range loaded.Manifest.Entries {
		if e.WorkflowType == ref.WorkflowType && e.WorkflowID == ref.WorkflowID && e.RunID == ref.RunID {
			entryFile = e.File
		}
	}
	onDisk, err := os.ReadFile(dir + "/" + entryFile)
	if err != nil {
		t.Fatalf("reading history file: %v", err)
	}
	reMarshaled, err := protojson.MarshalOptions{Indent: "  "}.Marshal(hist)
	if err != nil {
		t.Fatalf("re-marshaling loaded history: %v", err)
	}
	if string(reMarshaled) != string(onDisk) {
		t.Fatalf("round trip is not byte-identical:\non disk:\n%s\nre-marshaled:\n%s", onDisk, reMarshaled)
	}
}
