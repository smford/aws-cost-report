package renderer

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

const (
	tableWidth = 104
	divDouble  = "========================================================================================================"
	divSingle  = "--------------------------------------------------------------------------------------------------------"
)

// RenderTable formats the CostReport as a fixed-width terminal ASCII table.
func RenderTable(w io.Writer, r *awsclient.CostReport, noColor bool) error {
	if r.MonthlyBreakdown != nil && len(r.MonthlyBreakdown.Months) > 1 {
		return renderMonthlyTable(w, r, noColor)
	}

	var b strings.Builder

	// Header banner
	b.WriteString(divDouble + "\n")
	title := "AWS MONTHLY BILLING & USAGE REPORT"
	leftPad := (tableWidth - len(title)) / 2
	b.WriteString(strings.Repeat(" ", leftPad) + title + "\n")
	b.WriteString(divDouble + "\n")

	// Metadata section
	b.WriteString(fmt.Sprintf(" Account ID:         %s\n", r.Metadata.AccountID))
	b.WriteString(fmt.Sprintf(" Billing Month:      %s\n", r.Metadata.BillingMonth))
	b.WriteString(fmt.Sprintf(" Period:             %s to %s (inclusive)\n", r.Metadata.PeriodStart, r.Metadata.PeriodEnd))
	b.WriteString(fmt.Sprintf(" Currency:           %s\n", r.Metadata.Currency))
	b.WriteString(fmt.Sprintf(" Estimated / Final:  %s\n", r.Metadata.EstimatedOrFinal))
	if r.Metadata.TagFilter != "" {
		b.WriteString(fmt.Sprintf(" Tag Filter:         %s\n", r.Metadata.TagFilter))
	}
	groupLabel := "Services"
	if r.Metadata.GroupKey != "" && !strings.EqualFold(r.Metadata.GroupKey, "SERVICE") {
		groupLabel = r.Metadata.GroupKey
	}
	b.WriteString(fmt.Sprintf(" Active %-12s %d\n", groupLabel+":", r.Metadata.ActiveServices))

	// Totals summary section
	sym := r.Metadata.CurrencySymbol()
	diffHeader := fmt.Sprintf("Diff (%s)", sym)

	b.WriteString(divSingle + "\n")
	if r.HasComparison {
		b.WriteString(fmt.Sprintf(" %-24s | %18s | %17s | %16s | %16s\n", "Metric", "Amount ("+r.Metadata.Currency+")", "Prev ("+r.Metadata.Currency+")", diffHeader, "% Δ"))
		b.WriteString(divSingle + "\n")

		unblendedDiffStr := colorize(r.Totals.MoMDeltaAmount, formatTableDiff(r.Totals.MoMDeltaAmount, 16), noColor)
		unblendedPctStr := colorize(r.Totals.MoMDeltaPercent, formatTablePct(r.Totals.MoMDeltaPercent, 16), noColor)
		b.WriteString(fmt.Sprintf(" %-24s | %18.2f | %17.2f | %s | %s\n", "Unblended Cost", r.Totals.UnblendedCost, r.Totals.PrevUnblendedCost, unblendedDiffStr, unblendedPctStr))

		blendedDiff := r.Totals.BlendedCost - r.Totals.PrevBlendedCost
		blendedPct := calcPercent(r.Totals.BlendedCost, r.Totals.PrevBlendedCost)
		blendedDiffStr := colorize(blendedDiff, formatTableDiff(blendedDiff, 16), noColor)
		blendedPctStr := colorize(blendedPct, formatTablePct(blendedPct, 16), noColor)
		b.WriteString(fmt.Sprintf(" %-24s | %18.2f | %17.2f | %s | %s\n", "Blended Cost", r.Totals.BlendedCost, r.Totals.PrevBlendedCost, blendedDiffStr, blendedPctStr))

		amortDiff := r.Totals.AmortizedCost - r.Totals.PrevAmortizedCost
		amortPct := calcPercent(r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost)
		amortDiffStr := colorize(amortDiff, formatTableDiff(amortDiff, 16), noColor)
		amortPctStr := colorize(amortPct, formatTablePct(amortPct, 16), noColor)
		b.WriteString(fmt.Sprintf(" %-24s | %18.2f | %17.2f | %s | %s\n", "Amortized Cost", r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost, amortDiffStr, amortPctStr))

		if r.Totals.NetUnblendedCost > 0 || r.Totals.PrevNetUnblendedCost > 0 {
			netDiff := r.Totals.NetUnblendedCost - r.Totals.PrevNetUnblendedCost
			netPct := calcPercent(r.Totals.NetUnblendedCost, r.Totals.PrevNetUnblendedCost)
			netDiffStr := colorize(netDiff, formatTableDiff(netDiff, 16), noColor)
			netPctStr := colorize(netPct, formatTablePct(netPct, 16), noColor)
			b.WriteString(fmt.Sprintf(" %-24s | %18.2f | %17.2f | %s | %s\n", "Net Unblended Cost", r.Totals.NetUnblendedCost, r.Totals.PrevNetUnblendedCost, netDiffStr, netPctStr))
		}
		if r.Totals.NetAmortizedCost > 0 || r.Totals.PrevNetAmortizedCost > 0 {
			netAmortDiff := r.Totals.NetAmortizedCost - r.Totals.PrevNetAmortizedCost
			netAmortPct := calcPercent(r.Totals.NetAmortizedCost, r.Totals.PrevNetAmortizedCost)
			netAmortDiffStr := colorize(netAmortDiff, formatTableDiff(netAmortDiff, 16), noColor)
			netAmortPctStr := colorize(netAmortPct, formatTablePct(netAmortPct, 16), noColor)
			b.WriteString(fmt.Sprintf(" %-24s | %18.2f | %17.2f | %s | %s\n", "Net Amortized Cost", r.Totals.NetAmortizedCost, r.Totals.PrevNetAmortizedCost, netAmortDiffStr, netAmortPctStr))
		}
	} else {
		b.WriteString(fmt.Sprintf(" %-18s | %18s\n", "Metric", "Amount ("+r.Metadata.Currency+")"))
		b.WriteString(divSingle + "\n")
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Blended Cost", r.Totals.BlendedCost))
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Unblended Cost", r.Totals.UnblendedCost))
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Amortized Cost", r.Totals.AmortizedCost))
		if r.Totals.NetUnblendedCost > 0 {
			b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Net Unblended Cost", r.Totals.NetUnblendedCost))
		}
		if r.Totals.NetAmortizedCost > 0 {
			b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Net Amortized Cost", r.Totals.NetAmortizedCost))
		}
	}
	b.WriteString(divDouble + "\n\n")

	// Service/Item breakdown table
	b.WriteString(divSingle + "\n")
	headerCol := "Service Name"
	if r.Metadata.GroupKey != "" && !strings.EqualFold(r.Metadata.GroupKey, "SERVICE") {
		headerCol = r.Metadata.GroupKey
	}

	if r.HasComparison {
		b.WriteString(fmt.Sprintf(" %-42s | %15s | %14s | %10s | %10s\n", truncate(headerCol, 42), "Unblended ("+r.Metadata.Currency+")", "Prev ("+r.Metadata.Currency+")", truncate(diffHeader, 10), "% Δ"))
		b.WriteString(divSingle + "\n")
		for _, item := range r.Items {
			diffStr := formatTableDiff(item.MoMDeltaAmount, 10)
			pctStr := formatTablePct(item.MoMDeltaPercent, 10)

			coloredDiff := colorize(item.MoMDeltaAmount, diffStr, noColor)
			coloredPct := colorize(item.MoMDeltaPercent, pctStr, noColor)

			b.WriteString(fmt.Sprintf(" %-42s | %15.2f | %14.2f | %s | %s\n",
				truncate(item.Name, 42),
				item.UnblendedCost,
				item.PrevUnblendedCost,
				coloredDiff,
				coloredPct,
			))
		}
	} else {
		b.WriteString(fmt.Sprintf(" %-46s | %16s | %16s | %16s\n", truncate(headerCol, 46), "Unblended ("+r.Metadata.Currency+")", "Blended ("+r.Metadata.Currency+")", "Amortized ("+r.Metadata.Currency+")"))
		b.WriteString(divSingle + "\n")
		for _, item := range r.Items {
			b.WriteString(fmt.Sprintf(" %-46s | %16.2f | %16.2f | %16.2f\n",
				truncate(item.Name, 46),
				item.UnblendedCost,
				item.BlendedCost,
				item.AmortizedCost,
			))
		}
	}
	b.WriteString(divDouble + "\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func colorize(val float64, s string, noColor bool) string {
	if noColor || val == 0 {
		return s
	}
	if val < 0 {
		return "\033[32m" + s + "\033[0m" // green for decrease/savings
	}
	return "\033[31m" + s + "\033[0m" // red for increase
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func calcPercent(current, prev float64) float64 {
	if prev == 0 {
		if current == 0 {
			return 0
		}
		return 100
	}
	return ((current - prev) / prev) * 100.0
}

func formatTableDiff(val float64, width int) string {
	if math.Abs(val) < 0.005 {
		return fmt.Sprintf("%*.2f", width, 0.0)
	}
	return fmt.Sprintf("%+*.2f", width, val)
}

func formatTablePct(val float64, width int) string {
	if math.Abs(val) < 0.005 {
		return fmt.Sprintf("%*.2f%%", width-1, 0.0)
	}
	return fmt.Sprintf("%+*.2f%%", width-1, val)
}

func renderMonthlyTable(w io.Writer, r *awsclient.CostReport, noColor bool) error {
	var b strings.Builder
	mb := r.MonthlyBreakdown

	// Calculate column widths
	serviceColWidth := 24
	for _, it := range mb.Items {
		if len(it.Name) > serviceColWidth {
			serviceColWidth = len(it.Name)
		}
	}
	if serviceColWidth > 32 {
		serviceColWidth = 32
	}

	monthColWidth := 8
	totalColWidth := 10

	calcTableWidth := 1 + serviceColWidth + 3 + len(mb.Months)*(monthColWidth+3) + totalColWidth + 1
	if calcTableWidth < tableWidth {
		calcTableWidth = tableWidth
	}
	divDbl := strings.Repeat("=", calcTableWidth)
	divSgl := strings.Repeat("-", calcTableWidth)

	// Header banner
	b.WriteString(divDbl + "\n")
	title := "AWS MONTHLY BILLING & USAGE REPORT (HISTORICAL TREND)"
	leftPad := (calcTableWidth - len(title)) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	b.WriteString(strings.Repeat(" ", leftPad) + title + "\n")
	b.WriteString(divDbl + "\n")

	// Metadata
	b.WriteString(fmt.Sprintf(" Account ID:         %s\n", r.Metadata.AccountID))
	if r.Metadata.AccountName != "" {
		b.WriteString(fmt.Sprintf(" Account Name:       %s\n", r.Metadata.AccountName))
	}
	b.WriteString(fmt.Sprintf(" Billing Period:     %s (%d Months)\n", r.Metadata.BillingMonth, len(mb.Months)))
	b.WriteString(fmt.Sprintf(" Period:             %s to %s (inclusive)\n", r.Metadata.PeriodStart, r.Metadata.PeriodEnd))
	b.WriteString(fmt.Sprintf(" Currency:           %s\n", r.Metadata.Currency))
	b.WriteString(fmt.Sprintf(" Estimated / Final:  %s\n", r.Metadata.EstimatedOrFinal))
	if r.Metadata.TagFilter != "" {
		b.WriteString(fmt.Sprintf(" Tag Filter:         %s\n", r.Metadata.TagFilter))
	}
	groupLabel := "Services"
	if r.Metadata.GroupKey != "" && !strings.EqualFold(r.Metadata.GroupKey, "SERVICE") {
		groupLabel = r.Metadata.GroupKey
	}
	b.WriteString(fmt.Sprintf(" Active %-12s %d\n", groupLabel+":", len(mb.Items)))
	b.WriteString(fmt.Sprintf(" Grand Total:        %s%.2f\n", r.Metadata.CurrencySymbol(), mb.GrandTotal))
	b.WriteString(divSgl + "\n")

	// Table header
	headerCol := "Service Name"
	if r.Metadata.GroupKey != "" && !strings.EqualFold(r.Metadata.GroupKey, "SERVICE") {
		headerCol = r.Metadata.GroupKey
	}
	b.WriteString(fmt.Sprintf(" %-*s", serviceColWidth, truncate(headerCol, serviceColWidth)))
	for _, m := range mb.Months {
		b.WriteString(fmt.Sprintf(" | %*s", monthColWidth, m.Label))
	}
	b.WriteString(fmt.Sprintf(" | %*s\n", totalColWidth, "Total ("+r.Metadata.Currency+")"))
	b.WriteString(divSgl + "\n")

	// Table rows
	for _, it := range mb.Items {
		b.WriteString(fmt.Sprintf(" %-*s", serviceColWidth, truncate(it.Name, serviceColWidth)))
		for _, m := range mb.Months {
			cost := it.Costs[m.Key]
			b.WriteString(fmt.Sprintf(" | %*.2f", monthColWidth, cost))
		}
		b.WriteString(fmt.Sprintf(" | %*.2f\n", totalColWidth, it.Total))
	}
	b.WriteString(divSgl + "\n")

	// Table total row
	totalLabel := fmt.Sprintf("Total (%s)", r.Metadata.Currency)
	b.WriteString(fmt.Sprintf(" %-*s", serviceColWidth, totalLabel))
	for _, m := range mb.Months {
		mTot := mb.Totals[m.Key]
		b.WriteString(fmt.Sprintf(" | %*.2f", monthColWidth, mTot))
	}
	b.WriteString(fmt.Sprintf(" | %*.2f\n", totalColWidth, mb.GrandTotal))
	b.WriteString(divDbl + "\n")

	_, err := io.WriteString(w, b.String())
	return err
}
