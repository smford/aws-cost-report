package renderer

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderCSV writes the CostReport line items as CSV records.
func RenderCSV(w io.Writer, r *awsclient.CostReport) error {
	if r.MonthlyBreakdown != nil && len(r.MonthlyBreakdown.Months) > 1 {
		return renderMonthlyCSV(w, r)
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()

	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" || strings.EqualFold(groupLabel, "SERVICE") {
		groupLabel = "Service"
	}

	if r.HasComparison {
		headers := []string{
			groupLabel,
			"UnblendedCost",
			"PreviousUnblendedCost",
			"DeltaAmount",
			"DeltaPercent",
			"BlendedCost",
			"AmortizedCost",
			"Currency",
			"PeriodStart",
			"PeriodEnd",
		}
		if err := writer.Write(headers); err != nil {
			return err
		}

		for _, item := range r.Items {
			record := []string{
				item.Name,
				fmt.Sprintf("%.2f", item.UnblendedCost),
				fmt.Sprintf("%.2f", item.PrevUnblendedCost),
				fmt.Sprintf("%.2f", item.MoMDeltaAmount),
				fmt.Sprintf("%.2f", item.MoMDeltaPercent),
				fmt.Sprintf("%.2f", item.BlendedCost),
				fmt.Sprintf("%.2f", item.AmortizedCost),
				r.Metadata.Currency,
				r.Metadata.PeriodStart,
				r.Metadata.PeriodEnd,
			}
			if err := writer.Write(record); err != nil {
				return err
			}
		}
	} else {
		headers := []string{
			groupLabel,
			"UnblendedCost",
			"BlendedCost",
			"AmortizedCost",
			"Currency",
			"PeriodStart",
			"PeriodEnd",
		}
		if err := writer.Write(headers); err != nil {
			return err
		}

		for _, item := range r.Items {
			record := []string{
				item.Name,
				fmt.Sprintf("%.2f", item.UnblendedCost),
				fmt.Sprintf("%.2f", item.BlendedCost),
				fmt.Sprintf("%.2f", item.AmortizedCost),
				r.Metadata.Currency,
				r.Metadata.PeriodStart,
				r.Metadata.PeriodEnd,
			}
			if err := writer.Write(record); err != nil {
				return err
			}
		}
	}

	return nil
}

func renderMonthlyCSV(w io.Writer, r *awsclient.CostReport) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" || strings.EqualFold(groupLabel, "SERVICE") {
		groupLabel = "Service"
	}

	mb := r.MonthlyBreakdown
	headers := []string{groupLabel}
	for _, m := range mb.Months {
		headers = append(headers, m.Key)
	}
	headers = append(headers, "Total", "Average", "Currency")
	if err := writer.Write(headers); err != nil {
		return err
	}

	for _, it := range mb.Items {
		record := []string{it.Name}
		for _, m := range mb.Months {
			record = append(record, fmt.Sprintf("%.2f", it.Costs[m.Key]))
		}
		record = append(record, fmt.Sprintf("%.2f", it.Total), fmt.Sprintf("%.2f", it.Average), r.Metadata.Currency)
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	totalRecord := []string{"Total"}
	for _, m := range mb.Months {
		totalRecord = append(totalRecord, fmt.Sprintf("%.2f", mb.Totals[m.Key]))
	}
	avgMonthly := 0.0
	if len(mb.Months) > 0 {
		avgMonthly = math.Round((mb.GrandTotal/float64(len(mb.Months)))*100) / 100
	}
	totalRecord = append(totalRecord, fmt.Sprintf("%.2f", mb.GrandTotal), fmt.Sprintf("%.2f", avgMonthly), r.Metadata.Currency)
	return writer.Write(totalRecord)
}
