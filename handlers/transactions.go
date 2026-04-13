package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/models"
	"github.com/satheeshds/portal/store"
)

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
	s := store.New(getDB(r))
	txns, err := s.ListTransactions(
		r.URL.Query().Get("type"),
		r.URL.Query().Get("account_id"),
		r.URL.Query().Get("from"),
		r.URL.Query().Get("to"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	t, err := s.GetTransaction(id)
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
	s := store.New(getDB(r))
	var input models.TransactionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	t, err := s.CreateTransaction(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
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
	s := store.New(getDB(r))
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
	t, err := s.UpdateTransaction(id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "transaction not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
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
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeleteTransaction(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "transaction not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
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
	s := store.New(getDB(r))
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	docs, err := s.ListTransactionLinks(txnID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	s := store.New(getDB(r))
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

	// Check transaction exists and get its unallocated balance
	txn, err := s.GetTransaction(txnID)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	if input.Amount > txn.Unallocated {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("transaction only has %d paise unallocated (requested %d)", txn.Unallocated, input.Amount))
		return
	}

	// Check document exists and get its unallocated balance
	docAmount, docAllocated, err := s.GetDocumentAmountAndAllocated(input.DocumentType, input.DocumentID)
	if err != nil {
		if input.DocumentType == "bill" || input.DocumentType == "invoice" || input.DocumentType == "payout" || input.DocumentType == "recurring_payment_occurrence" {
			writeError(w, http.StatusNotFound, fmt.Sprintf("%s not found", input.DocumentType))
		} else {
			writeError(w, http.StatusBadRequest, "invalid document type")
		}
		return
	}
	docUnallocated := models.Money(int64(docAmount) - int64(docAllocated))
	if input.Amount > docUnallocated {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s only has %d paise unallocated (requested %d)", input.DocumentType, docUnallocated, input.Amount))
		return
	}

	td, err := s.CreateTransactionLink(txnID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.UpdateDocumentStatus(input.DocumentType, input.DocumentID)
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
	s := store.New(getDB(r))
	txnID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	linkID, _ := strconv.Atoi(chi.URLParam(r, "linkId"))

	if err := s.DeleteTransactionLink(txnID, linkID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "link not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
