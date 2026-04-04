package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
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

// ListContacts lists all contacts
// @Summary      List contacts
// @Description  Get a list of all vendors and customers with financial summaries.
// @Tags         contacts
// @Produce      json
// @Param        type    query     string  false  "Filter by type (vendor/customer)"
// @Param        search  query     string  false  "Search by name, email, or phone"
// @Success      200    {object}  Response{data=[]models.Contact}
// @Router       /contacts [get]
// @Security     BasicAuth
func ListContacts(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	query := contactSelectQuery
	var args []any
	var conditions []string

	if t := r.URL.Query().Get("type"); t != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, t)
	}

	if search := r.URL.Query().Get("search"); search != "" {
		conditions = append(conditions, "(name LIKE ? OR email LIKE ? OR phone LIKE ?)")
		s := "%" + search + "%"
		args = append(args, s, s, s)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY name"

	rows, err := d.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var contacts []models.Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		contacts = append(contacts, c)
	}
	if contacts == nil {
		contacts = []models.Contact{}
	}
	writeJSON(w, http.StatusOK, contacts)
}

// GetContact retrieves a single contact by ID
// @Summary      Get contact
// @Description  Get details and financial summary of a specific contact.
// @Tags         contacts
// @Produce      json
// @Param        id   path      int  true  "Contact ID"
// @Success      200  {object}  Response{data=models.Contact}
// @Failure      404  {object}  Response{error=string}
// @Router       /contacts/{id} [get]
// @Security     BasicAuth
func GetContact(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	c, err := scanContact(d.QueryRow(contactSelectQuery+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusNotFound, "contact not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// CreateContact creates a new contact
// @Summary      Create contact
// @Description  Create a new vendor or customer.
// @Tags         contacts
// @Accept       json
// @Produce      json
// @Param        contact  body      models.ContactInput  true  "Contact contents"
// @Success      201      {object}  Response{data=models.Contact}
// @Failure      400      {object}  Response{error=string}
// @Router       /contacts [post]
// @Security     BasicAuth
func CreateContact(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	var input models.ContactInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var id int
	err := d.QueryRow("INSERT INTO contacts (name, type, email, phone) VALUES (?, ?, ?, ?) RETURNING id",
		input.Name, input.Type, input.Email, input.Phone).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c, err := scanContact(d.QueryRow(contactSelectQuery+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created contact: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// UpdateContact updates an existing contact
// @Summary      Update contact
// @Description  Update details of an existing contact.
// @Tags         contacts
// @Accept       json
// @Produce      json
// @Param        id       path      int                  true  "Contact ID"
// @Param        contact  body      models.ContactInput  true  "Updated contact contents"
// @Success      200      {object}  Response{data=models.Contact}
// @Failure      400      {object}  Response{error=string}
// @Failure      404      {object}  Response{error=string}
// @Router       /contacts/{id} [put]
// @Security     BasicAuth
func UpdateContact(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
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

	res, err := d.Exec("UPDATE contacts SET name = ?, type = ?, email = ?, phone = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		input.Name, input.Type, input.Email, input.Phone, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "contact not found")
		return
	}

	c, err := scanContact(d.QueryRow(contactSelectQuery+" WHERE id = ?", id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated contact: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// DeleteContact deletes a contact
// @Summary      Delete contact
// @Description  Remove a contact.
// @Tags         contacts
// @Produce      json
// @Param        id   path      int  true  "Contact ID"
// @Success      200  {object}  Response{data=map[string]string}
// @Failure      404  {object}  Response{error=string}
// @Router       /contacts/{id} [delete]
// @Security     BasicAuth
func DeleteContact(w http.ResponseWriter, r *http.Request) {
	d := getDB(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	res, err := d.Exec("DELETE FROM contacts WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "contact not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
