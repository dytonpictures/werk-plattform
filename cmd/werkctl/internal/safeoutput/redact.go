// Package safeoutput removes configured secrets from command-line diagnostics.
package safeoutput

import (
	"sort"
	"strings"
)

// Redact replaces configured database URLs and named secret values in a
// diagnostic message. Database URLs are removed as a whole because userinfo,
// query parameters and connection options can all contain credentials.
// It does not mutate the configuration map.
func Redact(message string, values map[string]string) string {
	// URLs must be removed before shorter named secrets. Otherwise replacing an
	// overlapping password first could prevent the complete URL match.
	databaseURLs := make([]string, 0)
	for key, value := range values {
		if value == "" || !strings.HasSuffix(strings.ToUpper(key), "DATABASE_URL") {
			continue
		}
		databaseURLs = append(databaseURLs, value)
	}
	sortByLengthDescending(databaseURLs)
	for _, value := range databaseURLs {
		message = strings.ReplaceAll(message, value, "[redacted-database-url]")
	}

	secrets := make([]string, 0)
	for key, value := range values {
		upperKey := strings.ToUpper(key)
		if value == "" {
			continue
		}
		if strings.Contains(upperKey, "PASSWORD") || strings.Contains(upperKey, "SECRET") ||
			strings.Contains(upperKey, "_KEY") {
			secrets = append(secrets, value)
		}
	}
	sortByLengthDescending(secrets)
	for _, value := range secrets {
		message = strings.ReplaceAll(message, value, "[redacted]")
	}
	return message
}

func sortByLengthDescending(values []string) {
	sort.SliceStable(values, func(left, right int) bool {
		return len(values[left]) > len(values[right])
	})
}
