package models

import (
	"encoding/json"
	"testing"
)

func TestMoney_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Money
		wantErr bool
	}{
		{"integer", "100", 10000, false},
		{"float", "12.34", 1234, false},
		{"string float", "\"12.34\"", 1234, false},
		{"string integer", "\"100\"", 10000, false},
		{"zero", "0", 0, false},
		{"negative float", "-1.50", -150, false},
		{"invalid string", "\"abc\"", 0, true},
		{"null", "null", 0, false},
		{"rupees with paise", "2122.90", 212290, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Money
			err := json.Unmarshal([]byte(tt.input), &m)
			if (err != nil) != tt.wantErr {
				t.Errorf("Money.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && m != tt.want {
				t.Errorf("Money.UnmarshalJSON() = %v, want %v", m, tt.want)
			}
		})
	}
}

func TestMoney_ToFloat(t *testing.T) {
	m := Money(1234)
	got := m.ToFloat()
	want := 12.34
	if got != want {
		t.Errorf("Money.ToFloat() = %v, want %v", got, want)
	}
}
