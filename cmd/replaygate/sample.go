package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/sayarakscodes/replay-gate/internal/sampler"
	"github.com/spf13/cobra"
)

// newSampleCmd wires F1 (TRD §5.3): connect to a live cluster via the env
// vars in TRD §10, load defaults from replaygate.yaml (if given), let flags
// override them, and write a corpus in the format internal/corpus defines.
func newSampleCmd() *cobra.Command {
	var configPath, out string
	var cap_, maxEvents, typeScanLimit int
	var openClosedSplit float64
	var closedWindow string
	var visibilityRPS, historyRPS float64

	cmd := &cobra.Command{
		Use:   "sample",
		Short: "Sample workflow histories from a live cluster into a corpus",
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				return fmt.Errorf("--out is required: path to write the corpus directory")
			}

			cfg, err := sampler.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Flags override the config file only when the user actually set them.
			if cmd.Flags().Changed("cap") {
				cfg.Cap = cap_
			}
			if cmd.Flags().Changed("max-events") {
				cfg.MaxEvents = maxEvents
			}
			if cmd.Flags().Changed("type-scan-limit") {
				cfg.TypeScanLimit = typeScanLimit
			}
			if cmd.Flags().Changed("open-closed-split") {
				cfg.OpenClosedSplit = openClosedSplit
			}
			if cmd.Flags().Changed("closed-window") {
				d, err := time.ParseDuration(closedWindow)
				if err != nil {
					return fmt.Errorf("--closed-window: %w", err)
				}
				cfg.ClosedWindow = d
			}
			if cmd.Flags().Changed("visibility-rps") {
				cfg.RateLimit.VisibilityRPS = visibilityRPS
			}
			if cmd.Flags().Changed("history-rps") {
				cfg.RateLimit.HistoryRPS = historyRPS
			}

			c, namespace, err := sampler.DialFromEnv()
			if err != nil {
				return fmt.Errorf("connecting to cluster: %w", err)
			}
			defer c.Close()

			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))
			s := sampler.New(c, namespace, cfg, logger)

			result, err := s.Run(cmd.Context(), out)
			if err != nil {
				return fmt.Errorf("sampling: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "sampled %d histories across %d workflow types into %s (%d skipped)\n",
				result.Written, len(result.WorkflowTypesDiscovered), out, len(result.Skipped))
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "path to replaygate.yaml (optional; flags override its values)")
	cmd.Flags().StringVar(&out, "out", "", "directory to write the sampled corpus to")
	cmd.Flags().IntVar(&cap_, "cap", 0, "maximum total histories to sample (default from config: 200)")
	cmd.Flags().IntVar(&maxEvents, "max-events", 0, "skip histories with more events than this (default from config: 10000)")
	cmd.Flags().IntVar(&typeScanLimit, "type-scan-limit", 0, "max executions to scan while discovering workflow types (default from config: 1000)")
	cmd.Flags().Float64Var(&openClosedSplit, "open-closed-split", 0, "fraction of each type's quota reserved for open workflows (default from config: 0.7)")
	cmd.Flags().StringVar(&closedWindow, "closed-window", "", "how far back to look for recently-closed workflows, e.g. 168h (default from config: 168h)")
	cmd.Flags().Float64Var(&visibilityRPS, "visibility-rps", 0, "rate limit for ListWorkflow calls (default from config: 5)")
	cmd.Flags().Float64Var(&historyRPS, "history-rps", 0, "rate limit for GetWorkflowExecutionHistory calls (default from config: 10)")
	cmd.MarkFlagRequired("out")
	return cmd
}
