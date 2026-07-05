# PRD — Replay Gate: A CI Non-Determinism Guard for Temporal

**Status:** Draft v1
**Owner:** Sayam Rakshit
**Last updated:** July 2026

---

## 1. Problem

Temporal workflows are durable because the SDK reconstructs their state by **replaying recorded event history against the current workflow code**. This only works if the code is deterministic: on replay, the commands the code generates (schedule activity, start timer, complete workflow) must match, in order, the events already recorded in history.

The moment a code change causes the replayed command stream to diverge from an in-flight workflow's recorded history, that workflow **fails with a non-determinism error** and gets stuck — potentially thousands of long-running executions, discovered only after the bad build is already deployed to production. Common triggers are mundane and easy to ship by accident:

- Reordering two activity or child-workflow calls
- Adding or removing an activity/timer/signal on an existing code path
- Renaming an activity type or changing its input shape
- Changing control flow that gates a command (e.g., an `if` that now skips a timer)
- Introducing non-deterministic constructs (`time.Now()`, `rand`, map-range, goroutines)

**Why existing tooling is insufficient.** Temporal ships [`workflowcheck`](https://github.com/temporalio/sdk-go/tree/master/contrib/tools/workflowcheck), a static analyzer that flags non-deterministic *constructs*. It cannot catch the far more common failure: code that is perfectly deterministic in isolation but **incompatible with histories already in flight** because the logic changed. Detecting that requires replaying real histories — a dynamic check that today only happens manually, if at all, and usually after an incident.

**The gap:** there is no CI gate that samples real production histories, replays them against the proposed build, and blocks the merge/deploy when divergence is detected. That is Replay Gate.

---

## 2. Goals & Non-Goals

### Goals
- Catch non-determinism regressions **before deploy**, in CI, against real workflow histories.
- Produce a **human-readable divergence report**: which workflow, which history event, expected vs. generated command.
- Suggest the concrete `workflow.GetVersion` patch that safely branches old vs. new logic.
- Be trivially adoptable: a CLI and a GitHub Action, reproducible without live-cluster access on every run.
- Near-zero false positives — replay is deterministic, so a failure means a real incompatibility.

### Non-Goals (v1)
- **Non-Go SDKs.** Go only for v1; Java/TypeScript are future work.
- **Auto-opening PRs / auto-applying patches.** v1 emits suggestions as CI annotations; it does not mutate code.
- **Cluster auto-discovery / RBAC integration.** v1 assumes standard Visibility + frontend access via provided credentials.
- **Replacing `workflowcheck`.** Replay Gate is complementary; static + dynamic together give full coverage.
- **Guaranteeing zero missed regressions.** Coverage is bounded by the sampled corpus (see §7 risks).

---

## 3. Users

**Primary — Temporal platform / infra engineers.** Own the workers, gate deploys, get paged when workflows wedge. They want a merge check that says "this change breaks 3 in-flight order-fulfillment workflows, here's the fix."

**Secondary — application engineers shipping workflow code.** Want fast, specific feedback in the PR rather than a post-deploy incident.

---

## 4. How It Works (Flow)

```
┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐
│ Sampler  │──▶│  Corpus  │──▶│ Replayer │──▶│  Differ  │──▶│ Patcher  │
│          │   │ (JSON)   │   │          │   │          │   │          │
└──────────┘   └──────────┘   └──────────┘   └──────────┘   └──────────┘
 pull real      committed/      replay each     extract        map divergence
 histories      cached          history vs       expected vs    to GetVersion
 from cluster    fixtures       new build        generated      suggestion
                                                  command
```

1. **Sampler** queries the Visibility API for a stratified sample of workflows (by type, mixing open and recently-closed), fetches each history via `GetWorkflowExecutionHistory`, and writes them as a versioned JSON **corpus**.
2. The **corpus** is cached (or committed) so CI runs are reproducible and don't hammer the production cluster on every push. Refresh is a separate, scheduled job.
3. **Replayer** registers the build's workflow functions with the Go SDK's `worker.WorkflowReplayer` and replays every history in the corpus.
4. On failure, the **Differ** parses the replay error into a structured divergence: history ID, event index, recorded event vs. generated command, and a classification (reorder / removal / rename / added-command / non-deterministic-construct).
5. The **Patcher** maps each divergence class to a suggested `workflow.GetVersion(ctx, changeID, minSupported, maxSupported)` branch and renders it as a diff snippet in the CI annotation.

---

## 5. Requirements

### 5.1 Functional

| ID | Requirement | Priority |
|----|-------------|----------|
| F1 | Sample workflow histories from a live cluster via Visibility + `GetWorkflowExecutionHistory`, with stratification by workflow type and open/closed status, and a configurable cap. | P0 |
| F2 | Persist sampled histories as a versioned, human-inspectable JSON corpus; load corpus for replay without cluster access. | P0 |
| F3 | Replay every corpus history against the current build using the Go SDK `WorkflowReplayer`; return pass/fail per history. | P0 |
| F4 | On divergence, emit a structured report: history ID, workflow type, failing event index, expected recorded event, generated command, divergence class. | P0 |
| F5 | Exit non-zero on any divergence so CI blocks the merge/deploy. | P0 |
| F6 | Suggest a `GetVersion` patch snippet per divergence class. | P1 |
| F7 | Ship a GitHub Action wrapper that runs the gate on PRs and posts divergences as inline annotations. | P1 |
| F8 | (Phase 2 — AI) Generate a plain-English explanation of each divergence and a ready-to-apply patch diff. | P2 |

### 5.2 Non-Functional

| ID | Requirement |
|----|-------------|
| N1 | Replay throughput target: **≥ 500 histories/sec** on a standard CI runner (histories are small; replay is CPU-bound and parallelizable). |
| N2 | **False-positive rate ≈ 0** — a reported divergence must correspond to a genuine incompatibility. This is the tool's core trust guarantee. |
| N3 | Corpus sampling must be **read-only** against the cluster and rate-limited to avoid load on production frontend. |
| N4 | No secrets in the corpus: sampler must support **payload redaction/scrubbing** of activity inputs/outputs before persisting (histories can contain PII). |
| N5 | Deterministic, reproducible CI runs from a fixed corpus + build. |

---

## 6. Milestones

**M0 — Replay core (the credible hard part).** Load a hand-built corpus, replay against a build, pass/fail output. Proves the mechanism end to end. *Deliverable: CLI that replays a JSON corpus and exits non-zero on divergence.*

**M1 — Sampler + corpus format.** Live Visibility sampling, stratification, redaction, versioned JSON corpus. *Deliverable: `replaygate sample` command.*

**M2 — Differ.** Turn opaque replay errors into structured, classified divergences with event-level detail. *This is the highest-signal engineering work — the raw SDK error is nearly unreadable.*

**M3 — Patcher + GitHub Action.** `GetVersion` suggestions and PR annotations. *Deliverable: adoptable in one workflow file.*

**M4 — AI layer (Phase 2).** LLM explanation of each divergence + auto-generated patch diff, delivered as a PR comment. Mirrors the CRED MCP/on-call-automation narrative.

A defensible **MVP = M0 + M2 + M3** with a committed test corpus. M1's live sampling can be demoed against a local Temporal dev cluster.

---

## 7. Success Metrics

- **Detection coverage:** correctly flags a test battery of known regression classes — reordered activity, removed activity, changed timer, renamed activity type, added command, injected `time.Now()` — each as a before/after workflow pair with a real history. Target: 6/6 classes caught.
- **False positives:** 0 across a corpus of unchanged-code replays.
- **Throughput:** ≥ 500 histories/sec (N1), reported as a benchmark in the README.
- **Adoptability:** integrable into a repo's CI in under 10 lines of workflow YAML.

---

## 8. Risks & Open Questions

| Risk | Mitigation |
|------|------------|
| **Coverage is only as good as the sample.** A regression that only affects an unsampled workflow path slips through. | Stratify sampling by workflow type + state; document the limitation explicitly; recommend pairing with `workflowcheck` for static coverage. |
| **Histories contain PII / secrets.** | Payload redaction in the sampler (N4); support pluggable scrubbers; never commit unredacted corpora. |
| **Corpus staleness.** Old corpus misses recently-introduced workflow types. | Scheduled refresh job separate from the merge-gate run; surface corpus age in the report. |
| **Replay error parsing is SDK-version-coupled.** The Differ depends on internal error text/structure. | Pin to a tested SDK version range; add a test that fails loudly if the SDK error format changes. |
| **Non-determinism from external randomness** (config, feature flags) may cause replay noise. | Document that the build under test must be config-deterministic; treat as out of scope for v1. |

**Open questions:**
1. Corpus committed to the repo vs. fetched from object storage at CI time? (Leaning: small committed fixtures for the demo, object storage for real scale.)
2. Should the gate distinguish "breaks in-flight workflows" (hard fail) from "breaks only closed histories" (warn)? Closed workflows never replay in prod, so this could cut false urgency.
3. How to represent the corpus version so a build knows which corpus it was validated against?

---

## 9. Positioning (for the README / resume)

> An open-source CI gate that replays sampled production workflow histories against new Temporal worker builds, catching non-deterministic changes before deploy — with event-level divergence diffs and `GetVersion` patch suggestions. Complements Temporal's static `workflowcheck` by covering the dynamic, history-incompatibility failures static analysis cannot see.

**Resume bullets this unlocks:**
- *Distributed systems:* "Built an open-source CI gate that replays real Temporal workflow histories against candidate builds, detecting non-deterministic regressions pre-deploy with event-level divergence diffs — closing a gap static analysis (`workflowcheck`) cannot cover."
- *AI automation:* "Added an LLM layer that explains each divergence in plain English and generates a ready-to-apply `GetVersion` patch, delivered as a PR comment."
