// Command replaygate is the CLI entrypoint; subcommands are added incrementally per milestone (see TRD_Replay_Gate.md).
package main

import (
	"os"

	"github.com/spf13/cobra"
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
	}
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newReplayCmd())
	root.AddCommand(newSampleCmd())
	return root
}
