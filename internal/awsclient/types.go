package awsclient

import "strings"

// LineItem represents aggregated billing data for a single group (e.g. AWS service or linked account).
type LineItem struct {
	Name                 string  `json:"name"`
	UnblendedCost        float64 `json:"unblended_cost"`
	BlendedCost          float64 `json:"blended_cost"`
	AmortizedCost        float64 `json:"amortized_cost"`
	NetUnblendedCost     float64 `json:"net_unblended_cost,omitempty"`
	NetAmortizedCost     float64 `json:"net_amortized_cost,omitempty"`
	PrevUnblendedCost    float64 `json:"prev_unblended_cost,omitempty"`
	PrevBlendedCost      float64 `json:"prev_blended_cost,omitempty"`
	PrevAmortizedCost    float64 `json:"prev_amortized_cost,omitempty"`
	PrevNetUnblendedCost float64 `json:"prev_net_unblended_cost,omitempty"`
	PrevNetAmortizedCost float64 `json:"prev_net_amortized_cost,omitempty"`
	MoMDeltaAmount       float64 `json:"mom_delta_amount,omitempty"`
	MoMDeltaPercent      float64 `json:"mom_delta_percent,omitempty"`
	HasPreviousCost      bool    `json:"has_previous_cost,omitempty"`
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
	ChargeTypeFilter string `json:"charge_type_filter,omitempty"`
	ExcludeCredits   bool   `json:"exclude_credits,omitempty"`
}

// TotalCosts encapsulates aggregate metrics across all active services.
type TotalCosts struct {
	UnblendedCost        float64 `json:"unblended_cost"`
	BlendedCost          float64 `json:"blended_cost"`
	AmortizedCost        float64 `json:"amortized_cost"`
	NetUnblendedCost     float64 `json:"net_unblended_cost,omitempty"`
	NetAmortizedCost     float64 `json:"net_amortized_cost,omitempty"`
	PrevUnblendedCost    float64 `json:"prev_unblended_cost,omitempty"`
	PrevBlendedCost      float64 `json:"prev_blended_cost,omitempty"`
	PrevAmortizedCost    float64 `json:"prev_amortized_cost,omitempty"`
	PrevNetUnblendedCost float64 `json:"prev_net_unblended_cost,omitempty"`
	PrevNetAmortizedCost float64 `json:"prev_net_amortized_cost,omitempty"`
	MoMDeltaAmount       float64 `json:"mom_delta_amount,omitempty"`
	MoMDeltaPercent      float64 `json:"mom_delta_percent,omitempty"`
	HasPreviousCost      bool    `json:"has_previous_cost,omitempty"`
}

// MonthlyBucket represents a single month column in a multi-month breakdown.
type MonthlyBucket struct {
	Key       string  `json:"key"`        // Format: "YYYY-MM" (e.g. "2025-11")
	Label     string  `json:"label"`      // Short header label (e.g. "Nov 25")
	FullLabel string  `json:"full_label"` // Full header label (e.g. "November 2025")
	Total     float64 `json:"total"`
}

// MonthlyLineItem represents a service's monthly breakdown across all queried months.
type MonthlyLineItem struct {
	Name    string             `json:"name"`
	Costs   map[string]float64 `json:"costs"` // map[monthKey]cost
	Total   float64            `json:"total"`
	Average float64            `json:"average"`
}

// MonthlyBreakdown contains the tabular multi-month column breakdown.
type MonthlyBreakdown struct {
	Months     []MonthlyBucket    `json:"months"`
	Items      []MonthlyLineItem  `json:"items"`
	Totals     map[string]float64 `json:"totals"` // map[monthKey]total
	GrandTotal float64            `json:"grand_total"`
}

// DailyBucket represents a single day in a daily breakdown.
type DailyBucket struct {
	Date            string  `json:"date"` // "YYYY-MM-DD"
	DayOfWeek       string  `json:"day"`  // "Mon", "Tue", etc.
	Total           float64 `json:"total"`
	TopDriverName   string  `json:"top_driver_name,omitempty"`
	TopDriverAmount float64 `json:"top_driver_amount,omitempty"`
}

// DailyLineItem represents a service's cost breakdown across queried days.
type DailyLineItem struct {
	Name    string             `json:"name"`
	Costs   map[string]float64 `json:"costs"` // map[date]cost
	Total   float64            `json:"total"`
	Average float64            `json:"average"`
}

// DailyBreakdown contains the tabular daily breakdown over the billing window.
type DailyBreakdown struct {
	Days       []DailyBucket      `json:"days"`
	Items      []DailyLineItem    `json:"items"`
	Totals     map[string]float64 `json:"totals"` // map[date]total
	GrandTotal float64            `json:"grand_total"`
	Average    float64            `json:"average"`
}

// CostForecast holds month-end forecast data from AWS Cost Explorer.
type CostForecast struct {
	TotalAmount        float64 `json:"total_amount"`
	MeanAmount         float64 `json:"mean_amount"`
	PeriodStart        string  `json:"period_start"`
	PeriodEnd          string  `json:"period_end"`
	PredictionInterval int32   `json:"prediction_interval"`
	ThresholdBreached  bool    `json:"threshold_breached,omitempty"`
}

// CostReport is the root domain model containing metadata, totals, and line items.
type CostReport struct {
	Metadata         ReportMetadata    `json:"metadata"`
	Totals           TotalCosts        `json:"totals"`
	Items            []LineItem        `json:"items"`
	HasComparison    bool              `json:"has_comparison"`
	MonthlyBreakdown *MonthlyBreakdown `json:"monthly_breakdown,omitempty"`
	DailyBreakdown   *DailyBreakdown   `json:"daily_breakdown,omitempty"`
	Forecast         *CostForecast     `json:"forecast,omitempty"`
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
