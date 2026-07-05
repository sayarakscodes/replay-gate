package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// replaygate's replay command calls os.Exit directly (it propagates a
// subprocess's exact exit code), so it can't be exercised in-process without
// killing the test binary. TestMain builds the real CLI once and the tests
// below run it as a subprocess, the same way CI would.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "replaygate-cli-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "replaygate")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building replaygate CLI for tests: " + err.Error())
	}

	os.Exit(m.Run())
}

func TestReplay_AllMatching_ExitClean(t *testing.T) {
	cmd := exec.Command(binPath, "replay",
		"--corpus", "../../testdata/corpus",
		"--registrations", "../../testdata/replaymain_good",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected clean exit, got %v\noutput:\n%s", err, out)
	}
	if cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("expected exit code 0, got %d\noutput:\n%s", cmd.ProcessState.ExitCode(), out)
	}
}

func TestReplay_Regression_ExitDivergence(t *testing.T) {
	cmd := exec.Command(binPath, "replay",
		"--corpus", "../../testdata/corpus",
		"--registrations", "../../testdata/replaymain_bad",
	)
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState.ExitCode() != 1 {
		t.Fatalf("expected exit code 1 for a regressed workflow, got %d\noutput:\n%s", cmd.ProcessState.ExitCode(), out)
	}
	if !strings.Contains(string(out), "FAIL  SimpleOrder") {
		t.Errorf("expected report to call out the SimpleOrder divergence, got:\n%s", out)
	}
}

func TestReplay_MissingRegistrations_Errors(t *testing.T) {
	cmd := exec.Command(binPath, "replay", "--corpus", "../../testdata/corpus")
	out, _ := cmd.CombinedOutput()
	if cmd.ProcessState.ExitCode() == 0 {
		t.Fatalf("expected a non-zero exit when --registrations is missing, got 0\noutput:\n%s", out)
	}
}
