package renderer

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

func TestRenderMinimal_WithComparison(t *testing.T) {
	report := getTestReport(true)
	report.Metadata.AccountName = "Production Core"

	var buf bytes.Buffer
	err := RenderMinimal(&buf, report, 0.00)
	if err != nil {
		t.Fatalf("RenderMinimal failed: %v", err)
	}

	out := buf.String()

	// Check header (no emoji)
	if !strings.Contains(out, "*AWS Billing Report — ") {
		t.Errorf("expected header banner in minimal output")
	}
	if strings.Contains(out, "📊") || strings.Contains(out, "🚨") || strings.Contains(out, "⚠️") || strings.Contains(out, "🔮") {
		t.Errorf("expected no emojis in minimal output, got:\n%s", out)
	}

	// Check Account ID and Account Name displayed together
	if !strings.Contains(out, "*Account:* `715195480427` (Production Core)") {
		t.Errorf("expected Account ID and Name displayed together, got:\n%s", out)
	}

	// Check Period
	if !strings.Contains(out, "*Period:*") {
		t.Errorf("expected Period line in minimal output")
	}

	// Check Total Spend and MoM Change
	if !strings.Contains(out, "*Total Spend:*") || !strings.Contains(out, "prev:") {
		t.Errorf("expected Total Spend with prev amount in minimal output")
	}
	if !strings.Contains(out, "*MoM Change:*") {
		t.Errorf("expected MoM Change in minimal output")
	}

	// Check Cost Increases section (no emoji)
	if !strings.Contains(out, "*Cost Increases:*") {
		t.Errorf("expected Cost Increases section in minimal output with comparison")
	}
	if !strings.Contains(out, "Amazon DynamoDB") {
		t.Errorf("expected Amazon DynamoDB in cost increases")
	}
	// Verify "Tax" is removed as a service
	if strings.Contains(out, "• *Tax*") {
		t.Errorf("expected Tax to be excluded from cost increases")
	}

	// Check compact monospace code block table
	if !strings.Contains(out, "```\nServices (Top 5)") {
		t.Errorf("expected 'Services (Top 5)' header in monospace code block")
	}
	if !strings.Contains(out, "Spend") || !strings.Contains(out, "Diff") {
		t.Errorf("expected Spend and Diff column headers")
	}
	if !strings.Contains(out, "Other (") {
		t.Errorf("expected Other aggregation row for long-tail items")
	}
	if !strings.Contains(out, "Total") {
		t.Errorf("expected Total row in table")
	}

	// Verify line lengths in the table block do not exceed 50 characters (preventing mobile wrapping)
	codeBlockStart := strings.Index(out, "```\n")
	codeBlockEnd := strings.LastIndex(out, "```")
	if codeBlockStart != -1 && codeBlockEnd > codeBlockStart {
		tableContent := out[codeBlockStart+4 : codeBlockEnd]
		for _, line := range strings.Split(tableContent, "\n") {
			line = strings.TrimSpace(line)
			if len(line) > 50 {
				t.Errorf("table line exceeds 50 chars (%d chars): %q", len(line), line)
			}
		}
	}
}

func TestRenderMinimal_NoComparison(t *testing.T) {
	report := getTestReport(false)
	report.Metadata.AccountName = ""

	var buf bytes.Buffer
	err := RenderMinimal(&buf, report, 0.00)
	if err != nil {
		t.Fatalf("RenderMinimal failed: %v", err)
	}

	out := buf.String()

	// Without account name, just account ID
	if !strings.Contains(out, "*Account:* `715195480427`\n") {
		t.Errorf("expected Account ID alone when AccountName is empty, got:\n%s", out)
	}

	// No MoM Change or Cost Increases section
	if strings.Contains(out, "*MoM Change:*") {
		t.Errorf("did not expect MoM Change without comparison")
	}
	if strings.Contains(out, "*Cost Increases:*") {
		t.Errorf("did not expect Cost Increases section without comparison")
	}

	// Table headers for non-comparison
	if !strings.Contains(out, "Services (Top 5)") || !strings.Contains(out, "Unblended") || !strings.Contains(out, "Blended") {
		t.Errorf("expected Services (Top 5), Unblended and Blended headers")
	}
}

func TestRenderMinimal_ThresholdAlert(t *testing.T) {
	report := getTestReport(false)
	var buf bytes.Buffer
	err := RenderMinimal(&buf, report, 10.00) // report total is ~21.58
	if err != nil {
		t.Fatalf("RenderMinimal failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "*Budget Alert:* Spend of $21.58 USD exceeds threshold of $10.00 USD!") {
		t.Errorf("expected Budget Alert in minimal output, got:\n%s", out)
	}
}

func TestRenderMinimal_WithForecast(t *testing.T) {
	report := getTestReport(false)
	report.Forecast = &awsclient.CostForecast{
		TotalAmount:       55.00,
		MeanAmount:        20.00,
		ThresholdBreached: true,
	}

	var buf bytes.Buffer
	err := RenderMinimal(&buf, report, 0.00)
	if err != nil {
		t.Fatalf("RenderMinimal failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "*Forecast:* Projected $55.00 USD (ALERT (Exceeds Threshold))") {
		t.Errorf("expected forecast line, got:\n%s", out)
	}
}

func TestRenderMinimal_MonthlyBreakdown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Monthly = true
	cfg.MonthsBack = 3
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := awsclient.GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}
	report.Metadata.AccountName = "Dev Sandbox"

	var buf bytes.Buffer
	err = RenderMinimal(&buf, report, 0.00)
	if err != nil {
		t.Fatalf("RenderMinimal monthly failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Dev Sandbox") {
		t.Errorf("expected account name in monthly minimal output")
	}
	if !strings.Contains(out, "*Grand Total:*") {
		t.Errorf("expected Grand Total line in monthly minimal output")
	}
	if !strings.Contains(out, "Month") || !strings.Contains(out, "Total (USD)") {
		t.Errorf("expected Month and Total columns in monthly table")
	}
	if strings.Contains(out, "📊") {
		t.Errorf("expected no emojis in monthly minimal output")
	}
}
