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

// ListPayouts lists all payouts
// @Summary      List payouts
// @Description  Get a list of all platform payouts (Swiggy, Zomato, Swiggy-Dineout).
// @Tags         payouts
// @Produce      json
// @Param        platform     query     string  false  "Filter by platform (Swiggy, Zomato, Swiggy-Dineout)"
// @Param        outlet_name  query     string  false  "Filter by outlet name"
// @Param        from         query     string  false  "Filter by settlement date from (YYYY-MM-DD)"
// @Param        to           query     string  false  "Filter by settlement date to (YYYY-MM-DD)"
// @Success      200          {object}  Response{data=[]models.Payout}
// @Router       /payouts [get]
// @Security     BasicAuth
func ListPayouts(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	payouts, err := s.ListPayouts(
		r.URL.Query().Get("platform"),
		r.URL.Query().Get("outlet_name"),
		r.URL.Query().Get("from"),
		r.URL.Query().Get("to"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payouts)
}

// GetPayout retrieves a single payout by ID
// @Summary      Get payout
// @Description  Get details of a specific platform payout.
// @Tags         payouts
// @Produce      json
// @Param        id   path      int  true  "Payout ID"
// @Success      200  {object}  Response{data=models.Payout}
// @Failure      404  {object}  Response{error=string}
// @Router       /payouts/{id} [get]
// @Security     BasicAuth
func GetPayout(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	p, err := s.GetPayout(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "payout not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// GetPayoutLinks retrieves all transactions associated with a payout
// @Summary      Get payout links
// @Description  Get all payment transactions linked to a specific payout.
// @Tags         payouts
// @Produce      json
// @Param        id   path      int  true  "Payout ID"
// @Success      200  {object}  Response{data=[]PayoutLink}
// @Router       /payouts/{id}/links [get]
// @Security     BasicAuth
func GetPayoutLinks(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	links, err := s.GetPayoutLinks(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// PayoutLink is an alias for store.PayoutLink kept here for Swagger doc references.
type PayoutLink = store.PayoutLink

// CreatePayout creates a new payout record
// @Summary      Create payout
// @Description  Create a new platform payout record.
// @Tags         payouts
// @Accept       json
// @Produce      json
// @Param        payout  body      models.PayoutInput  true  "Payout contents"
// @Success      201     {object}  Response{data=models.Payout}
// @Failure      400     {object}  Response{error=string}
// @Router       /payouts [post]
// @Security     BasicAuth
func CreatePayout(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	var input models.PayoutInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	p, err := s.CreatePayout(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// UpdatePayout updates an existing payout record
// @Summary      Update payout
// @Description  Update details of an existing platform payout record.
// @Tags         payouts
// @Accept       json
// @Produce      json
// @Param        id      path      int                 true  "Payout ID"
// @Param        payout  body      models.PayoutInput  true  "Updated payout contents"
// @Success      200     {object}  Response{data=models.Payout}
// @Failure      400     {object}  Response{error=string}
// @Failure      404     {object}  Response{error=string}
// @Router       /payouts/{id} [put]
// @Security     BasicAuth
func UpdatePayout(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.PayoutInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	p, err := s.UpdatePayout(id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "payout not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// DeletePayout deletes a payout record
// @Summary      Delete payout
// @Description  Remove a platform payout record.
// @Tags         payouts
// @Produce      json
// @Param        id   path      int  true  "Payout ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /payouts/{id} [delete]
// @Security     BasicAuth
func DeletePayout(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeletePayout(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "payout not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
