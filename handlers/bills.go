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

// ListBills lists all bills
// @Summary      List bills
// @Description  Get a list of all payable bills, with current status and allocation info.
// @Tags         bills
// @Produce      json
// @Param        contact_id   query     int  false  "Filter by contact (vendor)"
// @Param        search       query     string  false  "Search by bill number, notes, or vendor name"
// @Success      200          {object}  Response{data=[]models.Bill}
// @Router       /bills [get]
// @Security     BasicAuth
func ListBills(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	bills, err := s.ListBills(
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
	writeJSON(w, http.StatusOK, bills)
}

// GetBill retrieves a single bill by ID
// @Summary      Get bill
// @Description  Get details and allocation status of a specific bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=models.Bill}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id} [get]
// @Security     BasicAuth
func GetBill(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	b, err := s.GetBill(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bill not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// CreateBill creates a new bill
// @Summary      Create bill
// @Description  Create a new payable bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        bill  body      models.BillInput  true  "Bill contents"
// @Success      201   {object}  Response{data=models.Bill}
// @Failure      400   {object}  Response{error=string}
// @Router       /bills [post]
// @Security     BasicAuth
func CreateBill(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	var input models.BillInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	b, err := s.CreateBill(input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// UpdateBill updates an existing bill
// @Summary      Update bill
// @Description  Update details of an existing bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        id    path      int               true  "Bill ID"
// @Param        bill  body      models.BillInput  true  "Updated bill contents"
// @Success      200   {object}  Response{data=models.Bill}
// @Failure      400   {object}  Response{error=string}
// @Failure      404   {object}  Response{error=string}
// @Router       /bills/{id} [put]
// @Security     BasicAuth
func UpdateBill(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var input models.BillInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	b, err := s.UpdateBill(id, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bill not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// DeleteBill deletes a bill
// @Summary      Delete bill
// @Description  Remove a bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id} [delete]
// @Security     BasicAuth
func DeleteBill(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := s.DeleteBill(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bill not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// GetBillLinks retrieves all transactions associated with a bill
// @Summary      Get bill links
// @Description  Get all payment transactions linked to a specific bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=[]BillLink}
// @Router       /bills/{id}/links [get]
// @Security     BasicAuth
func GetBillLinks(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	links, err := s.GetBillLinks(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// BillLink is an alias for store.BillLink kept here for Swagger doc references.
type BillLink = store.BillLink

// ListBillItems lists all line items for a bill
// @Summary      List bill items
// @Description  Get all line items for a specific bill.
// @Tags         bills
// @Produce      json
// @Param        id   path      int  true  "Bill ID"
// @Success      200  {object}  Response{data=[]models.BillItem}
// @Failure      404  {object}  Response{error=string}
// @Router       /bills/{id}/items [get]
// @Security     BasicAuth
func ListBillItems(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	exists, err := s.BillExists(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	items, err := s.ListBillItems(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CreateBillItem creates a new line item for a bill
// @Summary      Create bill item
// @Description  Add a new line item to an existing bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        id    path      int                    true  "Bill ID"
// @Param        item  body      models.BillItemInput   true  "Line item contents"
// @Success      201   {object}  Response{data=models.BillItem}
// @Failure      400   {object}  Response{error=string}
// @Failure      404   {object}  Response{error=string}
// @Router       /bills/{id}/items [post]
// @Security     BasicAuth
func CreateBillItem(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	exists, err := s.BillExists(billID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify bill existence: "+err.Error())
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "bill not found")
		return
	}

	var input models.BillItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	item, err := s.CreateBillItem(billID, input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// UpdateBillItem updates a line item for a bill
// @Summary      Update bill item
// @Description  Update an existing line item in a bill.
// @Tags         bills
// @Accept       json
// @Produce      json
// @Param        id      path      int                   true  "Bill ID"
// @Param        itemId  path      int                   true  "Item ID"
// @Param        item    body      models.BillItemInput  true  "Updated line item contents"
// @Success      200     {object}  Response{data=models.BillItem}
// @Failure      400     {object}  Response{error=string}
// @Failure      404     {object}  Response{error=string}
// @Router       /bills/{id}/items/{itemId} [put]
// @Security     BasicAuth
func UpdateBillItem(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	var input models.BillItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	item, err := s.UpdateBillItem(billID, itemID, input)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bill item not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// DeleteBillItem deletes a line item from a bill
// @Summary      Delete bill item
// @Description  Remove a line item from a bill.
// @Tags         bills
// @Produce      json
// @Param        id      path      int  true  "Bill ID"
// @Param        itemId  path      int  true  "Item ID"
// @Success      200     {object}  Response{data=map[string]string}
// @Failure      404     {object}  Response{error=string}
// @Router       /bills/{id}/items/{itemId} [delete]
// @Security     BasicAuth
func DeleteBillItem(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	billID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	itemID, _ := strconv.Atoi(chi.URLParam(r, "itemId"))

	if err := s.DeleteBillItem(billID, itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bill item not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
