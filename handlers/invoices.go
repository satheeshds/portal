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

func loadInvoiceItems(invoiceID int) ([]models.InvoiceItem, error) {
	rows, err := DB.Query(`SELECT id, invoice_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM invoice_items WHERE invoice_id = ? ORDER BY id ASC`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.InvoiceItem
	for rows.Next() {
		var item models.InvoiceItem
		if err := rows.Scan(&item.ID, &item.InvoiceID, &item.Description, &item.Quantity,
			&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []models.InvoiceItem{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func getInvoiceByID(id int) (models.Invoice, error) {
	inv, err := scanInvoice(DB.QueryRow(invoiceSelectQuery+" WHERE i.id = ?", id))
	if err != nil {
		return inv, err
	}
	inv.Items, err = loadInvoiceItems(id)
	return inv, err
}

func insertInvoiceItems(tx *sql.Tx, invoiceID int, items []models.InvoiceItemInput) error {
	stmt, err := tx.Prepare(`INSERT INTO invoice_items (invoice_id, description, quantity, unit, unit_price, amount)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		if _, err := stmt.Exec(invoiceID, item.Description, item.Quantity, item.Unit, item.UnitPrice, item.Amount); err != nil {
			return err
		}
	}
	return nil
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
		inv.Items = []models.InvoiceItem{}
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

	tx, err := DB.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var id int
	err = tx.QueryRow(`INSERT INTO invoices (contact_id, invoice_number, issue_date, due_date, amount, status, file_url, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.ContactID, input.InvoiceNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := insertInvoiceItems(tx, id, input.Items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
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

	tx, err := DB.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	res, err := tx.Exec(`UPDATE invoices SET contact_id = ?, invoice_number = ?, issue_date = ?, due_date = ?,
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

	if input.Items != nil {
		if _, err := tx.Exec("DELETE FROM invoice_items WHERE invoice_id = ?", id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := insertInvoiceItems(tx, id, input.Items); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	tx, err := DB.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Remove all transaction links for this invoice so transaction allocated amounts stay accurate.
	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE document_type = 'invoice' AND document_id = ?", id); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Remove all line items for this invoice.
	if _, err := tx.Exec("DELETE FROM invoice_items WHERE invoice_id = ?", id); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := tx.Exec("DELETE FROM invoices WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	n, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		_ = tx.Rollback()
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

// ListInvoiceItems lists all line items for an invoice
// @Summary      List invoice items
// @Description  Get all line items for a specific invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=[]models.InvoiceItem}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id}/items [get]
// @Security     BasicAuth
func ListInvoiceItems(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Verify invoice exists.
	var exists bool
	err := DB.QueryRow("SELECT COUNT(*) > 0 FROM invoices WHERE id = ?", id).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	items, err := loadInvoiceItems(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CreateInvoiceItem creates a new line item for an invoice
// @Summary      Create invoice item
// @Description  Add a new line item to an existing invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        id    path      int                      true  "Invoice ID"
// @Param        item  body      models.InvoiceItemInput  true  "Line item contents"
// @Success      201   {object}  Response{data=models.InvoiceItem}
// @Failure      400   {object}  Response{error=string}
// @Failure      404   {object}  Response{error=string}
// @Router       /invoices/{id}/items [post]
// @Security     BasicAuth
func CreateInvoiceItem(w http.ResponseWriter, r *http.Request) {
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Verify invoice exists.
	var exists bool
	if err := DB.QueryRow("SELECT COUNT(*) > 0 FROM invoices WHERE id = ?", invoiceID).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify invoice existence: "+err.Error())
		return
	} else if !exists {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	var input models.InvoiceItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var itemID int
	err := DB.QueryRow(`INSERT INTO invoice_items (invoice_id, description, quantity, unit, unit_price, amount)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		invoiceID, input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount).Scan(&itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var item models.InvoiceItem
	err = DB.QueryRow(`SELECT id, invoice_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM invoice_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.InvoiceID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created item: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// UpdateInvoiceItem updates a line item for an invoice
// @Summary      Update invoice item
// @Description  Update an existing line item in an invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        id      path      int                      true  "Invoice ID"
// @Param        itemId  path      int                      true  "Item ID"
// @Param        item    body      models.InvoiceItemInput  true  "Updated line item contents"
// @Success      200     {object}  Response{data=models.InvoiceItem}
// @Failure      400     {object}  Response{error=string}
// @Failure      404     {object}  Response{error=string}
// @Router       /invoices/{id}/items/{itemId} [put]
// @Security     BasicAuth
func UpdateInvoiceItem(w http.ResponseWriter, r *http.Request) {
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	var input models.InvoiceItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	res, err := DB.Exec(`UPDATE invoice_items SET description = ?, quantity = ?, unit = ?, unit_price = ?, amount = ?,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND invoice_id = ?`,
		input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount, itemID, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "invoice item not found")
		return
	}

	var item models.InvoiceItem
	err = DB.QueryRow(`SELECT id, invoice_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM invoice_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.InvoiceID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated item: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// DeleteInvoiceItem deletes a line item from an invoice
// @Summary      Delete invoice item
// @Description  Remove a line item from an invoice.
// @Tags         invoices
// @Produce      json
// @Param        id      path      int  true  "Invoice ID"
// @Param        itemId  path      int  true  "Item ID"
// @Success      200     {object}  Response{data=map[string]string}
// @Failure      404     {object}  Response{error=string}
// @Router       /invoices/{id}/items/{itemId} [delete]
// @Security     BasicAuth
func DeleteInvoiceItem(w http.ResponseWriter, r *http.Request) {
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	res, err := DB.Exec("DELETE FROM invoice_items WHERE id = ? AND invoice_id = ?", itemID, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "invoice item not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
