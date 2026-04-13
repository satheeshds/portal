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

// ListInvoices lists all invoices
// @Summary      List invoices
// @Description  Get a list of all receivable invoices, with current status and allocation info.
// @Tags         invoices
// @Produce      json
// @Param        contact_id   query     int  false  "Filter by contact (customer)"
// @Param        search       query     string  false  "Search by invoice number, notes, or customer name"
// @Success      200          {object}  Response{data=[]models.Invoice}
// @Router       /invoices [get]
// @Security     BasicAuth
func ListInvoices(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	invoices, err := s.ListInvoices(
		r.URL.Query().Get("status"),
		r.URL.Query().Get("contact_id"),
		r.URL.Query().Get("from"),
		r.URL.Query().Get("to"),
		r.URL.Query().Get("search"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

// GetInvoice retrieves a single invoice by ID
// @Summary      Get invoice
// @Description  Get details and allocation status of a specific invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=models.Invoice}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id} [get]
// @Security     BasicAuth
func GetInvoice(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	inv, err := s.GetInvoice(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// CreateInvoice creates a new invoice
// @Summary      Create invoice
// @Description  Create a new receivable invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        invoice  body      models.InvoiceInput  true  "Invoice contents"
// @Success      201      {object}  Response{data=models.Invoice}
// @Failure      400      {object}  Response{error=string}
// @Router       /invoices [post]
// @Security     BasicAuth
func CreateInvoice(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	var input models.InvoiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	inv, err := s.CreateInvoice(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

// UpdateInvoice updates an existing invoice
// @Summary      Update invoice
// @Description  Update details of an existing invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        id       path      int                  true  "Invoice ID"
// @Param        invoice  body      models.InvoiceInput  true  "Updated invoice contents"
// @Success      200      {object}  Response{data=models.Invoice}
// @Failure      400      {object}  Response{error=string}
// @Failure      404      {object}  Response{error=string}
// @Router       /invoices/{id} [put]
// @Security     BasicAuth
func UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.InvoiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	inv, err := s.UpdateInvoice(id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// DeleteInvoice deletes an invoice
// @Summary      Delete invoice
// @Description  Remove an invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id} [delete]
// @Security     BasicAuth
func DeleteInvoice(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeleteInvoice(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// GetInvoiceLinks retrieves all transactions associated with an invoice
// @Summary      Get invoice links
// @Description  Get all payment transactions linked to a specific invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=[]InvoiceLink}
// @Router       /invoices/{id}/links [get]
// @Security     BasicAuth
func GetInvoiceLinks(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	links, err := s.GetInvoiceLinks(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// InvoiceLink is an alias for store.InvoiceLink kept here for Swagger doc references.
type InvoiceLink = store.InvoiceLink

// ListInvoiceItems lists all line items for an invoice
// @Summary      List invoice items
// @Description  Get all line items for a specific invoice.
// @Tags         invoices
// @Produce      json
// @Param        id   path      int  true  "Invoice ID"
// @Success      200  {object}  Response{data=[]models.InvoiceItem}
// @Failure      404  {object}  Response{error=string}
// @Router       /invoices/{id}/items [get]
// @Security     BasicAuth
func ListInvoiceItems(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	exists, err := s.InvoiceExists(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	items, err := s.ListInvoiceItems(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CreateInvoiceItem creates a new line item for an invoice
// @Summary      Create invoice item
// @Description  Add a new line item to an existing invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        id    path      int                      true  "Invoice ID"
// @Param        item  body      models.InvoiceItemInput  true  "Line item contents"
// @Success      201   {object}  Response{data=models.InvoiceItem}
// @Failure      400   {object}  Response{error=string}
// @Failure      404   {object}  Response{error=string}
// @Router       /invoices/{id}/items [post]
// @Security     BasicAuth
func CreateInvoiceItem(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	exists, err := s.InvoiceExists(invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify invoice existence: "+err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	var input models.InvoiceItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	item, err := s.CreateInvoiceItem(invoiceID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// UpdateInvoiceItem updates a line item for an invoice
// @Summary      Update invoice item
// @Description  Update an existing line item in an invoice.
// @Tags         invoices
// @Accept       json
// @Produce      json
// @Param        id      path      int                      true  "Invoice ID"
// @Param        itemId  path      int                      true  "Item ID"
// @Param        item    body      models.InvoiceItemInput  true  "Updated line item contents"
// @Success      200     {object}  Response{data=models.InvoiceItem}
// @Failure      400     {object}  Response{error=string}
// @Failure      404     {object}  Response{error=string}
// @Router       /invoices/{id}/items/{itemId} [put]
// @Security     BasicAuth
func UpdateInvoiceItem(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	var input models.InvoiceItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	item, err := s.UpdateInvoiceItem(invoiceID, itemID, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice item not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// DeleteInvoiceItem deletes a line item from an invoice
// @Summary      Delete invoice item
// @Description  Remove a line item from an invoice.
// @Tags         invoices
// @Produce      json
// @Param        id      path      int  true  "Invoice ID"
// @Param        itemId  path      int  true  "Item ID"
// @Success      200     {object}  Response{data=map[string]string}
// @Failure      404     {object}  Response{error=string}
// @Router       /invoices/{id}/items/{itemId} [delete]
// @Security     BasicAuth
func DeleteInvoiceItem(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	invoiceID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	if err := s.DeleteInvoiceItem(invoiceID, itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice item not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
