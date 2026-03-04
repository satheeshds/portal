package models

import "time"

// RecurringPayment represents a scheduled recurring payment (income or expense).
type RecurringPayment struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	Type              string    `json:"type"` // income, expense
	Amount            Money     `json:"amount"`
	AccountID         int       `json:"account_id"`
	ContactID         *int      `json:"contact_id"`
	Frequency         string    `json:"frequency"` // daily, weekly, monthly, quarterly, yearly
	Interval          int       `json:"interval"`  // every N frequencies (e.g. 2 = every 2 months)
	StartDate         string    `json:"start_date"`
	EndDate           *string   `json:"end_date"`
	NextDueDate       *string   `json:"next_due_date"`
	LastGeneratedDate *string   `json:"last_generated_date"`
	Status            string    `json:"status"` // active, paused, cancelled, completed
	Description       *string   `json:"description"`
	Reference         *string   `json:"reference"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	// Computed fields
	AccountName *string `json:"account_name,omitempty"`
	ContactName *string `json:"contact_name,omitempty"`
}

// RecurringPaymentInput is used for creating/updating recurring payments.
type RecurringPaymentInput struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Amount            Money   `json:"amount"`
	AccountID         int     `json:"account_id"`
	ContactID         *int    `json:"contact_id"`
	Frequency         string  `json:"frequency"`
	Interval          int     `json:"interval"`
	StartDate         string  `json:"start_date"`
	EndDate           *string `json:"end_date"`
	NextDueDate       *string `json:"next_due_date"`
	LastGeneratedDate *string `json:"last_generated_date"`
	Status            string  `json:"status"`
	Description       *string `json:"description"`
	Reference         *string `json:"reference"`
}

func (r *RecurringPaymentInput) Validate() string {
	if r.Name == "" {
		return "name is required"
	}
	switch r.Type {
	case "income", "expense":
	default:
		return "type must be one of: income, expense"
	}
	if r.Amount <= 0 {
		return "amount must be positive"
	}
	if r.AccountID <= 0 {
		return "account_id is required"
	}
	switch r.Frequency {
	case "daily", "weekly", "monthly", "quarterly", "yearly":
	default:
		return "frequency must be one of: daily, weekly, monthly, quarterly, yearly"
	}
	if r.Interval <= 0 {
		return "interval must be greater than 0"
	}
	if r.StartDate == "" {
		return "start_date is required"
	}
	switch r.Status {
	case "", "active", "paused", "cancelled", "completed":
	default:
		return "status must be one of: active, paused, cancelled, completed"
	}
	if r.Status == "" {
		r.Status = "active"
	}
	return ""
}
