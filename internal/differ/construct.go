package differ

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
)

// suspiciousConstructs are the non-deterministic builtins workflowcheck
// itself flags (PRD §1: time.Now(), rand, map-range, goroutines). This is
// deliberately a plain substring scan, not an AST pass — a cheap, best-effort
// signal to disambiguate ClassRename from ClassNondeterministicConstruct,
// not a replacement for workflowcheck's real static analysis.
var suspiciousConstructs = []string{"time.Now(", "rand.", "go func("}

// sourceScanMaxLines is a hard cap on how far past the function's declaration
// line we'll ever scan, in case closing-brace detection (below) fails to find
// one — e.g. unusual formatting. Go's runtime doesn't expose a function's end
// line directly, so the real boundary is inferred, not exact.
const sourceScanMaxLines = 500

// scanForNondeterministicConstruct reads the source file the registered
// workflow function was compiled from and looks for known non-deterministic
// constructs near its declaration. fn may be nil (e.g. in tests that only
// have a raw error to classify) — that's a normal, non-error case where the
// heuristic simply doesn't fire.
//
// This can both false-positive (the construct appears in the function for an
// unrelated reason) and false-negative (no debug info, vendored/generated
// code, or the construct sits past the scan window) — it's a real signal,
// not a certainty, and Divergence.Note says so explicitly when it fires.
func scanForNondeterministicConstruct(fn any) (note string, found bool) {
	if fn == nil {
		return "", false
	}
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		return "", false
	}

	rf := runtime.FuncForPC(v.Pointer())
	if rf == nil {
		return "", false
	}
	file, startLine := rf.FileLine(v.Pointer())
	if file == "" {
		return "", false
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}

	lines := strings.Split(string(data), "\n")
	if startLine < 1 || startLine > len(lines) {
		return "", false
	}

	// Find the function's closing brace: in gofmt'd code, a top-level func's
	// body always ends with an unindented "}". That's a much tighter (and
	// far more accurate) boundary than any fixed line count, especially when
	// several functions sit close together in one file.
	endLine := startLine
	maxLine := startLine + sourceScanMaxLines
	if maxLine > len(lines) {
		maxLine = len(lines)
	}
	for i := startLine; i < maxLine; i++ {
		if lines[i] == "}" {
			endLine = i + 1
			break
		}
		endLine = i + 1
	}
	window := strings.Join(lines[startLine-1:endLine], "\n")

	for _, construct := range suspiciousConstructs {
		if strings.Contains(window, construct) {
			return fmt.Sprintf("workflow source %s (near line %d) contains %q — possible non-deterministic construct (best-effort; not a certainty)", file, startLine, construct), true
		}
	}
	return "", false
}
