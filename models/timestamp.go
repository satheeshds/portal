package models

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// Timestamp is a wrapper around time.Time that handles scanning from database
// strings (e.g. from DuckDB/Nexus) and JSON marshaling.
type Timestamp struct {
	time.Time
}

// timestampFormats lists the timestamp string layouts accepted by Timestamp.parse,
// ordered from most to least specific. Defined at package level to avoid
// repeated allocations when scanning many rows.
var timestampFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999-07",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05.999999 +0000 UTC",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Scan implements the sql.Scanner interface.
func (t *Timestamp) Scan(value interface{}) error {
	if value == nil {
		t.Time = time.Time{}
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		t.Time = v
		return nil
	case []byte:
		return t.parse(string(v))
	case string:
		return t.parse(v)
	default:
		return fmt.Errorf("cannot scan type %T into Timestamp", v)
	}
}

func (t *Timestamp) parse(s string) error {
	if s == "" {
		t.Time = time.Time{}
		return nil
	}

	// Try the common standard formats we expect from DuckDB/Postgres.
	for _, f := range timestampFormats {
		parsed, err := time.Parse(f, s)
		if err == nil {
			t.Time = parsed
			return nil
		}
	}

	return fmt.Errorf("cannot parse %q as timestamp", s)
}

// Value implements the driver.Valuer interface.
func (t Timestamp) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Time, nil
}

// MarshalJSON implements json.Marshaler.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return t.Time.MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.Time = time.Time{}
		return nil
	}
	return t.Time.UnmarshalJSON(data)
}
