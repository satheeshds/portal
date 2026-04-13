-- +goose Up
CREATE SEQUENCE IF NOT EXISTS payouts_id_seq START 1;
CREATE TABLE IF NOT EXISTS payouts (
    id INTEGER NOT NULL DEFAULT nextval('payouts_id_seq'),
    outlet_name TEXT NOT NULL,
    platform TEXT NOT NULL,
    period_start DATE,
    period_end DATE,
    settlement_date TEXT,
    total_orders INTEGER NOT NULL DEFAULT 0,
    gross_sales_amt INTEGER NOT NULL DEFAULT 0,
    restaurant_discount_amt INTEGER NOT NULL DEFAULT 0,
    platform_commission_amt INTEGER NOT NULL DEFAULT 0,
    taxes_tcs_tds_amt INTEGER NOT NULL DEFAULT 0,
    marketing_ads_amt INTEGER NOT NULL DEFAULT 0,
    final_payout_amt INTEGER NOT NULL DEFAULT 0,
    utr_number TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS payouts;
DROP SEQUENCE IF EXISTS payouts_id_seq;
