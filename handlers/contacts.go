package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/portal/models"
	"github.com/satheeshds/portal/store"
)

// ListContacts lists all contacts
//	@Summary		List contacts
//	@Description	Get a list of all vendors and customers with financial summaries.
//	@Tags			contacts
//	@Produce		json
//	@Param			type	query		string	false	"Filter by type (vendor/customer)"
//	@Param			search	query		string	false	"Search by name, email, or phone"
//	@Success		200		{object}	Response{data=[]models.Contact}
//	@Router			/contacts [get]
//	@Security		BearerAuth
func ListContacts(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	typeFilter := r.URL.Query().Get("type")
	search := r.URL.Query().Get("search")
	contacts, err := s.ListContacts(typeFilter, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, contacts)
}

// GetContact retrieves a single contact by ID
//	@Summary		Get contact
//	@Description	Get details and financial summary of a specific contact.
//	@Tags			contacts
//	@Produce		json
//	@Param			id	path		int	true	"Contact ID"
//	@Success		200	{object}	Response{data=models.Contact}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/contacts/{id} [get]
//	@Security		BearerAuth
func GetContact(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c, err := s.GetContact(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "contact not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// CreateContact creates a new contact
//	@Summary		Create contact
//	@Description	Create a new vendor or customer.
//	@Tags			contacts
//	@Accept			json
//	@Produce		json
//	@Param			contact	body		models.ContactInput	true	"Contact contents"
//	@Success		201		{object}	Response{data=models.Contact}
//	@Failure		400		{object}	Response{error=string}
//	@Router			/contacts [post]
//	@Security		BearerAuth
func CreateContact(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	var input models.ContactInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := s.CreateContact(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// UpdateContact updates an existing contact
//	@Summary		Update contact
//	@Description	Update details of an existing contact.
//	@Tags			contacts
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int					true	"Contact ID"
//	@Param			contact	body		models.ContactInput	true	"Updated contact contents"
//	@Success		200		{object}	Response{data=models.Contact}
//	@Failure		400		{object}	Response{error=string}
//	@Failure		404		{object}	Response{error=string}
//	@Router			/contacts/{id} [put]
//	@Security		BearerAuth
func UpdateContact(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.ContactInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	c, err := s.UpdateContact(id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "contact not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// DeleteContact deletes a contact
//	@Summary		Delete contact
//	@Description	Remove a contact.
//	@Tags			contacts
//	@Produce		json
//	@Param			id	path		int	true	"Contact ID"
//	@Success		200	{object}	Response{data=map[string]string}
//	@Failure		404	{object}	Response{error=string}
//	@Router			/contacts/{id} [delete]
//	@Security		BearerAuth
func DeleteContact(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeleteContact(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "contact not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
