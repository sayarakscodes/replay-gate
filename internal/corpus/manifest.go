// Package corpus implements the on-disk corpus format described in TRD_Replay_Gate.md §5.2:
// a directory of protojson-encoded workflow histories plus a manifest.json that
// content-hashes them, so a build can be tied to the exact corpus it was validated against.
package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

const FormatVersion = 1

// Workflow execution status as recorded at sampling time. Drives the
// open/closed severity split (fail-on=open vs fail-on=any) in the replay report.
const (
	StatusRunning    = "RUNNING"
	StatusCompleted  = "COMPLETED"
	StatusFailed     = "FAILED"
	StatusTerminated = "TERMINATED"
	StatusTimedOut   = "TIMED_OUT"
	StatusCanceled   = "CANCELED"
)

type ClusterInfo struct {
	Namespace string `json:"namespace"`
	Endpoint  string `json:"endpoint,omitempty"`
}

type RedactionInfo struct {
	Profile        string   `json:"profile"`
	FieldsScrubbed []string `json:"fieldsScrubbed,omitempty"`
}

// Entry describes one history file within the corpus and the hash the loader
// verifies it against before it's trusted for replay.
type Entry struct {
	File         string `json:"file"`
	WorkflowType string `json:"workflowType"`
	WorkflowID   string `json:"workflowID"`
	RunID        string `json:"runID"`
	Status       string `json:"status"`
	EventCount   int    `json:"eventCount"`
	SHA256       string `json:"sha256"`
}

func (e Entry) Label() string {
	return fmt.Sprintf("%s/%s/%s (%s)", e.WorkflowType, e.WorkflowID, e.RunID, e.File)
}

type Manifest struct {
	CorpusVersion        string        `json:"corpusVersion"`
	FormatVersion        int           `json:"formatVersion"`
	SampledAt            time.Time     `json:"sampledAt"`
	Cluster              ClusterInfo   `json:"cluster"`
	SDKVersionAtSampling string        `json:"sdkVersionAtSampling,omitempty"`
	Redaction            RedactionInfo `json:"redaction"`
	Entries              []Entry       `json:"entries"`
}

// ComputeCorpusVersion derives a content hash over all entries' file hashes, so any
// mutation to a history file or to the manifest's own entry list changes the version.
// Entries are sorted by file path first so the result doesn't depend on manifest order.
func ComputeCorpusVersion(entries []Entry) string {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].File < sorted[j].File })

	h := sha256.New()
	for _, e := range sorted {
		fmt.Fprintf(h, "%s:%s\n", e.File, e.SHA256)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
