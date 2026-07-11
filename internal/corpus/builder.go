package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	historypb "go.temporal.io/api/history/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// Builder incrementally assembles a corpus directory: each AddHistory call
// writes one history file and records its entry; Finish computes the
// content-hashed corpusVersion and writes manifest.json. Callers that produce
// histories (the sampler, fixture generators) use this instead of
// hand-rolling the manifest format themselves.
type Builder struct {
	dir         string
	cluster     ClusterInfo
	redaction   RedactionInfo
	sdkAtSample string
	entries     []Entry
}

func NewBuilder(dir string, cluster ClusterInfo, redaction RedactionInfo) *Builder {
	return &Builder{dir: dir, cluster: cluster, redaction: redaction}
}

// SetSDKVersion records the SDK version active at sampling time (manifest's
// sdkVersionAtSampling field).
func (b *Builder) SetSDKVersion(v string) { b.sdkAtSample = v }

// AddHistory writes hist to <dir>/histories/<workflowType>/<workflowID>_<runID>.json
// and appends its manifest entry.
func (b *Builder) AddHistory(workflowType, workflowID, runID, status string, hist *historypb.History) error {
	relDir := filepath.Join("histories", workflowType)
	if err := os.MkdirAll(filepath.Join(b.dir, relDir), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", relDir, err)
	}

	data, err := protojson.MarshalOptions{Indent: "  "}.Marshal(hist)
	if err != nil {
		return fmt.Errorf("marshaling history for %s/%s/%s: %w", workflowType, workflowID, runID, err)
	}

	relFile := filepath.Join(relDir, fmt.Sprintf("%s_%s.json", workflowID, runID))
	if err := os.WriteFile(filepath.Join(b.dir, relFile), data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", relFile, err)
	}

	b.entries = append(b.entries, Entry{
		File:         filepath.ToSlash(relFile),
		WorkflowType: workflowType,
		WorkflowID:   workflowID,
		RunID:        runID,
		Status:       status,
		EventCount:   len(hist.Events),
		SHA256:       sha256Hex(data),
	})
	return nil
}

// Finish writes manifest.json with a corpusVersion computed over every entry
// added so far. Call it once, after all AddHistory calls.
func (b *Builder) Finish() error {
	manifest := Manifest{
		CorpusVersion:        ComputeCorpusVersion(b.entries),
		FormatVersion:        FormatVersion,
		SampledAt:            time.Now().UTC(),
		Cluster:              b.cluster,
		SDKVersionAtSampling: b.sdkAtSample,
		Redaction:            b.redaction,
		Entries:              b.entries,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.dir, "manifest.json"), data, 0o644)
}

// Entries returns the entries added so far (read-only use, e.g. for logging
// which histories were skipped vs written).
func (b *Builder) Entries() []Entry {
	return append([]Entry(nil), b.entries...)
}
