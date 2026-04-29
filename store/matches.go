package store

import (
	"time"

	"github.com/satheeshds/portal/models"
)

// BillMatchCandidate holds raw bill data returned by SuggestBills.
type BillMatchCandidate struct {
	ID          int
	BillNumber  string
	DueDate     models.Date
	IssueDate   models.Date
	Amount      models.Money
	Notes       string
	ContactName string
	Allocated   models.Money
}

// InvoiceMatchCandidate holds raw invoice data returned by SuggestInvoices.
type InvoiceMatchCandidate struct {
	ID            int
	InvoiceNumber string
	DueDate       models.Date
	IssueDate     models.Date
	Amount        models.Money
	Notes         string
	ContactName   string
	Allocated     models.Money
}

// PayoutMatchCandidate holds raw payout data returned by SuggestPayouts.
type PayoutMatchCandidate struct {
	ID             int
	UtrNumber      string
	SettlementDate models.Date
	Amount         models.Money
	OutletName     string
	Allocated      models.Money
}

// RPOccurrenceCandidate holds raw recurring payment occurrence data.
type RPOccurrenceCandidate struct {
	ID          int
	DueDate     models.Date
	Amount      models.Money
	Allocated   models.Money
	Name        string
	Description string
	Reference   string
}

// TransactionCandidate holds raw transaction data for match suggestions.
type TransactionCandidate struct {
	ID          int
	Amount      models.Money
	Date        models.Date
	Description string
	Reference   string
	Allocated   models.Money
}

// BillMatchInfo holds the data needed to score transaction suggestions for a bill.
type BillMatchInfo struct {
	Amount      models.Money
	BillNumber  string
	DueDate     models.Date
	IssueDate   models.Date
	Notes       string
	ContactName string
}

// InvoiceMatchInfo holds the data needed to score transaction suggestions for an invoice.
type InvoiceMatchInfo struct {
	Amount        models.Money
	InvoiceNumber string
	DueDate       models.Date
	IssueDate     models.Date
	Notes         string
	ContactName   string
}

// PayoutMatchInfo holds the data needed to score transaction suggestions for a payout.
type PayoutMatchInfo struct {
	Amount         models.Money
	UtrNumber      string
	SettlementDate models.Date
	OutletName     string
}

// RPMatchInfo holds the data needed to score transaction suggestions for a recurring payment.
type RPMatchInfo struct {
	Amount      models.Money
	Type        string
	Name        string
	Description string
	Reference   string
	NextDueDate models.Date
}

// SuggestBills returns unallocated bill candidates for match scoring.
func (s *Store) SuggestBills(amount models.Money, txnDate time.Time, txnSearchText string) ([]BillMatchCandidate, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT b.id, COALESCE(b.bill_number, ''), b.due_date, b.issue_date,
			b.amount, COALESCE(b.notes, ''), COALESCE(c.name, ''),
			COALESCE(a.total_allocated, 0)
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id
		LEFT JOIN (
			SELECT document_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			WHERE document_type = 'bill'
			GROUP BY document_id
		) a ON a.document_id = b.id
		WHERE b.status NOT IN ('paid', 'cancelled')
		  AND b.amount > COALESCE(a.total_allocated, 0)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []BillMatchCandidate
	for rows.Next() {
		var c BillMatchCandidate
		if err := rows.Scan(&c.ID, &c.BillNumber, &c.DueDate, &c.IssueDate, &c.Amount, &c.Notes, &c.ContactName, &c.Allocated); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// SuggestInvoices returns unallocated invoice candidates for match scoring.
func (s *Store) SuggestInvoices(amount models.Money, txnDate time.Time, txnSearchText string) ([]InvoiceMatchCandidate, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT i.id, COALESCE(i.invoice_number, ''), i.due_date, i.issue_date,
			i.amount, COALESCE(i.notes, ''), COALESCE(c.name, ''),
			COALESCE(a.total_allocated, 0)
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id
		LEFT JOIN (
			SELECT document_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			WHERE document_type = 'invoice'
			GROUP BY document_id
		) a ON a.document_id = i.id
		WHERE i.status NOT IN ('received', 'cancelled')
		  AND i.amount > COALESCE(a.total_allocated, 0)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []InvoiceMatchCandidate
	for rows.Next() {
		var c InvoiceMatchCandidate
		if err := rows.Scan(&c.ID, &c.InvoiceNumber, &c.DueDate, &c.IssueDate, &c.Amount, &c.Notes, &c.ContactName, &c.Allocated); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// SuggestPayouts returns unallocated payout candidates for match scoring.
func (s *Store) SuggestPayouts(amount models.Money, txnDate time.Time, txnSearchText string) ([]PayoutMatchCandidate, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT p.id, COALESCE(p.utr_number, ''), p.settlement_date,
			p.final_payout_amt, COALESCE(p.outlet_name, ''),
			COALESCE(a.total_allocated, 0)
		FROM payouts p
		LEFT JOIN (
			SELECT document_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			WHERE document_type = 'payout'
			GROUP BY document_id
		) a ON a.document_id = p.id
		WHERE p.final_payout_amt > COALESCE(a.total_allocated, 0)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []PayoutMatchCandidate
	for rows.Next() {
		var c PayoutMatchCandidate
		if err := rows.Scan(&c.ID, &c.UtrNumber, &c.SettlementDate, &c.Amount, &c.OutletName, &c.Allocated); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// SuggestRecurringPaymentOccurrences returns pending occurrence candidates for match scoring.
func (s *Store) SuggestRecurringPaymentOccurrences(txnType string, amount models.Money, txnDate time.Time) ([]RPOccurrenceCandidate, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT o.id, o.due_date, o.amount,
			COALESCE(SUM(td.amount), 0) AS allocated,
			r.name, COALESCE(r.description, ''), COALESCE(r.reference, '')
		FROM recurring_payment_occurrences o
		JOIN recurring_payments r ON o.recurring_payment_id = r.id
		LEFT JOIN transaction_documents td
			ON td.document_type = 'recurring_payment_occurrence' AND td.document_id = o.id
		WHERE o.status = 'pending' AND r.status = 'active' AND r.type = ?
		GROUP BY o.id, o.due_date, o.amount, r.name, r.description, r.reference
		HAVING COALESCE(SUM(td.amount), 0) < o.amount
	`, txnType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []RPOccurrenceCandidate
	for rows.Next() {
		var c RPOccurrenceCandidate
		if err := rows.Scan(&c.ID, &c.DueDate, &c.Amount, &c.Allocated, &c.Name, &c.Description, &c.Reference); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// GetDocumentAllocated returns the total amount already allocated to a document.
func (s *Store) GetDocumentAllocated(docType string, docID int) (models.Money, error) {
	var allocated models.Money
	err := s.db.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE document_type = ? AND document_id = ?", docType, docID).Scan(&allocated)
	return allocated, err
}

// CreateTransactionDocumentLink inserts a new transaction_documents row and returns it.
func (s *Store) CreateTransactionDocumentLink(txnID int, docType string, docID int, amount models.Money) (models.TransactionDocument, error) {
	var id int
	err := s.db.QueryRow(`INSERT INTO transaction_documents (transaction_id, document_type, document_id, amount)
		VALUES (?, ?, ?, ?) RETURNING id`, txnID, docType, docID, amount).Scan(&id)
	if err != nil {
		return models.TransactionDocument{}, err
	}
	return s.GetTransactionDocument(id)
}

// GetTransactionDocument returns a single transaction_documents row by ID.
func (s *Store) GetTransactionDocument(id int) (models.TransactionDocument, error) {
	var td models.TransactionDocument
	err := s.db.QueryRow("SELECT id, transaction_id, document_type, document_id, amount, created_at FROM transaction_documents WHERE id = ?", id).
		Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt)
	return td, err
}

// SuggestTransactionsForDocument returns raw transaction candidates that could match a given document.
func (s *Store) SuggestTransactionsForDocument(txnType, docType string, docID int) ([]TransactionCandidate, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.amount, t.transaction_date, COALESCE(t.description, ''), COALESCE(t.reference, ''),
			COALESCE(a.total_allocated, 0)
		FROM transactions t
		LEFT JOIN (
			SELECT transaction_id, SUM(amount) AS total_allocated
			FROM transaction_documents
			GROUP BY transaction_id
		) a ON a.transaction_id = t.id
		WHERE t.type = ?
		  AND t.amount > COALESCE(a.total_allocated, 0)
		  AND NOT EXISTS (
			SELECT 1 FROM transaction_documents td2
			WHERE td2.transaction_id = t.id
			  AND td2.document_type = ?
			  AND td2.document_id = ?
		  )
	`, txnType, docType, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []TransactionCandidate
	for rows.Next() {
		var c TransactionCandidate
		if err := rows.Scan(&c.ID, &c.Amount, &c.Date, &c.Description, &c.Reference, &c.Allocated); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// GetBillForMatching returns the data needed to suggest transactions for a bill.
func (s *Store) GetBillForMatching(id int) (BillMatchInfo, error) {
	var info BillMatchInfo
	err := s.db.QueryRow(`
		SELECT b.amount, COALESCE(b.bill_number, ''), b.due_date, b.issue_date,
			COALESCE(b.notes, ''), COALESCE(c.name, '')
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id
		WHERE b.id = ?`, id).Scan(&info.Amount, &info.BillNumber, &info.DueDate, &info.IssueDate, &info.Notes, &info.ContactName)
	return info, err
}

// GetInvoiceForMatching returns the data needed to suggest transactions for an invoice.
func (s *Store) GetInvoiceForMatching(id int) (InvoiceMatchInfo, error) {
	var info InvoiceMatchInfo
	err := s.db.QueryRow(`
		SELECT i.amount, COALESCE(i.invoice_number, ''), i.due_date, i.issue_date,
			COALESCE(i.notes, ''), COALESCE(c.name, '')
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id
		WHERE i.id = ?`, id).Scan(&info.Amount, &info.InvoiceNumber, &info.DueDate, &info.IssueDate, &info.Notes, &info.ContactName)
	return info, err
}

// GetPayoutForMatching returns the data needed to suggest transactions for a payout.
func (s *Store) GetPayoutForMatching(id int) (PayoutMatchInfo, error) {
	var info PayoutMatchInfo
	err := s.db.QueryRow(`
		SELECT p.final_payout_amt, COALESCE(p.utr_number, ''), p.settlement_date, COALESCE(p.outlet_name, '')
		FROM payouts p
		WHERE p.id = ?`, id).Scan(&info.Amount, &info.UtrNumber, &info.SettlementDate, &info.OutletName)
	return info, err
}

// GetRecurringPaymentForMatching returns the data needed to suggest transactions for a recurring payment.
func (s *Store) GetRecurringPaymentForMatching(id int) (RPMatchInfo, error) {
	var info RPMatchInfo
	err := s.db.QueryRow(`
		SELECT r.amount, r.type, r.name, COALESCE(r.description, ''), COALESCE(r.reference, ''), r.next_due_date
		FROM recurring_payments r
		WHERE r.id = ?`, id).Scan(&info.Amount, &info.Type, &info.Name, &info.Description, &info.Reference, &info.NextDueDate)
	return info, err
}

// SuggestTransactionsForRecurringPayment returns raw transaction candidates for a recurring payment,
// excluding those already linked to any occurrence of this recurring payment.
func (s *Store) SuggestTransactionsForRecurringPayment(rpType string, rpID int) ([]TransactionCandidate, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.amount, t.transaction_date, COALESCE(t.description, ''), COALESCE(t.reference, ''),
			COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.transaction_id = t.id), 0)
		FROM transactions t
		WHERE t.type = ?
		  AND NOT EXISTS (
			SELECT 1 FROM transaction_documents td2
			WHERE td2.transaction_id = t.id
			  AND td2.document_type = 'recurring_payment_occurrence'
			  AND EXISTS (
			  	SELECT 1 FROM recurring_payment_occurrences rpo
			  	WHERE rpo.id = td2.document_id AND rpo.recurring_payment_id = ?
			  )
		  )
	`, rpType, rpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []TransactionCandidate
	for rows.Next() {
		var c TransactionCandidate
		if err := rows.Scan(&c.ID, &c.Amount, &c.Date, &c.Description, &c.Reference, &c.Allocated); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}
