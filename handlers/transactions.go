package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/accounting/models"
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

func getTransactionByID(id int) (models.Transaction, error) {
	return scanTransaction(DB.QueryRow(txnSelectQuery+" WHERE t.id = ?", id))
}

// ListTransactions lists all transactions
// @Summary      List transactions
// @Description  Get a list of all bank transactions (income, expense, transfer) with allocation info.
// @Tags         transactions
// @Produce      json
// @Param        account_id   query     int  false  "Filter by account"
// @Param        contact_id   query     int  false  "Filter by contact"
// @Success      200          {object}  Response{data=[]models.Transaction}
// @Router       /transactions [get]
// @Security     BasicAuth
func ListTransactions(w http.ResponseWriter, r *http.Request) {
	query := txnSelectQuery
	var conditions []string
	var args []any

	if tp := r.URL.Query().Get("type"); tp != "" {
		conditions = append(conditions, "t.type = ?")
		args = append(args, tp)
	}
	if aid := r.URL.Query().Get("account_id"); aid != "" {
		conditions = append(conditions, "t.account_id = ?")
		args = append(args, aid)
	}
	if from := r.URL.Query().Get("from"); from != "" {
		conditions = append(conditions, "t.transaction_date >= ?")
		args = append(args, from)
	}
	if to := r.URL.Query().Get("to"); to != "" {
		conditions = append(conditions, "t.transaction_date <= ?")
		args = append(args, to)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY t.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var txns []models.Transaction
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		txns = append(txns, t)
	}
	if txns == nil {
		txns = []models.Transaction{}
	}
	writeJSON(w, http.StatusOK, txns)
}

// GetTransaction retrieves a single transaction by ID
// @Summary      Get transaction
// @Description  Get details and allocation status of a specific transaction.
// @Tags         transactions
// @Produce      json
// @Param        id   path      int  true  "Transaction ID"
// @Success      200  {object}  Response{data=models.Transaction}
// @Failure      404  {object}  Response{error=string}
// @Router       /transactions/{id} [get]
// @Security     BasicAuth
func GetTransaction(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	t, err := getTransactionByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// CreateTransaction creates a new transaction
// @Summary      Create transaction
// @Description  Create a new bank transaction (income, expense, or transfer).
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        transaction  body      models.TransactionInput  true  "Transaction contents"
// @Success      201          {object}  Response{data=models.Transaction}
// @Router       /transactions [post]
// @Security     BasicAuth
func CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var input models.TransactionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// For transfers, create paired records in a transaction
	if input.Type == "transfer" {
		tx, err := DB.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer tx.Rollback()

		ref := input.Reference
		if ref == nil {
			autoRef := fmt.Sprintf("TRF-%d", 0) // will be updated after insert
			ref = &autoRef
		}

		// Expense on source account
		var id1 int
		err = tx.QueryRow(`INSERT INTO transactions (account_id, type, amount, transaction_date, description, reference, transfer_account_id, contact_id)
			VALUES (?, 'expense', ?, ?, ?, ?, ?, ?) RETURNING id`,
			input.AccountID, input.Amount, input.TransactionDate, input.Description, ref, input.TransferAccountID, input.ContactID).Scan(&id1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Income on destination account
		_, err = tx.Exec(`INSERT INTO transactions (account_id, type, amount, transaction_date, description, reference, transfer_account_id, contact_id)
			VALUES (?, 'income', ?, ?, ?, ?, ?, ?)`,
			*input.TransferAccountID, input.Amount, input.TransactionDate, input.Description, ref, &input.AccountID, input.ContactID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Update reference with actual IDs if auto-generated
		if input.Reference == nil {
			autoRef := fmt.Sprintf("TRF-%d", id1)
			tx.Exec("UPDATE transactions SET reference = ? WHERE reference = ?", autoRef, *ref)
		}

		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		t, err := getTransactionByID(id1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to re-fetch created transfer transaction: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, t)
		return
	}

	// Normal income/expense
	var id int
	err := DB.QueryRow(`INSERT INTO transactions (account_id, type, amount, transaction_date, description, reference, transfer_account_id, contact_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.AccountID, input.Type, input.Amount, input.TransactionDate,
		input.Description, input.Reference, input.TransferAccountID, input.ContactID).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	txn, err := getTransactionByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created transaction: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, txn)
}

// UpdateTransaction updates an existing transaction
// @Summary      Update transaction
// @Description  Update details of an existing transaction.
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        id           path      int                      true  "Transaction ID"
// @Param        transaction  body      models.TransactionInput  true  "Updated transaction contents"
// @Success      200          {object}  Response{data=models.Transaction}
// @Failure      404          {object}  Response{error=string}
// @Router       /transactions/{id} [put]
// @Security     BasicAuth
func UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.TransactionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	res, err := DB.Exec(`UPDATE transactions SET account_id = ?, type = ?, amount = ?, transaction_date = ?,
		description = ?, reference = ?, transfer_account_id = ?, contact_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		input.AccountID, input.Type, input.Amount, input.TransactionDate,
		input.Description, input.Reference, input.TransferAccountID, input.ContactID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	t, err := getTransactionByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated transaction: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// DeleteTransaction deletes a transaction
// @Summary      Delete transaction
// @Description  Remove a transaction.
// @Tags         transactions
// @Produce      json
// @Param        id   path      int  true  "Transaction ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /transactions/{id} [delete]
// @Security     BasicAuth
func DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Start a DB transaction to ensure atomicity of the delete and related updates.
	tx, err := DB.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Collect affected documents before deleting links so we can update their statuses.
	type docRef struct {
		docType string
		docID   int
	}
	var affected []docRef

	rows, err := tx.Query("SELECT document_type, document_id FROM transaction_documents WHERE transaction_id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var dr docRef
		if err := rows.Scan(&dr.docType, &dr.docID); err != nil {
			_ = tx.Rollback()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		affected = append(affected, dr)
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Delete all links for this transaction.
	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE transaction_id = ?", id); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Delete the transaction itself.
	res, err := tx.Exec("DELETE FROM transactions WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, err := res.RowsAffected(); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if n == 0 {
		_ = tx.Rollback()
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	// Commit DB changes before updating document statuses.
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update document statuses now that the allocation has been removed.
	for _, dr := range affected {
		updateDocumentStatus(dr.docType, dr.docID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// --- Transaction Document Linking ---

// ListTransactionLinks lists all documents linked to a transaction
// @Summary      List transaction links
// @Description  Get all bills and invoices linked (paid) by this transaction.
// @Tags         transactions
// @Produce      json
// @Param        id   path      int  true  "Transaction ID"
// @Success      200  {object}  Response{data=[]models.TransactionDocument}
// @Router       /transactions/{id}/links [get]
// @Security     BasicAuth
func ListTransactionLinks(w http.ResponseWriter, r *http.Request) {
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	rows, err := DB.Query(`SELECT id, transaction_id, document_type, document_id, amount, created_at
		FROM transaction_documents WHERE transaction_id = ? ORDER BY created_at`, txnID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var docs []models.TransactionDocument
	for rows.Next() {
		var td models.TransactionDocument
		if err := rows.Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		docs = append(docs, td)
	}
	if docs == nil {
		docs = []models.TransactionDocument{}
	}
	writeJSON(w, http.StatusOK, docs)
}

// CreateTransactionLink links a transaction to a bill or invoice
// @Summary      Create transaction link
// @Description  Allocate an amount from a transaction to a specific bill or invoice.
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        id    path      int                            true  "Transaction ID"
// @Param        link  body      models.TransactionDocumentInput true "Link details"
// @Success      201   {object}  Response{data=models.TransactionDocument}
// @Router       /transactions/{id}/links [post]
// @Security     BasicAuth
func CreateTransactionLink(w http.ResponseWriter, r *http.Request) {
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	var input models.TransactionDocumentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// Check transaction exists and get its amount
	var txnAmount models.Money
	err := DB.QueryRow("SELECT amount FROM transactions WHERE id = ?", txnID).Scan(&txnAmount)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	// Check transaction unallocated balance
	var txnAllocated models.Money
	DB.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE transaction_id = ?", txnID).Scan(&txnAllocated)
	txnUnallocated := models.Money(int64(txnAmount) - int64(txnAllocated))
	if input.Amount > txnUnallocated {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("transaction only has %d paise unallocated (requested %d)", txnUnallocated, input.Amount))
		return
	}

	// Check document exists and get its amount
	var docAmount models.Money
	var docTable, amountField string
	switch input.DocumentType {
	case "bill":
		docTable = "bills"
		amountField = "amount"
	case "invoice":
		docTable = "invoices"
		amountField = "amount"
	case "payout":
		docTable = "payouts"
		amountField = "final_payout_amt"
	case "recurring_payment_occurrence":
		// Each occurrence represents a single period's payment — unallocated check applies.
		docTable = "recurring_payment_occurrences"
		amountField = "amount"
	default:
		writeError(w, http.StatusBadRequest, "invalid document type")
		return
	}
	err = DB.QueryRow(fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", amountField, docTable), input.DocumentID).Scan(&docAmount)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("%s not found", input.DocumentType))
		return
	}

	// Check document unallocated balance
	var docAllocated models.Money
	DB.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE document_type = ? AND document_id = ?",
		input.DocumentType, input.DocumentID).Scan(&docAllocated)
	docUnallocated := models.Money(int64(docAmount) - int64(docAllocated))
	if input.Amount > docUnallocated {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s only has %d paise unallocated (requested %d)", input.DocumentType, docUnallocated, input.Amount))
		return
	}

	// Create the link
	var id int
	err = DB.QueryRow(`INSERT INTO transaction_documents (transaction_id, document_type, document_id, amount)
		VALUES (?, ?, ?, ?) RETURNING id`, txnID, input.DocumentType, input.DocumentID, input.Amount).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Handle automated status updates
	updateDocumentStatus(input.DocumentType, input.DocumentID)

	var td models.TransactionDocument
	DB.QueryRow("SELECT id, transaction_id, document_type, document_id, amount, created_at FROM transaction_documents WHERE id = ?", id).
		Scan(&td.ID, &td.TransactionID, &td.DocumentType, &td.DocumentID, &td.Amount, &td.CreatedAt)
	writeJSON(w, http.StatusCreated, td)
}

// DeleteTransactionLink removes a link between a transaction and a document
// @Summary      Delete transaction link
// @Description  Deallocate an amount from a transaction to a bill or invoice.
// @Tags         transactions
// @Produce      json
// @Param        id       path      int  true  "Transaction ID"
// @Param        linkId   path      int  true  "Link ID"
// @Success      200      {object}  Response{data=map[string]string}
// @Router       /transactions/{id}/links/{linkId} [delete]
// @Security     BasicAuth
func DeleteTransactionLink(w http.ResponseWriter, r *http.Request) {
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	linkID, _ := strconv.Atoi(chi.URLParam(r, "linkId"))

	// Handle automated status updates
	// We need to know document type/id before we delete it, but here we only have linkID.
	// Let's find it first.
	var docType string
	var docID int
	DB.QueryRow("SELECT document_type, document_id FROM transaction_documents WHERE id = ?", linkID).Scan(&docType, &docID)

	res, err := DB.Exec("DELETE FROM transaction_documents WHERE id = ? AND transaction_id = ?", linkID, txnID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}

	if docType != "" {
		updateDocumentStatus(docType, docID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
func updateDocumentStatus(docType string, docID int) {
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
		// Payouts don't have a status field in the current schema
		return
	case "recurring_payment":
		// Legacy support: recurring payments use their own status lifecycle
		// (active/paused/cancelled/completed) and are not updated based on
		// transaction allocation. New links with this type cannot be created;
		// this case exists only for backward compatibility with legacy records.
		return
	case "recurring_payment_occurrence":
		// Occurrences track their own paid/pending status based on allocation
		table = "recurring_payment_occurrences"
		fullStatus = "paid"
		amountField = "amount"
	default:
		return
	}

	err := DB.QueryRow(fmt.Sprintf("SELECT %s, (SELECT COALESCE(SUM(amount), 0) FROM transaction_documents WHERE document_type = ? AND document_id = ?) FROM %s WHERE id = ?", amountField, table),
		docType, docID, docID).Scan(&total, &allocated)
	if err != nil {
		return
	}

	var newStatus string
	if docType == "recurring_payment_occurrence" {
		// Occurrences only have pending/paid/skipped
		if allocated >= total && total > 0 {
			newStatus = "paid"
		} else {
			newStatus = "pending"
		}
	} else {
		// Bills and invoices use draft/partial/paid|received
		if total <= 0 || allocated <= 0 {
			newStatus = "draft"
		} else if allocated < total {
			newStatus = "partial"
		} else {
			newStatus = fullStatus
		}
	}

	DB.Exec(fmt.Sprintf("UPDATE %s SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", table), newStatus, docID)
}
