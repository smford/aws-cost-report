package renderer

import (
	"fmt"
	"io"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

const (
	tableWidth = 104
	divDouble  = "========================================================================================================"
	divSingle  = "--------------------------------------------------------------------------------------------------------"
)

// RenderTable formats the CostReport as a fixed-width terminal ASCII table.
func RenderTable(w io.Writer, r *awsclient.CostReport) error {
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
	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" {
		groupLabel = "Services"
	}
	b.WriteString(fmt.Sprintf(" Active %-12s %d\n", groupLabel+":", r.Metadata.ActiveServices))

	// Totals summary section
	b.WriteString(divSingle + "\n")
	if r.HasComparison {
		b.WriteString(fmt.Sprintf(" %-18s | %18s | %18s | %18s | %16s\n", "Metric", "Amount ("+r.Metadata.Currency+")", "Prev ("+r.Metadata.Currency+")", "Diff ($)", "% Δ"))
		b.WriteString(divSingle + "\n")
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f | %18.2f | %18.2f | %15.2f%%\n", "Unblended Cost", r.Totals.UnblendedCost, r.Totals.PrevUnblendedCost, r.Totals.MoMDeltaAmount, r.Totals.MoMDeltaPercent))
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f | %18.2f | %18.2f | %15.2f%%\n", "Blended Cost", r.Totals.BlendedCost, r.Totals.PrevBlendedCost, r.Totals.BlendedCost-r.Totals.PrevBlendedCost, calcPercent(r.Totals.BlendedCost, r.Totals.PrevBlendedCost)))
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f | %18.2f | %18.2f | %15.2f%%\n", "Amortized Cost", r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost, r.Totals.AmortizedCost-r.Totals.PrevAmortizedCost, calcPercent(r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost)))
	} else {
		b.WriteString(fmt.Sprintf(" %-18s | %18s\n", "Metric", "Amount ("+r.Metadata.Currency+")"))
		b.WriteString(divSingle + "\n")
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Blended Cost", r.Totals.BlendedCost))
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Unblended Cost", r.Totals.UnblendedCost))
		b.WriteString(fmt.Sprintf(" %-18s | %18.2f\n", "Amortized Cost", r.Totals.AmortizedCost))
	}
	b.WriteString(divDouble + "\n\n")

	// Service/Item breakdown table
	b.WriteString(divSingle + "\n")
	headerCol := "Service Name"
	if r.Metadata.GroupKey != "" && !strings.EqualFold(r.Metadata.GroupKey, "SERVICE") {
		headerCol = r.Metadata.GroupKey
	}

	if r.HasComparison {
		b.WriteString(fmt.Sprintf(" %-46s | %16s | %16s | %10s | %8s\n", truncate(headerCol, 46), "Unblended ("+r.Metadata.Currency+")", "Prev ("+r.Metadata.Currency+")", "Diff ($)", "% Δ"))
		b.WriteString(divSingle + "\n")
		for _, item := range r.Items {
			diffSign := "+"
			if item.MoMDeltaAmount < 0 {
				diffSign = ""
			}
			pctSign := "+"
			if item.MoMDeltaPercent < 0 {
				pctSign = ""
			}
			b.WriteString(fmt.Sprintf(" %-46s | %16.2f | %16.2f | %9s%0.2f | %7s%0.2f%%\n",
				truncate(item.Name, 46),
				item.UnblendedCost,
				item.PrevUnblendedCost,
				diffSign,
				item.MoMDeltaAmount,
				pctSign,
				item.MoMDeltaPercent,
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
