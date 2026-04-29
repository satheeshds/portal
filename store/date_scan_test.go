package store

import (
	"testing"
	"time"
)

func TestNullableDateScan(t *testing.T) {
	t.Run("nil value", func(t *testing.T) {
		var d nullableDate
		if err := d.Scan(nil); err != nil {
			t.Fatalf("scan nil: %v", err)
		}
		if d.Value != nil {
			t.Fatalf("expected nil value, got %v", *d.Value)
		}
	})

	t.Run("time.Time value", func(t *testing.T) {
		var d nullableDate
		if err := d.Scan(time.Date(2026, 4, 27, 15, 4, 5, 0, time.UTC)); err != nil {
			t.Fatalf("scan time: %v", err)
		}
		if d.Value == nil || *d.Value != "2026-04-27" {
			t.Fatalf("expected 2026-04-27, got %v", d.Value)
		}
	})

	t.Run("timestamp string value", func(t *testing.T) {
		var d nullableDate
		if err := d.Scan("2026-04-27T15:04:05Z"); err != nil {
			t.Fatalf("scan string: %v", err)
		}
		if d.Value == nil || *d.Value != "2026-04-27" {
			t.Fatalf("expected 2026-04-27, got %v", d.Value)
		}
	})
}

