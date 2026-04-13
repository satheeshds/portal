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
	s := store.New(getDB(r))
	payments, err := s.ListRecurringPayments(
		r.URL.Query().Get("status"),
		r.URL.Query().Get("account_id"),
		r.URL.Query().Get("type"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	p, err := s.GetRecurringPayment(id)
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
	s := store.New(getDB(r))
	var input models.RecurringPaymentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	p, err := s.CreateRecurringPayment(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	s := store.New(getDB(r))
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
	p, err := s.UpdateRecurringPayment(id, input)
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
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeleteRecurringPayment(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recurring payment not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
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
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	links, err := s.GetRecurringPaymentLinks(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// RecurringPaymentLink is an alias for store.RecurringPaymentLink kept here for Swagger doc references.
type RecurringPaymentLink = store.RecurringPaymentLink

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
	s := store.New(getDB(r))
	rpID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	exists, err := s.RecurringPaymentExists(rpID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "recurring payment not found")
		return
	}

	occurrences, err := s.ListRecurringPaymentOccurrences(rpID, r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, occurrences)
}
