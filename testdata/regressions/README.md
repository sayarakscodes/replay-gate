# Regression fixture battery

Six known non-determinism regression classes, each proven against a **real**
recorded workflow history — not hand-crafted JSON. This backs the two core
guarantees: every known class is caught, and unchanged code replays with zero
false positives.

## Layout

Each class is a self-contained directory:

```
<class>/
  corpus/           # single-entry corpus (internal/corpus format), real recorded history
  before/main.go     # unmodified workflow — a registrations package calling pkg/gate.Main
  after/main.go      # the regressed workflow — same shape, same workflow type name
```

`before/` must replay clean against `corpus/`; `after/` must diverge. Both are
ordinary registrations packages, driven the same way any real user's package
is:

```
replaygate replay --corpus testdata/regressions/removed-activity/corpus \
                   --registrations testdata/regressions/removed-activity/before
```

`cmd/replaygate/regressions_test.go` runs this for all 6 classes in both
directions; `cmd/replaygate/falsepositive_test.go` additionally asserts the
JSON report's failure count is exactly 0 for every `before/` package.

## The six classes

| Class | What changed | Why it breaks in-flight workflows |
|---|---|---|
| `reorder-activity` | Two activity calls swapped | Command order no longer matches the recorded event order |
| `removed-activity` | An activity call dropped | A recorded `ActivityTaskScheduled` has no matching command |
| `changed-timer` | A timer removed from an existing path | A recorded timer command has nothing to match |
| `renamed-activity` | Activity type renamed at the call site | Recorded event references the old type name |
| `added-command` | An extra activity call appended | Generates a command with no corresponding recorded event |
| `time-now-regression` | Branches on `time.Now()` instead of `workflow.Now(ctx)` | Non-deterministic construct — see below |

`time-now-regression`'s `after/` branches on a **hardcoded past cutoff**
(`2020-01-01`), not real-time timing, so the divergence is 100% reproducible
in CI regardless of when the test runs — it doesn't depend on wall-clock
timing at test time. This mirrors the real bug pattern: a date-gated code
path that quietly flips for every already-in-flight workflow once the cutoff
passes, not a coin-flip race between record and replay.

## Regenerating fixtures / adding a new class

Histories are recorded by actually running the `before` workflow against a
live worker, then fetching the resulting history — the same code path the
sampler uses, not a replay-only shortcut.

1. Start a local dev server: `brew install temporal && temporal server start-dev --headless`
2. Add the new class to `classes` in `tools/gen-regressions/main.go` (a `before*`
   workflow function + task queue) and any new activities it needs.
3. `go run ./tools/gen-regressions` — writes `testdata/regressions/<class>/corpus/`.
4. Write `<class>/before/main.go` (same workflow logic as the generator, registered
   under the same workflow type name) and `<class>/after/main.go` (the regression).
5. Add the class name to `regressionClasses` in `cmd/replaygate/regressions_test.go`.

The generator is a real tool, not a one-off script — re-run it any time the
`before` workflow definitions change.
