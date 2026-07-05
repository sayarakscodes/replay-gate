package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	historypb "go.temporal.io/api/history/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// EntryRef identifies a single workflow execution's history within a corpus.
type EntryRef struct {
	WorkflowType string
	WorkflowID   string
	RunID        string
}

func (r EntryRef) String() string {
	return fmt.Sprintf("%s/%s/%s", r.WorkflowType, r.WorkflowID, r.RunID)
}

// Corpus is a fully loaded, integrity-verified set of workflow histories.
type Corpus struct {
	Dir       string
	Manifest  Manifest
	Histories map[EntryRef]*historypb.History
}

// Load reads manifest.json from dir, verifies every entry's history file against
// its recorded sha256, verifies the manifest's own corpusVersion against the
// entries it declares, and decodes each history. It fails on the first problem
// encountered rather than returning a partially loaded corpus — a corpus that
// can't be fully verified must never be reported as usable.
func Load(dir string) (*Corpus, error) {
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", manifestPath, err)
	}

	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", manifestPath, err)
	}

	if want := ComputeCorpusVersion(m.Entries); want != m.CorpusVersion {
		return nil, fmt.Errorf("corpus version mismatch: manifest declares %q, computed %q from its entries (manifest may be tampered or stale)", m.CorpusVersion, want)
	}

	histories := make(map[EntryRef]*historypb.History, len(m.Entries))
	for _, e := range m.Entries {
		histPath := filepath.Join(dir, e.File)
		data, err := os.ReadFile(histPath)
		if err != nil {
			return nil, fmt.Errorf("entry %s: reading history file: %w", e.Label(), err)
		}

		if got := sha256Hex(data); got != e.SHA256 {
			return nil, fmt.Errorf("entry %s: sha256 mismatch: manifest says %s, file hash is %s", e.Label(), e.SHA256, got)
		}

		var hist historypb.History
		if err := protojson.Unmarshal(data, &hist); err != nil {
			return nil, fmt.Errorf("entry %s: decoding history: %w", e.Label(), err)
		}

		ref := EntryRef{WorkflowType: e.WorkflowType, WorkflowID: e.WorkflowID, RunID: e.RunID}
		if _, exists := histories[ref]; exists {
			return nil, fmt.Errorf("entry %s: duplicate entry for %s in manifest", e.Label(), ref)
		}
		histories[ref] = &hist
	}

	return &Corpus{Dir: dir, Manifest: m, Histories: histories}, nil
}

// Verify runs the same integrity checks as Load without returning the decoded
// corpus — used by the `replaygate verify` command, which only cares whether
// the corpus is trustworthy, not about replaying it.
func Verify(dir string) error {
	_, err := Load(dir)
	return err
}
