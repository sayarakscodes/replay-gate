package sampler

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"golang.org/x/time/rate"

	"github.com/sayarakscodes/replay-gate/internal/corpus"
	"github.com/sayarakscodes/replay-gate/internal/redact"
)

// Sampler pulls a stratified sample of workflow histories from a live
// cluster (F1, TRD §5.3). All cluster calls go through visLimiter or
// histLimiter and are retried per retry.go.
type Sampler struct {
	client    client.Client
	namespace string
	cfg       Config
	scrubber  redact.Scrubber
	logger    *slog.Logger

	visLimiter  *rate.Limiter
	histLimiter *rate.Limiter
}

// New constructs a Sampler. scrubber runs on every payload before it's
// persisted (TRD §5.3, N4) — a nil scrubber defaults to redact.DefaultScrubber,
// never to a passthrough, so a caller can't silently end up unredacted.
func New(c client.Client, namespace string, cfg Config, scrubber redact.Scrubber, logger *slog.Logger) *Sampler {
	if logger == nil {
		logger = slog.Default()
	}
	if scrubber == nil {
		scrubber = redact.DefaultScrubber{}
	}
	return &Sampler{
		client:      c,
		namespace:   namespace,
		cfg:         cfg,
		scrubber:    scrubber,
		logger:      logger,
		visLimiter:  rate.NewLimiter(rate.Limit(cfg.RateLimit.VisibilityRPS), 1),
		histLimiter: rate.NewLimiter(rate.Limit(cfg.RateLimit.HistoryRPS), 1),
	}
}

// SkipReason records why a discovered execution didn't make it into the corpus.
type SkipReason struct {
	WorkflowType string
	WorkflowID   string
	RunID        string
	Reason       string
}

// Result summarizes what Run wrote.
type Result struct {
	WorkflowTypesDiscovered []string
	Written                 int
	Skipped                 []SkipReason
}

// Run discovers workflow types, stratified-samples executions per type
// (TRD §5.3: per-type quota, openClosedSplit between open/closed, reservoir
// sampling within each bucket), fetches each selected history, and writes a
// corpus to dir via corpus.Builder.
func (s *Sampler) Run(ctx context.Context, dir string) (*Result, error) {
	types, err := s.discoverWorkflowTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovering workflow types: %w", err)
	}
	if len(types) == 0 {
		return &Result{}, nil
	}

	quotaPerType := s.cfg.Cap / len(types)
	if quotaPerType < 1 {
		quotaPerType = 1
	}
	openQuota := int(float64(quotaPerType) * s.cfg.OpenClosedSplit)
	closedQuota := quotaPerType - openQuota

	builder := corpus.NewBuilder(dir,
		corpus.ClusterInfo{Namespace: s.namespace},
		corpus.RedactionInfo{Profile: s.cfg.Redaction, FieldsScrubbed: redact.FieldsScrubbed(s.cfg.Redaction)},
	)
	builder.SetSDKVersion(temporal.SDKVersion)

	result := &Result{WorkflowTypesDiscovered: types}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for _, wfType := range types {
		picks := s.sampleBucket(ctx, fmt.Sprintf("WorkflowType='%s' AND ExecutionStatus='Running'", wfType), openQuota, rng)
		since := time.Now().Add(-s.cfg.ClosedWindow).Format(time.RFC3339)
		picks = append(picks, s.sampleBucket(ctx,
			fmt.Sprintf("WorkflowType='%s' AND ExecutionStatus!='Running' AND CloseTime>'%s'", wfType, since),
			closedQuota, rng)...)

		for _, p := range picks {
			wfID, runID := p.Execution.WorkflowId, p.Execution.RunId
			status := statusName(p.Status)

			hist, truncated, err := s.fetchHistory(ctx, wfID, runID)
			if err != nil {
				s.logger.Warn("skipping history: fetch failed", "workflowType", wfType, "workflowId", wfID, "runId", runID, "error", err)
				result.Skipped = append(result.Skipped, SkipReason{wfType, wfID, runID, fmt.Sprintf("fetch failed: %v", err)})
				continue
			}
			if truncated {
				s.logger.Warn("skipping history: exceeds max-events", "workflowType", wfType, "workflowId", wfID, "runId", runID, "maxEvents", s.cfg.MaxEvents)
				result.Skipped = append(result.Skipped, SkipReason{wfType, wfID, runID, fmt.Sprintf("exceeds max-events (%d)", s.cfg.MaxEvents)})
				continue
			}

			// Redaction must run before anything reaches disk — this is the
			// only place a fetched history is written to the corpus (N4).
			redact.RedactHistory(hist, s.scrubber)

			if err := builder.AddHistory(wfType, wfID, runID, status, hist); err != nil {
				return nil, fmt.Errorf("writing history %s/%s/%s: %w", wfType, wfID, runID, err)
			}
			result.Written++
		}
	}

	if err := builder.Finish(); err != nil {
		return nil, fmt.Errorf("finishing corpus: %w", err)
	}
	return result, nil
}

// discoverWorkflowTypes pages through a broad listing (up to TypeScanLimit
// executions) collecting distinct workflow type names. This avoids depending
// on visibility GROUP BY support, which basic (non-Elasticsearch) visibility
// backends — including the local dev server — don't provide.
func (s *Sampler) discoverWorkflowTypes(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	var types []string
	var pageToken []byte
	scanned := 0

	for scanned < s.cfg.TypeScanLimit {
		resp, err := s.listWorkflows(ctx, "", pageToken)
		if err != nil {
			return nil, err
		}
		for _, e := range resp.Executions {
			name := e.Type.GetName()
			if !seen[name] {
				seen[name] = true
				types = append(types, name)
			}
		}
		scanned += len(resp.Executions)
		pageToken = resp.NextPageToken
		if len(pageToken) == 0 || len(resp.Executions) == 0 {
			break
		}
	}

	sort.Strings(types)
	return types, nil
}

// sampleBucket reservoir-samples up to quota executions matching query,
// paging through all matches without buffering more than quota+the current page.
func (s *Sampler) sampleBucket(ctx context.Context, query string, quota int, rng *rand.Rand) []*workflowpb.WorkflowExecutionInfo {
	if quota <= 0 {
		return nil
	}
	res := newReservoir[*workflowpb.WorkflowExecutionInfo](quota, rng)
	var pageToken []byte

	for {
		resp, err := s.listWorkflows(ctx, query, pageToken)
		if err != nil {
			s.logger.Warn("bucket listing failed, sampling what was found so far", "query", query, "error", err)
			break
		}
		for _, e := range resp.Executions {
			res.Add(e)
		}
		pageToken = resp.NextPageToken
		if len(pageToken) == 0 || len(resp.Executions) == 0 {
			break
		}
	}
	return res.Items()
}

func (s *Sampler) listWorkflows(ctx context.Context, query string, pageToken []byte) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	if err := s.visLimiter.Wait(ctx); err != nil {
		return nil, err
	}
	var resp *workflowservice.ListWorkflowExecutionsResponse
	err := withRetry(ctx, func() error {
		var err error
		resp, err = s.client.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
			Namespace:     s.namespace,
			PageSize:      100,
			Query:         query,
			NextPageToken: pageToken,
		})
		return err
	})
	return resp, err
}

// fetchHistory streams history events page by page, stopping (and reporting
// truncated=true) as soon as the count exceeds MaxEvents, so a single huge
// history can't be pulled in full before we realize it should be skipped.
func (s *Sampler) fetchHistory(ctx context.Context, workflowID, runID string) (hist *historypb.History, truncated bool, err error) {
	if err := s.histLimiter.Wait(ctx); err != nil {
		return nil, false, err
	}

	var events []*historypb.HistoryEvent
	err = withRetry(ctx, func() error {
		events = nil // reset in case a prior attempt partially populated it
		iter := s.client.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
		for iter.HasNext() {
			e, err := iter.Next()
			if err != nil {
				return err
			}
			events = append(events, e)
			if len(events) > s.cfg.MaxEvents {
				truncated = true
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if truncated {
		return nil, true, nil
	}
	return &historypb.History{Events: events}, false, nil
}

// statusName maps to the same uppercase convention corpus.Status* constants
// use, regardless of source (this sampler or the fixture generators) — the
// SDK's enum String() returns a human-friendly form ("Completed", "Running"),
// not the raw SCREAMING_SNAKE_CASE constant name.
func statusName(s enumspb.WorkflowExecutionStatus) string {
	switch s {
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return corpus.StatusRunning
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return corpus.StatusCompleted
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		return corpus.StatusFailed
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return corpus.StatusTerminated
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return corpus.StatusTimedOut
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return corpus.StatusCanceled
	default:
		return strings.ToUpper(strings.TrimPrefix(s.String(), "WORKFLOW_EXECUTION_STATUS_"))
	}
}
