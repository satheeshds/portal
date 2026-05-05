package db

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestLenientDateCodec_TextFormat(t *testing.T) {
	codec := lenientDateCodec{}

	cases := []struct {
		name    string
		src     []byte
		wantY   int
		wantM   time.Month
		wantD   int
		wantErr bool
	}{
		{name: "YYYY-MM-DD", src: []byte("2026-04-27"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "timestamp suffix", src: []byte("2026-04-27 00:00:00"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "timestamp microseconds", src: []byte("2026-04-27 15:04:05.123456"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "RFC3339", src: []byte("2026-04-27T00:00:00Z"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "nil src", src: nil},
		{name: "garbage", src: []byte("not-a-date"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d pgtype.Date
			plan := codec.PlanScan(nil, pgtype.DateOID, pgtype.TextFormatCode, &d)
			if plan == nil {
				t.Fatal("PlanScan returned nil for *pgtype.Date target")
			}

			err := plan.Scan(tc.src, &d)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.src == nil {
				if d.Valid {
					t.Fatal("expected invalid (NULL) date for nil src")
				}
				return
			}
			if !d.Valid {
				t.Fatal("expected valid date, got invalid")
			}
			if d.Time.Year() != tc.wantY || d.Time.Month() != tc.wantM || d.Time.Day() != tc.wantD {
				t.Fatalf("got %v, want %d-%02d-%02d", d.Time, tc.wantY, tc.wantM, tc.wantD)
			}
		})
	}
}

func TestLenientDateCodec_BinaryFormat(t *testing.T) {
	codec := lenientDateCodec{}

	// Binary DATE: 4-byte big-endian int32, days since 2000-01-01.
	// 2026-04-27 = 9613 days after 2000-01-01.
	days := int32(9613)
	src := []byte{byte(days >> 24), byte(days >> 16), byte(days >> 8), byte(days)}

	var d pgtype.Date
	plan := codec.PlanScan(nil, pgtype.DateOID, pgtype.BinaryFormatCode, &d)
	if plan == nil {
		t.Fatal("PlanScan returned nil for binary format")
	}
	if err := plan.Scan(src, &d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Valid {
		t.Fatal("expected valid date")
	}
	if d.Time.Year() != 2026 || d.Time.Month() != 4 || d.Time.Day() != 27 {
		t.Fatalf("got %v, want 2026-04-27", d.Time)
	}
}

// TestLenientDateCodec_DecodeDatabaseSQLValue tests the code path exercised by
// database/sql when scanning a DATE column through a pgx stdlib connection.
// database/sql calls DecodeDatabaseSQLValue (not PlanScan), which must return a
// time.Time driver.Value that models.Date.Scan can then accept.
func TestLenientDateCodec_DecodeDatabaseSQLValue(t *testing.T) {
	codec := lenientDateCodec{}

	// Binary DATE encoding helper: 4-byte big-endian int32, days since 2000-01-01.
	binDate := func(days int32) []byte {
		return []byte{byte(days >> 24), byte(days >> 16), byte(days >> 8), byte(days)}
	}

	cases := []struct {
		name    string
		format  int16
		src     []byte
		wantNil bool
		wantY   int
		wantM   time.Month
		wantD   int
		wantErr bool
	}{
		{name: "nil (NULL)", src: nil, wantNil: true},
		{name: "YYYY-MM-DD", format: pgtype.TextFormatCode, src: []byte("2026-04-27"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "timestamp suffix (Nexus format)", format: pgtype.TextFormatCode, src: []byte("2026-04-27 00:00:00"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "timestamp microseconds", format: pgtype.TextFormatCode, src: []byte("2026-04-27 15:04:05.123456"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "RFC3339", format: pgtype.TextFormatCode, src: []byte("2026-04-27T00:00:00Z"), wantY: 2026, wantM: 4, wantD: 27},
		{name: "binary", format: pgtype.BinaryFormatCode, src: binDate(9613), wantY: 2026, wantM: 4, wantD: 27},
		{name: "garbage", format: pgtype.TextFormatCode, src: []byte("not-a-date"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := codec.DecodeDatabaseSQLValue(nil, pgtype.DateOID, tc.format, tc.src)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if val != nil {
					t.Fatalf("expected nil driver.Value, got %v", val)
				}
				return
			}
			got, ok := val.(time.Time)
			if !ok {
				t.Fatalf("expected driver.Value to be time.Time, got %T — database/sql cannot scan this into models.Date", val)
			}
			if got.Year() != tc.wantY || got.Month() != tc.wantM || got.Day() != tc.wantD {
				t.Fatalf("got %v, want %d-%02d-%02d", got, tc.wantY, tc.wantM, tc.wantD)
			}
		})
	}
}

// TestRegisterLenientDateCodec verifies that registerLenientDateCodec (called
// from stdlib.OptionAfterConnect in openDB) replaces the strict
// pgtype.DateCodec with lenientDateCodec on a connection's type map.  This
// catches any regression where the OptionAfterConnect wiring is changed so
// that the lenient codec is no longer installed on stdlib connections.
func TestRegisterLenientDateCodec(t *testing.T) {
	m := pgtype.NewMap()

	// Default type map should have the strict DateCodec for DATE OID.
	before, ok := m.TypeForOID(pgtype.DateOID)
	if !ok {
		t.Fatal("default pgtype.Map has no type registered for DateOID")
	}
	if _, isStrict := before.Codec.(pgtype.DateCodec); !isStrict {
		t.Fatalf("expected default codec to be pgtype.DateCodec, got %T", before.Codec)
	}

	// After registration the codec must be lenientDateCodec.
	registerLenientDateCodec(m)

	after, ok := m.TypeForOID(pgtype.DateOID)
	if !ok {
		t.Fatal("pgtype.Map has no type for DateOID after registerLenientDateCodec")
	}
	if _, isLenient := after.Codec.(lenientDateCodec); !isLenient {
		t.Fatalf("expected lenientDateCodec after registration, got %T", after.Codec)
	}
}
