package util

import "strings"

// TruncateForLog shortens the provided string to the specified limit, appending an ellipsis when truncated.
func TruncateForLog(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}
