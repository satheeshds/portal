package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/db"
	"github.com/satheeshds/portal/models"
)

const billSelectQuery = `SELECT b.id, b.contact_id, b.bill_number, b.issue_date, b.due_date, b.amount,
		b.status, b.file_url, b.notes, b.created_at, b.updated_at,
		c.name,
		COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'bill' AND td.document_id = b.id), 0)
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id`

func scanBill(scanner interface{ Scan(...any) error }) (models.Bill, error) {
	var b models.Bill
	err := scanner.Scan(&b.ID, &b.ContactID, &b.BillNumber, &b.IssueDate, &b.DueDate,
		&b.Amount, &b.Status, &b.FileURL, &b.Notes, &b.CreatedAt, &b.UpdatedAt,
		&b.ContactName, &b.Allocated)
	if err == nil {
		b.Unallocated = models.Money(int64(b.Amount) - int64(b.Allocated))
	}
	return b, err
}

func loadBillItems(billID int) ([]models.BillItem, error) {
	rows, err := DB.Query(`SELECT id, bill_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM bill_items WHERE bill_id = ? ORDER BY id ASC`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.BillItem
	for rows.Next() {
		var item models.BillItem
		if err := rows.Scan(&item.ID, &item.BillID, &item.Description, &item.Quantity,
			&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []models.BillItem{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func getBillByID(id int) (models.Bill, error) {
	b, err := scanBill(DB.QueryRow(billSelectQuery+" WHERE b.id = ?", id))
	if err != nil {
		return b, err
	}
	b.Items, err = loadBillItems(id)
	return b, err
}

func insertBillItems(tx *db.PortalTx, billID int, items []models.BillItemInput) error {
	stmt, err := tx.Prepare(`INSERT INTO bill_items (bill_id, description, quantity, unit, unit_price, amount)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		if _, err := stmt.Exec(billID, item.Description, item.Quantity, item.Unit, item.UnitPrice, item.Amount); err != nil {
			return err
		}
	}
	return nil
}

// ListBills lists all bills
// @Summary      List bills
// @Description  Get a list of all payable bills, with current status and allocation info.
// @Tags         bills
// @Produce      json
// @Param        contact_id   query     int  false  "Filter by contact (vendor)"
// @Param        search       query     string  false  "Search by bill number, notes, or vendor name"
// @Success      200          {object}  Response{data=[]models.Bill}
// @Router       /bills [get]
// @Security     BasicAuth
func ListBills(w http.ResponseWriter, r *http.Request) {
	query := billSelectQuery
	var conditions []string
	var args []any

	if s := r.URL.Query().Get("status"); s != "" {
		conditions = append(conditions, "b.status = ?")
		args = append(args, s)
	}
	if cid := r.URL.Query().Get("contact_id"); cid != "" {
		conditions = append(conditions, "b.contact_id = ?")
		args = append(args, cid)
	}
	if from := r.URL.Query().Get("from"); from != "" {
		conditions = append(conditions, "b.issue_date >= ?")
		args = append(args, from)
	}
	if to := r.URL.Query().Get("to"); to != "" {
		conditions = append(conditions, "b.issue_date <= ?")
		args = append(args, to)
	}
	if search := r.URL.Query().Get("search"); search != "" {
		conditions = append(conditions, "(b.bill_number LIKE ? OR b.notes LIKE ? OR c.name LIKE ?)")
		s := "%" + search + "%"
		args = append(args, s, s, s)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY b.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var bills []models.Bill
	for rows.Next() {
		b, err := scanBill(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		b.Items = []models.BillItem{}
		bills = append(bills, b)
	}
	if bills == nil {
		bills = []models.Bill{}
	}
	writeJSON(w, http.StatusOK, bills)
}

// GetBill retrieves a single bill by ID
// @Summary      Get bill
// @Description  Get details and allocation status of a specific bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=models.Bill}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id} [get]
// @Security     BasicAuth
func GetBill(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	b, err := getBillByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bill not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// CreateBill creates a new bill
// @Summary      Create bill
// @Description  Create a new payable bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        bill  body      models.BillInput  true  "Bill contents"
// @Success      201   {object}  Response{data=models.Bill}
// @Failure      400   {object}  Response{error=string}
// @Router       /bills [post]
// @Security     BasicAuth
func CreateBill(w http.ResponseWriter, r *http.Request) {
	var input models.BillInput
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
	err = tx.QueryRow(`INSERT INTO bills (contact_id, bill_number, issue_date, due_date, amount, status, file_url, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.ContactID, input.BillNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := insertBillItems(tx, id, input.Items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	b, err := getBillByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created bill: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// UpdateBill updates an existing bill
// @Summary      Update bill
// @Description  Update details of an existing bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        id    path      int               true  "Bill ID"
// @Param        bill  body      models.BillInput  true  "Updated bill contents"
// @Success      200   {object}  Response{data=models.Bill}
// @Failure      400   {object}  Response{error=string}
// @Failure      404   {object}  Response{error=string}
// @Router       /bills/{id} [put]
// @Security     BasicAuth
func UpdateBill(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.BillInput
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

	res, err := tx.Exec(`UPDATE bills SET contact_id = ?, bill_number = ?, issue_date = ?, due_date = ?,
		amount = ?, status = ?, file_url = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		input.ContactID, input.BillNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	if input.Items != nil {
		if _, err := tx.Exec("DELETE FROM bill_items WHERE bill_id = ?", id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := insertBillItems(tx, id, input.Items); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	b, err := getBillByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated bill: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// DeleteBill deletes a bill
// @Summary      Delete bill
// @Description  Remove a bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id} [delete]
// @Security     BasicAuth
func DeleteBill(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tx, err := DB.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Remove all transaction links for this bill so transaction allocated amounts stay accurate.
	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE document_type = 'bill' AND document_id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Remove all line items for this bill.
	if _, err := tx.Exec("DELETE FROM bill_items WHERE bill_id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := tx.Exec("DELETE FROM bills WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// GetBillLinks retrieves all transactions associated with a bill
// @Summary      Get bill links
// @Description  Get all payment transactions linked to a specific bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=[]BillLink}
// @Router       /bills/{id}/links [get]
// @Security     BasicAuth
func GetBillLinks(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	rows, err := DB.Query(`SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
		COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name as account_name
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE td.document_type = 'bill' AND td.document_id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var links []BillLink
	for rows.Next() {
		var l BillLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		links = append(links, l)
	}
	if links == nil {
		links = []BillLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

// BillLink represents a linked transaction payment for a bill.
type BillLink struct {
	models.TransactionDocument
	TransactionDate string `json:"transaction_date"`
	Description     string `json:"description"`
	Reference       string `json:"reference"`
	AccountName     string `json:"account_name"`
}

// ListBillItems lists all line items for a bill
// @Summary      List bill items
// @Description  Get all line items for a specific bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=[]models.BillItem}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id}/items [get]
// @Security     BasicAuth
func ListBillItems(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Verify bill exists.
	var exists bool
	err := DB.QueryRow("SELECT COUNT(*) > 0 FROM bills WHERE id = ?", id).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	items, err := loadBillItems(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CreateBillItem creates a new line item for a bill
// @Summary      Create bill item
// @Description  Add a new line item to an existing bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        id    path      int                    true  "Bill ID"
// @Param        item  body      models.BillItemInput   true  "Line item contents"
// @Success      201   {object}  Response{data=models.BillItem}
// @Failure      400   {object}  Response{error=string}
// @Failure      404   {object}  Response{error=string}
// @Router       /bills/{id}/items [post]
// @Security     BasicAuth
func CreateBillItem(w http.ResponseWriter, r *http.Request) {
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Verify bill exists.
	var exists bool
	err := DB.QueryRow("SELECT COUNT(*) > 0 FROM bills WHERE id = ?", billID).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify bill existence: "+err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	var input models.BillItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var itemID int
	err = DB.QueryRow(`INSERT INTO bill_items (bill_id, description, quantity, unit, unit_price, amount)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		billID, input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount).Scan(&itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var item models.BillItem
	err = DB.QueryRow(`SELECT id, bill_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM bill_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.BillID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created item: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// UpdateBillItem updates a line item for a bill
// @Summary      Update bill item
// @Description  Update an existing line item in a bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        id      path      int                   true  "Bill ID"
// @Param        itemId  path      int                   true  "Item ID"
// @Param        item    body      models.BillItemInput  true  "Updated line item contents"
// @Success      200     {object}  Response{data=models.BillItem}
// @Failure      400     {object}  Response{error=string}
// @Failure      404     {object}  Response{error=string}
// @Router       /bills/{id}/items/{itemId} [put]
// @Security     BasicAuth
func UpdateBillItem(w http.ResponseWriter, r *http.Request) {
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	var input models.BillItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	res, err := DB.Exec(`UPDATE bill_items SET description = ?, quantity = ?, unit = ?, unit_price = ?, amount = ?,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND bill_id = ?`,
		input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount, itemID, billID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "bill item not found")
		return
	}

	var item models.BillItem
	err = DB.QueryRow(`SELECT id, bill_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM bill_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.BillID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated item: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// DeleteBillItem deletes a line item from a bill
// @Summary      Delete bill item
// @Description  Remove a line item from a bill.
// @Tags         bills
// @Produce      json
// @Param        id      path      int  true  "Bill ID"
// @Param        itemId  path      int  true  "Item ID"
// @Success      200     {object}  Response{data=map[string]string}
// @Failure      404     {object}  Response{error=string}
// @Router       /bills/{id}/items/{itemId} [delete]
// @Security     BasicAuth
func DeleteBillItem(w http.ResponseWriter, r *http.Request) {
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	res, err := DB.Exec("DELETE FROM bill_items WHERE id = ? AND bill_id = ?", itemID, billID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "bill item not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
