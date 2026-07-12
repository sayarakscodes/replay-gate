package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
)

func sampleReport() *Report {
	return &Report{
		CorpusDir:     "testdata/corpus",
		CorpusVersion: "sha256:deadbeef",
		Results: []EntryResult{
			{Ref: corpus.EntryRef{WorkflowType: "SimpleOrder", WorkflowID: "order-1", RunID: "run-a1"}, Status: "COMPLETED", Duration: 5 * time.Millisecond},
			{Ref: corpus.EntryRef{WorkflowType: "InvoiceFlow", WorkflowID: "invoice-9", RunID: "run-b2"}, Status: "COMPLETED", Err: errors.New("[TMPRL1100] lookup failed")},
			{Ref: corpus.EntryRef{WorkflowType: "ShipmentWorkflow", WorkflowID: "shipment-4", RunID: "run-c3"}, Status: "RUNNING", Skipped: true},
		},
	}
}

func TestWriteText(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sampleReport(), FormatText, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"PASS", "FAIL", "SKIP", "1 passed", "1 failed", "1 skipped", "3 total"} {
		if !strings.Contains(out, want) {
			t.Errorf("text report missing %q, got:\n%s", want, out)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sampleReport(), FormatJSON, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var decoded jsonReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding json report: %v", err)
	}
	if decoded.ReportVersion != ReportVersion {
		t.Errorf("expected reportVersion %d, got %d", ReportVersion, decoded.ReportVersion)
	}
	if decoded.Summary.Total != 3 || decoded.Summary.Passed != 1 || decoded.Summary.Failed != 1 || decoded.Summary.Skipped != 1 {
		t.Errorf("unexpected summary: %+v", decoded.Summary)
	}
	if len(decoded.Results) != 3 || decoded.Results[1].Error == "" {
		t.Errorf("expected the failed entry's error to be populated, got: %+v", decoded.Results)
	}
}

func TestWrite_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sampleReport(), "yaml", FailOnOpen); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
}

func TestReport_ExitCode_Clean(t *testing.T) {
	clean := &Report{Results: []EntryResult{{Status: "COMPLETED"}}}
	if got := clean.ExitCode(FailOnOpen); got != ExitClean {
		t.Errorf("expected ExitClean, got %d", got)
	}
}

func TestReport_ExitCode_ClosedOnlyDivergence(t *testing.T) {
	// sampleReport's one divergence (InvoiceFlow) is in a COMPLETED (closed)
	// workflow — under the default fail-on=open, that's a warning, not a
	// blocking failure.
	dirty := sampleReport()
	if got := dirty.ExitCode(FailOnOpen); got != ExitDivergenceWarn {
		t.Errorf("expected ExitDivergenceWarn for a closed-only divergence under fail-on=open, got %d", got)
	}
	if got := dirty.ExitCode(FailOnAny); got != ExitDivergence {
		t.Errorf("expected ExitDivergence under fail-on=any regardless of open/closed, got %d", got)
	}
}

func TestReport_ExitCode_OpenDivergenceAlwaysBlocks(t *testing.T) {
	openDivergence := &Report{Results: []EntryResult{
		{Status: "RUNNING", Err: errors.New("[TMPRL1100] lookup failed")},
	}}
	if got := openDivergence.ExitCode(FailOnOpen); got != ExitDivergence {
		t.Errorf("expected ExitDivergence for a divergence in a RUNNING workflow, got %d", got)
	}
	if got := openDivergence.ExitCode(FailOnAny); got != ExitDivergence {
		t.Errorf("expected ExitDivergence under fail-on=any too, got %d", got)
	}
}

func TestReport_ExitCode_SkippedEntryIgnored(t *testing.T) {
	skippedOnly := &Report{Results: []EntryResult{{Status: "RUNNING", Skipped: true}}}
	if got := skippedOnly.ExitCode(FailOnOpen); got != ExitClean {
		t.Errorf("a skipped-only report must not count as a divergence, got %d", got)
	}
}

func TestReport_OpenDivergences(t *testing.T) {
	rep := &Report{Results: []EntryResult{
		{Ref: corpus.EntryRef{WorkflowType: "A"}, Status: "RUNNING", Err: errors.New("x")},
		{Ref: corpus.EntryRef{WorkflowType: "B"}, Status: "COMPLETED", Err: errors.New("y")},
		{Ref: corpus.EntryRef{WorkflowType: "C"}, Status: "RUNNING"}, // passed, not a divergence
	}}
	open := rep.OpenDivergences()
	if len(open) != 1 || open[0].Ref.WorkflowType != "A" {
		t.Errorf("expected exactly the RUNNING divergence (A), got %+v", open)
	}
}
