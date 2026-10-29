package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

type TeamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TeamsSection struct {
	ActivityTitle string      `json:"activityTitle,omitempty"`
	Text          string      `json:"text,omitempty"`
	Facts         []TeamsFact `json:"facts,omitempty"`
	Markdown      bool        `json:"markdown"`
}

type TeamsPayload struct {
	Type       string         `json:"@type"`
	Context    string         `json:"@context"`
	Summary    string         `json:"summary"`
	ThemeColor string         `json:"themeColor"`
	Title      string         `json:"title"`
	Sections   []TeamsSection `json:"sections"`
}

// SendTeamsNotification posts a structured MessageCard to an incoming Microsoft Teams webhook URL.
func SendTeamsNotification(ctx context.Context, webhookURL string, report *awsclient.CostReport, threshold float64) error {
	if webhookURL == "" {
		return nil
	}

	sym := report.Metadata.CurrencySymbol()
	isBreached := threshold > 0 && report.Totals.UnblendedCost > threshold

	themeColor := "0076D7" // Teams Blue
	summary := "📊 AWS Monthly Billing & Usage Report"
	title := "📊 AWS Cost Explorer Report"

	if isBreached {
		themeColor = "E81123" // Alert Red
		summary = fmt.Sprintf("🚨 AWS Budget Alert: Spending (%s%.2f) exceeds threshold (%s%.2f)!", sym, report.Totals.UnblendedCost, sym, threshold)
		title = summary
	}

	accountDisplay := report.Metadata.AccountID
	if report.Metadata.AccountName != "" {
		accountDisplay = fmt.Sprintf("%s (%s)", report.Metadata.AccountID, report.Metadata.AccountName)
	}

	facts := []TeamsFact{
		{Name: "Account:", Value: accountDisplay},
		{Name: "Billing Month:", Value: report.Metadata.BillingMonth},
		{Name: "Period:", Value: fmt.Sprintf("%s to %s", report.Metadata.PeriodStart, report.Metadata.PeriodEnd)},
		{Name: "Total Unblended:", Value: fmt.Sprintf("%s%.2f %s", sym, report.Totals.UnblendedCost, report.Metadata.Currency)},
	}

	if report.HasComparison {
		facts = append(facts,
			TeamsFact{Name: "Previous Spend:", Value: fmt.Sprintf("%s%.2f %s", sym, report.Totals.PrevUnblendedCost, report.Metadata.Currency)},
			TeamsFact{Name: "MoM Delta:", Value: fmt.Sprintf("%s (%s)", formatCurrencyDelta(report.Totals.MoMDeltaAmount, sym), formatPercentDelta(report.Totals.MoMDeltaPercent))},
		)
	}

	if threshold > 0 {
		status := "✅ Within budget"
		if isBreached {
			status = "🚨 Threshold Breached"
		}
		facts = append(facts,
			TeamsFact{Name: "Budget Threshold:", Value: fmt.Sprintf("%s%.2f %s (%s)", sym, threshold, report.Metadata.Currency, status)},
		)
	}

	sections := []TeamsSection{
		{
			ActivityTitle: "Cost Summary",
			Facts:         facts,
			Markdown:      true,
		},
	}

	if len(report.Items) > 0 {
		var breakdown bytes.Buffer
		maxItems := 5
		if len(report.Items) < maxItems {
			maxItems = len(report.Items)
		}
		for i := 0; i < maxItems; i++ {
			item := report.Items[i]
			if report.HasComparison && item.HasPreviousCost {
				breakdown.WriteString(fmt.Sprintf("• **%s**: %s%.2f (%s)\n", item.Name, sym, item.UnblendedCost, formatCurrencyDelta(item.MoMDeltaAmount, sym)))
			} else {
				breakdown.WriteString(fmt.Sprintf("• **%s**: %s%.2f\n", item.Name, sym, item.UnblendedCost))
			}
		}
		sections = append(sections, TeamsSection{
			ActivityTitle: "Top Cost Drivers",
			Text:          breakdown.String(),
			Markdown:      true,
		})
	}

	payload := TeamsPayload{
		Type:       "MessageCard",
		Context:    "https://schema.org/extensions",
		Summary:    summary,
		ThemeColor: themeColor,
		Title:      title,
		Sections:   sections,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Microsoft Teams payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create Microsoft Teams request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to dispatch Microsoft Teams webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("microsoft teams webhook returned HTTP status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
