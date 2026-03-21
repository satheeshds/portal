package models

import (
	"testing"
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
