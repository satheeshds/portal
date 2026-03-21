package models

import (
	"strings"
	"time"
)

// Payout represents a platform payout record.
type Payout struct {
	ID                    int       `json:"id"`
	OutletName            string    `json:"outlet_name"`
	Platform              string    `json:"platform"` // swiggy, zomato, swiggy-dineout
	PeriodStart           *string   `json:"period_start"`
	PeriodEnd             *string   `json:"period_end"`
	SettlementDate        *string   `json:"settlement_date"`
	TotalOrders           int       `json:"total_orders"`
	GrossSalesAmt         Money     `json:"gross_sales_amt"`
	RestaurantDiscountAmt Money     `json:"restaurant_discount_amt"`
	PlatformCommissionAmt Money     `json:"platform_commission_amt"`
	TaxesTcsTdsAmt        Money     `json:"taxes_tcs_tds_amt"`
	MarketingAdsAmt       Money     `json:"marketing_ads_amt"`
	FinalPayoutAmt        Money     `json:"final_payout_amt"`
	UtrNumber             string    `json:"utr_number"`
	CreatedAt             time.Time `json:"created_at"`
	// Computed fields
	Allocated   Money `json:"allocated"`
	Unallocated Money `json:"unallocated"`
}

// PayoutInput is used for creating/updating payout records.
type PayoutInput struct {
	OutletName            string  `json:"outlet_name"`
	Platform              string  `json:"platform"`
	PeriodStart           *string `json:"period_start"`
	PeriodEnd             *string `json:"period_end"`
	SettlementDate        *string `json:"settlement_date"`
	TotalOrders           int     `json:"total_orders"`
	GrossSalesAmt         Money   `json:"gross_sales_amt"`
	RestaurantDiscountAmt Money   `json:"restaurant_discount_amt"`
	PlatformCommissionAmt Money   `json:"platform_commission_amt"`
	TaxesTcsTdsAmt        Money   `json:"taxes_tcs_tds_amt"`
	MarketingAdsAmt       Money   `json:"marketing_ads_amt"`
	FinalPayoutAmt        Money   `json:"final_payout_amt"`
	UtrNumber             string  `json:"utr_number"`
}

func (p *PayoutInput) Validate() string {
	if p.OutletName == "" {
		return "outlet_name is required"
	}

	// Normalize to lowercase
	p.Platform = strings.ToLower(p.Platform)

	switch p.Platform {
	case "swiggy", "zomato", "swiggy-dineout":
	default:
		return "platform must be one of: swiggy, zomato, swiggy-dineout"
	}
	if err := NormalizeDate(p.PeriodStart); err != nil {
		return "period_start: " + err.Error()
	}
	if err := NormalizeDate(p.PeriodEnd); err != nil {
		return "period_end: " + err.Error()
	}
	if err := NormalizeDate(p.SettlementDate); err != nil {
		return "settlement_date: " + err.Error()
	}
	return ""
}
