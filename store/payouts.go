package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/satheeshds/portal/models"
)

const payoutSelectQuery = `SELECT id, outlet_name, platform, period_start, period_end, settlement_date,
		total_orders, gross_sales_amt, restaurant_discount_amt, platform_commission_amt,
		taxes_tcs_tds_amt, marketing_ads_amt, final_payout_amt, utr_number, created_at,
		COALESCE((SELECT SUM(td.amount) FROM transaction_documents td WHERE td.document_type = 'payout' AND td.document_id = payouts.id), 0)
		FROM payouts`

// PayoutLink represents a linked transaction payment for a payout.
type PayoutLink struct {
	models.TransactionDocument
	TransactionDate string `json:"transaction_date"`
	Description     string `json:"description"`
	Reference       string `json:"reference"`
	AccountName     string `json:"account_name"`
}

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

func (s *Store) getPayoutByID(id int) (models.Payout, error) {
	return scanPayout(s.db.QueryRow(payoutSelectQuery+" WHERE id = ?", id))
}

// ListPayouts returns payouts filtered by the provided parameters (all may be empty).
func (s *Store) ListPayouts(platform, outletName, from, to string) ([]models.Payout, error) {
	query := payoutSelectQuery
	var conditions []string
	var args []any

	if platform != "" {
		conditions = append(conditions, "platform = ?")
		args = append(args, platform)
	}
	if outletName != "" {
		conditions = append(conditions, "outlet_name LIKE ?")
		args = append(args, "%"+outletName+"%")
	}
	if from != "" {
		conditions = append(conditions, "settlement_date >= ?")
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, "settlement_date <= ?")
		args = append(args, to)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY settlement_date DESC, created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payouts []models.Payout
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			return nil, err
		}
		payouts = append(payouts, p)
	}
	if payouts == nil {
		payouts = []models.Payout{}
	}
	return payouts, nil
}

// GetPayout returns a single payout by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetPayout(id int) (models.Payout, error) {
	return s.getPayoutByID(id)
}

// CreatePayout inserts a new payout record and returns it.
func (s *Store) CreatePayout(input models.PayoutInput) (models.Payout, error) {
	var id int
	err := s.db.QueryRow(`INSERT INTO payouts (outlet_name, platform, period_start, period_end, settlement_date,
		total_orders, gross_sales_amt, restaurant_discount_amt, platform_commission_amt,
		taxes_tcs_tds_amt, marketing_ads_amt, final_payout_amt, utr_number)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		input.OutletName, input.Platform, input.PeriodStart, input.PeriodEnd, input.SettlementDate,
		input.TotalOrders, input.GrossSalesAmt, input.RestaurantDiscountAmt, input.PlatformCommissionAmt,
		input.TaxesTcsTdsAmt, input.MarketingAdsAmt, input.FinalPayoutAmt, input.UtrNumber).Scan(&id)
	if err != nil {
		return models.Payout{}, err
	}
	return s.getPayoutByID(id)
}

// UpdatePayout updates an existing payout. Returns sql.ErrNoRows if not found.
func (s *Store) UpdatePayout(id int, input models.PayoutInput) (models.Payout, error) {
	res, err := s.db.Exec(`UPDATE payouts SET outlet_name = ?, platform = ?, period_start = ?, period_end = ?,
		settlement_date = ?, total_orders = ?, gross_sales_amt = ?, restaurant_discount_amt = ?,
		platform_commission_amt = ?, taxes_tcs_tds_amt = ?, marketing_ads_amt = ?, final_payout_amt = ?,
		utr_number = ? WHERE id = ?`,
		input.OutletName, input.Platform, input.PeriodStart, input.PeriodEnd, input.SettlementDate,
		input.TotalOrders, input.GrossSalesAmt, input.RestaurantDiscountAmt, input.PlatformCommissionAmt,
		input.TaxesTcsTdsAmt, input.MarketingAdsAmt, input.FinalPayoutAmt, input.UtrNumber, id)
	if err != nil {
		return models.Payout{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Payout{}, sql.ErrNoRows
	}
	return s.getPayoutByID(id)
}

// DeletePayout removes a payout and its transaction links. Returns sql.ErrNoRows if not found.
func (s *Store) DeletePayout(ctx context.Context, id int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM transaction_documents WHERE document_type = 'payout' AND document_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}

	res, err := tx.Exec("DELETE FROM payouts WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if n == 0 {
		_ = tx.Rollback()
		return sql.ErrNoRows
	}

	return tx.Commit()
}

// GetPayoutLinks returns all transaction links for the given payout.
func (s *Store) GetPayoutLinks(id int) ([]PayoutLink, error) {
	rows, err := s.db.Query(`SELECT td.id, td.transaction_id, td.document_type, td.document_id, td.amount, td.created_at,
		COALESCE(t.transaction_date, ''), COALESCE(t.description, ''), COALESCE(t.reference, ''), a.name as account_name
		FROM transaction_documents td
		JOIN transactions t ON td.transaction_id = t.id
		JOIN accounts a ON t.account_id = a.id
		WHERE td.document_type = 'payout' AND td.document_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []PayoutLink
	for rows.Next() {
		var l PayoutLink
		if err := rows.Scan(&l.ID, &l.TransactionID, &l.DocumentType, &l.DocumentID, &l.Amount, &l.CreatedAt,
			&l.TransactionDate, &l.Description, &l.Reference, &l.AccountName); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	if links == nil {
		links = []PayoutLink{}
	}
	return links, nil
}
