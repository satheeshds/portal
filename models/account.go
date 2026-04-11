package models

// Account represents a bank account, cash, or credit card.
type Account struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"` // bank, cash, credit_card
	OpeningBalance Money     `json:"opening_balance"`
	Balance        Money     `json:"balance"` // Computed
	CreatedAt      Timestamp `json:"created_at"`
	UpdatedAt      Timestamp `json:"updated_at"`
}

// AccountInput is used for creating/updating accounts.
type AccountInput struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	OpeningBalance Money  `json:"opening_balance"`
}

func (a *AccountInput) Validate() string {
	if a.Name == "" {
		return "name is required"
	}
	switch a.Type {
	case "bank", "cash", "credit_card":
	default:
		return "type must be one of: bank, cash, credit_card"
	}
	return ""
}
