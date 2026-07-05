// Command replaygate is the CLI entrypoint; subcommands are added incrementally per milestone (see TRD_Replay_Gate.md).
package main

import (
	"fmt"
	"os"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
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
	return root
}

func newVerifyCmd() *cobra.Command {
	var corpusDir string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify corpus integrity (manifest hashes, corpus version) without replaying it",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := corpus.Load(corpusDir)
			if err != nil {
				return fmt.Errorf("corpus invalid: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "corpus OK: %d histories, version %s\n", len(c.Histories), c.Manifest.CorpusVersion)
			return nil
		},
	}
	cmd.Flags().StringVar(&corpusDir, "corpus", "", "path to the corpus directory")
	cmd.MarkFlagRequired("corpus")
	return cmd
}
