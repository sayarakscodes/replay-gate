// Command replaygate is the CLI entrypoint for the verify, replay, and sample subcommands.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version and commit are set at release time via -ldflags -X (see .goreleaser.yaml);
// they stay "dev"/"none" for local builds.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "replaygate",
		Short:        "Replay Gate: a CI non-determinism guard for Temporal",
		SilenceUsage: true,
		Version:      fmt.Sprintf("%s (commit %s)", version, commit),
	}
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newReplayCmd())
	root.AddCommand(newSampleCmd())
	return root
}
