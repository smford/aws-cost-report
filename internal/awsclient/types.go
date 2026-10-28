package awsclient

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
	BillingMonth     string `json:"billing_month"`
	PeriodStart      string `json:"period_start"`
	PeriodEnd        string `json:"period_end"`
	AWSPeriodStart   string `json:"aws_period_start"`
	AWSPeriodEnd     string `json:"aws_period_end"`
	Currency         string `json:"currency"`
	EstimatedOrFinal string `json:"estimated_or_final"`
	ActiveServices   int    `json:"active_services"`
	GroupKey         string `json:"group_key"`
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
