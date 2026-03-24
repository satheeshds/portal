package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/models"
)

const recurringPaymentSelectQuery = `SELECT r.id, r.name, r.type, r.amount, r.account_id, r.contact_id,
	r.frequency, r.interval, r.start_date, r.end_date, r.next_due_date, r.last_generated_date,
	r.status, r.description, r.reference, r.created_at, r.updated_at,
	a.name,
	c.name
	FROM recurring_payments r
	LEFT JOIN accounts a ON r.account_id = a.id
	LEFT JOIN contacts c ON r.contact_id = c.id`

func scanRecurringPayment(scanner interface{ Scan(...any) error }) (models.RecurringPayment, error) {
	var r models.RecurringPayment
	err := scanner.Scan(
		&r.ID, &r.Name, &r.Type, &r.Amount, &r.AccountID, &r.ContactID,
		&r.Frequency, &r.Interval, &r.StartDate, &r.EndDate, &r.NextDueDate, &r.LastGeneratedDate,
		&r.Status, &r.Description, &r.Reference, &r.CreatedAt, &r.UpdatedAt,
		&r.AccountName, &r.ContactName,
	)
	return r, err
}

func getRecurringPaymentByID(id int) (models.RecurringPayment, error) {
	return scanRecurringPayment(DB.QueryRow(recurringPaymentSelectQuery+" WHERE r.id = ?", id))
}

// ListRecurringPayments lists all recurring payments
// @Summary      List recurring payments
// @Description  Get a list of all scheduled recurring payments (income or expense).
// @Tags         recurring_payments
// @Produce      json
// @Param        status      query  string  false  "Filter by status (active, paused, cancelled, completed)"
// @Param        account_id  query  int     false  "Filter by account"
// @Param        type        query  string  false  "Filter by type (income, expense)"
// @Success      200  {object}  Response{data=[]models.RecurringPayment}
// @Router       /recurring-payments [get]
// @Security     BasicAuth
func ListRecurringPayments(w http.ResponseWriter, r *http.Request) {
	query := recurringPaymentSelectQuery
	var conditions []string
	var args []any

	if s := r.URL.Query().Get("status"); s != "" {
		conditions = append(conditions, "r.status = ?")
		args = append(args, s)
	}
	if aid := r.URL.Query().Get("account_id"); aid != "" {
		accountID, err := strconv.Atoi(aid)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid account_id")
			return
		}
		conditions = append(conditions, "r.account_id = ?")
		args = append(args, accountID)
	}
	if tp := r.URL.Query().Get("type"); tp != "" {
		conditions = append(conditions, "r.type = ?")
		args = append(args, tp)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY r.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var payments []models.RecurringPayment
	for rows.Next() {
		p, err := scanRecurringPayment(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payments = append(payments, p)
	}
	if payments == nil {
		payments = []models.RecurringPayment{}
	}
	writeJSON(w, http.StatusOK, payments)
}

// GetRecurringPayment retrieves a single recurring payment by ID
// @Summary      Get recurring payment
// @Description  Get details of a specific recurring payment.
// @Tags         recurring_payments
// @Produce      json
// @Param        id   path      int  true  "Recurring Payment ID"
// @Success      200  {object}  Response{data=models.RecurringPayment}
// @Failure      404  {object}  Response{error=string}
// @Router       /recurring-payments/{id} [get]
// @Security     BasicAuth
func GetRecurringPayment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	p, err := getRecurringPaymentByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recurring payment not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CreateRecurringPayment creates a new recurring payment
// @Summary      Create recurring payment
// @Description  Create a new scheduled recurring payment.
// @Tags         recurring_payments
// @Accept       json
// @Produce      json
// @Param        recurring_payment  body      models.RecurringPaymentInput  true  "Recurring payment details"
// @Success      201                {object}  Response{data=models.RecurringPayment}
// @Failure      400                {object}  Response{error=string}
// @Router       /recurring-payments [post]
// @Security     BasicAuth
func CreateRecurringPayment(w http.ResponseWriter, r *http.Request) {
	var input models.RecurringPaymentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var id int
	err := DB.QueryRow(`INSERT INTO recurring_payments
		(name, type, amount, account_id, contact_id, frequency, interval, start_date, end_date, next_due_date, status, description, reference)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.Name, input.Type, input.Amount, input.AccountID, input.ContactID,
		input.Frequency, input.Interval, input.StartDate, input.EndDate, input.NextDueDate,
		input.Status, input.Description, input.Reference).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p, err := getRecurringPaymentByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created recurring payment: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// UpdateRecurringPayment updates an existing recurring payment
// @Summary      Update recurring payment
// @Description  Update details of an existing recurring payment.
// @Tags         recurring_payments
// @Accept       json
// @Produce      json
// @Param        id                 path      int                          true  "Recurring Payment ID"
// @Param        recurring_payment  body      models.RecurringPaymentInput true  "Updated recurring payment details"
// @Success      200  {object}  Response{data=models.RecurringPayment}
// @Failure      400  {object}  Response{error=string}
// @Failure      404  {object}  Response{error=string}
// @Router       /recurring-payments/{id} [put]
// @Security     BasicAuth
func UpdateRecurringPayment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.RecurringPaymentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	res, err := DB.Exec(`UPDATE recurring_payments SET
		name = ?, type = ?, amount = ?, account_id = ?, contact_id = ?,
		frequency = ?, interval = ?, start_date = ?, end_date = ?, next_due_date = ?, last_generated_date = ?,
		status = ?, description = ?, reference = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		input.Name, input.Type, input.Amount, input.AccountID, input.ContactID,
		input.Frequency, input.Interval, input.StartDate, input.EndDate, input.NextDueDate, input.LastGeneratedDate,
		input.Status, input.Description, input.Reference, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "recurring payment not found")
		return
	}

	p, err := getRecurringPaymentByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated recurring payment: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DeleteRecurringPayment deletes a recurring payment
// @Summary      Delete recurring payment
// @Description  Remove a recurring payment.
// @Tags         recurring_payments
// @Produce      json
// @Param        id   path      int  true  "Recurring Payment ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /recurring-payments/{id} [delete]
// @Security     BasicAuth
func DeleteRecurringPayment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	res, err := DB.Exec("DELETE FROM recurring_payments WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "recurring payment not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// GetRecurringPaymentLinks retrieves all transactions linked to a recurring payment, with occurrence details
// @Summary      Get recurring payment links
// @Description  Get all payment transactions linked to a specific recurring payment, including the due date and status of the occurrence each transaction covers.
// @Tags         recurring_payments
// @Produce      json
// @Param        id   path      int  true  "Recurring Payment ID"
// @Success      200  {object}  Response{data=[]RecurringPaymentLink}
// @Router       /recurring-payments/{id}/links [get]
// @Security     BasicAuth
func GetRecurringPaymentLinks(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	rows, err := DB.Query(`
		SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
			COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name,
			rpo.due_date, rpo.status
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		JOIN recurring_payment_occurrences rpo
			ON td.document_type = 'recurring_payment_occurrence' AND td.document_id = rpo.id
		WHERE rpo.recurring_payment_id = ?
		ORDER BY rpo.due_date DESC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var links []RecurringPaymentLink
	for rows.Next() {
		var l RecurringPaymentLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName,
			&l.OccurrenceDueDate, &l.OccurrenceStatus); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		links = append(links, l)
	}
	if links == nil {
		links = []RecurringPaymentLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

// RecurringPaymentLink represents a linked transaction payment for a recurring payment, at the occurrence level.
type RecurringPaymentLink struct {
	models.TransactionDocument
	TransactionDate   string `json:"transaction_date"`
	Description       string `json:"description"`
	Reference         string `json:"reference"`
	AccountName       string `json:"account_name"`
	OccurrenceDueDate string `json:"occurrence_due_date"` // the billing period this transaction covers
	OccurrenceStatus  string `json:"occurrence_status"`   // pending, paid, or skipped
}

// GetRecurringPaymentOccurrences lists all auto-generated occurrences for a recurring payment.
// @Summary      List recurring payment occurrences
// @Description  Returns all scheduled occurrences (instances) for a recurring payment. Occurrences are auto-generated by the server for each due date up to today. Each occurrence tracks whether it has been paid via a linked bank transaction.
// @Tags         recurring_payments
// @Produce      json
// @Param        id      path   int     true   "Recurring Payment ID"
// @Param        status  query  string  false  "Filter by status (pending, paid, skipped)"
// @Success      200  {object}  Response{data=[]models.RecurringPaymentOccurrence}
// @Failure      404  {object}  Response{error=string}
// @Router       /recurring-payments/{id}/occurrences [get]
// @Security     BasicAuth
func GetRecurringPaymentOccurrences(w http.ResponseWriter, r *http.Request) {
	rpID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Verify the recurring payment exists
	var exists bool
	if err := DB.QueryRow("SELECT COUNT(*) > 0 FROM recurring_payments WHERE id = ?", rpID).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "recurring payment not found")
		return
	}

	query := `
		SELECT o.id, o.recurring_payment_id, o.due_date, o.amount, o.status,
			o.created_at, o.updated_at,
			COALESCE(SUM(td.amount), 0) AS allocated,
			r.name
		FROM recurring_payment_occurrences o
		JOIN recurring_payments r ON o.recurring_payment_id = r.id
		LEFT JOIN transaction_documents td
			ON td.document_type = 'recurring_payment_occurrence' AND td.document_id = o.id
		WHERE o.recurring_payment_id = ?`
	args := []any{rpID}

	if s := r.URL.Query().Get("status"); s != "" {
		query += " AND o.status = ?"
		args = append(args, s)
	}
	query += " GROUP BY o.id, o.recurring_payment_id, o.due_date, o.amount, o.status, o.created_at, o.updated_at, r.name"
	query += " ORDER BY o.due_date DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var occurrences []models.RecurringPaymentOccurrence
	for rows.Next() {
		var o models.RecurringPaymentOccurrence
		if err := rows.Scan(&o.ID, &o.RecurringPaymentID, &o.DueDate, &o.Amount, &o.Status,
			&o.CreatedAt, &o.UpdatedAt, &o.Allocated, &o.PaymentName); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		o.Unallocated = models.Money(int64(o.Amount) - int64(o.Allocated))
		occurrences = append(occurrences, o)
	}
	if occurrences == nil {
		occurrences = []models.RecurringPaymentOccurrence{}
	}
	writeJSON(w, http.StatusOK, occurrences)
}
