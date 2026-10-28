package renderer

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderCSV writes the CostReport line items as CSV records.
func RenderCSV(w io.Writer, r *awsclient.CostReport) error {
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
				fmt.Sprintf("%.2f%%", item.MoMDeltaPercent),
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
