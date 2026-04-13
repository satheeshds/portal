package store

import (
	"database/sql"
	"strings"

	"github.com/satheeshds/portal/db"
	"github.com/satheeshds/portal/models"
)

const invoiceSelectQuery = `SELECT i.id, i.contact_id, i.invoice_number, i.issue_date, i.due_date, i.amount,
		i.status, i.file_url, i.notes, i.created_at, i.updated_at,
		c.name,
		COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'invoice' AND td.document_id = i.id), 0)
		FROM invoices i
		LEFT JOIN contacts c ON i.contact_id = c.id`

// InvoiceLink represents a linked transaction payment for an invoice.
type InvoiceLink struct {
	models.TransactionDocument
	TransactionDate string `json:"transaction_date"`
	Description     string `json:"description"`
	Reference       string `json:"reference"`
	AccountName     string `json:"account_name"`
}

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

func (s *Store) getInvoiceByID(id int) (models.Invoice, error) {
	inv, err := scanInvoice(s.db.QueryRow(invoiceSelectQuery+" WHERE i.id = ?", id))
	if err != nil {
		return inv, err
	}
	inv.Items, err = s.ListInvoiceItems(id)
	return inv, err
}

func insertInvoiceItems(tx *db.PortalTx, invoiceID int, items []models.InvoiceItemInput) error {
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

// ListInvoices returns invoices filtered by the provided parameters (all may be empty).
func (s *Store) ListInvoices(status, contactID, from, to, search string) ([]models.Invoice, error) {
	query := invoiceSelectQuery
	var conditions []string
	var args []any

	if status != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, status)
	}
	if contactID != "" {
		conditions = append(conditions, "i.contact_id = ?")
		args = append(args, contactID)
	}
	if from != "" {
		conditions = append(conditions, "i.issue_date >= ?")
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, "i.issue_date <= ?")
		args = append(args, to)
	}
	if search != "" {
		conditions = append(conditions, "(i.invoice_number LIKE ? OR i.notes LIKE ? OR c.name LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY i.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []models.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		inv.Items = []models.InvoiceItem{}
		invoices = append(invoices, inv)
	}
	if invoices == nil {
		invoices = []models.Invoice{}
	}
	return invoices, nil
}

// GetInvoice returns a single invoice by ID, including its line items. Returns sql.ErrNoRows if not found.
func (s *Store) GetInvoice(id int) (models.Invoice, error) {
	return s.getInvoiceByID(id)
}

// CreateInvoice inserts a new invoice (and its items) in a transaction and returns the created record.
func (s *Store) CreateInvoice(input models.InvoiceInput) (models.Invoice, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Invoice{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int
	err = tx.QueryRow(`INSERT INTO invoices (contact_id, invoice_number, issue_date, due_date, amount, status, file_url, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.ContactID, input.InvoiceNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes).Scan(&id)
	if err != nil {
		return models.Invoice{}, err
	}

	if err := insertInvoiceItems(tx, id, input.Items); err != nil {
		return models.Invoice{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Invoice{}, err
	}
	return s.getInvoiceByID(id)
}

// UpdateInvoice updates an existing invoice and its items. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateInvoice(id int, input models.InvoiceInput) (models.Invoice, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Invoice{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`UPDATE invoices SET contact_id = ?, invoice_number = ?, issue_date = ?, due_date = ?,
		amount = ?, status = ?, file_url = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		input.ContactID, input.InvoiceNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes, id)
	if err != nil {
		return models.Invoice{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Invoice{}, sql.ErrNoRows
	}

	if input.Items != nil {
		if _, err := tx.Exec("DELETE FROM invoice_items WHERE invoice_id = ?", id); err != nil {
			return models.Invoice{}, err
		}
		if err := insertInvoiceItems(tx, id, input.Items); err != nil {
			return models.Invoice{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Invoice{}, err
	}
	return s.getInvoiceByID(id)
}

// DeleteInvoice removes an invoice and its items/links. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteInvoice(id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE document_type = 'invoice' AND document_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM invoice_items WHERE invoice_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}

	res, err := tx.Exec("DELETE FROM invoices WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if n == 0 {
		_ = tx.Rollback()
		return sql.ErrNoRows
	}

	return tx.Commit()
}

// GetInvoiceLinks returns all transaction links for the given invoice.
func (s *Store) GetInvoiceLinks(id int) ([]InvoiceLink, error) {
	rows, err := s.db.Query(`SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
		COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name as account_name
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE td.document_type = 'invoice' AND td.document_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []InvoiceLink
	for rows.Next() {
		var l InvoiceLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	if links == nil {
		links = []InvoiceLink{}
	}
	return links, nil
}

// InvoiceExists reports whether an invoice with the given ID exists.
func (s *Store) InvoiceExists(id int) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT COUNT(*) > 0 FROM invoices WHERE id = ?", id).Scan(&exists)
	return exists, err
}

// ListInvoiceItems returns all line items for an invoice.
func (s *Store) ListInvoiceItems(invoiceID int) ([]models.InvoiceItem, error) {
	rows, err := s.db.Query(`SELECT id, invoice_id, description, quantity, unit, unit_price, amount, created_at, updated_at
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

// CreateInvoiceItem inserts a new line item for an invoice and returns it.
func (s *Store) CreateInvoiceItem(invoiceID int, input models.InvoiceItemInput) (models.InvoiceItem, error) {
	var itemID int
	err := s.db.QueryRow(`INSERT INTO invoice_items (invoice_id, description, quantity, unit, unit_price, amount)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		invoiceID, input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount).Scan(&itemID)
	if err != nil {
		return models.InvoiceItem{}, err
	}

	var item models.InvoiceItem
	err = s.db.QueryRow(`SELECT id, invoice_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM invoice_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.InvoiceID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// UpdateInvoiceItem updates a line item. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateInvoiceItem(invoiceID, itemID int, input models.InvoiceItemInput) (models.InvoiceItem, error) {
	res, err := s.db.Exec(`UPDATE invoice_items SET description = ?, quantity = ?, unit = ?, unit_price = ?, amount = ?,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND invoice_id = ?`,
		input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount, itemID, invoiceID)
	if err != nil {
		return models.InvoiceItem{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.InvoiceItem{}, sql.ErrNoRows
	}

	var item models.InvoiceItem
	err = s.db.QueryRow(`SELECT id, invoice_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM invoice_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.InvoiceID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// DeleteInvoiceItem removes a line item. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteInvoiceItem(invoiceID, itemID int) error {
	res, err := s.db.Exec("DELETE FROM invoice_items WHERE id = ? AND invoice_id = ?", itemID, invoiceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
