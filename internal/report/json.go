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
	WorkflowType string          `json:"workflowType"`
	WorkflowID   string          `json:"workflowID"`
	RunID        string          `json:"runID"`
	Status       string          `json:"status"`
	Passed       bool            `json:"passed"`
	Skipped      bool            `json:"skipped,omitempty"`
	Error        string          `json:"error,omitempty"`
	DurationMS   int64           `json:"durationMs"`
	Divergence   *jsonDivergence `json:"divergence,omitempty"`
	Patch        *jsonPatch      `json:"patch,omitempty"`
}

// jsonDivergence mirrors differ.Divergence's exported shape (report doesn't
// import differ's types directly into the wire schema, so the JSON contract
// stays stable even if internal/differ's Go types evolve).
type jsonDivergence struct {
	Class     string              `json:"class"`
	EventID   int64               `json:"eventId,omitempty"`
	Expected  *jsonEventSummary   `json:"expected,omitempty"`
	Generated *jsonCommandSummary `json:"generated,omitempty"`
	Note      string              `json:"note,omitempty"`
}

type jsonEventSummary struct {
	EventID    int64  `json:"eventId,omitempty"`
	EventType  string `json:"eventType,omitempty"`
	Name       string `json:"name,omitempty"`
	ActivityID string `json:"activityId,omitempty"`
}

type jsonCommandSummary struct {
	CommandType string `json:"commandType,omitempty"`
	Name        string `json:"name,omitempty"`
	ActivityID  string `json:"activityId,omitempty"`
}

type jsonPatch struct {
	ChangeID string `json:"changeId,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	Guidance string `json:"guidance,omitempty"`
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
		if d := r.Divergence; d != nil {
			jd := &jsonDivergence{Class: string(d.Class), EventID: d.EventID, Note: d.Note}
			if d.Expected != nil {
				jd.Expected = &jsonEventSummary{
					EventID: d.Expected.EventID, EventType: d.Expected.EventType,
					Name: d.Expected.Name, ActivityID: d.Expected.ActivityID,
				}
			}
			if d.Generated != nil {
				jd.Generated = &jsonCommandSummary{
					CommandType: d.Generated.CommandType, Name: d.Generated.Name, ActivityID: d.Generated.ActivityID,
				}
			}
			jr.Divergence = jd
		}
		if p := r.Patch; p != nil && (p.Snippet != "" || p.Guidance != "") {
			jr.Patch = &jsonPatch{ChangeID: p.ChangeID, Snippet: p.Snippet, Guidance: p.Guidance}
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
