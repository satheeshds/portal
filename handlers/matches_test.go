package handlers

import (
	"testing"
	"time"
)

func TestBuildMatchSearchText(t *testing.T) {
	str := func(s string) *string { return &s }

	tests := []struct {
		name string
		desc *string
		ref  *string
		want string
	}{
		{"both set", str("Payment for INV-001"), str("INV-001"), "payment for inv-001 inv-001"},
		{"desc only", str("Salary deposit"), nil, "salary deposit"},
		{"ref only", nil, str("UTR123456"), "utr123456"},
		{"both nil", nil, nil, ""},
		{"empty strings", str(""), str(""), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMatchSearchText(tt.desc, tt.ref)
			if got != tt.want {
				t.Errorf("buildMatchSearchText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchDateScore(t *testing.T) {
	parseDate := func(s string) time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return d
	}

	tests := []struct {
		name       string
		txnDate    time.Time
		docDate    string
		windowDays int
		wantScore  float64
		wantReason string
	}{
		{
			name:       "exact same day",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2024-01-15",
			windowDays: 30,
			wantScore:  0.4,
			wantReason: "exact_date_match",
		},
		{
			name:       "within window",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2024-01-10",
			windowDays: 30,
			wantScore:  0.25, // 0.3 * (1 - 5/30) ≈ 0.25
			wantReason: "date_proximity",
		},
		{
			name:       "outside window enters stale zone",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2023-12-01",
			windowDays: 30,
			wantScore:  0.09, // diff=45; 0.1 * (1 - (diff-windowDays)/staleRange) = 0.1 * (1 - 15/150) = 0.09
			wantReason: "stale_date_proximity",
		},
		{
			name:       "zero txn date returns no score",
			txnDate:    time.Time{},
			docDate:    "2024-01-15",
			windowDays: 30,
			wantScore:  0,
			wantReason: "",
		},
		{
			name:       "empty doc date returns no score",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "",
			windowDays: 30,
			wantScore:  0,
			wantReason: "",
		},
		{
			name:       "at window boundary enters stale zone",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2024-02-14",
			windowDays: 30,
			wantScore:  0.1, // 0.1 * (1 - 0/150) = 0.1
			wantReason: "stale_date_proximity",
		},
		{
			name:       "one day apart gives date_proximity, not exact_date_match",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2024-01-16",
			windowDays: 30,
			wantScore:  0.29, // 0.3 * (1 - 1/30) ≈ 0.29
			wantReason: "date_proximity",
		},
		{
			name:       "stale zone mid-point",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2023-10-07", // 100 days before
			windowDays: 30,
			wantScore:  0.05, // 0.1 * (1 - (diff-windowDays)/staleRange) = 0.1 * (1 - 70/150) ≈ 0.0533 → rounds to 0.05
			wantReason: "stale_date_proximity",
		},
		{
			name:       "beyond stale window returns no score",
			txnDate:    parseDate("2024-01-15"),
			docDate:    "2023-06-25", // 204 days before, beyond staleDateWindow=180
			windowDays: 30,
			wantScore:  0,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotReason := matchDateScore(tt.txnDate, tt.docDate, tt.windowDays)
			// Round to 2 decimal places for comparison
			rounded := float64(int(gotScore*100+0.5)) / 100
			if rounded != tt.wantScore {
				t.Errorf("matchDateScore() score = %v, want %v", gotScore, tt.wantScore)
			}
			if gotReason != tt.wantReason {
				t.Errorf("matchDateScore() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestMatchDateScoreTime(t *testing.T) {
	parseDate := func(s string) time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return d
	}

	tests := []struct {
		name       string
		txnDate    time.Time
		docDate    time.Time
		windowDays int
		wantScore  float64
		wantReason string
	}{
		{
			name:       "exact same day",
			txnDate:    parseDate("2024-01-15"),
			docDate:    parseDate("2024-01-15"),
			windowDays: 30,
			wantScore:  0.4,
			wantReason: "exact_date_match",
		},
		{
			name:       "within window",
			txnDate:    parseDate("2024-01-15"),
			docDate:    parseDate("2024-01-10"),
			windowDays: 30,
			wantScore:  0.25, // 0.3 * (1 - 5/30)
			wantReason: "date_proximity",
		},
		{
			name:       "in stale zone",
			txnDate:    parseDate("2024-01-15"),
			docDate:    parseDate("2023-12-01"), // 45 days
			windowDays: 30,
			wantScore:  0.09, // 0.1 * (1 - 15/150) = 0.1 * 0.9
			wantReason: "stale_date_proximity",
		},
		{
			name:       "beyond stale window",
			txnDate:    parseDate("2024-01-15"),
			docDate:    parseDate("2023-06-25"), // 204 days
			windowDays: 30,
			wantScore:  0,
			wantReason: "",
		},
		{
			name:       "zero txnDate returns no score",
			txnDate:    time.Time{},
			docDate:    parseDate("2024-01-15"),
			windowDays: 30,
			wantScore:  0,
			wantReason: "",
		},
		{
			name:       "zero docDate returns no score",
			txnDate:    parseDate("2024-01-15"),
			docDate:    time.Time{},
			windowDays: 30,
			wantScore:  0,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotReason := matchDateScoreTime(tt.txnDate, tt.docDate, tt.windowDays)
			rounded := float64(int(gotScore*100+0.5)) / 100
			if rounded != tt.wantScore {
				t.Errorf("matchDateScoreTime() score = %v, want %v", gotScore, tt.wantScore)
			}
			if gotReason != tt.wantReason {
				t.Errorf("matchDateScoreTime() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestMatchDescScore(t *testing.T) {
	tests := []struct {
		name          string
		txnSearchText string
		docRef        string
		docContext     string
		wantScore     float64
		wantReason    string
	}{
		{
			name:          "reference found in search text",
			txnSearchText: "payment for inv-001 from acme",
			docRef:        "INV-001",
			docContext:    "Acme Corp notes",
			wantScore:     0.2,
			wantReason:    "reference_match",
		},
		{
			name:          "reference not found but context token matches",
			txnSearchText: "payment from acme corp",
			docRef:        "INV-999",
			docContext:    "Acme Corp",
			wantScore:     0.1,
			wantReason:    "description_match",
		},
		{
			name:          "no match",
			txnSearchText: "salary deposit",
			docRef:        "INV-001",
			docContext:    "Widget vendor",
			wantScore:     0,
			wantReason:    "",
		},
		{
			name:          "empty search text",
			txnSearchText: "",
			docRef:        "INV-001",
			docContext:    "Some context",
			wantScore:     0,
			wantReason:    "",
		},
		{
			name:          "empty docRef and docContext",
			txnSearchText: "payment for inv-001",
			docRef:        "",
			docContext:    "",
			wantScore:     0,
			wantReason:    "",
		},
		{
			name:          "short context tokens ignored",
			txnSearchText: "the at is",
			docRef:        "",
			docContext:    "the at",
			wantScore:     0,
			wantReason:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotReason := matchDescScore(tt.txnSearchText, tt.docRef, tt.docContext)
			if gotScore != tt.wantScore {
				t.Errorf("matchDescScore() score = %v, want %v", gotScore, tt.wantScore)
			}
			if gotReason != tt.wantReason {
				t.Errorf("matchDescScore() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestDerefDate(t *testing.T) {
	str := func(s string) *string { return &s }

	tests := []struct {
		name  string
		dates []*string
		want  string
	}{
		{"first non-empty", []*string{str("2024-01-15"), str("2024-01-01")}, "2024-01-15"},
		{"skip nil, take second", []*string{nil, str("2024-01-01")}, "2024-01-01"},
		{"skip empty, take second", []*string{str(""), str("2024-01-01")}, "2024-01-01"},
		{"all nil", []*string{nil, nil}, ""},
		{"no args", []*string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derefDate(tt.dates...)
			if got != tt.want {
				t.Errorf("derefDate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildMatchSuggestionsRouting(t *testing.T) {
	// Verify that buildMatchSuggestions calls the correct suggesters based on txnType.
	// Since DB is nil in unit tests, we expect nil/empty slices rather than panics.
	// The purpose is to verify no panics occur when DB is nil and functions return nil.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("buildMatchSuggestions panicked: %v", r)
		}
	}()

	txnDate, _ := time.Parse("2006-01-02", "2024-01-15")

	// These will fail DB queries gracefully (nil DB) and should return empty slices.
	expenseSuggestions := buildMatchSuggestions("expense", 10000, txnDate, "test payment")
	if len(expenseSuggestions) != 0 {
		t.Errorf("buildMatchSuggestions(\"expense\") returned %d suggestions, want 0 in nil-DB case", len(expenseSuggestions))
	}

	incomeSuggestions := buildMatchSuggestions("income", 10000, txnDate, "test payment")
	if len(incomeSuggestions) != 0 {
		t.Errorf("buildMatchSuggestions(\"income\") returned %d suggestions, want 0 in nil-DB case", len(incomeSuggestions))
	}
}
