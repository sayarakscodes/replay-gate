package patcher

import "fmt"

// kebab converts a PascalCase/camelCase identifier to kebab-case, e.g.
// "NotifyLegacy" -> "notify-legacy", "SendNotificationV2" -> "send-notification-v2".
// Matches the changeID format illustrates.
func kebab(s string) string {
	var out []rune
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out = append(out, '-')
			}
			out = append(out, r-'A'+'a')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// identifier returns a changeID-safe fragment for name, falling back to the
// event ID (and finally a fixed "unknown") when name wasn't recoverable —
// Divergence fields are best-effort, so the patcher must degrade gracefully
// rather than emit an empty or malformed changeID.
func identifier(name string, eventID int64) string {
	if name != "" {
		return kebab(name)
	}
	if eventID != 0 {
		return fmt.Sprintf("event-%d", eventID)
	}
	return "unknown"
}

// quotedOr renders name as a Go string literal, or fallback (typically a
// comment placeholder) when name is empty.
func quotedOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return fmt.Sprintf("%q", name)
}
