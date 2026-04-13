package store

import (
	"database/sql"

	"github.com/satheeshds/portal/models"
)

const accountSelectQuery = `SELECT id, name, type, opening_balance, created_at, updated_at,
	(opening_balance + 
	 COALESCE((SELECT SUM(amount) FROM transactions WHERE account_id = accounts.id AND type = 'income'), 0) -
	 COALESCE((SELECT SUM(amount) FROM transactions WHERE account_id = accounts.id AND type = 'expense'), 0)
	) as balance
	FROM accounts`

func scanAccount(scanner interface{ Scan(...any) error }) (models.Account, error) {
	var a models.Account
	err := scanner.Scan(&a.ID, &a.Name, &a.Type, &a.OpeningBalance, &a.CreatedAt, &a.UpdatedAt, &a.Balance)
	return a, err
}

func (s *Store) getAccountByID(id int) (models.Account, error) {
	return scanAccount(s.db.QueryRow(accountSelectQuery+" WHERE accounts.id = ?", id))
}

// ListAccounts returns all accounts, optionally filtered by search.
func (s *Store) ListAccounts(search string) ([]models.Account, error) {
	query := accountSelectQuery
	var args []any
	if search != "" {
		query += " WHERE name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	query += " ORDER BY name"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []models.Account{}
	}
	return accounts, nil
}

// GetAccount returns a single account by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetAccount(id int) (models.Account, error) {
	return scanAccount(s.db.QueryRow(accountSelectQuery+" WHERE accounts.id = ?", id))
}

// CreateAccount inserts a new account and returns the created record.
func (s *Store) CreateAccount(input models.AccountInput) (models.Account, error) {
	var id int
	err := s.db.QueryRow("INSERT INTO accounts (name, type, opening_balance) VALUES (?, ?, ?) RETURNING id",
		input.Name, input.Type, input.OpeningBalance).Scan(&id)
	if err != nil {
		return models.Account{}, err
	}
	return s.getAccountByID(id)
}

// UpdateAccount updates an existing account. Returns sql.ErrNoRows if not found.
func (s *Store) UpdateAccount(id int, input models.AccountInput) (models.Account, error) {
	res, err := s.db.Exec("UPDATE accounts SET name = ?, type = ?, opening_balance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		input.Name, input.Type, input.OpeningBalance, id)
	if err != nil {
		return models.Account{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Account{}, sql.ErrNoRows
	}
	return s.getAccountByID(id)
}

// DeleteAccount removes an account. Returns sql.ErrNoRows if not found.
func (s *Store) DeleteAccount(id int) error {
	res, err := s.db.Exec("DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}


