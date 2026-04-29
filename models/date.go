package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Date is a nullable date value that can be scanned from a database time.Time (as returned
// by pgx for DATE/TIMESTAMP columns) or from a string in YYYY-MM-DD format.
// A zero Time represents a NULL or absent date.
type Date struct {
	time.Time
}

// Scan implements the sql.Scanner interface, accepting time.Time, string, []byte, or nil.
func (d *Date) Scan(value interface{}) error {
	if value == nil {
		d.Time = time.Time{}
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		d.Time = v
		return nil
	case []byte:
		return d.parseDate(string(v))
	case string:
		return d.parseDate(v)
	default:
		return fmt.Errorf("cannot scan type %T into Date", v)
	}
}

func (d *Date) parseDate(s string) error {
	if s == "" {
		d.Time = time.Time{}
		return nil
	}
	// Try YYYY-MM-DD (canonical format)
	t, err := time.Parse("2006-01-02", s)
	if err == nil {
		d.Time = t
		return nil
	}
	// Try full timestamp formats (e.g. "2006-01-02 15:04:05 +0000 UTC")
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999 +0000 UTC",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			d.Time = t
			return nil
		}
	}
	return fmt.Errorf("cannot parse %q as date", s)
}

// Value implements the driver.Valuer interface.
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.Time.Format("2006-01-02"), nil
}

// MarshalJSON marshals the date as a JSON string "YYYY-MM-DD" or null for zero/absent dates.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Time.Format("2006-01-02") + `"`), nil
}

// UnmarshalJSON accepts a "YYYY-MM-DD" JSON string or null.
func (d *Date) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" {
		d.Time = time.Time{}
		return nil
	}
	s = strings.Trim(s, `"`)
	return d.parseDate(s)
}

// String returns the date formatted as "YYYY-MM-DD", or "" for a zero/absent date.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

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
	// Ensure all components are numeric to avoid propagating invalid dates downstream.
	if !isAllDigits(dd) || !isAllDigits(mm) || !isAllDigits(yyyy) {
		return "", fmt.Errorf("unexpected format")
	}
	return yyyy + "-" + mm + "-" + dd, nil
}

// isAllDigits reports whether s consists only of ASCII digits 0–9.
func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
