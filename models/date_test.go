package models

import (
	"encoding/json"
	"testing"
	"time"
)

func strPtr(s string) *string { return &s }

func TestNormalizeDate(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		want    *string // nil means the pointer itself should be nil
		wantErr bool
	}{
		{"nil pointer", nil, nil, false},
		{"empty string", strPtr(""), strPtr(""), false},
		{"already YYYY-MM-DD", strPtr("2026-02-26"), strPtr("2026-02-26"), false},
		{"DD-MM-YYYY", strPtr("26-02-2026"), strPtr("2026-02-26"), false},
		{"DD/MM/YYYY", strPtr("26/02/2026"), strPtr("2026-02-26"), false},
		{"DD-MM-YYYY with out-of-range month (reordered, DB will reject)", strPtr("02-26-2026"), strPtr("2026-26-02"), false},
		{"nonsense", strPtr("not-a-date"), nil, true},
		{"short string", strPtr("26-2-26"), nil, true},
		{"non-digit day component", strPtr("aa-02-2026"), nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NormalizeDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.input == nil {
				if tt.want != nil {
					t.Errorf("NormalizeDate() got nil, want %q", *tt.want)
				}
				return
			}
			if tt.want != nil && *tt.input != *tt.want {
				t.Errorf("NormalizeDate() = %q, want %q", *tt.input, *tt.want)
			}
		})
	}
}

func TestDate_Scan(t *testing.T) {
	fixedDate := time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC)
	fixedTimestamp := time.Date(2026, 4, 27, 1, 56, 2, 196451000, time.UTC)

	tests := []struct {
		name    string
		input   interface{}
		want    time.Time
		wantErr bool
	}{
		{
			name:  "nil value yields zero time",
			input: nil,
			want:  time.Time{},
		},
		{
			name:  "time.Time date value",
			input: fixedDate,
			want:  fixedDate,
		},
		{
			name:  "time.Time timestamp value (only date part)",
			input: fixedTimestamp,
			want:  fixedTimestamp,
		},
		{
			name:  "string YYYY-MM-DD",
			input: "2026-02-26",
			want:  fixedDate,
		},
		{
			name:  "[]byte YYYY-MM-DD",
			input: []byte("2026-02-26"),
			want:  fixedDate,
		},
		{
			name:  "empty string yields zero time",
			input: "",
			want:  time.Time{},
		},
		{
			name:    "unsupported type",
			input:   12345,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Date
			err := d.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !d.Time.Equal(tt.want) {
				t.Errorf("Scan() = %v, want %v", d.Time, tt.want)
			}
		})
	}
}

func TestDate_MarshalJSON(t *testing.T) {
	fixedDate := time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC)

	t.Run("zero time marshals to null", func(t *testing.T) {
		d := Date{}
		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		if string(data) != "null" {
			t.Errorf("MarshalJSON() = %q, want %q", string(data), "null")
		}
	})

	t.Run("non-zero date marshals to YYYY-MM-DD", func(t *testing.T) {
		d := Date{Time: fixedDate}
		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		if string(data) != `"2026-02-26"` {
			t.Errorf("MarshalJSON() = %q, want %q", string(data), `"2026-02-26"`)
		}
	})

	t.Run("timestamp marshals with only date component", func(t *testing.T) {
		// time.Time with time component: only the date part should appear in JSON
		ts := time.Date(2026, 4, 27, 1, 56, 2, 0, time.UTC)
		d := Date{Time: ts}
		data, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		if string(data) != `"2026-04-27"` {
			t.Errorf("MarshalJSON() = %q, want %q", string(data), `"2026-04-27"`)
		}
	})
}

func TestDate_UnmarshalJSON(t *testing.T) {
	fixedDate := time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{"null yields zero time", "null", time.Time{}, false},
		{"valid YYYY-MM-DD", `"2026-02-26"`, fixedDate, false},
		{"invalid format", `"not-a-date"`, time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Date
			err := json.Unmarshal([]byte(tt.input), &d)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !d.Time.Equal(tt.want) {
				t.Errorf("UnmarshalJSON(%q) = %v, want %v", tt.input, d.Time, tt.want)
			}
		})
	}
}

func TestDate_String(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  string
	}{
		{"zero time returns empty string", time.Time{}, ""},
		{"non-zero date returns YYYY-MM-DD", time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), "2026-02-26"},
		{"timestamp returns only date part", time.Date(2026, 4, 27, 1, 56, 2, 0, time.UTC), "2026-04-27"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Date{Time: tt.input}
			got := d.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDate_ScanTimeTime_FromGateway(t *testing.T) {
	// Simulate the Nexus gateway returning time.Time for a DATE column
	// (e.g. "2026-02-26 00:00:00 +0000 UTC")
	gatewayValue := time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC)

	var d Date
	if err := d.Scan(gatewayValue); err != nil {
		t.Fatalf("Scan(time.Time) error = %v", err)
	}

	got, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(got) != `"2026-02-26"` {
		t.Errorf("MarshalJSON after Scan(time.Time) = %q, want %q", string(got), `"2026-02-26"`)
	}
}
