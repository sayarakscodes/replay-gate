package report

import (
	"fmt"
	"io"
	"strings"
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
			if r.Divergence != nil {
				fmt.Fprintf(w, "      class: %s\n", r.Divergence.Class)
			}
			if r.Patch != nil {
				switch {
				case r.Patch.Snippet != "":
					fmt.Fprintf(w, "      suggested patch (changeID %q):\n", r.Patch.ChangeID)
					for _, line := range strings.Split(r.Patch.Snippet, "\n") {
						fmt.Fprintf(w, "        %s\n", line)
					}
				case r.Patch.Guidance != "":
					fmt.Fprintf(w, "      guidance: %s\n", r.Patch.Guidance)
				}
			}
		default:
			passed++
			fmt.Fprintf(w, "PASS  %s [%s] (%s)\n", r.Ref, r.Status, r.Duration)
		}
	}

	fmt.Fprintf(w, "\n%d total, %d passed, %d failed, %d skipped\n", len(rep.Results), passed, failed, skipped)
	return nil
}
