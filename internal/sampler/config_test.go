package sampler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_EmptyPathReturnsDefaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := defaultConfig()
	if cfg != want {
		t.Fatalf("expected defaults %+v, got %+v", want, cfg)
	}
}

func TestLoadConfig_FileOverridesSomeFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replaygate.yaml")
	yaml := `
sample:
  cap: 50
  closedWindow: 24h
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Cap != 50 {
		t.Errorf("expected cap=50, got %d", cfg.Cap)
	}
	if cfg.ClosedWindow != 24*time.Hour {
		t.Errorf("expected closedWindow=24h, got %s", cfg.ClosedWindow)
	}
	// Fields not set in the file should keep their defaults.
	want := defaultConfig()
	if cfg.MaxEvents != want.MaxEvents {
		t.Errorf("expected maxEvents to keep default %d, got %d", want.MaxEvents, cfg.MaxEvents)
	}
	if cfg.RateLimit != want.RateLimit {
		t.Errorf("expected rateLimit to keep defaults %+v, got %+v", want.RateLimit, cfg.RateLimit)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/does/not/exist.yaml")
	if err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}
