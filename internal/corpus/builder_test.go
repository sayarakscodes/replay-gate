package corpus

import (
	"testing"

	historypb "go.temporal.io/api/history/v1"
)

func TestBuilder_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	b := NewBuilder(dir, ClusterInfo{Namespace: "default"}, RedactionInfo{Profile: "none"})
	b.SetSDKVersion("test")

	hist := &historypb.History{Events: []*historypb.HistoryEvent{{EventId: 1}}}
	if err := b.AddHistory("Foo", "wf-1", "run-1", StatusCompleted, hist); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if err := b.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Manifest.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(c.Manifest.Entries))
	}
	ref := EntryRef{WorkflowType: "Foo", WorkflowID: "wf-1", RunID: "run-1"}
	if _, ok := c.Histories[ref]; !ok {
		t.Fatalf("expected history for %s", ref)
	}
}
