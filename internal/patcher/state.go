package patcher

import (
	"fmt"
	"strings"

	"github.com/sayarakscodes/replay-gate/internal/differ"
)

// Patcher tracks changeIDs already used within one report, suffixing on
// collision. Package
// Suggest is deterministic per divergence, which is right in isolation, but
// two divergences with the same class and name in the same report — e.g. the
// same activity renamed the same way in two different corpus entries — would
// otherwise collide; Patcher is what a report-level caller should use.
type Patcher struct {
	seen map[string]int
}

func New() *Patcher {
	return &Patcher{seen: make(map[string]int)}
}

// Suggest behaves like the package-level Suggest, but suffixes ChangeID
// (and the matching literal inside Snippet) if this Patcher has already
// produced that changeID before.
func (p *Patcher) Suggest(d differ.Divergence) (Patch, bool) {
	patch, ok := Suggest(d)
	if !ok || patch.ChangeID == "" {
		return patch, ok
	}

	base := patch.ChangeID
	count := p.seen[base]
	p.seen[base] = count + 1
	if count == 0 {
		return patch, ok
	}

	suffixed := fmt.Sprintf("%s-%d", base, count+1)
	patch.Snippet = strings.Replace(patch.Snippet, fmt.Sprintf("%q", base), fmt.Sprintf("%q", suffixed), 1)
	patch.ChangeID = suffixed
	return patch, ok
}
