package handlers

import (
	"net/http"
)

type dashboardData struct {
	TotalAccounts     int `json:"total_accounts"`
	TotalContacts     int `json:"total_contacts"`
	TotalBills        int `json:"total_bills"`
	TotalInvoices     int `json:"total_invoices"`
	TotalPayouts      int `json:"total_payouts"`
	TotalTransactions int `json:"total_transactions"`

	BillsPayable       int `json:"bills_payable"`
	InvoicesReceivable int `json:"invoices_receivable"`
	PayoutsReceived    int `json:"payouts_received"` // sum of final_payout_amt

	OverdueBills    int `json:"overdue_bills"`
	OverdueInvoices int `json:"overdue_invoices"`

	RecentTransactions []map[string]any `json:"recent_transactions"`
}

// GetDashboard retrieves dashboard summary statistics
// @Summary      Get dashboard
// @Description  Get totals for accounts, contacts, bills, invoices, and recent transactions.
// @Tags         dashboard
// @Produce      json
// @Success      200  {object}  Response{data=dashboardData}
// @Router       /dashboard [get]
// @Security     BasicAuth
func GetDashboard(w http.ResponseWriter, r *http.Request) {
	dbConn := getDB(r)
	var d dashboardData

	dbConn.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&d.TotalAccounts)
	dbConn.QueryRow("SELECT COUNT(*) FROM contacts").Scan(&d.TotalContacts)
	dbConn.QueryRow("SELECT COUNT(*) FROM bills").Scan(&d.TotalBills)
	dbConn.QueryRow("SELECT COUNT(*) FROM invoices").Scan(&d.TotalInvoices)
	dbConn.QueryRow("SELECT COUNT(*) FROM payouts").Scan(&d.TotalPayouts)
	dbConn.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&d.TotalTransactions)

	dbConn.QueryRow(`SELECT COALESCE(SUM(amount - (SELECT COALESCE(SUM(td.amount), 0) FROM transaction_documents td WHERE td.document_type = 'bill' AND td.document_id = bills.id)), 0) 
		FROM bills WHERE status NOT IN ('paid', 'cancelled')`).Scan(&d.BillsPayable)
	dbConn.QueryRow(`SELECT COALESCE(SUM(amount - (SELECT COALESCE(SUM(td.amount), 0) FROM transaction_documents td WHERE td.document_type = 'invoice' AND td.document_id = invoices.id)), 0) 
		FROM invoices WHERE status NOT IN ('paid', 'received', 'cancelled')`).Scan(&d.InvoicesReceivable)
	dbConn.QueryRow("SELECT COALESCE(SUM(final_payout_amt), 0) FROM payouts").Scan(&d.PayoutsReceived)

	dbConn.QueryRow("SELECT COUNT(*) FROM bills WHERE status = 'overdue'").Scan(&d.OverdueBills)
	dbConn.QueryRow("SELECT COUNT(*) FROM invoices WHERE status = 'overdue'").Scan(&d.OverdueInvoices)

	// Recent 5 transactions
	rows, err := dbConn.Query(`SELECT t.id, t.type, t.amount, t.transaction_date, t.description, a.name as account_name
		FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id
		ORDER BY t.created_at DESC LIMIT 5`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var tp, desc, date, acct *string
			var amount int
			rows.Scan(&id, &tp, &amount, &date, &desc, &acct)
			d.RecentTransactions = append(d.RecentTransactions, map[string]any{
				"id":               id,
				"type":             tp,
				"amount":           amount,
				"transaction_date": date,
				"description":      desc,
				"account_name":     acct,
			})
		}
	}
	if d.RecentTransactions == nil {
		d.RecentTransactions = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, d)
}
