# Replay Gate

A CI gate that replays sampled Temporal workflow histories against new worker
builds, catching non-deterministic regressions before deploy — with
event-level divergence diffs and `GetVersion` patch suggestions. See
[PRD_Replay_Gate.md](PRD_Replay_Gate.md) and [TRD_Replay_Gate.md](TRD_Replay_Gate.md)
for the full design.

## Quick start (GitHub Action)

Add a small Go `main` package that registers your workflows and calls
`gate.Main` (see [examples/replaygate](examples/replaygate)), commit a corpus,
then add this to a workflow file:

```yaml
- uses: sayarakscodes/replay-gate/action@v1
  with:
    corpus-path: ./corpus
    registrations: ./cmd/replaygate
```

Divergences in in-flight (RUNNING) workflows fail the check; divergences only
in closed histories warn by default (set `fail-on: any` to block on those
too). Each divergence is posted as an inline annotation with a suggested
`GetVersion` fix, and a summary table is written to the job summary.

## Commands

### `replaygate verify`

Checks a corpus directory's integrity (manifest hashes, content-hashed corpus
version) without replaying anything:

```
replaygate verify --corpus testdata/corpus
```

### `replaygate replay`

Replays every history in a corpus against a build. `--registrations` points
at a small Go `main` package that registers workflows and calls `gate.Main`
(see `pkg/gate/main.go` and `testdata/replaymain_good/main.go` for an example):

```
replaygate replay --corpus testdata/corpus --registrations ./path/to/registrations
```

Report formats (`--format`): `text` (default), `json` (stable
`reportVersion`-tagged schema), `github` (inline annotations + a Markdown job
summary written to `$GITHUB_STEP_SUMMARY`), and `sarif` (for GitHub code
scanning).

Exit codes: `0` clean; `1` a divergence that blocks (in a RUNNING workflow, or
any divergence under `--fail-on=any`); `2` a divergence found only in closed
histories under the default `--fail-on=open` (warn, doesn't block a merge);
`3` operational error (bad corpus, compile failure, unregistered workflow type
with the default `--on-unregistered=fail`).

### `replaygate sample`

Connects to a live Temporal cluster and writes a stratified sample of
workflow histories as a corpus, in the same format `verify`/`replay` consume.

**Required environment variables:**

| Variable | Purpose |
|---|---|
| `TEMPORAL_ADDRESS` | Cluster gRPC endpoint, e.g. `127.0.0.1:7233` |
| `TEMPORAL_NAMESPACE` | Namespace to sample from (defaults to `default`) |
| `TEMPORAL_API_KEY` | API key auth (optional; enables TLS automatically) |
| `TEMPORAL_TLS_CERT` / `TEMPORAL_TLS_KEY` | mTLS client cert/key (optional, used if no API key) |
| `TEMPORAL_TLS_CA` | Additional CA bundle for TLS (optional) |
| `REPLAYGATE_REDACTION_KEY` | HMAC key, only required for `--redaction hash` |

```
export TEMPORAL_ADDRESS=127.0.0.1:7233
export TEMPORAL_NAMESPACE=default

replaygate sample --out ./corpus --cap 200 --closed-window 168h
```

#### Redaction

Sampled histories can contain PII in activity/workflow/signal payloads, so
every payload byte is scrubbed *before* it's written to disk — there is no
code path that persists an unredacted payload except the explicit opt-out
below. Select a profile with `--redaction` (default: `default`):

| Profile | Behavior |
|---|---|
| `default` | Blanks payload data (same length, zeroed) while keeping metadata like `encoding` — replay only needs command/event shape, never content |
| `hash` | Replaces payload data with an HMAC-SHA256 (keyed by `REPLAYGATE_REDACTION_KEY`) — preserves whether two payloads are equal/different (useful for detecting input-shape-change regressions) without exposing content |
| `none` | Passes payloads through unmodified. Prints a loud warning; only use this for a corpus you know contains no sensitive data |

The applied profile is recorded in the corpus manifest's `redaction.profile`
field, so anyone inspecting a corpus can see what was (or wasn't) scrubbed.
Redaction applies uniformly across every payload-bearing field in a history —
activity/workflow/signal inputs and results, headers, memos — via a generic
walk over the event tree (`internal/redact`), not a hardcoded list of known
fields, so newly-added Temporal event types are covered automatically.

Flags (`--cap`, `--max-events`, `--open-closed-split`, `--closed-window`,
`--visibility-rps`, `--history-rps`, `--type-scan-limit`) override the
`sample:` section of a `--config replaygate.yaml` file if one is given;
otherwise sensible defaults apply (see `internal/sampler/config.go`).

Sampling is read-only and rate-limited (default 5 RPS for `ListWorkflow`
calls, 10 RPS for history fetches) — it's safe to point at a production
cluster.

#### Local demo against a dev server

```
brew install temporal          # or: curl the released binary — see .github/workflows/go.yml
temporal server start-dev --headless --port 7233 --ip 127.0.0.1 &

# run some workflows against it, then:
export TEMPORAL_ADDRESS=127.0.0.1:7233
export TEMPORAL_NAMESPACE=default
replaygate sample --out ./corpus --cap 20 --closed-window 24h
replaygate verify --corpus ./corpus
```

`internal/sampler`'s test suite includes a round-trip test
(`TestSampler_RoundTrip`) that does exactly this against a real dev server; it
skips itself gracefully if one isn't reachable at `127.0.0.1:7233`.

## Repository layout

See TRD_Replay_Gate.md §3 for the full rationale. Notable directories:

- `pkg/gate` — the only public package; the embedding API (`gate.New`,
  `RegisterWorkflow`, `ReplayAll`) and the Mode B CLI entrypoint (`gate.Main`).
- `internal/corpus` — the on-disk corpus format: manifest schema, loader,
  integrity verification, and a `Builder` for writing new corpora.
- `internal/sampler` — live-cluster sampling (this issue).
- `internal/replayer`, `internal/report` — replay engine and report rendering.
- `testdata/regressions/` — the 6-class regression fixture battery; see its
  own README for how fixtures are generated.
- `tools/gen-regressions` — the generator behind those fixtures.
