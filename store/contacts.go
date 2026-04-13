package store

import (
	"database/sql"
	"strings"

	"github.com/satheeshds/portal/models"
)

const contactSelectQuery = `SELECT id, name, type, email, phone, created_at, updated_at,
	CASE 
		WHEN type = 'vendor' THEN COALESCE((SELECT SUM(amount) FROM bills WHERE contact_id = contacts.id), 0)
		WHEN type = 'customer' THEN COALESCE((SELECT SUM(amount) FROM invoices WHERE contact_id = contacts.id), 0)
		ELSE 0
	END as total_amount,
	CASE 
		WHEN type = 'vendor' THEN COALESCE((SELECT SUM(td.amount) FROM transaction_documents td JOIN bills b ON td.document_id = b.id WHERE td.document_type = 'bill' AND b.contact_id = contacts.id), 0)
		WHEN type = 'customer' THEN COALESCE((SELECT SUM(td.amount) FROM transaction_documents td JOIN invoices i ON td.document_id = i.id WHERE td.document_type = 'invoice' AND i.contact_id = contacts.id), 0)
		ELSE 0
	END as allocated_amount
	FROM contacts`

func scanContact(scanner interface{ Scan(...any) error }) (models.Contact, error) {
	var c models.Contact
	err := scanner.Scan(&c.ID, &c.Name, &c.Type, &c.Email, &c.Phone, &c.CreatedAt, &c.UpdatedAt, &c.TotalAmount, &c.AllocatedAmount)
	c.Balance = c.TotalAmount - c.AllocatedAmount
	return c, err
}

// ListContacts returns contacts, optionally filtered by type and/or search term.
func (s *Store) ListContacts(typeFilter, search string) ([]models.Contact, error) {
	query := contactSelectQuery
	var args []any
	var conditions []string

	if typeFilter != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, typeFilter)
	}
	if search != "" {
		conditions = append(conditions, "(name LIKE ? OR email LIKE ? OR phone LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY name"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []models.Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	if contacts == nil {
		contacts = []models.Contact{}
	}
	return contacts, nil
}

// GetContact returns a single contact by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetContact(id int) (models.Contact, error) {
	return scanContact(s.db.QueryRow(contactSelectQuery+" WHERE id = ?", id))
}

// CreateContact inserts a new contact and returns the created record.
func (s *Store) CreateContact(input models.ContactInput) (models.Contact, error) {
	var id int
	err := s.db.QueryRow("INSERT INTO contacts (name, type, email, phone) VALUES (?, ?, ?, ?) RETURNING id",
		input.Name, input.Type, input.Email, input.Phone).Scan(&id)
	if err != nil {
		return models.Contact{}, err
	}
	return scanContact(s.db.QueryRow(contactSelectQuery+" WHERE id = ?", id))
}

// UpdateContact updates an existing contact. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateContact(id int, input models.ContactInput) (models.Contact, error) {
	res, err := s.db.Exec("UPDATE contacts SET name = ?, type = ?, email = ?, phone = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		input.Name, input.Type, input.Email, input.Phone, id)
	if err != nil {
		return models.Contact{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Contact{}, sql.ErrNoRows
	}
	return scanContact(s.db.QueryRow(contactSelectQuery+" WHERE id = ?", id))
}

// DeleteContact removes a contact. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteContact(id int) error {
	res, err := s.db.Exec("DELETE FROM contacts WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
