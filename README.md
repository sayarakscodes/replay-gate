# Replay Gate

A CI gate that replays recorded Temporal workflow histories against your
current worker build and fails the check when the code has become
non-deterministic — catching a whole class of production incidents before they
ship, with event-level divergence diffs and suggested `workflow.GetVersion`
fixes.

## Why

Temporal workflows are durable because the SDK reconstructs their state by
**replaying recorded event history against the current workflow code**. That
only works while the code stays deterministic: on replay, the commands the code
produces (schedule activity, start timer, complete workflow) must match, in
order, the events already recorded in a running workflow's history.

The moment a change makes the replayed command stream diverge from an in-flight
workflow's recorded history, that workflow **wedges with a non-determinism
error** — often thousands of long-running executions, discovered only after the
bad build is already in production. The triggers are mundane and easy to ship
by accident:

- reordering two activity or child-workflow calls
- adding or removing an activity/timer/signal on an existing path
- renaming an activity type or changing its input shape
- control-flow changes that gate a command (an `if` that now skips a timer)
- non-deterministic constructs (`time.Now()`, `rand`, map iteration, goroutines)

Static analysis (Temporal's `workflowcheck`) flags non-deterministic
*constructs*, but it can't catch code that is perfectly deterministic in
isolation yet **incompatible with histories already in flight** because the
logic changed. Detecting that requires replaying real histories. Replay Gate
does exactly that, in CI, and blocks the merge when it finds a divergence.

## How it works

```
 sample ──▶ corpus ──▶ replay ──▶ diff ──▶ suggest
 pull real   versioned  replay each  classify   map each
 histories   JSON on    history vs   the        divergence to
 from a      disk       the new      divergence  a GetVersion
 cluster                build                    fix
```

1. **Sample** a stratified set of real histories from a live cluster (by
   workflow type, mixing in-flight and recently-closed) into a versioned,
   human-inspectable JSON **corpus** on disk. Payloads are scrubbed before
   anything is written.
2. **Replay** every history in the corpus against your build. Replay is
   deterministic, so a failure is a genuine incompatibility, not a flake.
3. **Diff** each failure into a structured divergence: which history, which
   event, expected-vs-generated command, and a classification (reorder,
   removal, rename, added command, non-deterministic construct, …).
4. **Suggest** a `workflow.GetVersion` patch per divergence, rendered inline in
   the report / as a GitHub annotation.

The corpus is committed (or cached) so CI runs are reproducible and don't hit
the production cluster on every push; refreshing it is a separate scheduled job.

## Quick start (GitHub Action)

Add a small Go `main` package that registers your workflows and hands off to
`gate.Main` — see [examples/replaygate](examples/replaygate):

```go
func main() {
    g := gate.New(gate.Config{})
    g.RegisterWorkflowWithOptions(myapp.OrderWorkflow, workflow.RegisterOptions{Name: "OrderWorkflow"})
    os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
}
```

Commit a corpus, then add this to a workflow file:

```yaml
- uses: sayarakscodes/replay-gate/action@v1
  with:
    corpus-path: ./corpus
    registrations: ./cmd/replaygate
```

Divergences in in-flight (`RUNNING`) workflows fail the check; divergences only
in closed histories warn by default (set `fail-on: any` to block on those too).
Each divergence is posted as an inline annotation with a suggested `GetVersion`
fix, and a summary table is written to the job summary.

## CLI

The `replaygate` binary has three subcommands.

### `replaygate verify`

Check a corpus directory's integrity (manifest hashes, content-hashed corpus
version) without replaying anything:

```
replaygate verify --corpus ./corpus
```

### `replaygate replay`

Replay every history in a corpus against a build. `--registrations` points at
the small Go `main` package that registers your workflows and calls `gate.Main`
(see [examples/replaygate](examples/replaygate)):

```
replaygate replay --corpus ./corpus --registrations ./cmd/replaygate
```

Report formats (`--format`): `text` (default), `json` (stable
`reportVersion`-tagged schema), `github` (inline annotations plus a Markdown job
summary written to `$GITHUB_STEP_SUMMARY`), and `sarif` (for GitHub code
scanning).

Exit codes: `0` clean; `1` a blocking divergence (in a `RUNNING` workflow, or
any divergence under `--fail-on=any`); `2` a divergence found only in closed
histories under the default `--fail-on=open` (warns, doesn't block a merge);
`3` operational error (bad corpus, compile failure, or an unregistered workflow
type with the default `--on-unregistered=fail`).

### `replaygate sample`

Connect to a live Temporal cluster and write a stratified sample of workflow
histories as a corpus, in the same format `verify`/`replay` consume.

**Environment variables:**

| Variable | Purpose |
|---|---|
| `TEMPORAL_ADDRESS` | Cluster gRPC endpoint, e.g. `127.0.0.1:7233` (required) |
| `TEMPORAL_NAMESPACE` | Namespace to sample from (defaults to `default`) |
| `TEMPORAL_API_KEY` | API key auth (optional; enables TLS automatically) |
| `TEMPORAL_TLS_CERT` / `TEMPORAL_TLS_KEY` | mTLS client cert/key (optional, used if no API key) |
| `TEMPORAL_TLS_CA` | Additional CA bundle for TLS (optional) |
| `REPLAYGATE_REDACTION_KEY` | HMAC key, required only for `--redaction hash` |

```
export TEMPORAL_ADDRESS=127.0.0.1:7233
export TEMPORAL_NAMESPACE=default

replaygate sample --out ./corpus --cap 200 --closed-window 168h
```

Sampling is read-only and rate-limited (default 5 RPS for listing, 10 RPS for
history fetches), so it's safe to point at a production cluster. Flags
(`--cap`, `--max-events`, `--open-closed-split`, `--closed-window`,
`--visibility-rps`, `--history-rps`, `--type-scan-limit`, `--redaction`)
override the `sample:` section of a `--config replaygate.yaml` file when one is
given; otherwise sensible defaults apply.

#### Redaction

Sampled histories can contain PII in activity/workflow/signal payloads, so every
payload byte is scrubbed *before* it's written to disk — there is no code path
that persists an unredacted payload except the explicit `none` opt-out. Select a
profile with `--redaction` (default: `default`):

| Profile | Behavior |
|---|---|
| `default` | Blanks payload data (same length, zeroed) while keeping metadata like `encoding` — replay only needs command/event shape, never content |
| `hash` | Replaces payload data with an HMAC-SHA256 (keyed by `REPLAYGATE_REDACTION_KEY`), preserving whether two payloads are equal/different without exposing content |
| `none` | Passes payloads through unmodified. Prints a loud warning; only use this for a corpus you know carries no sensitive data |

The applied profile is recorded in the corpus manifest's `redaction.profile`, so
anyone inspecting a corpus can see what was (or wasn't) scrubbed. Redaction
applies to every payload-bearing field via a generic walk over the event tree,
not a hardcoded list, so new Temporal event types are covered automatically.

#### Local demo against a dev server

```
brew install temporal
temporal server start-dev --headless --port 7233 --ip 127.0.0.1 &

# run some workflows against it, then:
export TEMPORAL_ADDRESS=127.0.0.1:7233
export TEMPORAL_NAMESPACE=default
replaygate sample --out ./corpus --cap 20 --closed-window 24h
replaygate verify --corpus ./corpus
```

## Building

```
go build ./...
go test ./...
```

Released binaries (linux/amd64, linux/arm64, darwin/arm64) are published on tag
push and consumed by the GitHub Action.

## Repository layout

- `cmd/replaygate` — the CLI (`verify`, `replay`, `sample`).
- `pkg/gate` — the public embedding API (`gate.New`, `RegisterWorkflow`,
  `ReplayAll`, and the `gate.Main` entrypoint used by a registrations package).
- `internal/corpus` — the on-disk corpus format: manifest schema, loader,
  integrity verification, and a builder for writing new corpora.
- `internal/sampler` — live-cluster sampling and stratification.
- `internal/redact` — payload scrubbing profiles.
- `internal/replayer` — the replay engine.
- `internal/differ` — turns opaque replay errors into structured divergences.
- `internal/patcher` — `GetVersion` fix suggestions per divergence.
- `internal/report` — text / json / github / sarif rendering.
- `action/` — the composite GitHub Action.
- `examples/` — a minimal worker plus a corpus, wired end to end.
- `testdata/regressions/` — a battery of known regression classes with real
  recorded histories; see its own README for how the fixtures are generated.
- `tools/gen-regressions` — the generator behind those fixtures.
