package models

import (
	"fmt"
	"strings"
)

// NormalizeDate converts a date string from DD-MM-YYYY or DD/MM/YYYY to YYYY-MM-DD.
// Strings already in YYYY-MM-DD format are returned unchanged.
// A nil pointer is returned unchanged. An error is returned for unrecognised formats.
func NormalizeDate(s *string) error {
	if s == nil || *s == "" {
		return nil
	}

	v := *s

	// Already YYYY-MM-DD
	if isYYYYMMDD(v, '-') {
		return nil
	}

	// DD-MM-YYYY
	if len(v) == 10 && v[2] == '-' && v[5] == '-' {
		normalized, err := reorderDMY(v, '-')
		if err != nil {
			return fmt.Errorf("invalid date %q: %w", v, err)
		}
		*s = normalized
		return nil
	}

	// DD/MM/YYYY
	if len(v) == 10 && v[2] == '/' && v[5] == '/' {
		normalized, err := reorderDMY(v, '/')
		if err != nil {
			return fmt.Errorf("invalid date %q: %w", v, err)
		}
		*s = normalized
		return nil
	}

	return fmt.Errorf("invalid date format %q, expected YYYY-MM-DD, DD-MM-YYYY, or DD/MM/YYYY", v)
}

// isYYYYMMDD checks whether v looks like YYYY-MM-DD (all digits except separators).
func isYYYYMMDD(v string, sep byte) bool {
	if len(v) != 10 {
		return false
	}
	if v[4] != sep || v[7] != sep {
		return false
	}
	for i, c := range []byte(v) {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// reorderDMY rearranges "DD<sep>MM<sep>YYYY" → "YYYY-MM-DD".
func reorderDMY(v string, sep byte) (string, error) {
	parts := strings.SplitN(v, string(sep), 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("unexpected format")
	}
	dd, mm, yyyy := parts[0], parts[1], parts[2]
	if len(dd) != 2 || len(mm) != 2 || len(yyyy) != 4 {
		return "", fmt.Errorf("unexpected format")
	}
	return yyyy + "-" + mm + "-" + dd, nil
}
