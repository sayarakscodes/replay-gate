package differ

import "regexp"

// firstSubmatch returns re's first capture group applied to s, or "" if re
// doesn't match — used for best-effort extraction from the SDK's attribute
// blobs, where a missing field just means less detail, not an error.
func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
