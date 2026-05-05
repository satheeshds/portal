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
