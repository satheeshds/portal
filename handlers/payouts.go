package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/satheeshds/accounting/models"
)

const payoutSelectQuery = `SELECT id, outlet_name, platform, period_start, period_end, settlement_date,
		total_orders, gross_sales_amt, restaurant_discount_amt, platform_commission_amt,
		taxes_tcs_tds_amt, marketing_ads_amt, final_payout_amt, utr_number, created_at,
		COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'payout' AND td.document_id = payouts.id), 0)
		FROM payouts`

func scanPayout(scanner interface{ Scan(...any) error }) (models.Payout, error) {
	var p models.Payout
	err := scanner.Scan(&p.ID, &p.OutletName, &p.Platform, &p.PeriodStart, &p.PeriodEnd, &p.SettlementDate,
		&p.TotalOrders, &p.GrossSalesAmt, &p.RestaurantDiscountAmt, &p.PlatformCommissionAmt,
		&p.TaxesTcsTdsAmt, &p.MarketingAdsAmt, &p.FinalPayoutAmt, &p.UtrNumber, &p.CreatedAt, &p.Allocated)
	if err == nil {
		p.Unallocated = models.Money(int64(p.FinalPayoutAmt) - int64(p.Allocated))
	}
	return p, err
}

func getPayoutByID(id int) (models.Payout, error) {
	return scanPayout(DB.QueryRow(payoutSelectQuery+" WHERE id = ?", id))
}

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
	query := payoutSelectQuery
	var conditions []string
	var args []any

	if p := r.URL.Query().Get("platform"); p != "" {
		conditions = append(conditions, "platform = ?")
		args = append(args, p)
	}
	if o := r.URL.Query().Get("outlet_name"); o != "" {
		conditions = append(conditions, "outlet_name LIKE ?")
		args = append(args, "%"+o+"%")
	}
	if from := r.URL.Query().Get("from"); from != "" {
		conditions = append(conditions, "settlement_date >= ?")
		args = append(args, from)
	}
	if to := r.URL.Query().Get("to"); to != "" {
		conditions = append(conditions, "settlement_date <= ?")
		args = append(args, to)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY settlement_date DESC, created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var payouts []models.Payout
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payouts = append(payouts, p)
	}
	if payouts == nil {
		payouts = []models.Payout{}
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
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	p, err := getPayoutByID(id)
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
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	rows, err := DB.Query(`SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
		COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name as account_name
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE td.document_type = 'payout' AND td.document_id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var links []PayoutLink
	for rows.Next() {
		var l PayoutLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		links = append(links, l)
	}
	if links == nil {
		links = []PayoutLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

// PayoutLink represents a linked transaction payment for a payout.
type PayoutLink struct {
	models.TransactionDocument
	TransactionDate string `json:"transaction_date"`
	Description     string `json:"description"`
	Reference       string `json:"reference"`
	AccountName     string `json:"account_name"`
}

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
	var input models.PayoutInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := input.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	var id int
	err := DB.QueryRow(`INSERT INTO payouts (outlet_name, platform, period_start, period_end, settlement_date,
		total_orders, gross_sales_amt, restaurant_discount_amt, platform_commission_amt,
		taxes_tcs_tds_amt, marketing_ads_amt, final_payout_amt, utr_number)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.OutletName, input.Platform, input.PeriodStart, input.PeriodEnd, input.SettlementDate,
		input.TotalOrders, input.GrossSalesAmt, input.RestaurantDiscountAmt, input.PlatformCommissionAmt,
		input.TaxesTcsTdsAmt, input.MarketingAdsAmt, input.FinalPayoutAmt, input.UtrNumber).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	p, err := getPayoutByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch created payout: "+err.Error())
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

	res, err := DB.Exec(`UPDATE payouts SET outlet_name = ?, platform = ?, period_start = ?, period_end = ?,
		settlement_date = ?, total_orders = ?, gross_sales_amt = ?, restaurant_discount_amt = ?,
		platform_commission_amt = ?, taxes_tcs_tds_amt = ?, marketing_ads_amt = ?, final_payout_amt = ?,
		utr_number = ? WHERE id = ?`,
		input.OutletName, input.Platform, input.PeriodStart, input.PeriodEnd, input.SettlementDate,
		input.TotalOrders, input.GrossSalesAmt, input.RestaurantDiscountAmt, input.PlatformCommissionAmt,
		input.TaxesTcsTdsAmt, input.MarketingAdsAmt, input.FinalPayoutAmt, input.UtrNumber, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "payout not found")
		return
	}

	p, err := getPayoutByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to re-fetch updated payout: "+err.Error())
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
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	tx, err := DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Remove all transaction links for this payout so transaction allocated amounts stay accurate.
	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE document_type = 'payout' AND document_id = ?", id); err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := tx.Exec("DELETE FROM payouts WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	n, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		_ = tx.Rollback()
		writeError(w, http.StatusNotFound, "payout not found")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
