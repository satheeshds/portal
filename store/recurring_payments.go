package store

import (
	"database/sql"
	"strings"

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

// RecurringPaymentLink represents a linked transaction payment for a recurring payment occurrence.
type RecurringPaymentLink struct {
	models.TransactionDocument
	TransactionDate   string `json:"transaction_date"`
	Description       string `json:"description"`
	Reference         string `json:"reference"`
	AccountName       string `json:"account_name"`
	OccurrenceDueDate string `json:"occurrence_due_date"`
	OccurrenceStatus  string `json:"occurrence_status"`
}

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

func (s *Store) getRecurringPaymentByID(id int) (models.RecurringPayment, error) {
	return scanRecurringPayment(s.db.QueryRow(recurringPaymentSelectQuery+" WHERE r.id = ?", id))
}

// ListRecurringPayments returns recurring payments filtered by the provided parameters.
func (s *Store) ListRecurringPayments(status, accountID, rpType string) ([]models.RecurringPayment, error) {
	query := recurringPaymentSelectQuery
	var conditions []string
	var args []any

	if status != "" {
		conditions = append(conditions, "r.status = ?")
		args = append(args, status)
	}
	if accountID != "" {
		conditions = append(conditions, "r.account_id = ?")
		args = append(args, accountID)
	}
	if rpType != "" {
		conditions = append(conditions, "r.type = ?")
		args = append(args, rpType)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY r.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []models.RecurringPayment
	for rows.Next() {
		p, err := scanRecurringPayment(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	if payments == nil {
		payments = []models.RecurringPayment{}
	}
	return payments, nil
}

// GetRecurringPayment returns a single recurring payment by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetRecurringPayment(id int) (models.RecurringPayment, error) {
	return s.getRecurringPaymentByID(id)
}

// CreateRecurringPayment inserts a new recurring payment and returns it.
func (s *Store) CreateRecurringPayment(input models.RecurringPaymentInput) (models.RecurringPayment, error) {
	var id int
	err := s.db.QueryRow(`INSERT INTO recurring_payments
		(name, type, amount, account_id, contact_id, frequency, interval, start_date, end_date, next_due_date, status, description, reference)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.Name, input.Type, input.Amount, input.AccountID, input.ContactID,
		input.Frequency, input.Interval, input.StartDate, input.EndDate, input.NextDueDate,
		input.Status, input.Description, input.Reference).Scan(&id)
	if err != nil {
		return models.RecurringPayment{}, err
	}
	return s.getRecurringPaymentByID(id)
}

// UpdateRecurringPayment updates an existing recurring payment. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateRecurringPayment(id int, input models.RecurringPaymentInput) (models.RecurringPayment, error) {
	res, err := s.db.Exec(`UPDATE recurring_payments SET
		name = ?, type = ?, amount = ?, account_id = ?, contact_id = ?,
		frequency = ?, interval = ?, start_date = ?, end_date = ?, next_due_date = ?, last_generated_date = ?,
		status = ?, description = ?, reference = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		input.Name, input.Type, input.Amount, input.AccountID, input.ContactID,
		input.Frequency, input.Interval, input.StartDate, input.EndDate, input.NextDueDate, input.LastGeneratedDate,
		input.Status, input.Description, input.Reference, id)
	if err != nil {
		return models.RecurringPayment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.RecurringPayment{}, sql.ErrNoRows
	}
	return s.getRecurringPaymentByID(id)
}

// DeleteRecurringPayment removes a recurring payment. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteRecurringPayment(id int) error {
	res, err := s.db.Exec("DELETE FROM recurring_payments WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetRecurringPaymentLinks returns all transaction links for the given recurring payment.
func (s *Store) GetRecurringPaymentLinks(id int) ([]RecurringPaymentLink, error) {
	rows, err := s.db.Query(`
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
		return nil, err
	}
	defer rows.Close()

	var links []RecurringPaymentLink
	for rows.Next() {
		var l RecurringPaymentLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName,
			&l.OccurrenceDueDate, &l.OccurrenceStatus); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	if links == nil {
		links = []RecurringPaymentLink{}
	}
	return links, nil
}

// RecurringPaymentExists reports whether a recurring payment with the given ID exists.
func (s *Store) RecurringPaymentExists(id int) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT COUNT(*) > 0 FROM recurring_payments WHERE id = ?", id).Scan(&exists)
	return exists, err
}

// ListRecurringPaymentOccurrences returns occurrences for a recurring payment, optionally filtered by status.
func (s *Store) ListRecurringPaymentOccurrences(rpID int, statusFilter string) ([]models.RecurringPaymentOccurrence, error) {
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

	if statusFilter != "" {
		query += " AND o.status = ?"
		args = append(args, statusFilter)
	}
	query += " GROUP BY o.id, o.recurring_payment_id, o.due_date, o.amount, o.status, o.created_at, o.updated_at, r.name"
	query += " ORDER BY o.due_date DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var occurrences []models.RecurringPaymentOccurrence
	for rows.Next() {
		var o models.RecurringPaymentOccurrence
		if err := rows.Scan(&o.ID, &o.RecurringPaymentID, &o.DueDate, &o.Amount, &o.Status,
			&o.CreatedAt, &o.UpdatedAt, &o.Allocated, &o.PaymentName); err != nil {
			return nil, err
		}
		o.Unallocated = models.Money(int64(o.Amount) - int64(o.Allocated))
		occurrences = append(occurrences, o)
	}
	if occurrences == nil {
		occurrences = []models.RecurringPaymentOccurrence{}
	}
	return occurrences, nil
}
