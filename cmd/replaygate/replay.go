package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/sayarakscodes/replay-gate/pkg/gate"
	"github.com/spf13/cobra"
)

// newReplayCmd wires the replay command: --registrations names a Go main
// package that has already registered its workflows and calls gate.Main. This
// command builds that package into a temporary binary and execs it, forwarding
// its own flags and propagating the subprocess's stdout/stderr/exit code
// unchanged. See pkg/gate/main.go for why this is shaped this way.
//
// We build-then-exec rather than `go run` deliberately: `go run` collapses any
// non-zero program exit to 1, which would erase the distinction between exit 1
// (blocking divergence), 2 (warn-only, closed histories), and 3 (operational
// error) that the exit-code contract depends on. A built binary
// preserves the exact code.
func newReplayCmd() *cobra.Command {
	var corpusDir, registrations, format, onUnregistered, failOn string
	var parallelism int

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay every history in a corpus against a build and report divergences",
		RunE: func(cmd *cobra.Command, args []string) error {
			if registrations == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "Error: --registrations is required: path to a Go main package that registers workflows and calls gate.Main")
				os.Exit(gate.ExitOperationalError)
			}

			// `go build`/`go run` treat a bare relative path as an import-path
			// lookup (and specially exclude "testdata" directories from those),
			// so resolve to an absolute path to make this a directory reference.
			absRegistrations, err := filepath.Abs(registrations)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: resolving --registrations path: %v\n", err)
				os.Exit(gate.ExitOperationalError)
			}

			tmpDir, err := os.MkdirTemp("", "replaygate-modeb")
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: creating temp build dir: %v\n", err)
				os.Exit(gate.ExitOperationalError)
			}
			defer os.RemoveAll(tmpDir)
			binPath := filepath.Join(tmpDir, "replaygate-modeb")

			build := exec.Command("go", "build", "-o", binPath, absRegistrations)
			build.Stdout = cmd.OutOrStdout()
			build.Stderr = cmd.ErrOrStderr()
			if err := build.Run(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: building registrations package: %v\n", err)
				os.Exit(gate.ExitOperationalError)
			}

			sub := exec.Command(binPath,
				"--corpus", corpusDir,
				"--format", format,
				"--on-unregistered", onUnregistered,
				"--fail-on", failOn,
				"--parallelism", strconv.Itoa(parallelism),
			)
			sub.Stdout = cmd.OutOrStdout()
			sub.Stderr = cmd.ErrOrStderr()

			runErr := sub.Run()
			if runErr == nil {
				os.Exit(gate.ExitClean)
			}
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintln(cmd.ErrOrStderr(), runErr)
			os.Exit(gate.ExitOperationalError)
			return nil // unreachable
		},
	}

	cmd.Flags().StringVar(&corpusDir, "corpus", "", "path to the corpus directory")
	cmd.Flags().StringVar(&registrations, "registrations", "", "path to a Go main package that registers workflows and calls gate.Main")
	cmd.Flags().StringVar(&format, "format", "text", "report format: text|json|github|sarif")
	cmd.Flags().StringVar(&onUnregistered, "on-unregistered", "fail", "behavior for unregistered workflow types: fail|skip-warn")
	cmd.Flags().StringVar(&failOn, "fail-on", "open", "which divergences block: open (default, RUNNING workflows only) | any")
	cmd.Flags().IntVar(&parallelism, "parallelism", 0, "concurrent replay workers (0 = GOMAXPROCS)")
	cmd.MarkFlagRequired("corpus")
	cmd.MarkFlagRequired("registrations")
	return cmd
}
