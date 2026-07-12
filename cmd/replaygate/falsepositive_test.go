package main

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// jsonReportSummary mirrors the "summary" object in internal/report's JSON
// output; duplicated here (rather than importing internal/report) since this
// package only needs to read the field, not own the schema.
type jsonReportSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type jsonReport struct {
	Summary jsonReportSummary `json:"summary"`
}

func replayJSON(t *testing.T, corpusDir, registrations string) jsonReport {
	t.Helper()
	cmd := exec.Command(binPath, "replay",
		"--corpus", corpusDir,
		"--registrations", registrations,
		"--format", "json",
	)
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("running replaygate replay: %v", err)
		}
		// A non-zero exit is fine (and expected for divergent corpora) — the
		// report is still emitted on stdout.
	}
	var rep jsonReport
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatalf("decoding json report: %v\noutput:\n%s", err, out)
	}
	return rep
}

// TestFalsePositiveGuard is the other core trust guarantee: zero false
// positives across a corpus of unchanged-code replays. Every unmodified
// workflow, replayed against its own recorded history, must report zero
// failures. A failure here means the replayer itself is unreliable, which is
// worse than a missed regression.
func TestFalsePositiveGuard(t *testing.T) {
	t.Run("fixture_corpus", func(t *testing.T) {
		rep := replayJSON(t, "../../testdata/corpus", "../../testdata/replaymain_good")
		if rep.Summary.Failed != 0 {
			t.Errorf("expected 0 failures replaying the fixture corpus against unchanged code, got %d (of %d)", rep.Summary.Failed, rep.Summary.Total)
		}
	})

	for _, class := range regressionClasses {
		t.Run(class, func(t *testing.T) {
			rep := replayJSON(t, "../../testdata/regressions/"+class+"/corpus", "../../testdata/regressions/"+class+"/before")
			if rep.Summary.Failed != 0 {
				t.Errorf("expected 0 failures replaying %s's unmodified workflow against its own history, got %d", class, rep.Summary.Failed)
			}
		})
	}
}
