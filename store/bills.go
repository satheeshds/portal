package store

import (
	"database/sql"
	"strings"

	"github.com/satheeshds/portal/db"
	"github.com/satheeshds/portal/models"
)

const billSelectQuery = `SELECT b.id, b.contact_id, b.bill_number, b.issue_date, b.due_date, b.amount,
		b.status, b.file_url, b.notes, b.created_at, b.updated_at,
		c.name,
		COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'bill' AND td.document_id = b.id), 0)
		FROM bills b
		LEFT JOIN contacts c ON b.contact_id = c.id`

// BillLink represents a linked transaction payment for a bill.
type BillLink struct {
	models.TransactionDocument
	TransactionDate string `json:"transaction_date"`
	Description     string `json:"description"`
	Reference       string `json:"reference"`
	AccountName     string `json:"account_name"`
}

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

func (s *Store) getBillByID(id int) (models.Bill, error) {
	b, err := scanBill(s.db.QueryRow(billSelectQuery+" WHERE b.id = ?", id))
	if err != nil {
		return b, err
	}
	b.Items, err = s.ListBillItems(id)
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

// ListBills returns bills filtered by the provided parameters (all may be empty).
func (s *Store) ListBills(status, contactID, from, to, search string) ([]models.Bill, error) {
	query := billSelectQuery
	var conditions []string
	var args []any

	if status != "" {
		conditions = append(conditions, "b.status = ?")
		args = append(args, status)
	}
	if contactID != "" {
		conditions = append(conditions, "b.contact_id = ?")
		args = append(args, contactID)
	}
	if from != "" {
		conditions = append(conditions, "b.issue_date >= ?")
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, "b.issue_date <= ?")
		args = append(args, to)
	}
	if search != "" {
		conditions = append(conditions, "(b.bill_number LIKE ? OR b.notes LIKE ? OR c.name LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY b.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bills []models.Bill
	for rows.Next() {
		b, err := scanBill(rows)
		if err != nil {
			return nil, err
		}
		b.Items = []models.BillItem{}
		bills = append(bills, b)
	}
	if bills == nil {
		bills = []models.Bill{}
	}
	return bills, nil
}

// GetBill returns a single bill by ID, including its line items. Returns sql.ErrNoRows if not found.
func (s *Store) GetBill(id int) (models.Bill, error) {
	return s.getBillByID(id)
}

// CreateBill inserts a new bill (and its items) in a transaction and returns the created record.
func (s *Store) CreateBill(input models.BillInput) (models.Bill, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Bill{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var id int
	err = tx.QueryRow(`INSERT INTO bills (contact_id, bill_number, issue_date, due_date, amount, status, file_url, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.ContactID, input.BillNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes).Scan(&id)
	if err != nil {
		return models.Bill{}, err
	}

	if err := insertBillItems(tx, id, input.Items); err != nil {
		return models.Bill{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Bill{}, err
	}
	return s.getBillByID(id)
}

// UpdateBill updates an existing bill and its items. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateBill(id int, input models.BillInput) (models.Bill, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Bill{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`UPDATE bills SET contact_id = ?, bill_number = ?, issue_date = ?, due_date = ?,
		amount = ?, status = ?, file_url = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		input.ContactID, input.BillNumber, input.IssueDate, input.DueDate,
		input.Amount, input.Status, input.FileURL, input.Notes, id)
	if err != nil {
		return models.Bill{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Bill{}, sql.ErrNoRows
	}

	if input.Items != nil {
		if _, err := tx.Exec("DELETE FROM bill_items WHERE bill_id = ?", id); err != nil {
			return models.Bill{}, err
		}
		if err := insertBillItems(tx, id, input.Items); err != nil {
			return models.Bill{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Bill{}, err
	}
	return s.getBillByID(id)
}

// DeleteBill removes a bill and its items/links. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteBill(id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE document_type = 'bill' AND document_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM bill_items WHERE bill_id = ?", id); err != nil {
		return err
	}

	res, err := tx.Exec("DELETE FROM bills WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

// GetBillLinks returns all transaction links for the given bill.
func (s *Store) GetBillLinks(id int) ([]BillLink, error) {
	rows, err := s.db.Query(`SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
		COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name as account_name
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE td.document_type = 'bill' AND td.document_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []BillLink
	for rows.Next() {
		var l BillLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	if links == nil {
		links = []BillLink{}
	}
	return links, nil
}

// BillExists reports whether a bill with the given ID exists.
func (s *Store) BillExists(id int) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT COUNT(*) > 0 FROM bills WHERE id = ?", id).Scan(&exists)
	return exists, err
}

// ListBillItems returns all line items for a bill.
func (s *Store) ListBillItems(billID int) ([]models.BillItem, error) {
	rows, err := s.db.Query(`SELECT id, bill_id, description, quantity, unit, unit_price, amount, created_at, updated_at
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

// CreateBillItem inserts a new line item for a bill and returns it.
func (s *Store) CreateBillItem(billID int, input models.BillItemInput) (models.BillItem, error) {
	var itemID int
	err := s.db.QueryRow(`INSERT INTO bill_items (bill_id, description, quantity, unit, unit_price, amount)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		billID, input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount).Scan(&itemID)
	if err != nil {
		return models.BillItem{}, err
	}

	var item models.BillItem
	err = s.db.QueryRow(`SELECT id, bill_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM bill_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.BillID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// UpdateBillItem updates a line item. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateBillItem(billID, itemID int, input models.BillItemInput) (models.BillItem, error) {
	res, err := s.db.Exec(`UPDATE bill_items SET description = ?, quantity = ?, unit = ?, unit_price = ?, amount = ?,
		updated_at = CURRENT_TIMESTAMP WHERE id = ? AND bill_id = ?`,
		input.Description, input.Quantity, input.Unit, input.UnitPrice, input.Amount, itemID, billID)
	if err != nil {
		return models.BillItem{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.BillItem{}, sql.ErrNoRows
	}

	var item models.BillItem
	err = s.db.QueryRow(`SELECT id, bill_id, description, quantity, unit, unit_price, amount, created_at, updated_at
		FROM bill_items WHERE id = ?`, itemID).Scan(
		&item.ID, &item.BillID, &item.Description, &item.Quantity,
		&item.Unit, &item.UnitPrice, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// DeleteBillItem removes a line item. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteBillItem(billID, itemID int) error {
	res, err := s.db.Exec("DELETE FROM bill_items WHERE id = ? AND bill_id = ?", itemID, billID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
