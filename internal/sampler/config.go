// Package sampler implements F1 (TRD §5.3): stratified sampling of workflow
// histories from a live Temporal cluster into the corpus format internal/corpus
// defines. All cluster calls are read-only, rate-limited, and retried.
package sampler

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sayarakscodes/replay-gate/internal/redact"
)

// Config mirrors the `sample:` section of replaygate.yaml (TRD §6). Zero
// values are replaced with the defaults below by Load / applyDefaults.
type Config struct {
	Cap             int           `yaml:"cap"`
	OpenClosedSplit float64       `yaml:"openClosedSplit"`
	ClosedWindow    time.Duration `yaml:"closedWindow"`
	MaxEvents       int           `yaml:"maxEvents"`
	RateLimit       RateLimit     `yaml:"rateLimit"`
	// Redaction names the payload-scrubbing profile: "none", "default", or
	// "hash" (TRD §5.3, §6). The "hash" profile also needs a key, which is
	// deliberately not part of this file-loadable config — see dial.go's
	// sibling, REPLAYGATE_REDACTION_KEY, so a secret never has to live in
	// a checked-in replaygate.yaml.
	Redaction string `yaml:"redaction"`
	// TypeScanLimit bounds how many executions the workflow-type discovery
	// scan will page through before giving up (TRD doesn't specify this
	// explicitly; without a bound, discovery on a large namespace never
	// terminates on its own).
	TypeScanLimit int `yaml:"typeScanLimit"`
}

type RateLimit struct {
	VisibilityRPS float64 `yaml:"visibilityRPS"`
	HistoryRPS    float64 `yaml:"historyRPS"`
}

type fileConfig struct {
	Sample Config `yaml:"sample"`
}

func defaultConfig() Config {
	return Config{
		Cap:             200,
		OpenClosedSplit: 0.7,
		ClosedWindow:    7 * 24 * time.Hour,
		MaxEvents:       10000,
		RateLimit:       RateLimit{VisibilityRPS: 5, HistoryRPS: 10},
		Redaction:       redact.ProfileDefault,
		TypeScanLimit:   1000,
	}
}

// LoadConfig reads the `sample:` section of a replaygate.yaml file, filling
// in defaults for anything unset. An empty path returns defaults unchanged.
func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var fc fileConfig
	fc.Sample = cfg // start from defaults so unset fields survive the unmarshal
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return Config{}, err
	}
	return fc.Sample, nil
}
