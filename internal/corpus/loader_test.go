package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const fixtureDir = "../../testdata/corpus"

func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.Walk(fixtureDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fixtureDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying fixture: %v", err)
	}
	return dst
}

func TestLoad_Valid(t *testing.T) {
	c, err := Load(fixtureDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Manifest.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(c.Manifest.Entries))
	}
	if len(c.Histories) != 3 {
		t.Fatalf("expected 3 decoded histories, got %d", len(c.Histories))
	}
	ref := EntryRef{WorkflowType: "SimpleOrder", WorkflowID: "order-1", RunID: "run-a1"}
	hist, ok := c.Histories[ref]
	if !ok {
		t.Fatalf("missing history for %s", ref)
	}
	if len(hist.Events) != 10 {
		t.Fatalf("expected 10 events for %s, got %d", ref, len(hist.Events))
	}
}

func TestVerify_Valid(t *testing.T) {
	if err := Verify(fixtureDir); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestLoad_HashMismatch(t *testing.T) {
	dir := copyFixture(t)
	target := filepath.Join(dir, "histories", "SimpleOrder", "order-1_run-a1.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	// Tamper the file's bytes without updating the manifest's recorded hash.
	if err := os.WriteFile(target, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(dir)
	if err == nil {
		t.Fatal("expected error for tampered history file, got nil")
	}
}

func TestLoad_MalformedManifest(t *testing.T) {
	dir := copyFixture(t)
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for malformed manifest, got nil")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := copyFixture(t)
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	// Point one entry at a file that doesn't exist, keeping the manifest otherwise
	// self-consistent (corpusVersion recomputed) so the missing-file check, not the
	// version check, is what's actually exercised.
	m.Entries[0].File = "histories/SimpleOrder/does-not-exist.json"
	m.CorpusVersion = ComputeCorpusVersion(m.Entries)
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(dir)
	if err == nil {
		t.Fatal("expected error for missing history file, got nil")
	}
}

func TestLoad_CorpusVersionTamper(t *testing.T) {
	dir := copyFixture(t)
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	m.CorpusVersion = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(dir)
	if err == nil {
		t.Fatal("expected error for corpusVersion/entries mismatch, got nil")
	}
}
