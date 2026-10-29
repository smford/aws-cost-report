package awsclient

import "strings"

// LineItem represents aggregated billing data for a single group (e.g. AWS service or linked account).
type LineItem struct {
	Name              string  `json:"name"`
	UnblendedCost     float64 `json:"unblended_cost"`
	BlendedCost       float64 `json:"blended_cost"`
	AmortizedCost     float64 `json:"amortized_cost"`
	PrevUnblendedCost float64 `json:"prev_unblended_cost,omitempty"`
	PrevBlendedCost   float64 `json:"prev_blended_cost,omitempty"`
	PrevAmortizedCost float64 `json:"prev_amortized_cost,omitempty"`
	MoMDeltaAmount    float64 `json:"mom_delta_amount,omitempty"`
	MoMDeltaPercent   float64 `json:"mom_delta_percent,omitempty"`
	HasPreviousCost   bool    `json:"has_previous_cost,omitempty"`
}

// ReportMetadata contains contextual details about the generated cost report.
type ReportMetadata struct {
	AccountID        string `json:"account_id"`
	AccountName      string `json:"account_name,omitempty"`
	BillingMonth     string `json:"billing_month"`
	PeriodStart      string `json:"period_start"`
	PeriodEnd        string `json:"period_end"`
	AWSPeriodStart   string `json:"aws_period_start"`
	AWSPeriodEnd     string `json:"aws_period_end"`
	Currency         string `json:"currency"`
	EstimatedOrFinal string `json:"estimated_or_final"`
	ActiveServices   int    `json:"active_services"`
	GroupKey         string `json:"group_key"`
	TagFilter        string `json:"tag_filter,omitempty"`
}

// TotalCosts encapsulates aggregate metrics across all active services.
type TotalCosts struct {
	UnblendedCost     float64 `json:"unblended_cost"`
	BlendedCost       float64 `json:"blended_cost"`
	AmortizedCost     float64 `json:"amortized_cost"`
	PrevUnblendedCost float64 `json:"prev_unblended_cost,omitempty"`
	PrevBlendedCost   float64 `json:"prev_blended_cost,omitempty"`
	PrevAmortizedCost float64 `json:"prev_amortized_cost,omitempty"`
	MoMDeltaAmount    float64 `json:"mom_delta_amount,omitempty"`
	MoMDeltaPercent   float64 `json:"mom_delta_percent,omitempty"`
	HasPreviousCost   bool    `json:"has_previous_cost,omitempty"`
}

// CostReport is the root domain model containing metadata, totals, and line items.
type CostReport struct {
	Metadata      ReportMetadata `json:"metadata"`
	Totals        TotalCosts     `json:"totals"`
	Items         []LineItem     `json:"items"`
	HasComparison bool           `json:"has_comparison"`
}

// CurrencySymbol maps a 3-letter ISO currency code to its standard symbol.
// If the currency code is unrecognized or already a symbol, it returns the trimmed symbol or code.
func CurrencySymbol(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY", "CNY":
		return "¥"
	case "INR":
		return "₹"
	case "AUD":
		return "A$"
	case "CAD":
		return "CA$"
	case "NZD":
		return "NZ$"
	case "CHF":
		return "CHF"
	case "BRL":
		return "R$"
	case "SEK", "NOK", "DKK":
		return "kr"
	case "SGD":
		return "S$"
	case "HKD":
		return "HK$"
	case "MXN":
		return "Mex$"
	case "ZAR":
		return "R"
	case "KRW":
		return "₩"
	case "ILS":
		return "₪"
	default:
		trimmed := strings.TrimSpace(currency)
		if trimmed == "" {
			return "$"
		}
		return trimmed
	}
}

// CurrencySymbol returns the representative symbol for the report's currency.
func (m ReportMetadata) CurrencySymbol() string {
	return CurrencySymbol(m.Currency)
}
