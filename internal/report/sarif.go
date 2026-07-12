package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// SARIF 2.1.0 (https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html),
// the format GitHub code scanning ingests. We emit only the subset needed to
// surface one result per divergence: a single run, one rule per divergence
// Class, and a result per failing history.
const (
	sarifVersion = "2.1.0"
	sarifSchema  = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	toolName     = "replaygate"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name  string      `json:"name"`
	Rules []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	ShortDescription sarifText `json:"shortDescription"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifText       `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

func writeSARIF(w io.Writer, rep *Report, failOn string) error {
	divs := rep.Divergences()

	rulesSeen := map[string]bool{}
	var rules []sarifRule
	var results []sarifResult

	for _, r := range divs {
		class := divergenceClass(r)
		ruleID := "replaygate/" + class
		if !rulesSeen[ruleID] {
			rulesSeen[ruleID] = true
			rules = append(rules, sarifRule{
				ID:               ruleID,
				Name:             class,
				ShortDescription: sarifText{Text: fmt.Sprintf("Replay non-determinism divergence: %s", class)},
			})
		}

		level := "warning"
		if blocks(r.Status, failOn) {
			level = "error"
		}

		results = append(results, sarifResult{
			RuleID:  ruleID,
			Level:   level,
			Message: sarifText{Text: sarifMessage(r)},
			// Divergences aren't tied to a source line (the patcher doesn't
			// locate source — TRD §5.5), so we anchor the result at the corpus
			// history file, which is a real, inspectable artifact.
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: historyURI(rep, r)},
				},
			}},
		})
	}

	// GitHub rejects a SARIF run whose rules array is null; keep it an empty
	// slice (and results too) when there are no divergences.
	if rules == nil {
		rules = []sarifRule{}
	}
	if results == nil {
		results = []sarifResult{}
	}

	doc := sarifLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool:    sarifTool{Driver: sarifDriver{Name: toolName, Rules: rules}},
			Results: results,
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func sarifMessage(r EntryResult) string {
	msg := fmt.Sprintf("%s [%s]: %s divergence", r.Ref, r.Status, divergenceClass(r))
	if r.Patch != nil && r.Patch.ChangeID != "" {
		msg += fmt.Sprintf("; suggested GetVersion changeID %q", r.Patch.ChangeID)
	}
	if r.Err != nil {
		msg += ". " + oneLine(r.Err.Error())
	}
	return msg
}

func historyURI(rep *Report, r EntryResult) string {
	return fmt.Sprintf("%s/histories/%s/%s_%s.json", rep.CorpusDir, r.Ref.WorkflowType, r.Ref.WorkflowID, r.Ref.RunID)
}
