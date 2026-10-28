package renderer

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderMarkdown generates a GitHub-Flavored Markdown report and optionally writes to $GITHUB_STEP_SUMMARY.
func RenderMarkdown(w io.Writer, r *awsclient.CostReport, writeJobSummary bool, failOnThreshold float64) error {
	var b strings.Builder

	b.WriteString("# 📊 AWS Monthly Billing & Usage Report\n\n")

	if failOnThreshold > 0.00 && r.Totals.UnblendedCost > failOnThreshold {
		b.WriteString(fmt.Sprintf("> [!CAUTION]\n> **Budget Guardrail Alert**: Total billing of **$%.2f %s** exceeds the configured threshold of **$%.2f %s**!\n\n",
			r.Totals.UnblendedCost, r.Metadata.Currency, failOnThreshold, r.Metadata.Currency))
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
	b.WriteString(fmt.Sprintf("| **Active Line Items** | %d |\n\n", r.Metadata.ActiveServices))

	// Total Metrics Table
	b.WriteString("### 💰 Overall Cost Summary\n\n")
	if r.HasComparison {
		b.WriteString("| Metric | Amount (" + r.Metadata.Currency + ") | Previous (" + r.Metadata.Currency + ") | Delta ($) | Change (%) |\n")
		b.WriteString("|:---|---:|---:|---:|---:|\n")
		b.WriteString(fmt.Sprintf("| **Unblended Cost** | **$%.2f** | $%.2f | %s$%.2f | %s%.2f%% |\n",
			r.Totals.UnblendedCost, r.Totals.PrevUnblendedCost, formatSign(r.Totals.MoMDeltaAmount), r.Totals.MoMDeltaAmount, formatSign(r.Totals.MoMDeltaPercent), r.Totals.MoMDeltaPercent))
		b.WriteString(fmt.Sprintf("| **Blended Cost** | **$%.2f** | $%.2f | %s$%.2f | %s%.2f%% |\n",
			r.Totals.BlendedCost, r.Totals.PrevBlendedCost, formatSign(r.Totals.BlendedCost-r.Totals.PrevBlendedCost), r.Totals.BlendedCost-r.Totals.PrevBlendedCost, formatSign(calcPercent(r.Totals.BlendedCost, r.Totals.PrevBlendedCost)), calcPercent(r.Totals.BlendedCost, r.Totals.PrevBlendedCost)))
		b.WriteString(fmt.Sprintf("| **Amortized Cost** | **$%.2f** | $%.2f | %s$%.2f | %s%.2f%% |\n\n",
			r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost, formatSign(r.Totals.AmortizedCost-r.Totals.PrevAmortizedCost), r.Totals.AmortizedCost-r.Totals.PrevAmortizedCost, formatSign(calcPercent(r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost)), calcPercent(r.Totals.AmortizedCost, r.Totals.PrevAmortizedCost)))
	} else {
		b.WriteString("| Metric | Amount (" + r.Metadata.Currency + ") |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString(fmt.Sprintf("| **Blended Cost** | $%.2f |\n", r.Totals.BlendedCost))
		b.WriteString(fmt.Sprintf("| **Unblended Cost** | $%.2f |\n", r.Totals.UnblendedCost))
		b.WriteString(fmt.Sprintf("| **Amortized Cost** | $%.2f |\n\n", r.Totals.AmortizedCost))
	}

	// Service/Dimension Breakdown Table
	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" || strings.EqualFold(groupLabel, "SERVICE") {
		groupLabel = "Service Name"
	}
	b.WriteString(fmt.Sprintf("### 🔍 Itemized Breakdown by %s\n\n", groupLabel))

	if r.HasComparison {
		b.WriteString(fmt.Sprintf("| %s | Unblended (%s) | Previous (%s) | Delta ($) | Change (%%) |\n", groupLabel, r.Metadata.Currency, r.Metadata.Currency))
		b.WriteString("|:---|---:|---:|---:|---:|\n")
		for _, item := range r.Items {
			b.WriteString(fmt.Sprintf("| %s | $%.2f | $%.2f | %s$%.2f | %s%.2f%% |\n",
				item.Name,
				item.UnblendedCost,
				item.PrevUnblendedCost,
				formatSign(item.MoMDeltaAmount),
				item.MoMDeltaAmount,
				formatSign(item.MoMDeltaPercent),
				item.MoMDeltaPercent,
			))
		}
	} else {
		b.WriteString(fmt.Sprintf("| %s | Unblended (%s) | Blended (%s) | Amortized (%s) |\n", groupLabel, r.Metadata.Currency, r.Metadata.Currency, r.Metadata.Currency))
		b.WriteString("|:---|---:|---:|---:|\n")
		for _, item := range r.Items {
			b.WriteString(fmt.Sprintf("| %s | $%.2f | $%.2f | $%.2f |\n",
				item.Name,
				item.UnblendedCost,
				item.BlendedCost,
				item.AmortizedCost,
			))
		}
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

func formatSign(v float64) string {
	if v > 0 {
		return "+"
	}
	return ""
}
