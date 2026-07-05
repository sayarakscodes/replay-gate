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

// newReplayCmd wires Mode B (TRD §4, §14 OQ1): --registrations names a Go main
// package that has already registered its workflows and calls gate.Main. This
// command is a thin `go run <registrations> <flags>` wrapper — it does no code
// generation and no import-path inference, so it just forwards its own flags
// to the subprocess and propagates that process's stdout/stderr/exit code
// unchanged. See pkg/gate/main.go for why Mode B is shaped this way.
func newReplayCmd() *cobra.Command {
	var corpusDir, registrations, format, onUnregistered string
	var parallelism int

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay every history in a corpus against a build and report divergences",
		RunE: func(cmd *cobra.Command, args []string) error {
			if registrations == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "Error: --registrations is required: path to a Go main package that registers workflows and calls gate.Main (see TRD_Replay_Gate.md §4)")
				os.Exit(gate.ExitOperationalError)
			}

			// go run treats a bare relative path as an import-path lookup (and
			// specially excludes "testdata" directories from those), so resolve
			// to an absolute path to make this a directory reference instead.
			absRegistrations, err := filepath.Abs(registrations)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error: resolving --registrations path: %v\n", err)
				os.Exit(gate.ExitOperationalError)
			}

			goArgs := []string{
				"run", absRegistrations,
				"--corpus", corpusDir,
				"--format", format,
				"--on-unregistered", onUnregistered,
				"--parallelism", strconv.Itoa(parallelism),
			}
			sub := exec.Command("go", goArgs...)
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
	cmd.Flags().StringVar(&format, "format", "text", "report format: text|json")
	cmd.Flags().StringVar(&onUnregistered, "on-unregistered", "fail", "behavior for unregistered workflow types: fail|skip-warn")
	cmd.Flags().IntVar(&parallelism, "parallelism", 0, "concurrent replay workers (0 = GOMAXPROCS)")
	cmd.MarkFlagRequired("corpus")
	cmd.MarkFlagRequired("registrations")
	return cmd
}
