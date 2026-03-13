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

func getBillByID(id int) (models.Bill, error) {
	return scanBill(DB.QueryRow(billSelectQuery+" WHERE b.id = ?", id))
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

	var id int
	err := DB.QueryRow(`INSERT INTO bills (contact_id, bill_number, issue_date, due_date, amount, status, file_url, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.ContactID, input.BillNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes).Scan(&id)
	if err != nil {
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

	res, err := DB.Exec(`UPDATE bills SET contact_id = ?, bill_number = ?, issue_date = ?, due_date = ?,
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

	// Remove all transaction links for this bill so transaction allocated amounts stay accurate.
	DB.Exec("DELETE FROM transaction_documents WHERE document_type = 'bill' AND document_id = ?", id)

	res, err := DB.Exec("DELETE FROM bills WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "bill not found")
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
