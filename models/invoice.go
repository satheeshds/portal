package models

import (
	"fmt"
)

// Invoice represents a receivable invoice to a customer.
type Invoice struct {
	ID            int       `json:"id"`
	ContactID     *int      `json:"contact_id"`
	InvoiceNumber string    `json:"invoice_number"`
	IssueDate     Date      `json:"issue_date"`
	DueDate       Date      `json:"due_date"`
	Amount        Money     `json:"amount"`
	Status        string    `json:"status"`
	FileURL       *string   `json:"file_url"`
	Notes         *string   `json:"notes"`
	CreatedAt     Timestamp `json:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at"`
	// Computed fields
	ContactName *string       `json:"contact_name,omitempty"`
	Allocated   Money         `json:"allocated"`
	Unallocated Money         `json:"unallocated"`
	Items       []InvoiceItem `json:"items"`
}

// InvoiceInput is used for creating/updating invoices.
type InvoiceInput struct {
	ContactID     *int               `json:"contact_id"`
	InvoiceNumber string             `json:"invoice_number"`
	IssueDate     *string            `json:"issue_date"`
	DueDate       *string            `json:"due_date"`
	Amount        Money              `json:"amount"`
	Status        string             `json:"status"`
	FileURL       *string            `json:"file_url"`
	Notes         *string            `json:"notes"`
	Items         []InvoiceItemInput `json:"items"`
}

func (i *InvoiceInput) Validate() string {
	if i.Amount < 0 {
		return "amount must be non-negative"
	}
	switch i.Status {
	case "", "draft", "partial", "sent", "paid", "received", "overdue", "cancelled":
	default:
		return "status must be one of: draft, partial, sent, paid, received, overdue, cancelled"
	}
	if i.Status == "" {
		i.Status = "draft"
	}
	if err := NormalizeDate(i.IssueDate); err != nil {
		return "issue_date: " + err.Error()
	}
	if err := NormalizeDate(i.DueDate); err != nil {
		return "due_date: " + err.Error()
	}
	for idx := range i.Items {
		if msg := i.Items[idx].Validate(); msg != "" {
			return fmt.Sprintf("items[%d]: %s", idx, msg)
		}
	}
	return ""
}

// InvoiceItem represents a line item within an invoice.
type InvoiceItem struct {
	ID          int       `json:"id"`
	InvoiceID   int       `json:"invoice_id"`
	Description string    `json:"description"`
	Quantity    float64   `json:"quantity"`
	Unit        *string   `json:"unit"`
	UnitPrice   Money     `json:"unit_price"`
	Amount      Money     `json:"amount"`
	CreatedAt   Timestamp `json:"created_at"`
	UpdatedAt   Timestamp `json:"updated_at"`
}

// InvoiceItemInput is used for creating/updating invoice line items.
type InvoiceItemInput struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Unit        *string `json:"unit"`
	UnitPrice   Money   `json:"unit_price"`
	Amount      Money   `json:"amount"`
}

func (i *InvoiceItemInput) Validate() string {
	if i.Description == "" {
		return "description is required"
	}
	if i.Quantity <= 0 {
		return "quantity must be positive"
	}
	if i.UnitPrice < 0 {
		return "unit_price must be non-negative"
	}
	if i.Amount <= 0 {
		return "amount must be positive"
	}
	return ""
}
