package gate

import (
	"flag"
	"fmt"
	"io"

	"github.com/sayarakscodes/replay-gate/internal/report"
)

// Main is the entrypoint a registrations package hands off to: call it from a
// small main package that has already registered its workflows on g.
//
//	func main() {
//	    g := gate.New(gate.Config{})
//	    g.RegisterWorkflow(myworkflow.Foo)
//	    os.Exit(gate.Main(g, os.Args[1:], os.Stdout, os.Stderr))
//	}
//
// It owns flag parsing, replay, report rendering, and the process exit code.
// `replaygate replay --registrations <dir>` builds and runs a package written
// this way. Requiring a user-authored main package (rather than generating one
// around a bare registrations file) keeps things simple: no import-path
// inference and no subprocess handoff, at the cost of a few lines of
// boilerplate the user commits once.
func Main(g *Gate, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("replaygate", flag.ContinueOnError)
	corpusDir := fs.String("corpus", "", "corpus directory (overrides the CorpusDir passed to gate.New)")
	parallelism := fs.Int("parallelism", 0, "number of concurrent replay workers (0 = GOMAXPROCS)")
	format := fs.String("format", report.FormatText, "report format: text|json|github|sarif")
	onUnregistered := fs.String("on-unregistered", OnUnregisteredFail, "behavior for unregistered workflow types: fail|skip-warn")
	failOn := fs.String("fail-on", FailOnOpen, "which divergences block: open (default, RUNNING workflows only) | any")
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return ExitOperationalError
	}
	if *failOn != FailOnOpen && *failOn != FailOnAny {
		fmt.Fprintf(stderr, "invalid --fail-on value %q (want %q or %q)\n", *failOn, FailOnOpen, FailOnAny)
		return ExitOperationalError
	}

	if *corpusDir != "" {
		g.cfg.CorpusDir = *corpusDir
	}

	rep, err := g.ReplayAll(ReplayAllOptions{
		Parallelism:    *parallelism,
		OnUnregistered: *onUnregistered,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitOperationalError
	}

	if err := report.Write(stdout, rep, *format, *failOn); err != nil {
		fmt.Fprintln(stderr, err)
		return ExitOperationalError
	}

	return rep.ExitCode(*failOn)
}
