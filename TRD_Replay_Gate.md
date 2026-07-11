# TRD — Replay Gate: A CI Non-Determinism Guard for Temporal

**Status:** Draft v1
**Owner:** Sayam Rakshit
**Last updated:** July 2026
**Companion doc:** [PRD_Replay_Gate.md](PRD_Replay_Gate.md)

---

## 1. Scope

This document specifies *how* Replay Gate is built: language, dependencies, module boundaries, data formats, CLI/Action contracts, the divergence-classification algorithm, concurrency model, and test strategy. Product rationale lives in the PRD; requirement IDs (F1–F8, N1–N5) reference the PRD tables.

**In scope (v1):** Go CLI (`replaygate`), JSON corpus format, replayer engine, differ, patcher (suggestion-only), GitHub Action.
**Out of scope (v1):** non-Go SDKs, auto-applied patches, cluster discovery, the M4 LLM layer (interfaces are stubbed for it, see §11).

---

## 2. Tech Stack

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go ≥ 1.22 | Must link the workflow code under test into the same process as the replayer (see §5.1); Temporal Go SDK is Go-native. |
| Temporal SDK | `go.temporal.io/sdk` — pinned range `>= 1.34.0, < 2.0.0`, tested matrix in CI | Differ parses SDK error structures; version coupling is a named risk (PRD §8), so the supported range is explicit and CI-tested. Floor verified empirically (see TRD §14 OQ3 resolution, issue #7): the `[TMPRL1100]` marker itself only appears from v1.26.0, and the "During replay, a matching %v command was expected..." template (used to classify the changed-timer regression class) only appears from v1.34.0 — below that, all classes still *detect* the divergence, but that one specific class falls back to `unknown` rather than `removed`. |
| History protos | `go.temporal.io/api` (`history/v1`, `enums/v1`, `workflowservice/v1`) | Canonical wire types; corpus files are protojson-encoded `history.History`. |
| CLI framework | `spf13/cobra` | Standard, subcommand-friendly (`sample`, `replay`, `report`). |
| Config | YAML file (`replaygate.yaml`) + flags + env vars, precedence: flags > env > file | CI-friendly; secrets only via env (§10). |
| Logging | `log/slog`, JSON in CI, text locally | No third-party logger needed. |
| GitHub Action | Composite action wrapping a released binary (goreleaser artifacts) | No Docker pull latency; runs on any Linux/macOS runner. |
| Release | goreleaser, `linux/amd64`, `linux/arm64`, `darwin/arm64` | CI runners + local dev. |

---

## 3. Repository Layout

```
replaygate/
├── cmd/replaygate/            # main.go, cobra wiring only
├── internal/
│   ├── sampler/               # F1: visibility queries, history fetch, stratification, rate limiting
│   ├── corpus/                # F2: corpus schema, read/write, versioning, redaction
│   ├── replayer/              # F3: WorkflowReplayer orchestration, worker pool
│   ├── differ/                # F4: SDK error → structured Divergence
│   ├── patcher/               # F6: Divergence → GetVersion suggestion
│   └── report/                # F4/F5/F7: human text, JSON, GitHub annotations, SARIF
├── pkg/gate/                  # public embedding API (§6.4) — the only exported package
├── action/                    # action.yml + entrypoint script (F7)
├── testdata/
│   ├── corpus/                # committed fixture corpus (MVP demo)
│   └── regressions/           # 6-class before/after workflow battery (§12)
└── examples/                  # example worker repo showing integration
```

`internal/` keeps the SDK-error-coupled machinery unexported; only `pkg/gate` (the embedding API) and the CLI are public surface.

---

## 4. Key Architectural Constraint: Replay Requires Linking the Workflow Code

`worker.WorkflowReplayer` replays a history against **registered Go workflow functions**. Therefore Replay Gate cannot be a standalone binary pointed at an arbitrary repo — the workflow code under test must be compiled into the process that replays.

Two supported integration modes:

**Mode A — embedded test (primary, v1).** The user adds a small Go test/binary in *their* repo that imports `pkg/gate`, registers their workflows, and points at a corpus:

```go
func TestReplayGate(t *testing.T) {
    g := gate.New(gate.Config{CorpusDir: "testdata/replay-corpus"})
    g.RegisterWorkflow(order.FulfillmentWorkflow)
    g.RegisterWorkflowWithOptions(billing.Invoice, workflow.RegisterOptions{Name: "InvoiceV2"})
    g.Run(t) // fails the test with a rendered divergence report
}
```

**Mode B — generated runner (CLI convenience).** `replaygate replay --registrations ./gate_registrations.go` compiles a temporary `main` that imports the user's registration file and the gate, runs it, and streams results back. This is sugar over Mode A; both share `pkg/gate`.

The CLI's `sample` and `report` subcommands are pure binaries (no user code needed); only `replay` needs Mode A/B.

---

## 5. Component Design

### 5.1 Replayer (F3, N1, N5)

- Wraps `worker.NewWorkflowReplayer()` — one replayer instance per goroutine (the SDK replayer is not safe for concurrent `ReplayWorkflowHistory` calls).
- **Worker pool:** `min(GOMAXPROCS, flag --parallelism)` goroutines; each pulls histories from a channel, calls `ReplayWorkflowHistory(nopLogger, hist)`, and emits a `ReplayResult{HistoryRef, Err, Duration}`.
- Histories are decoded once (protojson → `*historypb.History`) by the corpus loader and fanned out read-only.
- A panic inside user workflow code during replay is recovered per-history and reported as a divergence of class `panic` — one bad history must not kill the run.
- Determinism (N5): results are collected and sorted by corpus index before reporting, so output ordering is stable regardless of goroutine scheduling.
- Throughput (N1): budget is ~2 ms/history/core; 500/s needs 1 core at that rate. Benchmark `BenchmarkReplayCorpus` in CI guards the target with a committed 1 000-history synthetic corpus.

### 5.2 Corpus (F2, N4, N5)

On-disk layout — a directory, human-inspectable, git- and object-storage-friendly:

```
corpus/
├── manifest.json
└── histories/
    └── <workflowType>/<workflowID>_<runID>.json   # protojson historypb.History
```

`manifest.json` schema (answers PRD open question 3):

```json
{
  "corpusVersion": "sha256:<hash of sorted history file hashes>",
  "formatVersion": 1,
  "sampledAt": "2026-07-05T10:00:00Z",
  "cluster": { "namespace": "prod", "endpoint": "redacted-ok" },
  "sdkVersionAtSampling": "1.45.0",
  "redaction": { "profile": "default", "fieldsScrubbed": ["input", "result"] },
  "entries": [
    {
      "file": "histories/OrderFulfillment/wf-123_run-abc.json",
      "workflowType": "OrderFulfillment",
      "workflowID": "wf-123",
      "runID": "run-abc",
      "status": "RUNNING",          // RUNNING | COMPLETED | FAILED | ...
      "eventCount": 148,
      "sha256": "..."
    }
  ]
}
```

- `corpusVersion` is content-derived (Merkle-style hash), so any mutation changes it; the replay report embeds it, giving builds a verifiable "validated against corpus X" claim.
- `status` enables the open/closed split: entries with `status == RUNNING` are **hard-fail**, closed entries are **warn** by default (`--fail-on=open|any`, resolving PRD open question 2 with a flag; default `open`).
- History files use the same protojson encoding as `temporal workflow show --output json`, so users can hand-add histories from the CLI they already have.
- Loader validates every entry's sha256 against the manifest before replay; mismatch is a hard error (tamper/corruption guard).

### 5.3 Sampler (F1, N3, N4)

- Client: `client.Dial` from the Go SDK; auth via env (`TEMPORAL_ADDRESS`, `TEMPORAL_NAMESPACE`, `TEMPORAL_TLS_CERT/KEY` or `TEMPORAL_API_KEY`).
- **Stratification algorithm:**
  1. `ListWorkflowExecutions` with `GROUP BY` unavailable in visibility → enumerate types via `CountWorkflowExecutions` per known type, or paged listing with client-side bucketing.
  2. Per workflow type: quota = `cap × typeWeight`, split (default 70/30) between `ExecutionStatus = 'Running'` and closed-within-`--closed-window` (default 7d).
  3. Within each bucket, reservoir-sample to quota to avoid recency bias from list ordering.
- **Rate limiting (N3):** `golang.org/x/time/rate` token bucket, default 5 RPS on visibility calls and 10 RPS on `GetWorkflowExecutionHistory`, flag-tunable. All calls are read-only APIs; the sampler never touches workflow state.
- Long histories: fetch with pagination; skip histories over `--max-events` (default 10 000) with a manifest note, keeping corpus and replay time bounded.
- **Redaction (N4):** runs before any byte is persisted. A `Scrubber` interface:

  ```go
  type Scrubber interface {
      Scrub(payload *commonpb.Payload) *commonpb.Payload
  }
  ```

  Default profile blanks `Payloads.data` in activity/workflow/signal inputs and results, preserving metadata (encoding, size) — replay compatibility checking does not require payload *contents*, only command/event shape. Note: renamed-input-*shape* detection is limited under full redaction; a `hash` profile (HMAC of payload with a user key) preserves inequality detection without exposing data. Profiles: `none | default(blank) | hash`, plus user-pluggable via `pkg/gate`.

### 5.4 Differ (F4, N2) — the SDK-coupled core

Input: the `error` from `ReplayWorkflowHistory` plus the history being replayed. Output:

```go
type Divergence struct {
    HistoryRef   corpus.EntryRef
    EventID      int64             // history event where mismatch surfaced
    Class        Class             // reorder | removal | rename | added | nondeterministic-construct | panic | unknown
    Expected     EventSummary      // recorded event: type, activity/timer name, attrs digest
    Generated    CommandSummary    // command the new code produced (when recoverable)
    RawError     string            // always preserved verbatim
    OpenWorkflow bool              // drives hard-fail vs warn
}
```

Strategy, in order of preference:

1. **Typed extraction.** Match SDK error chains with `errors.As` where typed (e.g., workflow panics surface a wrapped panic error carrying the workflow stack trace).
2. **Structured-text extraction.** Non-determinism errors carry the `[TMPRL1100]` marker and a recognizable grammar ("nondeterministic workflow: history event ... command ..."). A table of anchored regexes — one per known message shape per supported SDK minor version — extracts event ID, expected event type, and generated command type.
3. **History cross-reference.** With the failing event ID, the differ re-reads the history around it to fill `Expected` (event attributes: activity type name, timer ID, signal name) and to classify:
   - expected event's command type appears later in generated stream → `reorder`
   - expected activity/timer never generated → `removal`
   - same command type, different activity type name → `rename`
   - command generated with no matching recorded event → `added`
   - panic mentioning `time.Now`/`rand`/map iteration in workflow stack frames → `nondeterministic-construct`
4. **Fallback.** Anything unmatched is `Class: unknown` with the raw error — *never* dropped, never guessed (N2: a report line must always correspond to a real replay failure; classification confidence may vary but the failure itself is ground truth).

**SDK-drift guard (PRD risk §8):** a canary test replays each of the 6 regression fixtures and asserts the differ classifies them — if a new SDK version changes error text, this test fails loudly and the regex table gets a new entry. The supported SDK range in §2 is exactly the range this test matrix covers.

### 5.5 Patcher (F6)

Pure function `Suggest(d Divergence) (Patch, bool)`. Template per class — e.g. for `removal` of activity `NotifyLegacy`:

```go
v := workflow.GetVersion(ctx, "remove-notify-legacy", workflow.DefaultVersion, 1)
if v == workflow.DefaultVersion {
    // old path: keep the call so in-flight histories still match
    workflow.ExecuteActivity(ctx, NotifyLegacy, input).Get(ctx, nil)
}
```

- `changeID` is derived from the divergence (`<class>-<kebab(activityName)>`), guaranteed unique per report via suffixing.
- v1 emits snippets only (PRD non-goal: no code mutation) — the patcher does **not** locate the source line; it renders the snippet in the report/annotation with the workflow type as context. Source-location mapping is the M4/LLM layer's job.
- Classes `panic`, `nondeterministic-construct`, and `unknown` get remediation *guidance* (e.g., "wrap with `workflow.Now(ctx)` / `workflow.SideEffect`") instead of a GetVersion snippet, since GetVersion does not fix those.

### 5.6 Report & CI surface (F4, F5, F7)

Output formats, selected with `--format` (multi-allowed):

- `text` — human summary to stdout (default locally).
- `json` — full machine-readable report: corpus version, per-history result, divergences, timings. Stable schema, versioned `reportVersion: 1`.
- `github` — GitHub Actions workflow commands (`::error file=...`) + a Markdown job-summary written to `$GITHUB_STEP_SUMMARY` with the divergence table and patch snippets.
- `sarif` — optional, enables the GitHub code-scanning UI without a PR-comment bot.

**Exit codes (F5):** `0` clean; `1` divergence in an open workflow (or any, with `--fail-on=any`); `2` divergence only in closed histories when `--fail-on=open` (still nonzero-visible but distinguishable — Action maps 2 to a warning, not a failure); `3` operational error (bad corpus, compile failure).

**GitHub Action (`action/action.yml`):** composite action; inputs `corpus-path`, `fail-on`, `go-version`, `registrations`; steps: setup-go → download pinned `replaygate` binary (checksum-verified) → run Mode B replay → upload JSON report as artifact. Target: usable in < 10 lines of workflow YAML (PRD success metric).

---

## 6. Configuration Reference

```yaml
# replaygate.yaml
corpus:
  path: ./testdata/replay-corpus
sample:
  cap: 200
  openClosedSplit: 0.7
  closedWindow: 168h
  maxEvents: 10000
  rateLimit: { visibilityRPS: 5, historyRPS: 10 }
  redaction: default          # none | default | hash
replay:
  parallelism: 0              # 0 = GOMAXPROCS
  failOn: open                # open | any
report:
  formats: [text, json]
```

Secrets (`TEMPORAL_ADDRESS`, TLS material, API keys) are env-only and never written to config, corpus, or reports.

---

## 7. CLI Contract

```
replaygate sample  --config replaygate.yaml [--namespace ...] [--out corpus/]
replaygate replay  --corpus corpus/ --registrations ./gate/registrations.go [--fail-on open]
replaygate verify  --corpus corpus/            # manifest/hash integrity check only
replaygate report  --from report.json --format github   # re-render a stored report
```

---

## 8. Concurrency & Performance Budget (N1)

| Stage | Model | Budget |
|---|---|---|
| Corpus load | sequential decode, streamed | ≤ 1 s per 1 000 histories |
| Replay | worker pool, 1 replayer/goroutine | ≥ 500 histories/s on 4-core runner |
| Diff | only on failures, sequential | negligible (failures are rare) |
| Sample | rate-limited, network-bound | not on the merge-gate path |

Memory: histories held decoded in memory; at ~50 KB/history × 10 000 cap ≈ 500 MB worst case — acceptable on standard runners; `--stream` mode (decode-per-worker) is a fallback if a user's corpus is pathological.

---

## 9. Error Handling Rules

- Corpus integrity failure (hash mismatch, unreadable file) → exit 3, no partial replay (a partial run must never report "clean").
- Unregistered workflow type in corpus → configurable: `--on-unregistered=fail|skip-warn` (default `fail`; a silently skipped history is a coverage hole).
- Per-history panic → recovered, classified, run continues (§5.1).
- Sampler API errors → retry with backoff (3 attempts) under the rate limiter; a history that can't be fetched is logged and excluded, never half-written.

---

## 10. Security & Privacy (N3, N4)

- Sampler uses only read APIs: `ListWorkflowExecutions`, `CountWorkflowExecutions`, `GetWorkflowExecutionHistory`, `DescribeNamespace`.
- Redaction is applied *pre-persistence*; there is no code path that writes an unredacted payload unless `redaction: none` is explicitly set, and `sample` prints a prominent warning in that mode.
- Reports never include payload bytes — only event/command type names and IDs.
- Released binaries: checksummed; Action pins by version + sha256.

---

## 11. Extension Points (forward-compat for M4, non-Go SDKs)

- `report.json` is the contract for the M4 LLM layer: it contains everything needed (divergence, raw error, history excerpt refs, patch template) — M4 is a separate consumer, not a change to the core.
- `Scrubber`, `Classifier` (differ rule table), and `PatchTemplate` are interfaces in `pkg/gate` so users can extend without forking.
- Corpus format is SDK-language-neutral (raw `historypb.History`); a future Java replayer reuses it unchanged.

---

## 12. Test Strategy

| Layer | Tests |
|---|---|
| Regression battery (PRD metric: 6/6) | `testdata/regressions/`: six before/after workflow pairs (reorder, removal, timer-change, rename, added-command, `time.Now()` injection). Histories generated once against the "before" code using the Temporal test framework + `temporal workflow show`, committed. CI asserts: before-code replays clean, after-code produces the expected `Class`. |
| False-positive guard (PRD metric: 0 FPs) | Replay the full fixture corpus against unchanged code across the SDK version matrix; any failure fails CI. |
| Differ unit tests | Golden raw-error strings per SDK version → expected `Divergence`. |
| Corpus round-trip | sample (against `temporalite`/dev-server in CI) → write → verify → load → byte-identical protojson. |
| Redaction | Property test: no `Payloads.data` bytes from input survive into corpus files under `default`/`hash` profiles. |
| Throughput | `BenchmarkReplayCorpus` with the 1 000-history synthetic corpus; CI fails below 500/s (with generous runner-variance margin, e.g. gate at 250/s in CI, report real number in README). |
| Action e2e | Workflow in this repo runs the Action against `examples/` with an intentional regression branch; asserts exit code + annotation presence. |

---

## 13. Milestone → Deliverable Mapping (from PRD §6)

| Milestone | Technical deliverables |
|---|---|
| M0 | `pkg/gate` (Mode A), `internal/replayer`, `internal/corpus` loader, `replay` command, exit codes, fixture corpus. |
| M1 | `internal/sampler`, redaction profiles, `sample` + `verify` commands, manifest hashing. |
| M2 | `internal/differ` full classification, SDK-drift canary test, regression battery green 6/6. |
| M3 | `internal/patcher`, `github`/`sarif` formats, `action/`, goreleaser pipeline, `examples/`. |
| M4 (phase 2) | Out of scope here; consumes `report.json` (§11). |

---

## 14. Resolved & Open Technical Decisions

**Resolved in this TRD:**
- Corpus location (PRD OQ1): directory format works both committed and in object storage; v1 ships committed-fixture flow, `--corpus` accepts a path only (object-storage fetch is a one-line pre-step in CI, not built in).
- Open vs closed severity (PRD OQ2): `--fail-on=open` default, exit-code 2 for closed-only divergence.
- Corpus versioning (PRD OQ3): content-hash `corpusVersion` in manifest, echoed in every report.
- Mode B implementation (issue #3): a user-authored main package calling `gate.Main`, not code generation — see `pkg/gate/main.go`.
- `Generated` field nullability (issue #7 spike): confirmed **nullable**. Only one of the SDK's ~10 `[TMPRL1100]` message templates carries both expected and generated; others carry one side or neither. `Divergence.Expected`/`Divergence.Generated` are both `*EventSummary`/`*CommandSummary`.
- Minimum SDK version floor (issue #7 spike): verified empirically by diffing SDK source across versions, **not** 1.25 as assumed — see the corrected range in §2's tech stack table above.
- rename vs. nondeterministic-construct ambiguity (issue #7): the SDK error is textually identical for both ("expected activity type A, got B") — it has no concept of *why* the code branched differently. The differ uses a best-effort source-scan heuristic (`runtime.FuncForPC` + grep the registered workflow function's own source for `time.Now(`/`rand.`/`go func(`) to upgrade the classification when it finds one; otherwise defaults to `rename`. This is a real signal, not a guess, but it is not a certainty — documented as such in `Divergence.Note`.

**Still open:** none from the original M0/M1/M2 list — all three items above are now resolved.
