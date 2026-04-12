package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/db"
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

func getAccountByID(d *db.PortalDB, id int) (models.Account, error) {
	return scanAccount(d.QueryRow(accountSelectQuery+" WHERE accounts.id = ?", id))
}

// ListAccounts lists all accounts
// @Summary      List accounts
// @Description  Get a list of all bank accounts, cash, and credit cards with current balances.
// @Tags         accounts
// @Produce      json
// @Param        search  query     string  false  "Search by name"
// @Success      200  {object}  Response{data=[]models.Account}
// @Router       /accounts [get]
// @Security     BearerAuth
func ListAccounts(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	search := r.URL.Query().Get("search")
	query := accountSelectQuery
	var args []any
	if search != "" {
		query += " WHERE name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	query += " ORDER BY name"
	rows, err := d.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var accounts []models.Account
	rowCount := 0
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.OpeningBalance, &a.CreatedAt, &a.UpdatedAt, &a.Balance); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rowCount++
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Debug("Accounts", "rowCount", rowCount)
	if accounts == nil {
		accounts = []models.Account{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

// GetAccount retrieves a single account by ID
// @Summary      Get account
// @Description  Get details and current balance of a specific account.
// @Tags         accounts
// @Produce      json
// @Param        id   path      int  true  "Account ID"
// @Success      200  {object}  Response{data=models.Account}
// @Failure      404  {object}  Response{error=string}
// @Router       /accounts/{id} [get]
// @Security     BearerAuth
func GetAccount(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var a models.Account
	err := d.QueryRow(accountSelectQuery+" WHERE id = ?", id).
		Scan(&a.ID, &a.Name, &a.Type, &a.OpeningBalance, &a.CreatedAt, &a.UpdatedAt, &a.Balance)
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// CreateAccount creates a new account
// @Summary      Create account
// @Description  Create a new bank account, cash or credit card.
// @Tags         accounts
// @Accept       json
// @Produce      json
// @Param        account  body      models.AccountInput  true  "Account contents"
// @Success      201      {object}  Response{data=models.Account}
// @Failure      400      {object}  Response{error=string}
// @Router       /accounts [post]
// @Security     BearerAuth
func CreateAccount(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	var input models.AccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var id int
	err := d.QueryRow("INSERT INTO accounts (name, type, opening_balance) VALUES (?, ?, ?) RETURNING id",
		input.Name, input.Type, input.OpeningBalance).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a, err := scanAccount(d.QueryRow(accountSelectQuery+" WHERE accounts.id = ?", id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created account: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// UpdateAccount updates an existing account
// @Summary      Update account
// @Description  Update details of an existing account.
// @Tags         accounts
// @Accept       json
// @Produce      json
// @Param        id       path      int                 true  "Account ID"
// @Param        account  body      models.AccountInput true  "Updated account contents"
// @Success      200      {object}  Response{data=models.Account}
// @Failure      400      {object}  Response{error=string}
// @Failure      404      {object}  Response{error=string}
// @Router       /accounts/{id} [put]
// @Security     BearerAuth
func UpdateAccount(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.AccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	res, err := d.Exec("UPDATE accounts SET name = ?, type = ?, opening_balance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		input.Name, input.Type, input.OpeningBalance, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	a, err := scanAccount(d.QueryRow(accountSelectQuery+" WHERE accounts.id = ?", id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated account: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// DeleteAccount deletes an account
// @Summary      Delete account
// @Description  Remove an account.
// @Tags         accounts
// @Produce      json
// @Param        id   path      int  true  "Account ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /accounts/{id} [delete]
// @Security     BearerAuth
func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	res, err := d.Exec("DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
