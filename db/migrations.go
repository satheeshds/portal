package db

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// Migrate runs all table creation statements. Safe to call multiple times
// due to IF NOT EXISTS clauses.
func Migrate(db *sql.DB) error {
	slog.Info("running database migrations")

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

	// Junction table: many-to-many transaction <-> bill/invoice/payout
	`CREATE TABLE IF NOT EXISTS transaction_documents (
		id INTEGER PRIMARY KEY DEFAULT nextval('transaction_documents_id_seq'),
		transaction_id INTEGER NOT NULL,
		document_type TEXT NOT NULL CHECK(document_type IN ('bill', 'invoice', 'payout')),
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
}
