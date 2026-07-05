package report

import (
	"fmt"
	"io"
)

func writeText(w io.Writer, rep *Report) error {
	fmt.Fprintf(w, "corpus: %s (version %s)\n", rep.CorpusDir, rep.CorpusVersion)

	var passed, failed, skipped int
	for _, r := range rep.Results {
		switch {
		case r.Skipped:
			skipped++
			fmt.Fprintf(w, "SKIP  %s [%s] (unregistered workflow type)\n", r.Ref, r.Status)
		case r.Err != nil:
			failed++
			fmt.Fprintf(w, "FAIL  %s [%s]: %v\n", r.Ref, r.Status, r.Err)
		default:
			passed++
			fmt.Fprintf(w, "PASS  %s [%s] (%s)\n", r.Ref, r.Status, r.Duration)
		}
	}

	fmt.Fprintf(w, "\n%d total, %d passed, %d failed, %d skipped\n", len(rep.Results), passed, failed, skipped)
	return nil
}
