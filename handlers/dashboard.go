package handlers

import (
	"net/http"

	"github.com/satheeshds/portal/store"
)

// GetDashboard retrieves dashboard summary statistics
// @Summary      Get dashboard
// @Description  Get totals for accounts, contacts, bills, invoices, and recent transactions.
// @Tags         dashboard
// @Produce      json
// @Success      200  {object}  Response{data=store.DashboardData}
// @Router       /dashboard [get]
// @Security     BasicAuth
func GetDashboard(w http.ResponseWriter, r *http.Request) {
	s := store.New(getDB(r))
	d, err := s.GetDashboard()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}
