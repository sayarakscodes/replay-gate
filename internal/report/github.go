package report

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// writeGitHub emits GitHub Actions workflow commands (annotations) to w, one
// per divergence, and — when GITHUB_STEP_SUMMARY names a file — appends a
// Markdown job summary to it. A divergence that blocks the build
// under failOn is an ::error annotation; a warn-only one is ::warning, so the
// annotation color matches the exit code.
func writeGitHub(w io.Writer, rep *Report, failOn string) error {
	for _, r := range rep.Divergences() {
		level := "warning"
		if blocks(r.Status, failOn) {
			level = "error"
		}
		title := fmt.Sprintf("Replay divergence (%s)", divergenceClass(r))
		msg := oneLine(fmt.Sprintf("%s [%s]: %s", r.Ref, r.Status, annotationBody(r)))
		fmt.Fprintf(w, "::%s title=%s::%s\n", level, escapeProp(title), escapeData(msg))
	}

	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("opening GITHUB_STEP_SUMMARY: %w", err)
		}
		defer f.Close()
		writeMarkdownSummary(f, rep)
	}
	return nil
}

func divergenceClass(r EntryResult) string {
	if r.Divergence != nil {
		return string(r.Divergence.Class)
	}
	return "unclassified"
}

// annotationBody is a compact one-line description for an inline annotation —
// the class and the changeID of the suggested fix, if any. The full snippet
// goes in the job summary, not the annotation (annotations are single-line).
func annotationBody(r EntryResult) string {
	var b strings.Builder
	if r.Divergence != nil && r.Divergence.Expected != nil && r.Divergence.Expected.Name != "" {
		fmt.Fprintf(&b, "expected %q; ", r.Divergence.Expected.Name)
	}
	if r.Patch != nil && r.Patch.ChangeID != "" {
		fmt.Fprintf(&b, "suggested GetVersion changeID %q. ", r.Patch.ChangeID)
	}
	if r.Err != nil {
		b.WriteString(r.Err.Error())
	}
	return b.String()
}

func writeMarkdownSummary(w io.Writer, rep *Report) {
	var passed, failed, skipped int
	for _, r := range rep.Results {
		switch {
		case r.Skipped:
			skipped++
		case r.Err != nil:
			failed++
		default:
			passed++
		}
	}

	fmt.Fprintf(w, "## Replay Gate\n\n")
	fmt.Fprintf(w, "Corpus `%s` (version `%s`)\n\n", rep.CorpusDir, rep.CorpusVersion)
	fmt.Fprintf(w, "**%d passed, %d failed, %d skipped** out of %d histories.\n\n", passed, failed, skipped, len(rep.Results))

	divs := rep.Divergences()
	if len(divs) == 0 {
		fmt.Fprintf(w, "No divergences. ✅\n")
		return
	}

	fmt.Fprintf(w, "### Divergences\n\n")
	fmt.Fprintf(w, "| Workflow | Status | Class | Suggested changeID |\n")
	fmt.Fprintf(w, "|---|---|---|---|\n")
	for _, r := range divs {
		changeID := ""
		if r.Patch != nil {
			changeID = r.Patch.ChangeID
		}
		fmt.Fprintf(w, "| `%s` | %s | %s | %s |\n", r.Ref, r.Status, divergenceClass(r), mdCode(changeID))
	}
	fmt.Fprintln(w)

	for _, r := range divs {
		if r.Patch == nil {
			continue
		}
		fmt.Fprintf(w, "<details><summary><code>%s</code> — %s</summary>\n\n", r.Ref, divergenceClass(r))
		switch {
		case r.Patch.Snippet != "":
			fmt.Fprintf(w, "Suggested patch (changeID `%s`):\n\n```go\n%s\n```\n\n", r.Patch.ChangeID, r.Patch.Snippet)
		case r.Patch.Guidance != "":
			fmt.Fprintf(w, "%s\n\n", r.Patch.Guidance)
		}
		fmt.Fprintf(w, "</details>\n\n")
	}
}

func mdCode(s string) string {
	if s == "" {
		return ""
	}
	return "`" + s + "`"
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " ")
}

// escapeData / escapeProp escape the special characters GitHub's workflow-command
// parser reserves, per the toolkit's command spec.
func escapeData(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	return r.Replace(s)
}

func escapeProp(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return r.Replace(s)
}
