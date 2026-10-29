package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

type SlackBlock struct {
	Type   string       `json:"type"`
	Text   *SlackText   `json:"text,omitempty"`
	Fields []*SlackText `json:"fields,omitempty"`
}

type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type SlackPayload struct {
	Text   string       `json:"text,omitempty"`
	Blocks []SlackBlock `json:"blocks,omitempty"`
}

// SendSlackNotification posts a structured Block Kit message to an incoming Slack webhook URL.
func SendSlackNotification(ctx context.Context, webhookURL string, report *awsclient.CostReport, threshold float64) error {
	if webhookURL == "" {
		return nil
	}

	fallbackTitle := "📊 AWS Monthly Billing & Usage Report"
	if threshold > 0 && report.Totals.UnblendedCost > threshold {
		fallbackTitle = fmt.Sprintf("🚨 AWS Budget Alert: Spending ($%.2f) exceeds threshold ($%.2f)!", report.Totals.UnblendedCost, threshold)
	}

	blocks := []SlackBlock{
		{
			Type: "header",
			Text: &SlackText{Type: "plain_text", Text: "📊 AWS Cost Explorer Report"},
		},
		{
			Type: "section",
			Fields: []*SlackText{
				{Type: "mrkdwn", Text: fmt.Sprintf("*Account ID:*\n`%s`", report.Metadata.AccountID)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Billing Month:*\n%s", report.Metadata.BillingMonth)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Period:*\n%s to %s", report.Metadata.PeriodStart, report.Metadata.PeriodEnd)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*Total Unblended:*\n*$%.2f %s*", report.Totals.UnblendedCost, report.Metadata.Currency)},
			},
		},
	}

	if report.HasComparison {
		blocks = append(blocks, SlackBlock{
			Type: "section",
			Fields: []*SlackText{
				{Type: "mrkdwn", Text: fmt.Sprintf("*Previous Spend:*\n$%.2f %s", report.Totals.PrevUnblendedCost, report.Metadata.Currency)},
				{Type: "mrkdwn", Text: fmt.Sprintf("*MoM Delta:*\n%s (%s)", formatCurrencyDelta(report.Totals.MoMDeltaAmount), formatPercentDelta(report.Totals.MoMDeltaPercent))},
			},
		})
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
				breakdown.WriteString(fmt.Sprintf("• *%s*: $%.2f (%s)\n", item.Name, item.UnblendedCost, formatCurrencyDelta(item.MoMDeltaAmount)))
			} else {
				breakdown.WriteString(fmt.Sprintf("• *%s*: $%.2f\n", item.Name, item.UnblendedCost))
			}
		}
		blocks = append(blocks, SlackBlock{
			Type: "section",
			Text: &SlackText{Type: "mrkdwn", Text: "*Top Cost Drivers:*\n" + breakdown.String()},
		})
	}

	payload := SlackPayload{
		Text:   fallbackTitle,
		Blocks: blocks,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to dispatch Slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack webhook returned HTTP status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func formatCurrencyDelta(v float64) string {
	if math.Abs(v) < 0.005 {
		return "$0.00"
	}
	if v > 0 {
		return fmt.Sprintf("+$%.2f", v)
	}
	return fmt.Sprintf("-$%.2f", math.Abs(v))
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
