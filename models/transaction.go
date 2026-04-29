package models

// Transaction represents a bank transaction (income, expense, or transfer).
type Transaction struct {
	ID                int       `json:"id"`
	AccountID         int       `json:"account_id"`
	Type              string    `json:"type"` // income, expense, transfer
	Amount            Money     `json:"amount"`
	TransactionDate   Date      `json:"transaction_date"`
	Description       *string   `json:"description"`
	Reference         *string   `json:"reference"`
	TransferAccountID *int      `json:"transfer_account_id"`
	ContactID         *int      `json:"contact_id"`
	CreatedAt         Timestamp `json:"created_at"`
	UpdatedAt         Timestamp `json:"updated_at"`
	// Computed fields
	AccountName         *string `json:"account_name,omitempty"`
	TransferAccountName *string `json:"transfer_account_name,omitempty"`
	ContactName         *string `json:"contact_name,omitempty"`
	Allocated           Money   `json:"allocated"`
	Unallocated         Money   `json:"unallocated"`
}

// TransactionInput is used for creating/updating transactions.
type TransactionInput struct {
	AccountID         int     `json:"account_id"`
	Type              string  `json:"type"`
	Amount            Money   `json:"amount"`
	TransactionDate   *string `json:"transaction_date"`
	Description       *string `json:"description"`
	Reference         *string `json:"reference"`
	TransferAccountID *int    `json:"transfer_account_id"`
	ContactID         *int    `json:"contact_id"`
}

func (t *TransactionInput) Validate() string {
	if t.AccountID <= 0 {
		return "account_id is required"
	}
	if t.Amount <= 0 {
		return "amount must be positive"
	}
	switch t.Type {
	case "income", "expense", "transfer":
	default:
		return "type must be one of: income, expense, transfer"
	}
	if t.Type == "transfer" && (t.TransferAccountID == nil || *t.TransferAccountID <= 0) {
		return "transfer_account_id is required for transfers"
	}
	if t.Type == "transfer" && t.TransferAccountID != nil && *t.TransferAccountID == t.AccountID {
		return "transfer_account_id must differ from account_id"
	}
	if err := NormalizeDate(t.TransactionDate); err != nil {
		return "transaction_date: " + err.Error()
	}
	return ""
}
