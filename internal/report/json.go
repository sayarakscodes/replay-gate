package report

import (
	"encoding/json"
	"io"
)

type jsonReport struct {
	ReportVersion int          `json:"reportVersion"`
	CorpusDir     string       `json:"corpusDir"`
	CorpusVersion string       `json:"corpusVersion"`
	Summary       jsonSummary  `json:"summary"`
	Results       []jsonResult `json:"results"`
}

type jsonSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type jsonResult struct {
	WorkflowType string `json:"workflowType"`
	WorkflowID   string `json:"workflowID"`
	RunID        string `json:"runID"`
	Status       string `json:"status"`
	Passed       bool   `json:"passed"`
	Skipped      bool   `json:"skipped,omitempty"`
	Error        string `json:"error,omitempty"`
	DurationMS   int64  `json:"durationMs"`
}

func writeJSON(w io.Writer, rep *Report) error {
	out := jsonReport{
		ReportVersion: ReportVersion,
		CorpusDir:     rep.CorpusDir,
		CorpusVersion: rep.CorpusVersion,
	}

	for _, r := range rep.Results {
		jr := jsonResult{
			WorkflowType: r.Ref.WorkflowType,
			WorkflowID:   r.Ref.WorkflowID,
			RunID:        r.Ref.RunID,
			Status:       r.Status,
			Passed:       r.Err == nil && !r.Skipped,
			Skipped:      r.Skipped,
			DurationMS:   r.Duration.Milliseconds(),
		}
		if r.Err != nil {
			jr.Error = r.Err.Error()
		}
		out.Results = append(out.Results, jr)

		out.Summary.Total++
		switch {
		case r.Skipped:
			out.Summary.Skipped++
		case r.Err != nil:
			out.Summary.Failed++
		default:
			out.Summary.Passed++
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
