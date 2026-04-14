package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/satheeshds/portal/models"
)

const txnSelectQuery = `SELECT t.id, t.account_id, t.type, t.amount, t.transaction_date,
	t.description, t.reference, t.transfer_account_id, t.contact_id,
	t.created_at, t.updated_at,
	a.name,
	ta.name,
	c.name,
	COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.transaction_id = t.id), 0)
	FROM transactions t
	LEFT JOIN accounts a ON t.account_id = a.id
	LEFT JOIN accounts ta ON t.transfer_account_id = ta.id
	LEFT JOIN contacts c ON t.contact_id = c.id`

func scanTransaction(scanner interface{ Scan(...any) error }) (models.Transaction, error) {
	var t models.Transaction
	err := scanner.Scan(&t.ID, &t.AccountID, &t.Type, &t.Amount, &t.TransactionDate,
		&t.Description, &t.Reference, &t.TransferAccountID, &t.ContactID,
		&t.CreatedAt, &t.UpdatedAt,
		&t.AccountName, &t.TransferAccountName, &t.ContactName, &t.Allocated)
	t.Unallocated = models.Money(int64(t.Amount) - int64(t.Allocated))
	return t, err
}

func (s *Store) getTransactionByID(id int) (models.Transaction, error) {
	return scanTransaction(s.db.QueryRow(txnSelectQuery+" WHERE t.id = ?", id))
}

// ListTransactions returns transactions filtered by the provided parameters (all may be empty).
func (s *Store) ListTransactions(txnType, accountID, from, to string) ([]models.Transaction, error) {
	query := txnSelectQuery
	var conditions []string
	var args []any

	if txnType != "" {
		conditions = append(conditions, "t.type = ?")
		args = append(args, txnType)
	}
	if accountID != "" {
		conditions = append(conditions, "t.account_id = ?")
		args = append(args, accountID)
	}
	if from != "" {
		conditions = append(conditions, "t.transaction_date >= ?")
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, "t.transaction_date <= ?")
		args = append(args, to)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY t.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []models.Transaction
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if txns == nil {
		txns = []models.Transaction{}
	}
	return txns, nil
}

// GetTransaction returns a single transaction by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetTransaction(id int) (models.Transaction, error) {
	return s.getTransactionByID(id)
}

// CreateTransaction inserts a new transaction (handling transfer pairs) and returns the created record.
func (s *Store) CreateTransaction(input models.TransactionInput) (models.Transaction, error) {
	if input.Type == "transfer" {
		tx, err := s.db.Begin()
		if err != nil {
			return models.Transaction{}, err
		}
		defer tx.Rollback()

		ref := input.Reference
		if ref == nil {
			autoRef := fmt.Sprintf("TRF-%d", 0)
			ref = &autoRef
		}

		var id1 int
		err = tx.QueryRow(`INSERT INTO transactions (account_id, type, amount, transaction_date, description, reference, transfer_account_id, contact_id)
			VALUES (?, 'expense', ?, ?, ?, ?, ?, ?) RETURNING id`,
			input.AccountID, input.Amount, input.TransactionDate, input.Description, ref, input.TransferAccountID, input.ContactID).Scan(&id1)
		if err != nil {
			return models.Transaction{}, err
		}

		_, err = tx.Exec(`INSERT INTO transactions (account_id, type, amount, transaction_date, description, reference, transfer_account_id, contact_id)
			VALUES (?, 'income', ?, ?, ?, ?, ?, ?)`,
			*input.TransferAccountID, input.Amount, input.TransactionDate, input.Description, ref, &input.AccountID, input.ContactID)
		if err != nil {
			return models.Transaction{}, err
		}

		if input.Reference == nil {
			autoRef := fmt.Sprintf("TRF-%d", id1)
			if _, err := tx.Exec("UPDATE transactions SET reference = ? WHERE reference = ?", autoRef, *ref); err != nil {
				return models.Transaction{}, err
			}
		}

		if err := tx.Commit(); err != nil {
			return models.Transaction{}, err
		}
		return s.getTransactionByID(id1)
	}

	var id int
	err := s.db.QueryRow(`INSERT INTO transactions (account_id, type, amount, transaction_date, description, reference, transfer_account_id, contact_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.AccountID, input.Type, input.Amount, input.TransactionDate,
		input.Description, input.Reference, input.TransferAccountID, input.ContactID).Scan(&id)
	if err != nil {
		return models.Transaction{}, err
	}
	return s.getTransactionByID(id)
}

// UpdateTransaction updates an existing transaction. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateTransaction(id int, input models.TransactionInput) (models.Transaction, error) {
	res, err := s.db.Exec(`UPDATE transactions SET account_id = ?, type = ?, amount = ?, transaction_date = ?,
		description = ?, reference = ?, transfer_account_id = ?, contact_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		input.AccountID, input.Type, input.Amount, input.TransactionDate,
		input.Description, input.Reference, input.TransferAccountID, input.ContactID, id)
	if err != nil {
		return models.Transaction{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Transaction{}, sql.ErrNoRows
	}
	return s.getTransactionByID(id)
}

// DeleteTransaction removes a transaction and its document links, then updates affected document statuses.
// Returns sql.ErrNoRows if not found.
func (s *Store) DeleteTransaction(id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	type docRef struct {
		docType string
		docID   int
	}
	var affected []docRef

	rows, err := tx.Query("SELECT document_type, document_id FROM transaction_documents WHERE transaction_id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var dr docRef
		if err := rows.Scan(&dr.docType, &dr.docID); err != nil {
			_ = tx.Rollback()
			return err
		}
		affected = append(affected, dr)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return err
	}

	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE transaction_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}

	res, err := tx.Exec("DELETE FROM transactions WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		_ = tx.Rollback()
		return err
	} else if n == 0 {
		_ = tx.Rollback()
		return sql.ErrNoRows
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, dr := range affected {
		s.UpdateDocumentStatus(dr.docType, dr.docID)
	}

	return nil
}

// ListTransactionLinks returns all document links for a transaction.
func (s *Store) ListTransactionLinks(txnID int) ([]models.TransactionDocument, error) {
	rows, err := s.db.Query(`SELECT id, transaction_id, document_type, document_id, amount, created_at
		FROM transaction_documents WHERE transaction_id = ? ORDER BY created_at`, txnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []models.TransactionDocument
	for rows.Next() {
		var td models.TransactionDocument
		if err := rows.Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, td)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if docs == nil {
		docs = []models.TransactionDocument{}
	}
	return docs, nil
}

// GetDocumentAmountAndAllocated returns the total amount and already-allocated amount of a document.
// docType must be one of "bill", "invoice", "payout", or "recurring_payment_occurrence".
// Returns sql.ErrNoRows if the document does not exist.
func (s *Store) GetDocumentAmountAndAllocated(docType string, docID int) (amount, allocated models.Money, err error) {
	var table, amountField string
	switch docType {
	case "bill":
		table, amountField = "bills", "amount"
	case "invoice":
		table, amountField = "invoices", "amount"
	case "payout":
		table, amountField = "payouts", "final_payout_amt"
	case "recurring_payment_occurrence":
		table, amountField = "recurring_payment_occurrences", "amount"
	default:
		return 0, 0, fmt.Errorf("unsupported document type: %s", docType)
	}
	err = s.db.QueryRow(fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", amountField, table), docID).Scan(&amount)
	if err != nil {
		return 0, 0, err
	}
	err = s.db.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE document_type = ? AND document_id = ?",
		docType, docID).Scan(&allocated)
	if err != nil {
		return 0, 0, err
	}
	return amount, allocated, nil
}

// CreateTransactionLink creates a link between a transaction and a document and returns it.
func (s *Store) CreateTransactionLink(txnID int, input models.TransactionDocumentInput) (models.TransactionDocument, error) {
	var id int
	err := s.db.QueryRow(`INSERT INTO transaction_documents (transaction_id, document_type, document_id, amount)
		VALUES (?, ?, ?, ?) RETURNING id`, txnID, input.DocumentType, input.DocumentID, input.Amount).Scan(&id)
	if err != nil {
		return models.TransactionDocument{}, err
	}

	var td models.TransactionDocument
	err = s.db.QueryRow("SELECT id, transaction_id, document_type, document_id, amount, created_at FROM transaction_documents WHERE id = ?", id).
		Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt)
	if err != nil {
		return models.TransactionDocument{}, err
	}
	return td, nil
}

// DeleteTransactionLink removes a transaction link. Returns sql.ErrNoRows if not found.
// It also calls UpdateDocumentStatus for the affected document.
func (s *Store) DeleteTransactionLink(txnID, linkID int) error {
	var docType string
	var docID int
	if err := s.db.QueryRow("SELECT document_type, document_id FROM transaction_documents WHERE id = ?", linkID).Scan(&docType, &docID); err != nil && err != sql.ErrNoRows {
		return err
	}

	res, err := s.db.Exec("DELETE FROM transaction_documents WHERE id = ? AND transaction_id = ?", linkID, txnID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	if docType != "" {
		s.UpdateDocumentStatus(docType, docID)
	}
	return nil
}

// UpdateDocumentStatus recalculates and updates the status field of a bill, invoice, or recurring_payment_occurrence
// based on how much has been allocated via transaction_documents.
func (s *Store) UpdateDocumentStatus(docType string, docID int) {
	var total, allocated models.Money
	var table, fullStatus, amountField string
	switch docType {
	case "bill":
		table = "bills"
		fullStatus = "paid"
		amountField = "amount"
	case "invoice":
		table = "invoices"
		fullStatus = "received"
		amountField = "amount"
	case "payout":
		return
	case "recurring_payment":
		return
	case "recurring_payment_occurrence":
		table = "recurring_payment_occurrences"
		fullStatus = "paid"
		amountField = "amount"
	default:
		return
	}

	err := s.db.QueryRow(fmt.Sprintf("SELECT %s, (SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE document_type = ? AND document_id = ?) FROM %s WHERE id = ?", amountField, table),
		docType, docID, docID).Scan(&total, &allocated)
	if err != nil {
		return
	}

	var newStatus string
	if docType == "recurring_payment_occurrence" {
		if allocated >= total && total > 0 {
			newStatus = "paid"
		} else {
			newStatus = "pending"
		}
	} else {
		if total <= 0 || allocated <= 0 {
			newStatus = "draft"
		} else if allocated < total {
			newStatus = "partial"
		} else {
			newStatus = fullStatus
		}
	}

	if _, err := s.db.Exec(fmt.Sprintf("UPDATE %s SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", table), newStatus, docID); err != nil {
		slog.Warn("UpdateDocumentStatus: failed to update status", "docType", docType, "docID", docID, "error", err)
	}
}
