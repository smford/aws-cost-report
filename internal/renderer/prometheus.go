package renderer

import (
	"fmt"
	"io"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

func escapePrometheusLabel(val string) string {
	val = strings.ReplaceAll(val, `\`, `\\`)
	val = strings.ReplaceAll(val, `"`, `\"`)
	return strings.ReplaceAll(val, "\n", `\n`)
}

// RenderPrometheus writes the CostReport in Prometheus text exposition format (version 0.0.4).
func RenderPrometheus(w io.Writer, r *awsclient.CostReport, threshold float64, version, commit, date string) error {
	var sb strings.Builder

	// Build info
	sb.WriteString("# HELP aws_cost_report_build_info Build metadata for aws-cost-report binary\n")
	sb.WriteString("# TYPE aws_cost_report_build_info gauge\n")
	sb.WriteString(fmt.Sprintf("aws_cost_report_build_info{version=\"%s\",commit=\"%s\",build_date=\"%s\"} 1\n",
		escapePrometheusLabel(version), escapePrometheusLabel(commit), escapePrometheusLabel(date)))

	accID := escapePrometheusLabel(r.Metadata.AccountID)
	accName := escapePrometheusLabel(r.Metadata.AccountName)
	currency := escapePrometheusLabel(r.Metadata.Currency)
	groupKey := escapePrometheusLabel(r.Metadata.GroupKey)
	month := escapePrometheusLabel(r.Metadata.BillingMonth)

	labels := fmt.Sprintf("account=\"%s\",account_name=\"%s\",currency=\"%s\",billing_month=\"%s\"",
		accID, accName, currency, month)

	// Total metrics
	sb.WriteString("# HELP aws_cost_report_total_unblended_dollars Total unblended cost for the billing period\n")
	sb.WriteString("# TYPE aws_cost_report_total_unblended_dollars gauge\n")
	sb.WriteString(fmt.Sprintf("aws_cost_report_total_unblended_dollars{%s} %.2f\n", labels, r.Totals.UnblendedCost))

	sb.WriteString("# HELP aws_cost_report_total_blended_dollars Total blended cost for the billing period\n")
	sb.WriteString("# TYPE aws_cost_report_total_blended_dollars gauge\n")
	sb.WriteString(fmt.Sprintf("aws_cost_report_total_blended_dollars{%s} %.2f\n", labels, r.Totals.BlendedCost))

	sb.WriteString("# HELP aws_cost_report_total_amortized_dollars Total amortized cost for the billing period\n")
	sb.WriteString("# TYPE aws_cost_report_total_amortized_dollars gauge\n")
	sb.WriteString(fmt.Sprintf("aws_cost_report_total_amortized_dollars{%s} %.2f\n", labels, r.Totals.AmortizedCost))

	if r.Totals.NetUnblendedCost > 0 {
		sb.WriteString("# HELP aws_cost_report_total_net_unblended_dollars Total net unblended cost reflecting EDP discounts\n")
		sb.WriteString("# TYPE aws_cost_report_total_net_unblended_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_total_net_unblended_dollars{%s} %.2f\n", labels, r.Totals.NetUnblendedCost))
	}

	if r.Totals.NetAmortizedCost > 0 {
		sb.WriteString("# HELP aws_cost_report_total_net_amortized_dollars Total net amortized cost reflecting EDP discounts\n")
		sb.WriteString("# TYPE aws_cost_report_total_net_amortized_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_total_net_amortized_dollars{%s} %.2f\n", labels, r.Totals.NetAmortizedCost))
	}

	if r.HasComparison {
		sb.WriteString("# HELP aws_cost_report_previous_total_unblended_dollars Previous period total unblended cost\n")
		sb.WriteString("# TYPE aws_cost_report_previous_total_unblended_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_previous_total_unblended_dollars{%s} %.2f\n", labels, r.Totals.PrevUnblendedCost))

		sb.WriteString("# HELP aws_cost_report_mom_delta_dollars Month-over-Month absolute cost delta amount\n")
		sb.WriteString("# TYPE aws_cost_report_mom_delta_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_mom_delta_dollars{%s} %.2f\n", labels, r.Totals.MoMDeltaAmount))

		sb.WriteString("# HELP aws_cost_report_mom_delta_percent Month-over-Month cost percentage change\n")
		sb.WriteString("# TYPE aws_cost_report_mom_delta_percent gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_mom_delta_percent{%s} %.2f\n", labels, r.Totals.MoMDeltaPercent))
	}

	if threshold > 0.00 {
		sb.WriteString("# HELP aws_cost_report_budget_threshold_dollars Configured budget alert threshold\n")
		sb.WriteString("# TYPE aws_cost_report_budget_threshold_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_budget_threshold_dollars{%s} %.2f\n", labels, threshold))

		sb.WriteString("# HELP aws_cost_report_budget_threshold_breached Budget breach indicator (1 = breached, 0 = within budget)\n")
		sb.WriteString("# TYPE aws_cost_report_budget_threshold_breached gauge\n")
		breached := 0
		if r.Totals.UnblendedCost > threshold {
			breached = 1
		}
		sb.WriteString(fmt.Sprintf("aws_cost_report_budget_threshold_breached{%s} %d\n", labels, breached))
	}

	if r.Forecast != nil {
		sb.WriteString("# HELP aws_cost_report_forecast_total_dollars Projected month-end total spend\n")
		sb.WriteString("# TYPE aws_cost_report_forecast_total_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_forecast_total_dollars{%s} %.2f\n", labels, r.Forecast.TotalAmount))

		sb.WriteString("# HELP aws_cost_report_forecast_remaining_dollars Forecasted remaining spend until month end\n")
		sb.WriteString("# TYPE aws_cost_report_forecast_remaining_dollars gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_forecast_remaining_dollars{%s} %.2f\n", labels, r.Forecast.MeanAmount))

		forecastBreach := 0
		if r.Forecast.ThresholdBreached {
			forecastBreach = 1
		}
		sb.WriteString("# HELP aws_cost_report_forecast_threshold_breached 1 if projected spend breaches budget threshold\n")
		sb.WriteString("# TYPE aws_cost_report_forecast_threshold_breached gauge\n")
		sb.WriteString(fmt.Sprintf("aws_cost_report_forecast_threshold_breached{%s} %d\n", labels, forecastBreach))
	}

	if r.DailyBreakdown != nil {
		sb.WriteString("# HELP aws_cost_report_daily_spend_dollars Daily spend breakdown over billing period\n")
		sb.WriteString("# TYPE aws_cost_report_daily_spend_dollars gauge\n")
		for _, day := range r.DailyBreakdown.Days {
			dayLabels := fmt.Sprintf("%s,date=\"%s\",day=\"%s\"", labels, day.Date, day.DayOfWeek)
			sb.WriteString(fmt.Sprintf("aws_cost_report_daily_spend_dollars{%s} %.2f\n", dayLabels, day.Total))
		}
	}

	// Service/Item breakdown metrics
	sb.WriteString("# HELP aws_cost_report_service_unblended_dollars Unblended cost breakdown by service or dimension\n")
	sb.WriteString("# TYPE aws_cost_report_service_unblended_dollars gauge\n")
	for _, it := range r.Items {
		itemLabels := fmt.Sprintf("%s,dimension=\"%s\",item=\"%s\"", labels, groupKey, escapePrometheusLabel(it.Name))
		sb.WriteString(fmt.Sprintf("aws_cost_report_service_unblended_dollars{%s} %.2f\n", itemLabels, it.UnblendedCost))
	}

	if r.HasComparison {
		sb.WriteString("# HELP aws_cost_report_service_mom_delta_dollars MoM cost delta by service or dimension\n")
		sb.WriteString("# TYPE aws_cost_report_service_mom_delta_dollars gauge\n")
		for _, it := range r.Items {
			if it.HasPreviousCost {
				itemLabels := fmt.Sprintf("%s,dimension=\"%s\",item=\"%s\"", labels, groupKey, escapePrometheusLabel(it.Name))
				sb.WriteString(fmt.Sprintf("aws_cost_report_service_mom_delta_dollars{%s} %.2f\n", itemLabels, it.MoMDeltaAmount))
			}
		}
	}

	_, err := io.WriteString(w, sb.String())
	return err
}
