package redact

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"

	commonpb "go.temporal.io/api/common/v1"
)

const (
	ProfileNone    = "none"
	ProfileDefault = "default"
	ProfileHash    = "hash"
)

// NoneScrubber passes payloads through unmodified. Selecting it means
// unredacted payload content (which can contain PII) gets written to the
// corpus — callers must warn loudly when this is chosen (TRD §5.3, N4).
type NoneScrubber struct{}

func (NoneScrubber) Scrub(p *commonpb.Payload) *commonpb.Payload { return p }

// DefaultScrubber blanks payload data while preserving metadata (encoding)
// and size (Data's length is kept, its content zeroed) — replay compatibility
// checks only need command/event shape, never payload content.
type DefaultScrubber struct{}

func (DefaultScrubber) Scrub(p *commonpb.Payload) *commonpb.Payload {
	if p == nil {
		return nil
	}
	return &commonpb.Payload{
		Metadata: p.GetMetadata(),
		Data:     make([]byte, len(p.GetData())),
	}
}

// HashScrubber replaces payload data with an HMAC-SHA256 of its bytes,
// preserving equality/inequality between payloads — so an input-shape-change
// regression is still detectable across a corpus — without exposing content.
type HashScrubber struct {
	Key []byte
}

func (h HashScrubber) Scrub(p *commonpb.Payload) *commonpb.Payload {
	if p == nil {
		return nil
	}
	mac := hmac.New(sha256.New, h.Key)
	mac.Write(p.GetData())
	return &commonpb.Payload{
		Metadata: p.GetMetadata(),
		Data:     mac.Sum(nil),
	}
}

// NewScrubber builds the scrubber for a named profile. key is only required
// (and only used) for ProfileHash.
func NewScrubber(profile string, key []byte) (Scrubber, error) {
	switch profile {
	case "", ProfileDefault:
		return DefaultScrubber{}, nil
	case ProfileNone:
		return NoneScrubber{}, nil
	case ProfileHash:
		if len(key) == 0 {
			return nil, fmt.Errorf("redaction profile %q requires a key (set REPLAYGATE_REDACTION_KEY)", ProfileHash)
		}
		return HashScrubber{Key: key}, nil
	default:
		return nil, fmt.Errorf("unknown redaction profile %q (want %q, %q, or %q)", profile, ProfileNone, ProfileDefault, ProfileHash)
	}
}

// FieldsScrubbed describes what a profile touches, for the corpus manifest's
// redaction.fieldsScrubbed (TRD §5.2).
func FieldsScrubbed(profile string) []string {
	if profile == ProfileNone {
		return nil
	}
	return []string{"payloads"}
}
