package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/accounting/models"
)

const invoiceSelectQuery = `SELECT i.id, i.contact_id, i.invoice_number, i.issue_date, i.due_date, i.amount,
		i.status, i.file_url, i.notes, i.created_at, i.updated_at,
		c.name,
		COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'invoice' AND td.document_id = i.id), 0)
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id`

func scanInvoice(scanner interface{ Scan(...any) error }) (models.Invoice, error) {
	var inv models.Invoice
	err := scanner.Scan(&inv.ID, &inv.ContactID, &inv.InvoiceNumber, &inv.IssueDate, &inv.DueDate,
		&inv.Amount, &inv.Status, &inv.FileURL, &inv.Notes, &inv.CreatedAt, &inv.UpdatedAt,
		&inv.ContactName, &inv.Allocated)
	if err == nil {
		inv.Unallocated = models.Money(int64(inv.Amount) - int64(inv.Allocated))
	}
	return inv, err
}

func getInvoiceByID(id int) (models.Invoice, error) {
	return scanInvoice(DB.QueryRow(invoiceSelectQuery+" WHERE i.id = ?", id))
}

// ListInvoices lists all invoices
// @Summary      List invoices
// @Description  Get a list of all receivable invoices, with current status and allocation info.
// @Tags         invoices
// @Produce      json
// @Param        contact_id   query     int  false  "Filter by contact (customer)"
// @Param        search       query     string  false  "Search by invoice number, notes, or customer name"
// @Success      200          {object}  Response{data=[]models.Invoice}
// @Router       /invoices [get]
// @Security     BasicAuth
func ListInvoices(w http.ResponseWriter, r *http.Request) {
	query := invoiceSelectQuery
	var conditions []string
	var args []any

	if s := r.URL.Query().Get("status"); s != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, s)
	}
	if cid := r.URL.Query().Get("contact_id"); cid != "" {
		conditions = append(conditions, "i.contact_id = ?")
		args = append(args, cid)
	}
	if from := r.URL.Query().Get("from"); from != "" {
		conditions = append(conditions, "i.issue_date >= ?")
		args = append(args, from)
	}
	if to := r.URL.Query().Get("to"); to != "" {
		conditions = append(conditions, "i.issue_date <= ?")
		args = append(args, to)
	}
	if search := r.URL.Query().Get("search"); search != "" {
		conditions = append(conditions, "(i.invoice_number LIKE ? OR i.notes LIKE ? OR c.name LIKE ?)")
		s := "%" + search + "%"
		args = append(args, s, s, s)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var invoices []models.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		invoices = append(invoices, inv)
	}
	if invoices == nil {
		invoices = []models.Invoice{}
	}
	writeJSON(w, http.StatusOK, invoices)
}

// GetInvoice retrieves a single invoice by ID
// @Summary      Get invoice
// @Description  Get details and allocation status of a specific invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=models.Invoice}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id} [get]
// @Security     BasicAuth
func GetInvoice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	inv, err := getInvoiceByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// CreateInvoice creates a new invoice
// @Summary      Create invoice
// @Description  Create a new receivable invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        invoice  body      models.InvoiceInput  true  "Invoice contents"
// @Success      201      {object}  Response{data=models.Invoice}
// @Failure      400      {object}  Response{error=string}
// @Router       /invoices [post]
// @Security     BasicAuth
func CreateInvoice(w http.ResponseWriter, r *http.Request) {
	var input models.InvoiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var id int
	err := DB.QueryRow(`INSERT INTO invoices (contact_id, invoice_number, issue_date, due_date, amount, status, file_url, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.ContactID, input.InvoiceNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	inv, err := getInvoiceByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created invoice: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

// UpdateInvoice updates an existing invoice
// @Summary      Update invoice
// @Description  Update details of an existing invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        id       path      int                  true  "Invoice ID"
// @Param        invoice  body      models.InvoiceInput  true  "Updated invoice contents"
// @Success      200      {object}  Response{data=models.Invoice}
// @Failure      400      {object}  Response{error=string}
// @Failure      404      {object}  Response{error=string}
// @Router       /invoices/{id} [put]
// @Security     BasicAuth
func UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.InvoiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	res, err := DB.Exec(`UPDATE invoices SET contact_id = ?, invoice_number = ?, issue_date = ?, due_date = ?,
		amount = ?, status = ?, file_url = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		input.ContactID, input.InvoiceNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	inv, err := getInvoiceByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated invoice: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// DeleteInvoice deletes an invoice
// @Summary      Delete invoice
// @Description  Remove an invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id} [delete]
// @Security     BasicAuth
func DeleteInvoice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Remove all transaction links for this invoice so transaction allocated amounts stay accurate.
	DB.Exec("DELETE FROM transaction_documents WHERE document_type = 'invoice' AND document_id = ?", id)

	res, err := DB.Exec("DELETE FROM invoices WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// GetInvoiceLinks retrieves all transactions associated with an invoice
// @Summary      Get invoice links
// @Description  Get all payment transactions linked to a specific invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=[]InvoiceLink}
// @Router       /invoices/{id}/links [get]
// @Security     BasicAuth
func GetInvoiceLinks(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	rows, err := DB.Query(`SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
		COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name as account_name
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE td.document_type = 'invoice' AND td.document_id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var links []InvoiceLink
	for rows.Next() {
		var l InvoiceLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		links = append(links, l)
	}
	if links == nil {
		links = []InvoiceLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

// InvoiceLink represents a linked transaction payment for an invoice.
type InvoiceLink struct {
	models.TransactionDocument
	TransactionDate string `json:"transaction_date"`
	Description     string `json:"description"`
	Reference       string `json:"reference"`
	AccountName     string `json:"account_name"`
}
