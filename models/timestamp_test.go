package models

import (
	"encoding/json"
	"testing"
	"time"
)

func mustParseTime(layout, value string) time.Time {
	t, err := time.Parse(layout, value)
	if err != nil {
		panic(err)
	}
	return t
}

func TestTimestamp_Scan(t *testing.T) {
	fixedTime := mustParseTime(time.RFC3339, "2024-03-15T10:30:00Z")

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
			name:  "time.Time value",
			input: fixedTime,
			want:  fixedTime,
		},
		{
			name:  "[]byte RFC3339",
			input: []byte("2024-03-15T10:30:00Z"),
			want:  fixedTime,
		},
		{
			name:  "string RFC3339",
			input: "2024-03-15T10:30:00Z",
			want:  fixedTime,
		},
		{
			name:  "string empty yields zero time",
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
			var ts Timestamp
			err := ts.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !ts.Time.Equal(tt.want) {
				t.Errorf("Scan() = %v, want %v", ts.Time, tt.want)
			}
		})
	}
}

func TestTimestamp_parse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantUTC string // expected time formatted as RFC3339 in UTC; empty means zero time
		wantErr bool
	}{
		{
			name:    "empty string yields zero time",
			input:   "",
			wantUTC: "",
		},
		{
			name:    "RFC3339Nano",
			input:   "2024-03-15T10:30:00.123456789Z",
			wantUTC: "2024-03-15T10:30:00Z",
		},
		{
			name:    "RFC3339 UTC",
			input:   "2024-03-15T10:30:00Z",
			wantUTC: "2024-03-15T10:30:00Z",
		},
		{
			name:    "RFC3339 with timezone offset",
			input:   "2024-03-15T10:30:00+05:30",
			wantUTC: "2024-03-15T05:00:00Z",
		},
		{
			name:    "DuckDB space-separated with microseconds and offset",
			input:   "2024-03-15 10:30:00.123456-07",
			wantUTC: "2024-03-15T17:30:00Z",
		},
		{
			name:    "space-separated with microseconds no timezone",
			input:   "2024-03-15 10:30:00.123456",
			wantUTC: "2024-03-15T10:30:00Z",
		},
		{
			name:    "space-separated with microseconds and UTC suffix",
			input:   "2024-03-15 10:30:00.000000 +0000 UTC",
			wantUTC: "2024-03-15T10:30:00Z",
		},
		{
			name:    "space-separated datetime no sub-seconds",
			input:   "2024-03-15 10:30:00",
			wantUTC: "2024-03-15T10:30:00Z",
		},
		{
			name:    "date only",
			input:   "2024-03-15",
			wantUTC: "2024-03-15T00:00:00Z",
		},
		{
			name:    "invalid format",
			input:   "not-a-timestamp",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ts Timestamp
			err := ts.parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.wantUTC == "" {
				if !ts.Time.IsZero() {
					t.Errorf("parse(%q) = %v, want zero time", tt.input, ts.Time)
				}
				return
			}
			gotUTC := ts.Time.UTC().Format(time.RFC3339)
			if gotUTC != tt.wantUTC {
				t.Errorf("parse(%q) = %v, want %v", tt.input, gotUTC, tt.wantUTC)
			}
		})
	}
}

func TestTimestamp_Value(t *testing.T) {
	fixedTime := mustParseTime(time.RFC3339, "2024-03-15T10:30:00Z")

	t.Run("zero time returns nil", func(t *testing.T) {
		ts := Timestamp{}
		v, err := ts.Value()
		if err != nil {
			t.Fatalf("Value() error = %v", err)
		}
		if v != nil {
			t.Errorf("Value() = %v, want nil", v)
		}
	})

	t.Run("non-zero time returns time.Time", func(t *testing.T) {
		ts := Timestamp{Time: fixedTime}
		v, err := ts.Value()
		if err != nil {
			t.Fatalf("Value() error = %v", err)
		}
		got, ok := v.(time.Time)
		if !ok {
			t.Fatalf("Value() type = %T, want time.Time", v)
		}
		if !got.Equal(fixedTime) {
			t.Errorf("Value() = %v, want %v", got, fixedTime)
		}
	})
}

func TestTimestamp_MarshalJSON(t *testing.T) {
	fixedTime := mustParseTime(time.RFC3339, "2024-03-15T10:30:00Z")

	t.Run("zero time marshals to null", func(t *testing.T) {
		ts := Timestamp{}
		data, err := json.Marshal(ts)
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		if string(data) != "null" {
			t.Errorf("MarshalJSON() = %q, want %q", string(data), "null")
		}
	})

	t.Run("non-zero time marshals to RFC3339 JSON", func(t *testing.T) {
		ts := Timestamp{Time: fixedTime}
		data, err := json.Marshal(ts)
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		// Unmarshal back to verify round-trip
		var roundTrip Timestamp
		if err := json.Unmarshal(data, &roundTrip); err != nil {
			t.Fatalf("round-trip Unmarshal error = %v", err)
		}
		if !roundTrip.Time.Equal(fixedTime) {
			t.Errorf("MarshalJSON() round-trip = %v, want %v", roundTrip.Time, fixedTime)
		}
	})
}

func TestTimestamp_UnmarshalJSON(t *testing.T) {
	fixedTime := mustParseTime(time.RFC3339, "2024-03-15T10:30:00Z")

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "null yields zero time",
			input: "null",
			want:  time.Time{},
		},
		{
			name:  "valid RFC3339 string",
			input: `"2024-03-15T10:30:00Z"`,
			want:  fixedTime,
		},
		{
			name:    "invalid JSON string",
			input:   `"not-a-timestamp"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ts Timestamp
			err := json.Unmarshal([]byte(tt.input), &ts)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !ts.Time.Equal(tt.want) {
				t.Errorf("UnmarshalJSON(%q) = %v, want %v", tt.input, ts.Time, tt.want)
			}
		})
	}
}
