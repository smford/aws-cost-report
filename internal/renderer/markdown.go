package renderer

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderMarkdown generates a GitHub-Flavored Markdown report and optionally writes to $GITHUB_STEP_SUMMARY.
func RenderMarkdown(w io.Writer, r *awsclient.CostReport, writeJobSummary bool, failOnThreshold float64) error {
	if r.MonthlyBreakdown != nil && len(r.MonthlyBreakdown.Months) > 1 {
		return renderMonthlyMarkdown(w, r, writeJobSummary, failOnThreshold)
	}

	var b strings.Builder
	sym := r.Metadata.CurrencySymbol()
	deltaHeader := fmt.Sprintf("Delta (%s)", sym)

	b.WriteString("# 📊 AWS Monthly Billing & Usage Report\n\n")

	if failOnThreshold > 0.00 && r.Totals.UnblendedCost > failOnThreshold {
		b.WriteString(fmt.Sprintf("> [!CAUTION]\n> **Budget Guardrail Alert**: Total billing of **%s%.2f %s** exceeds the configured threshold of **%s%.2f %s**!\n\n",
			sym, r.Totals.UnblendedCost, r.Metadata.Currency, sym, failOnThreshold, r.Metadata.Currency))
	}

	// Metadata Table
	b.WriteString("### 📋 Report Metadata\n\n")
	b.WriteString("| Attribute | Value |\n")
	b.WriteString("|:---|:---|\n")
	b.WriteString(fmt.Sprintf("| **Account ID** | `%s` |\n", r.Metadata.AccountID))
	b.WriteString(fmt.Sprintf("| **Billing Month** | %s |\n", r.Metadata.BillingMonth))
	b.WriteString(fmt.Sprintf("| **Period** | %s to %s (inclusive) |\n", r.Metadata.PeriodStart, r.Metadata.PeriodEnd))
	b.WriteString(fmt.Sprintf("| **Currency** | %s |\n", r.Metadata.Currency))
	b.WriteString(fmt.Sprintf("| **Status** | `%s` |\n", r.Metadata.EstimatedOrFinal))
	if r.Metadata.TagFilter != "" {
		b.WriteString(fmt.Sprintf("| **Tag Filter** | `%s` |\n", r.Metadata.TagFilter))
	}
	if r.Metadata.ChargeTypeFilter != "" {
		b.WriteString(fmt.Sprintf("| **Charge Types** | `%s` |\n", r.Metadata.ChargeTypeFilter))
	} else if r.Metadata.ExcludeCredits {
		b.WriteString("| **Credits / Refunds** | `Excluded` |\n")
	}
	b.WriteString(fmt.Sprintf("| **Active Line Items** | %d |\n\n", r.Metadata.ActiveServices))

	// Total Metrics Table
	b.WriteString("### 💰 Overall Cost Summary\n\n")
	if r.HasComparison {
		b.WriteString("| Metric | Amount (" + r.Metadata.Currency + ") | Previous (" + r.Metadata.Currency + ") | " + deltaHeader + " | Change (%) |\n")
		b.WriteString("|:---|---:|---:|---:|---:|\n")
		b.WriteString(fmt.Sprintf("| **Unblended Cost** | **%s%.2f** | %s%.2f | %s | %s |\n",
			sym, r.Totals.UnblendedCost, sym, r.Totals.PrevUnblendedCost, formatCurrencyDelta(r.Totals.MoMDeltaAmount, sym), formatPercentDelta(r.Totals.MoMDeltaPercent)))
		blendedDelta := r.Totals.BlendedCost - r.Totals.PrevBlendedCost
		b.WriteString(fmt.Sprintf("| **Blended Cost** | **%s%.2f** | %s%.2f | %s | %s |\n",
			sym, r.Totals.BlendedCost, sym, r.Totals.PrevBlendedCost, formatCurrencyDelta(blendedDelta, sym), formatPercentDelta(calcPercent(r.Totals.BlendedCost, r.Totals.PrevBlendedCost))))
		amortDelta := r.Totals.AmortizedCost - r.Totals.PrevAmortizedCost
		b.WriteString(fmt.Sprintf("| **Amortized Cost** | **%s%.2f** | %s%.2f | %s | %s |\n",
			sym, r.Totals.AmortizedCost, sym, r.Totals.PrevAmortizedCost, formatCurrencyDelta(amortDelta, sym), formatPercentDelta(calcPercent(r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost))))
		if r.Totals.NetUnblendedCost > 0 || r.Totals.PrevNetUnblendedCost > 0 {
			netDelta := r.Totals.NetUnblendedCost - r.Totals.PrevNetUnblendedCost
			b.WriteString(fmt.Sprintf("| **Net Unblended Cost** | **%s%.2f** | %s%.2f | %s | %s |\n",
				sym, r.Totals.NetUnblendedCost, sym, r.Totals.PrevNetUnblendedCost, formatCurrencyDelta(netDelta, sym), formatPercentDelta(calcPercent(r.Totals.NetUnblendedCost, r.Totals.PrevNetUnblendedCost))))
		}
		if r.Totals.NetAmortizedCost > 0 || r.Totals.PrevNetAmortizedCost > 0 {
			netAmortDelta := r.Totals.NetAmortizedCost - r.Totals.PrevNetAmortizedCost
			b.WriteString(fmt.Sprintf("| **Net Amortized Cost** | **%s%.2f** | %s%.2f | %s | %s |\n",
				sym, r.Totals.NetAmortizedCost, sym, r.Totals.PrevNetAmortizedCost, formatCurrencyDelta(netAmortDelta, sym), formatPercentDelta(calcPercent(r.Totals.NetAmortizedCost, r.Totals.PrevNetAmortizedCost))))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("| Metric | Amount (" + r.Metadata.Currency + ") |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString(fmt.Sprintf("| **Blended Cost** | %s%.2f |\n", sym, r.Totals.BlendedCost))
		b.WriteString(fmt.Sprintf("| **Unblended Cost** | %s%.2f |\n", sym, r.Totals.UnblendedCost))
		b.WriteString(fmt.Sprintf("| **Amortized Cost** | %s%.2f |\n", sym, r.Totals.AmortizedCost))
		if r.Totals.NetUnblendedCost > 0 {
			b.WriteString(fmt.Sprintf("| **Net Unblended Cost** | %s%.2f |\n", sym, r.Totals.NetUnblendedCost))
		}
		if r.Totals.NetAmortizedCost > 0 {
			b.WriteString(fmt.Sprintf("| **Net Amortized Cost** | %s%.2f |\n", sym, r.Totals.NetAmortizedCost))
		}
		b.WriteString("\n")
	}

	// Service/Dimension Breakdown Table
	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" || strings.EqualFold(groupLabel, "SERVICE") {
		groupLabel = "Service Name"
	}
	b.WriteString(fmt.Sprintf("### 🔍 Itemized Breakdown by %s\n\n", groupLabel))

	if r.HasComparison {
		b.WriteString(fmt.Sprintf("| %s | Unblended (%s) | Previous (%s) | %s | Change (%%) |\n", groupLabel, r.Metadata.Currency, r.Metadata.Currency, deltaHeader))
		b.WriteString("|:---|---:|---:|---:|---:|\n")
		for _, item := range r.Items {
			b.WriteString(fmt.Sprintf("| %s | %s%.2f | %s%.2f | %s | %s |\n",
				item.Name,
				sym,
				item.UnblendedCost,
				sym,
				item.PrevUnblendedCost,
				formatCurrencyDelta(item.MoMDeltaAmount, sym),
				formatPercentDelta(item.MoMDeltaPercent),
			))
		}
	} else {
		b.WriteString(fmt.Sprintf("| %s | Unblended (%s) | Blended (%s) | Amortized (%s) |\n", groupLabel, r.Metadata.Currency, r.Metadata.Currency, r.Metadata.Currency))
		b.WriteString("|:---|---:|---:|---:|\n")
		for _, item := range r.Items {
			b.WriteString(fmt.Sprintf("| %s | %s%.2f | %s%.2f | %s%.2f |\n",
				item.Name,
				sym,
				item.UnblendedCost,
				sym,
				item.BlendedCost,
				sym,
				item.AmortizedCost,
			))
		}
	}
	if r.Forecast != nil {
		statusBadge := "✅ `Within Budget`"
		if r.Forecast.ThresholdBreached {
			statusBadge = "🚨 **PROJECTED TO BREACH BUDGET**"
		}
		b.WriteString("\n### 🔮 Projected Month-End Spend Forecast\n\n")
		b.WriteString(fmt.Sprintf("| Metric | Value (%s) |\n", r.Metadata.Currency))
		b.WriteString("|:---|---:|\n")
		b.WriteString(fmt.Sprintf("| **Projected Month-End Spend** | **%s%.2f** |\n", sym, r.Forecast.TotalAmount))
		b.WriteString(fmt.Sprintf("| Forecasted Remaining Spend | %s%.2f |\n", sym, r.Forecast.MeanAmount))
		b.WriteString(fmt.Sprintf("| Forecast Window | `%s` to `%s` |\n", r.Forecast.PeriodStart, r.Forecast.PeriodEnd))
		b.WriteString(fmt.Sprintf("| Confidence Level | %d%% |\n", r.Forecast.PredictionInterval))
		b.WriteString(fmt.Sprintf("| Guardrail Status | %s |\n", statusBadge))
	}

	if r.DailyBreakdown != nil && len(r.DailyBreakdown.Days) > 0 {
		b.WriteString("\n### 📅 Daily Spend Breakdown\n\n")
		b.WriteString(fmt.Sprintf("| Date | Day | Spend (%s) | Top Cost Driver |\n", r.Metadata.Currency))
		b.WriteString("|:---|:---:|---:|:---|\n")
		for _, day := range r.DailyBreakdown.Days {
			topDriverStr := "-"
			if day.TopDriverName != "" {
				topDriverStr = fmt.Sprintf("%s (%s%.2f)", day.TopDriverName, sym, day.TopDriverAmount)
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s%.2f | %s |\n", day.Date, day.DayOfWeek, sym, day.Total, topDriverStr))
		}
		b.WriteString(fmt.Sprintf("\n**Total (%d days):** %s%.2f &nbsp;•&nbsp; **Daily Average:** %s%.2f / day\n", len(r.DailyBreakdown.Days), sym, r.DailyBreakdown.GrandTotal, sym, r.DailyBreakdown.Average))
	}

	b.WriteString("\n> _Generated automatically by [aws-cost-report](https://github.com/smford/aws-cost-report)_\n")

	mdContent := b.String()

	if _, err := io.WriteString(w, mdContent); err != nil {
		return err
	}

	// Append to GitHub Actions Job Summary if available and enabled
	if writeJobSummary {
		if summaryFile := os.Getenv("GITHUB_STEP_SUMMARY"); summaryFile != "" {
			f, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				defer f.Close()
				_, _ = f.WriteString(mdContent + "\n")
			}
		}
	}

	return nil
}

func formatCurrencyDelta(v float64, sym string) string {
	if sym == "" {
		sym = "$"
	}
	if math.Abs(v) < 0.005 {
		return fmt.Sprintf("%s0.00", sym)
	}
	if v > 0 {
		return fmt.Sprintf("+%s%.2f", sym, v)
	}
	return fmt.Sprintf("-%s%.2f", sym, math.Abs(v))
}

func formatPercentDelta(v float64) string {
	if math.Abs(v) < 0.005 {
		return "0.00%"
	}
	if v > 0 {
		return fmt.Sprintf("+%.2f%%", v)
	}
	return fmt.Sprintf("%.2f%%", v)
}

func renderMonthlyMarkdown(w io.Writer, r *awsclient.CostReport, writeJobSummary bool, failOnThreshold float64) error {
	var b strings.Builder
	sym := r.Metadata.CurrencySymbol()
	mb := r.MonthlyBreakdown

	b.WriteString("# 📊 AWS Monthly Billing & Usage Report\n\n")

	if failOnThreshold > 0.00 && mb.GrandTotal > failOnThreshold {
		b.WriteString(fmt.Sprintf("> [!CAUTION]\n> **Budget Guardrail Alert**: Grand total billing of **%s%.2f %s** exceeds the configured threshold of **%s%.2f %s**!\n\n",
			sym, mb.GrandTotal, r.Metadata.Currency, sym, failOnThreshold, r.Metadata.Currency))
	}

	// Metadata Table
	b.WriteString("### 📋 Report Metadata\n\n")
	b.WriteString("| Attribute | Value |\n")
	b.WriteString("|:---|:---|\n")
	b.WriteString(fmt.Sprintf("| **Account ID** | `%s` |\n", r.Metadata.AccountID))
	if r.Metadata.AccountName != "" {
		b.WriteString(fmt.Sprintf("| **Account Name** | `%s` |\n", r.Metadata.AccountName))
	}
	b.WriteString(fmt.Sprintf("| **Billing Period** | %s (%d Months) |\n", r.Metadata.BillingMonth, len(mb.Months)))
	b.WriteString(fmt.Sprintf("| **Period** | %s to %s (inclusive) |\n", r.Metadata.PeriodStart, r.Metadata.PeriodEnd))
	b.WriteString(fmt.Sprintf("| **Currency** | %s |\n", r.Metadata.Currency))
	b.WriteString(fmt.Sprintf("| **Status** | `%s` |\n", r.Metadata.EstimatedOrFinal))
	if r.Metadata.TagFilter != "" {
		b.WriteString(fmt.Sprintf("| **Tag Filter** | `%s` |\n", r.Metadata.TagFilter))
	}
	if r.Metadata.ChargeTypeFilter != "" {
		b.WriteString(fmt.Sprintf("| **Charge Types** | `%s` |\n", r.Metadata.ChargeTypeFilter))
	} else if r.Metadata.ExcludeCredits {
		b.WriteString("| **Credits / Refunds** | `Excluded` |\n")
	}
	b.WriteString(fmt.Sprintf("| **Active Line Items** | %d |\n", len(mb.Items)))
	b.WriteString(fmt.Sprintf("| **Grand Total** | **%s%.2f %s** |\n\n", sym, mb.GrandTotal, r.Metadata.Currency))

	// Monthly Breakdown Table
	b.WriteString(fmt.Sprintf("### 📅 Monthly Cost Breakdown (%d Months)\n\n", len(mb.Months)))

	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" || strings.EqualFold(groupLabel, "SERVICE") {
		groupLabel = "Service Name"
	}

	b.WriteString(fmt.Sprintf("| %s", groupLabel))
	for _, m := range mb.Months {
		b.WriteString(fmt.Sprintf(" | %s", m.Label))
	}
	b.WriteString(fmt.Sprintf(" | Total (%s) |\n", r.Metadata.Currency))

	b.WriteString("|:---")
	for range mb.Months {
		b.WriteString("|---:")
	}
	b.WriteString("|---:|\n")

	for _, it := range mb.Items {
		b.WriteString(fmt.Sprintf("| %s", it.Name))
		for _, m := range mb.Months {
			b.WriteString(fmt.Sprintf(" | %s%.2f", sym, it.Costs[m.Key]))
		}
		b.WriteString(fmt.Sprintf(" | **%s%.2f** |\n", sym, it.Total))
	}

	// Total row
	b.WriteString("| **Total**")
	for _, m := range mb.Months {
		b.WriteString(fmt.Sprintf(" | **%s%.2f**", sym, mb.Totals[m.Key]))
	}
	b.WriteString(fmt.Sprintf(" | **%s%.2f** |\n\n", sym, mb.GrandTotal))

	output := b.String()
	if writeJobSummary {
		if summaryFile := os.Getenv("GITHUB_STEP_SUMMARY"); summaryFile != "" {
			_ = os.WriteFile(summaryFile, []byte(output), 0644)
		}
	}

	_, err := io.WriteString(w, output)
	return err
}
