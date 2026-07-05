package main

import (
	"fmt"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/spf13/cobra"
)

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
