package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Migrate runs all DDL statements through the Nexus control endpoint so they
// apply to all tenants. The control base URL is read from NEXUS_CONTROL_URL
// (default: http://nexus-control:8080) and the admin key from ADMIN_API_KEY.
func Migrate() error {
	controlURL := os.Getenv("NEXUS_CONTROL_URL")
	if controlURL == "" {
		controlURL = "http://nexus-control:8080"
	}
	adminKey := os.Getenv("ADMIN_API_KEY")
	if adminKey == "" {
		slog.Warn("ADMIN_API_KEY is not set; requests to nexus-control will be unauthenticated")
	}
	endpoint := controlURL + "/api/v1/admin/query"

	slog.Info("running database migrations", "endpoint", endpoint)

	client := &http.Client{Timeout: 30 * time.Second}
	for _, stmt := range migrations {
		if err := execAdminQuery(client, endpoint, adminKey, stmt); err != nil {
			return fmt.Errorf("migration failed: %w\nstatement: %s", err, stmt)
		}
	}

	slog.Info("database migrations complete")
	return nil
}

// execAdminQuery posts a single SQL statement to the Nexus control admin endpoint.
func execAdminQuery(client *http.Client, endpoint, adminKey, query string) error {
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if adminKey != "" {
		req.Header.Set("X-Admin-API-Key", adminKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin query returned status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// MigrateDB runs all DDL statements directly against the provided database.
// This is intended for use in tests where a live Nexus control endpoint is
// not available (e.g. an in-process DuckDB instance).
func MigrateDB(db *PortalDB) error {
	slog.Info("running database migrations via db connection")

	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w\nstatement: %s", err, stmt)
		}
	}

	slog.Info("database migrations complete")
	return nil
}

var migrations = []string{
	// Sequences for autoincrementing IDs (DuckDB requirement)
	"CREATE SEQUENCE IF NOT EXISTS accounts_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS contacts_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS bills_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS invoices_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS transactions_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS transaction_documents_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS payouts_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS recurring_payments_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS recurring_payment_occurrences_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS bill_items_id_seq",
	"CREATE SEQUENCE IF NOT EXISTS invoice_items_id_seq",

	// Accounts: bank, cash, credit card
	`CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY DEFAULT nextval('accounts_id_seq'),
		name TEXT NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('bank', 'cash', 'credit_card')),
		opening_balance INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Contacts: vendors and customers
	`CREATE TABLE IF NOT EXISTS contacts (
		id INTEGER PRIMARY KEY DEFAULT nextval('contacts_id_seq'),
		name TEXT NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('vendor', 'customer')),
		email TEXT,
		phone TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Bills: payable to vendors
	`CREATE TABLE IF NOT EXISTS bills (
		id INTEGER PRIMARY KEY DEFAULT nextval('bills_id_seq'),
		contact_id INTEGER,
		bill_number TEXT,
		issue_date DATE,
		due_date DATE,
		amount INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'partial', 'received', 'paid', 'overdue', 'cancelled')),
		file_url TEXT,
		notes TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Invoices: receivable from customers
	`CREATE TABLE IF NOT EXISTS invoices (
		id INTEGER PRIMARY KEY DEFAULT nextval('invoices_id_seq'),
		contact_id INTEGER,
		invoice_number TEXT,
		issue_date DATE,
		due_date DATE,
		amount INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'partial', 'sent', 'paid', 'received', 'overdue', 'cancelled')),
		file_url TEXT,
		notes TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Bank transactions: income, expense, transfer
	`CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY DEFAULT nextval('transactions_id_seq'),
		account_id INTEGER NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('income', 'expense', 'transfer')),
		amount INTEGER NOT NULL DEFAULT 0,
		transaction_date DATE,
		description TEXT,
		reference TEXT,
		transfer_account_id INTEGER,
		contact_id INTEGER,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Junction table: many-to-many transaction <-> bill/invoice/payout/recurring_payment_occurrence.
	// No CHECK constraint on document_type — valid types are enforced by the application layer.
	`CREATE TABLE IF NOT EXISTS transaction_documents (
		id INTEGER PRIMARY KEY DEFAULT nextval('transaction_documents_id_seq'),
		transaction_id INTEGER NOT NULL,
		document_type TEXT NOT NULL,
		document_id INTEGER NOT NULL,
		amount INTEGER NOT NULL CHECK(amount > 0),
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Payouts from Swiggy/Zomato/Swiggy-Dineout
	`CREATE TABLE IF NOT EXISTS payouts (
		id INTEGER PRIMARY KEY DEFAULT nextval('payouts_id_seq'),
		outlet_name TEXT NOT NULL,
		platform TEXT NOT NULL,
		period_start DATE,
		period_end DATE,
		settlement_date TEXT,
		total_orders INTEGER NOT NULL DEFAULT 0,
		gross_sales_amt INTEGER NOT NULL DEFAULT 0,
		restaurant_discount_amt INTEGER NOT NULL DEFAULT 0,
		platform_commission_amt INTEGER NOT NULL DEFAULT 0,
		taxes_tcs_tds_amt INTEGER NOT NULL DEFAULT 0,
		marketing_ads_amt INTEGER NOT NULL DEFAULT 0,
		final_payout_amt INTEGER NOT NULL DEFAULT 0,
		utr_number TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// Indexes for common queries
	`CREATE INDEX IF NOT EXISTS idx_bills_contact ON bills(contact_id)`,
	`CREATE INDEX IF NOT EXISTS idx_bills_status ON bills(status)`,
	`CREATE INDEX IF NOT EXISTS idx_invoices_contact ON invoices(contact_id)`,
	`CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status)`,
	`CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions(account_id)`,
	`CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type)`,
	`CREATE INDEX IF NOT EXISTS idx_transaction_documents_txn ON transaction_documents(transaction_id)`,
	`CREATE INDEX IF NOT EXISTS idx_transaction_documents_doc ON transaction_documents(document_type, document_id)`,
	`CREATE INDEX IF NOT EXISTS idx_payouts_platform ON payouts(platform)`,
	`CREATE INDEX IF NOT EXISTS idx_payouts_outlet ON payouts(outlet_name)`,

	// Recurring payments: scheduled income or expense
	`CREATE TABLE IF NOT EXISTS recurring_payments (
		id INTEGER PRIMARY KEY DEFAULT nextval('recurring_payments_id_seq'),
		name TEXT NOT NULL,
		type TEXT NOT NULL CHECK(type IN ('income', 'expense')),
		amount INTEGER NOT NULL CHECK(amount > 0),
		account_id INTEGER NOT NULL,
		contact_id INTEGER,
		frequency TEXT NOT NULL CHECK(frequency IN ('daily', 'weekly', 'monthly', 'quarterly', 'yearly')),
		interval INTEGER NOT NULL DEFAULT 1 CHECK(interval > 0),
		start_date DATE NOT NULL,
		end_date DATE,
		next_due_date DATE,
		last_generated_date DATE,
		status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'paused', 'cancelled', 'completed')),
		description TEXT,
		reference TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_recurring_payments_account ON recurring_payments(account_id)`,
	`CREATE INDEX IF NOT EXISTS idx_recurring_payments_status ON recurring_payments(status)`,
	`CREATE INDEX IF NOT EXISTS idx_recurring_payments_next_due ON recurring_payments(next_due_date)`,

	// Recurring payment occurrences: one row per scheduled occurrence of a recurring payment.
	// Auto-generated by the server on startup and via a daily background job.
	`CREATE TABLE IF NOT EXISTS recurring_payment_occurrences (
		id INTEGER PRIMARY KEY DEFAULT nextval('recurring_payment_occurrences_id_seq'),
		recurring_payment_id INTEGER NOT NULL,
		due_date DATE NOT NULL,
		amount INTEGER NOT NULL CHECK(amount > 0),
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_recurring_payment_occurrences_unique ON recurring_payment_occurrences(recurring_payment_id, due_date)`,
	`CREATE INDEX IF NOT EXISTS idx_recurring_payment_occurrences_rp ON recurring_payment_occurrences(recurring_payment_id)`,
	`CREATE INDEX IF NOT EXISTS idx_recurring_payment_occurrences_status ON recurring_payment_occurrences(status)`,
	`CREATE INDEX IF NOT EXISTS idx_recurring_payment_occurrences_due_date ON recurring_payment_occurrences(due_date)`,

	// Bill items: individual line items for a bill
	`CREATE TABLE IF NOT EXISTS bill_items (
		id INTEGER PRIMARY KEY DEFAULT nextval('bill_items_id_seq'),
		bill_id INTEGER NOT NULL,
		description TEXT NOT NULL,
		quantity DOUBLE NOT NULL DEFAULT 1,
		unit TEXT,
		unit_price INTEGER NOT NULL DEFAULT 0,
		amount INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_bill_items_bill ON bill_items(bill_id)`,

	// Invoice items: individual line items for an invoice
	`CREATE TABLE IF NOT EXISTS invoice_items (
		id INTEGER PRIMARY KEY DEFAULT nextval('invoice_items_id_seq'),
		invoice_id INTEGER NOT NULL,
		description TEXT NOT NULL,
		quantity DOUBLE NOT NULL DEFAULT 1,
		unit TEXT,
		unit_price INTEGER NOT NULL DEFAULT 0,
		amount INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id)`,

	// Add unit column to existing bill_items and invoice_items tables (idempotent).
	`ALTER TABLE bill_items ADD COLUMN IF NOT EXISTS unit TEXT`,
	`ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS unit TEXT`,
}
