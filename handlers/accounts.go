package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/models"
	"github.com/satheeshds/portal/store"
)

// ListAccounts lists all accounts
//	@Summary		List accounts
//	@Description	Get a list of all bank accounts, cash, and credit cards with current balances.
//	@Tags			accounts
//	@Produce		json
//	@Param			search	query		string	false	"Search by name"
//	@Success		200		{object}	Response{data=[]models.Account}
//	@Router			/accounts [get]
//	@Security		BearerAuth
func ListAccounts(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	search := r.URL.Query().Get("search")
	accounts, err := s.ListAccounts(search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Debug("Accounts", "rowCount", len(accounts))
	writeJSON(w, http.StatusOK, accounts)
}

// GetAccount retrieves a single account by ID
//	@Summary		Get account
//	@Description	Get details and current balance of a specific account.
//	@Tags			accounts
//	@Produce		json
//	@Param			id	path		int	true	"Account ID"
//	@Success		200	{object}	Response{data=models.Account}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/accounts/{id} [get]
//	@Security		BearerAuth
func GetAccount(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	a, err := s.GetAccount(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "account not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// CreateAccount creates a new account
//	@Summary		Create account
//	@Description	Create a new bank account, cash or credit card.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			account	body		models.AccountInput	true	"Account contents"
//	@Success		201		{object}	Response{data=models.Account}
//	@Failure		400		{object}	Response{error=string}
//	@Router			/accounts [post]
//	@Security		BearerAuth
func CreateAccount(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	var input models.AccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	a, err := s.CreateAccount(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// UpdateAccount updates an existing account
//	@Summary		Update account
//	@Description	Update details of an existing account.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int					true	"Account ID"
//	@Param			account	body		models.AccountInput	true	"Updated account contents"
//	@Success		200		{object}	Response{data=models.Account}
//	@Failure		400		{object}	Response{error=string}
//	@Failure		404		{object}	Response{error=string}
//	@Router			/accounts/{id} [put]
//	@Security		BearerAuth
func UpdateAccount(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
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

	a, err := s.UpdateAccount(id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "account not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// DeleteAccount deletes an account
//	@Summary		Delete account
//	@Description	Remove an account.
//	@Tags			accounts
//	@Produce		json
//	@Param			id	path		int	true	"Account ID"
//	@Success		200	{object}	Response{data=map[string]string}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/accounts/{id} [delete]
//	@Security		BearerAuth
func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeleteAccount(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "account not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
