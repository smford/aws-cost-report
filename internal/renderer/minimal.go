package renderer

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderMinimal generates a compact, high-density cost report suitable for messaging platforms,
// chat webhooks, mobile viewports, or low-bandwidth environments.
func RenderMinimal(w io.Writer, r *awsclient.CostReport, failOnThreshold float64) error {
	if r.MonthlyBreakdown != nil && len(r.MonthlyBreakdown.Months) > 1 {
		return renderMonthlyMinimal(w, r, failOnThreshold)
	}

	var b strings.Builder
	sym := r.Metadata.CurrencySymbol()

	// Header line (no emoji)
	b.WriteString(fmt.Sprintf("*AWS Billing Report — %s*\n", r.Metadata.BillingMonth))

	// Account line: account ID and account name displayed together
	b.WriteString(fmt.Sprintf("*Account:* %s\n", formatAccountDisplay(r.Metadata.AccountID, r.Metadata.AccountName)))

	// Billing period
	b.WriteString(fmt.Sprintf("*Period:* %s to %s", r.Metadata.PeriodStart, r.Metadata.PeriodEnd))
	if r.Metadata.EstimatedOrFinal != "" {
		b.WriteString(fmt.Sprintf(" (%s)", r.Metadata.EstimatedOrFinal))
	}
	b.WriteString("\n")

	// Spend summary & MoM delta
	if r.HasComparison {
		b.WriteString(fmt.Sprintf("*Total Spend:* *%s%.2f %s* (prev: %s%.2f)\n",
			sym, r.Totals.UnblendedCost, r.Metadata.Currency, sym, r.Totals.PrevUnblendedCost))
		b.WriteString(fmt.Sprintf("*MoM Change:* *%s* (%s)\n",
			formatPercentDelta(r.Totals.MoMDeltaPercent), formatCurrencyDelta(r.Totals.MoMDeltaAmount, sym)))
	} else {
		b.WriteString(fmt.Sprintf("*Total Spend:* *%s%.2f %s*\n",
			sym, r.Totals.UnblendedCost, r.Metadata.Currency))
	}

	// Budget threshold alert if breached (no emoji)
	if failOnThreshold > 0 && r.Totals.UnblendedCost > failOnThreshold {
		b.WriteString(fmt.Sprintf("*Budget Alert:* Spend of %s%.2f %s exceeds threshold of %s%.2f %s!\n",
			sym, r.Totals.UnblendedCost, r.Metadata.Currency, sym, failOnThreshold, r.Metadata.Currency))
	}

	// Filter out non-service items like "Tax"
	serviceItems := filterNonServiceItems(r.Items)

	// Large Cost Increases / Spikes (no emoji)
	if r.HasComparison && len(serviceItems) > 0 {
		var increases []awsclient.LineItem
		for _, item := range serviceItems {
			if item.MoMDeltaAmount > 0.005 {
				increases = append(increases, item)
			}
		}

		if len(increases) > 0 {
			sort.Slice(increases, func(i, j int) bool {
				return increases[i].MoMDeltaAmount > increases[j].MoMDeltaAmount
			})

			b.WriteString("\n*Cost Increases:*\n")
			maxSpikes := 3
			if len(increases) < maxSpikes {
				maxSpikes = len(increases)
			}
			for i := 0; i < maxSpikes; i++ {
				spk := increases[i]
				b.WriteString(fmt.Sprintf("• *%s*: %s%.2f (%s / %s)\n",
					truncate(spk.Name, 26), sym, spk.UnblendedCost,
					formatCurrencyDelta(spk.MoMDeltaAmount, sym),
					formatPercentDelta(spk.MoMDeltaPercent)))
			}
		}
	}

	// Forecast if available (no emoji)
	if r.Forecast != nil {
		status := "Within Budget"
		if r.Forecast.ThresholdBreached {
			status = "ALERT (Exceeds Threshold)"
		}
		b.WriteString(fmt.Sprintf("\n*Forecast:* Projected %s%.2f %s (%s)\n",
			sym, r.Forecast.TotalAmount, r.Metadata.Currency, status))
	}

	// Compact Monospace Table
	b.WriteString("\n```\n")
	renderCompactTable(&b, r, serviceItems, sym)
	b.WriteString("```\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func filterNonServiceItems(items []awsclient.LineItem) []awsclient.LineItem {
	var filtered []awsclient.LineItem
	for _, item := range items {
		if strings.EqualFold(item.Name, "Tax") {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func renderCompactTable(b *strings.Builder, r *awsclient.CostReport, items []awsclient.LineItem, sym string) {
	const svcWidth = 18
	const spendWidth = 8
	const diffWidth = 9
	const pctWidth = 7

	if r.HasComparison {
		header := fmt.Sprintf("%-*s %*s %*s %*s",
			svcWidth, "Services (Top 5)",
			spendWidth, "Spend",
			diffWidth, "Diff",
			pctWidth, "%")
		divider := strings.Repeat("-", len(header))

		b.WriteString(header + "\n")
		b.WriteString(divider + "\n")

		maxDisplay := 5
		displayItems := items
		hasOther := false
		var other awsclient.LineItem

		if len(items) > maxDisplay {
			displayItems = items[:maxDisplay]
			hasOther = true
			other.Name = fmt.Sprintf("Other (%d svcs)", len(items)-maxDisplay)
			for _, item := range items[maxDisplay:] {
				other.UnblendedCost += item.UnblendedCost
				other.PrevUnblendedCost += item.PrevUnblendedCost
				other.MoMDeltaAmount += item.MoMDeltaAmount
			}
			other.MoMDeltaPercent = calcPercent(other.UnblendedCost, other.PrevUnblendedCost)
		}

		for _, item := range displayItems {
			b.WriteString(fmt.Sprintf("%-*s %*s %*s %*s\n",
				svcWidth, truncate(item.Name, svcWidth),
				spendWidth, fmt.Sprintf("%s%.2f", sym, item.UnblendedCost),
				diffWidth, formatMinimalDiff(item.MoMDeltaAmount, sym),
				pctWidth, formatMinimalPct(item.MoMDeltaPercent),
			))
		}

		if hasOther {
			b.WriteString(fmt.Sprintf("%-*s %*s %*s %*s\n",
				svcWidth, truncate(other.Name, svcWidth),
				spendWidth, fmt.Sprintf("%s%.2f", sym, other.UnblendedCost),
				diffWidth, formatMinimalDiff(other.MoMDeltaAmount, sym),
				pctWidth, formatMinimalPct(other.MoMDeltaPercent),
			))
		}

		b.WriteString(divider + "\n")
		b.WriteString(fmt.Sprintf("%-*s %*s %*s %*s\n",
			svcWidth, "Total",
			spendWidth, fmt.Sprintf("%s%.2f", sym, r.Totals.UnblendedCost),
			diffWidth, formatMinimalDiff(r.Totals.MoMDeltaAmount, sym),
			pctWidth, formatMinimalPct(r.Totals.MoMDeltaPercent),
		))
	} else {
		const colWidth = 10
		header := fmt.Sprintf("%-*s %*s %*s",
			svcWidth, "Services (Top 5)",
			colWidth, "Unblended",
			colWidth, "Blended")
		divider := strings.Repeat("-", len(header))

		b.WriteString(header + "\n")
		b.WriteString(divider + "\n")

		maxDisplay := 5
		displayItems := items
		hasOther := false
		var other awsclient.LineItem

		if len(items) > maxDisplay {
			displayItems = items[:maxDisplay]
			hasOther = true
			other.Name = fmt.Sprintf("Other (%d svcs)", len(items)-maxDisplay)
			for _, item := range items[maxDisplay:] {
				other.UnblendedCost += item.UnblendedCost
				other.BlendedCost += item.BlendedCost
			}
		}

		for _, item := range displayItems {
			b.WriteString(fmt.Sprintf("%-*s %*s %*s\n",
				svcWidth, truncate(item.Name, svcWidth),
				colWidth, fmt.Sprintf("%s%.2f", sym, item.UnblendedCost),
				colWidth, fmt.Sprintf("%s%.2f", sym, item.BlendedCost),
			))
		}

		if hasOther {
			b.WriteString(fmt.Sprintf("%-*s %*s %*s\n",
				svcWidth, truncate(other.Name, svcWidth),
				colWidth, fmt.Sprintf("%s%.2f", sym, other.UnblendedCost),
				colWidth, fmt.Sprintf("%s%.2f", sym, other.BlendedCost),
			))
		}

		b.WriteString(divider + "\n")
		b.WriteString(fmt.Sprintf("%-*s %*s %*s\n",
			svcWidth, "Total",
			colWidth, fmt.Sprintf("%s%.2f", sym, r.Totals.UnblendedCost),
			colWidth, fmt.Sprintf("%s%.2f", sym, r.Totals.BlendedCost),
		))
	}
}

func formatMinimalDiff(v float64, sym string) string {
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

func formatMinimalPct(v float64) string {
	if math.Abs(v) < 0.005 {
		return "0.0%"
	}
	if v > 0 {
		return fmt.Sprintf("+%.1f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

func renderMonthlyMinimal(w io.Writer, r *awsclient.CostReport, failOnThreshold float64) error {
	var b strings.Builder
	sym := r.Metadata.CurrencySymbol()
	mb := r.MonthlyBreakdown

	b.WriteString(fmt.Sprintf("*AWS Billing Report — %s (%d Months)*\n", r.Metadata.BillingMonth, len(mb.Months)))
	b.WriteString(fmt.Sprintf("*Account:* %s\n", formatAccountDisplay(r.Metadata.AccountID, r.Metadata.AccountName)))
	b.WriteString(fmt.Sprintf("*Period:* %s to %s\n", r.Metadata.PeriodStart, r.Metadata.PeriodEnd))
	b.WriteString(fmt.Sprintf("*Grand Total:* *%s%.2f %s*\n", sym, mb.GrandTotal, r.Metadata.Currency))

	if failOnThreshold > 0 && mb.GrandTotal > failOnThreshold {
		b.WriteString(fmt.Sprintf("*Budget Alert:* Grand total of %s%.2f %s exceeds threshold of %s%.2f %s!\n",
			sym, mb.GrandTotal, r.Metadata.Currency, sym, failOnThreshold, r.Metadata.Currency))
	}

	b.WriteString("\n```\n")
	header := fmt.Sprintf("%-12s %12s", "Month", "Total ("+r.Metadata.Currency+")")
	divider := strings.Repeat("-", len(header))
	b.WriteString(header + "\n")
	b.WriteString(divider + "\n")
	for _, m := range mb.Months {
		b.WriteString(fmt.Sprintf("%-12s %12.2f\n", m.Label, mb.Totals[m.Key]))
	}
	b.WriteString(divider + "\n")
	b.WriteString(fmt.Sprintf("%-12s %12.2f\n", "Grand Total", mb.GrandTotal))
	b.WriteString("```\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func formatAccountDisplay(accountID, accountName string) string {
	if strings.Contains(accountID, " (") && strings.HasSuffix(accountID, ")") {
		parts := strings.SplitN(accountID, " (", 2)
		id := parts[0]
		name := strings.TrimSuffix(parts[1], ")")
		return fmt.Sprintf("`%s` (%s)", id, name)
	}
	if accountName != "" {
		return fmt.Sprintf("`%s` (%s)", accountID, accountName)
	}
	return fmt.Sprintf("`%s`", accountID)
}
