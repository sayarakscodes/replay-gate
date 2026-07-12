package gate

import "github.com/sayarakscodes/replay-gate/internal/redact"

// Scrubber transforms a single payload before it's persisted to a corpus.
// Implement this to plug in custom redaction. Built-in profiles (none,
// default, hash) are constructed via internal/redact.NewScrubber and used by
// `replaygate sample`.
type Scrubber = redact.Scrubber
