package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/differ"
)

var diffRemoved = differ.Divergence{Class: differ.ClassRemoved}

func refFor(wfType string) corpus.EntryRef {
	return corpus.EntryRef{WorkflowType: wfType, WorkflowID: "id", RunID: "run"}
}

func TestWriteSARIF_WellFormed(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, githubSampleReport(), FormatSARIF, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("SARIF output is not valid JSON: %v\n%s", err, buf.String())
	}

	if doc["version"] != sarifVersion {
		t.Errorf("expected version %q, got %v", sarifVersion, doc["version"])
	}
	if _, ok := doc["$schema"]; !ok {
		t.Error("SARIF doc missing $schema")
	}

	runs, ok := doc["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("expected exactly 1 run, got %v", doc["runs"])
	}
	run := runs[0].(map[string]any)

	results, ok := run["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("expected 2 results (one per divergence), got %v", run["results"])
	}

	// First divergence (RUNNING) is error-level; second (COMPLETED) is warning.
	levels := map[string]int{}
	for _, r := range results {
		levels[r.(map[string]any)["level"].(string)]++
	}
	if levels["error"] != 1 || levels["warning"] != 1 {
		t.Errorf("expected 1 error + 1 warning level, got %v", levels)
	}

	// Rules are deduplicated by class: removed + rename = 2 distinct rules.
	driver := run["tool"].(map[string]any)["driver"].(map[string]any)
	rules := driver["rules"].([]any)
	if len(rules) != 2 {
		t.Errorf("expected 2 distinct rules (removed, rename), got %d", len(rules))
	}
}

func TestWriteSARIF_EmptyReportHasNonNullArrays(t *testing.T) {
	// GitHub code scanning rejects a run whose rules/results are JSON null.
	clean := &Report{CorpusDir: "c", Results: []EntryResult{{Status: "COMPLETED"}}}
	var buf bytes.Buffer
	if err := Write(&buf, clean, FormatSARIF, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s := buf.String()
	if strings.Contains(s, "null") {
		t.Errorf("SARIF for a clean report must not contain null arrays, got:\n%s", s)
	}
}

func TestWriteSARIF_DedupesRulesAcrossSameClass(t *testing.T) {
	rep := &Report{
		CorpusDir: "c",
		Results: []EntryResult{
			{Ref: refFor("A"), Status: "RUNNING", Err: errString("x"), Divergence: &diffRemoved},
			{Ref: refFor("B"), Status: "RUNNING", Err: errString("y"), Divergence: &diffRemoved},
		},
	}
	var buf bytes.Buffer
	if err := Write(&buf, rep, FormatSARIF, FailOnOpen); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var doc sarifLog
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if got := len(doc.Runs[0].Tool.Driver.Rules); got != 1 {
		t.Errorf("expected the two same-class divergences to share 1 rule, got %d", got)
	}
	if got := len(doc.Runs[0].Results); got != 2 {
		t.Errorf("expected 2 results, got %d", got)
	}
}
