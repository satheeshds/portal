package db

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// lenientDateCodec is a pgtype.Codec for PostgreSQL DATE columns that accepts
// both standard YYYY-MM-DD text and timestamp-like strings (e.g. "2026-04-27
// 00:00:00") as returned by some Nexus gateway versions.  Only the date portion
// is preserved; the time component is discarded.
type lenientDateCodec struct{}

func (lenientDateCodec) FormatSupported(format int16) bool {
	return format == pgtype.TextFormatCode || format == pgtype.BinaryFormatCode
}

func (lenientDateCodec) PreferredFormat() int16 {
	return pgtype.TextFormatCode
}

func (lenientDateCodec) PlanEncode(m *pgtype.Map, oid uint32, format int16, value any) pgtype.EncodePlan {
	return pgtype.DateCodec{}.PlanEncode(m, oid, format, value)
}

func (lenientDateCodec) PlanScan(m *pgtype.Map, oid uint32, format int16, target any) pgtype.ScanPlan {
	if _, ok := target.(pgtype.DateScanner); ok {
		return lenientDateScanPlan{format: format}
	}
	return nil
}

func (lenientDateCodec) DecodeDatabaseSQLValue(m *pgtype.Map, oid uint32, format int16, src []byte) (driver.Value, error) {
	if src == nil {
		return nil, nil
	}
	t, err := decodeDateBytes(format, src)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (lenientDateCodec) DecodeValue(m *pgtype.Map, oid uint32, format int16, src []byte) (any, error) {
	if src == nil {
		return nil, nil
	}
	return decodeDateBytes(format, src)
}

// lenientDateScanPlan implements pgtype.ScanPlan for DateScanner targets.
type lenientDateScanPlan struct {
	format int16
}

func (p lenientDateScanPlan) Scan(src []byte, dst any) error {
	scanner := dst.(pgtype.DateScanner)
	if src == nil {
		return scanner.ScanDate(pgtype.Date{})
	}
	t, err := decodeDateBytes(p.format, src)
	if err != nil {
		return err
	}
	return scanner.ScanDate(pgtype.Date{Time: t, Valid: true})
}

// registerLenientDateCodec overrides the strict pgtype.DateCodec for DATE OID
// on the given type map with lenientDateCodec.  It is called from
// stdlib.OptionAfterConnect so every new pgx stdlib connection accepts the
// timestamp-like date strings returned by the Nexus gateway.
func registerLenientDateCodec(m *pgtype.Map) {
	m.RegisterType(&pgtype.Type{
		Name:  "date",
		OID:   pgtype.DateOID,
		Codec: lenientDateCodec{},
	})
}

// decodeDateBytes converts raw database bytes to time.Time for a DATE column.
// Binary format: 4-byte signed int32, days since 2000-01-01.
// Text format:   tries YYYY-MM-DD first, then common timestamp layouts.
func decodeDateBytes(format int16, src []byte) (time.Time, error) {
	if format == pgtype.BinaryFormatCode {
		if len(src) != 4 {
			return time.Time{}, fmt.Errorf("invalid length for date: %d", len(src))
		}
		dayOffset := int32(binary.BigEndian.Uint32(src))
		epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		return epoch.AddDate(0, 0, int(dayOffset)), nil
	}

	// Text format: try several layouts.
	s := string(src)
	if s == "" {
		return time.Time{}, nil
	}

	layouts := []string{
		"2006-01-02",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid date format: %q", s)
}
