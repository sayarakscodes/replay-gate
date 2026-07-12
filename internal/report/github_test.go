package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/differ"
	"github.com/sayarakscodes/replay-gate/internal/patcher"
)

// githubSampleReport has one open (RUNNING) and one closed (COMPLETED)
// divergence, so annotation levels (error vs warning) can both be exercised.
func githubSampleReport() *Report {
	return &Report{
		CorpusDir:     "testdata/corpus",
		CorpusVersion: "sha256:deadbeef",
		Results: []EntryResult{
			{
				Ref:    corpus.EntryRef{WorkflowType: "OrderFlow", WorkflowID: "o-1", RunID: "r-1"},
				Status: "RUNNING",
				Err:    errString("[TMPRL1100] lookup failed"),
				Divergence: &differ.Divergence{Class: differ.ClassRemoved,
					Expected: &differ.EventSummary{Name: "ChargeCard"}},
				Patch: &patcher.Patch{ChangeID: "removed-charge-card", Snippet: "v := workflow.GetVersion(...)"},
			},
			{
				Ref:        corpus.EntryRef{WorkflowType: "InvoiceFlow", WorkflowID: "i-9", RunID: "r-2"},
				Status:     "COMPLETED",
				Err:        errString("[TMPRL1100] nondeterministic"),
				Divergence: &differ.Divergence{Class: differ.ClassRename},
				Patch:      &patcher.Patch{ChangeID: "rename-a-to-b", Snippet: "v := workflow.GetVersion(...)"},
			},
			{
				Ref:    corpus.EntryRef{WorkflowType: "ShipFlow", WorkflowID: "s-4", RunID: "r-3"},
				Status: "COMPLETED",
			},
		},
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestWriteGitHub_Annotations(t *testing.T) {
	// Isolate from any real Actions env so we exercise only the annotation path.
	t.Setenv("GITHUB_STEP_SUMMARY", "")

	var buf bytes.Buffer
	if err := Write(&buf, githubSampleReport(), FormatGitHub, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()

	// Open workflow -> ::error, closed workflow -> ::warning.
	if !strings.Contains(out, "::error title=") {
		t.Errorf("expected an ::error annotation for the RUNNING divergence, got:\n%s", out)
	}
	if !strings.Contains(out, "::warning title=") {
		t.Errorf("expected a ::warning annotation for the COMPLETED divergence, got:\n%s", out)
	}
	// One annotation line per divergence, not per result (ShipFlow passed).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected exactly 2 annotation lines, got %d:\n%s", len(lines), out)
	}
	// Annotations must be single-line (newlines in the message get escaped).
	for _, line := range lines {
		if !strings.HasPrefix(line, "::") {
			t.Errorf("annotation output contains a non-command line (multi-line leak?): %q", line)
		}
	}
}

func TestWriteGitHub_FailOnAnyMakesAllErrors(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "")

	var buf bytes.Buffer
	if err := Write(&buf, githubSampleReport(), FormatGitHub, FailOnAny); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	// Under fail-on=any, even the COMPLETED divergence blocks, so both
	// annotations are ::error and there are no warnings.
	if strings.Contains(out, "::warning") {
		t.Errorf("under fail-on=any there should be no warning annotations, got:\n%s", out)
	}
	if got := strings.Count(out, "::error title="); got != 2 {
		t.Errorf("expected 2 ::error annotations under fail-on=any, got %d:\n%s", got, out)
	}
}

func TestWriteGitHub_JobSummary(t *testing.T) {
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	var buf bytes.Buffer
	if err := Write(&buf, githubSampleReport(), FormatGitHub, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("reading job summary: %v", err)
	}
	summary := string(data)

	for _, want := range []string{
		"## Replay Gate",
		"1 passed, 2 failed",
		"### Divergences",
		"removed-charge-card",
		"```go", // patch snippet fenced code block
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("job summary missing %q, got:\n%s", want, summary)
		}
	}
}

func TestWriteGitHub_NoDivergences(t *testing.T) {
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	clean := &Report{CorpusDir: "c", Results: []EntryResult{{Status: "COMPLETED"}}}
	var buf bytes.Buffer
	if err := Write(&buf, clean, FormatGitHub, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no annotations for a clean report, got:\n%s", buf.String())
	}
	data, _ := os.ReadFile(summaryPath)
	if !strings.Contains(string(data), "No divergences") {
		t.Errorf("expected the clean-report summary to say so, got:\n%s", data)
	}
}
