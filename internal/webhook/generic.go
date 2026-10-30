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

// GenericWebhookPayload represents the JSON payload sent to a generic HTTP webhook.
type GenericWebhookPayload struct {
	Event             string                      `json:"event"`
	Alert             bool                        `json:"alert"`
	Threshold         float64                     `json:"threshold,omitempty"`
	ThresholdExceeded bool                        `json:"threshold_exceeded"`
	Metadata          awsclient.ReportMetadata    `json:"metadata"`
	Totals            awsclient.TotalCosts        `json:"totals"`
	Items             []awsclient.LineItem        `json:"items"`
	DailyBreakdown    *awsclient.DailyBreakdown   `json:"daily_breakdown,omitempty"`
	MonthlyBreakdown  *awsclient.MonthlyBreakdown `json:"monthly_breakdown,omitempty"`
	Forecast          *awsclient.CostForecast     `json:"forecast,omitempty"`
	GeneratedAt       time.Time                   `json:"generated_at"`
}

// SendGenericWebhook dispatches a structured JSON payload to any HTTP/HTTPS webhook endpoint.
func SendGenericWebhook(ctx context.Context, webhookURL string, report *awsclient.CostReport, threshold float64) error {
	if webhookURL == "" {
		return nil
	}

	alert := threshold > 0 && report.Totals.UnblendedCost > threshold

	payload := GenericWebhookPayload{
		Event:             "cost_report",
		Alert:             alert,
		Threshold:         threshold,
		ThresholdExceeded: alert,
		Metadata:          report.Metadata,
		Totals:            report.Totals,
		Items:             report.Items,
		DailyBreakdown:    report.DailyBreakdown,
		MonthlyBreakdown:  report.MonthlyBreakdown,
		Forecast:          report.Forecast,
		GeneratedAt:       time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal generic webhook payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aws-cost-report/1.0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to dispatch generic webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("generic webhook responded with status %d: %s", resp.StatusCode, string(bodySnippet))
	}

	return nil
}
