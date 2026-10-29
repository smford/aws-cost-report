package renderer

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

func getTestReport(hasComparison bool) *awsclient.CostReport {
	cfg := config.DefaultConfig()
	cfg.CompareMoM = hasComparison
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, _ := awsclient.GenerateMockReport(cfg, refTime)
	return report
}

func TestRenderTable(t *testing.T) {
	report := getTestReport(false)
	var buf bytes.Buffer
	err := RenderTable(&buf, report, true)
	if err != nil {
		t.Fatalf("RenderTable failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "AWS MONTHLY BILLING & USAGE REPORT") {
		t.Errorf("expected header banner in table output")
	}
	if !strings.Contains(out, "715195480427") {
		t.Errorf("expected account ID in table output")
	}
	if !strings.Contains(out, "Amazon DynamoDB") {
		t.Errorf("expected DynamoDB in table breakdown")
	}
	if !strings.Contains(out, "21.58") {
		t.Errorf("expected 21.58 unblended cost in totals")
	}
}

func TestRenderTableWithComparison(t *testing.T) {
	report := getTestReport(true)
	var buf bytes.Buffer
	err := RenderTable(&buf, report, true)
	if err != nil {
		t.Fatalf("RenderTable with comparison failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Diff ($)") || !strings.Contains(out, "% Δ") {
		t.Errorf("expected comparison columns in table output")
	}

	// Verify exact line length alignment (104 characters for dividers, headers, and rows)
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "====") || strings.HasPrefix(line, "----") {
			if len(line) != 104 {
				t.Errorf("line %d: expected divider length 104, got %d", i, len(line))
			}
		}
		if strings.Contains(line, "Unblended Cost") || strings.Contains(line, "Amazon DynamoDB") {
			if len(line) != 104 {
				t.Errorf("line %d: expected row length 104, got %d", i, len(line))
			}
		}
	}
}

func TestRenderTableColors(t *testing.T) {
	report := getTestReport(true)
	var buf bytes.Buffer
	err := RenderTable(&buf, report, false)
	if err != nil {
		t.Fatalf("RenderTable with color failed: %v", err)
	}

	out := buf.String()
	// Should contain ANSI color codes
	if !strings.Contains(out, "\033[31m") && !strings.Contains(out, "\033[32m") {
		t.Errorf("expected ANSI color codes in colored table output")
	}
}

func TestRenderMarkdown(t *testing.T) {
	report := getTestReport(false)
	var buf bytes.Buffer
	err := RenderMarkdown(&buf, report, false, 0.00)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "# 📊 AWS Monthly Billing & Usage Report") {
		t.Errorf("expected markdown header")
	}
	if !strings.Contains(out, "| **Account ID** | `715195480427` |") {
		t.Errorf("expected account ID row in markdown")
	}
	if !strings.Contains(out, "| Amazon DynamoDB | $5.51 | $5.51 | $5.51 |") {
		t.Errorf("expected DynamoDB line in markdown")
	}
}

func TestRenderMarkdownBudgetAlert(t *testing.T) {
	report := getTestReport(false)
	var buf bytes.Buffer
	// Total cost is 21.58, threshold is 10.00 -> should trigger [!CAUTION] alert
	err := RenderMarkdown(&buf, report, false, 10.00)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "> [!CAUTION]") || !strings.Contains(out, "Budget Guardrail Alert") {
		t.Errorf("expected budget alert callout in markdown output")
	}
}

func TestRenderMarkdownComparison(t *testing.T) {
	report := getTestReport(true)
	var buf bytes.Buffer
	err := RenderMarkdown(&buf, report, false, 0.00)
	if err != nil {
		t.Fatalf("RenderMarkdown comparison failed: %v", err)
	}

	out := buf.String()
	// Test positive delta formatting: +$0.31 | +5.96%
	if !strings.Contains(out, "+$0.31 | +5.96%") {
		t.Errorf("expected +$0.31 | +5.96%% in markdown output, got:\n%s", out)
	}
	// Test negative delta formatting: -$0.07 | -1.46% (not $-0.07)
	if !strings.Contains(out, "-$0.07 | -1.46%") {
		t.Errorf("expected -$0.07 | -1.46%% in markdown output, got:\n%s", out)
	}
	// Test zero delta formatting: $0.00 | 0.00%
	if !strings.Contains(out, "$0.00 | 0.00%") {
		t.Errorf("expected $0.00 | 0.00%% for zero delta items, got:\n%s", out)
	}
}

func TestRenderJSON(t *testing.T) {
	report := getTestReport(false)
	var buf bytes.Buffer
	err := RenderJSON(&buf, report)
	if err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	metadata, ok := parsed["metadata"].(map[string]interface{})
	if !ok || metadata["account_id"] != "715195480427" {
		t.Errorf("expected account ID in parsed JSON metadata")
	}
}

func TestRenderCSV(t *testing.T) {
	report := getTestReport(false)
	var buf bytes.Buffer
	err := RenderCSV(&buf, report)
	if err != nil {
		t.Fatalf("RenderCSV failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least header + 1 line, got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "Service,UnblendedCost,BlendedCost,AmortizedCost,Currency,PeriodStart,PeriodEnd") {
		t.Errorf("unexpected CSV headers: %s", lines[0])
	}
	if !strings.Contains(lines[1], "Amazon DynamoDB,5.51,5.51,5.51,USD") {
		t.Errorf("unexpected first record: %s", lines[1])
	}
}

func TestRenderMarkdownWithNonUSDCurrency(t *testing.T) {
	report := getTestReport(true)
	report.Metadata.Currency = "EUR"

	var buf bytes.Buffer
	err := RenderMarkdown(&buf, report, false, 0.00)
	if err != nil {
		t.Fatalf("RenderMarkdown with EUR failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Delta (€)") {
		t.Errorf("expected 'Delta (€)' header in markdown, got:\n%s", out)
	}
	if !strings.Contains(out, "**€21.58**") {
		t.Errorf("expected '**€21.58**' in markdown, got:\n%s", out)
	}
	if !strings.Contains(out, "+€0.31") {
		t.Errorf("expected '+€0.31' in markdown, got:\n%s", out)
	}
}

func TestRenderTableWithNonUSDCurrency(t *testing.T) {
	report := getTestReport(true)
	report.Metadata.Currency = "GBP"

	var buf bytes.Buffer
	err := RenderTable(&buf, report, true)
	if err != nil {
		t.Fatalf("RenderTable with GBP failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Diff (£)") {
		t.Errorf("expected 'Diff (£)' header in table, got:\n%s", out)
	}
	if !strings.Contains(out, "Amount (GBP)") {
		t.Errorf("expected 'Amount (GBP)' header in table, got:\n%s", out)
	}
}

func TestRenderMonthlyBreakdown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Monthly = true
	cfg.MonthsBack = 12
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := awsclient.GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}
	if report.MonthlyBreakdown == nil {
		t.Fatalf("expected MonthlyBreakdown to be populated")
	}
	if len(report.MonthlyBreakdown.Months) != 12 {
		t.Errorf("expected 12 months, got %d", len(report.MonthlyBreakdown.Months))
	}

	// 1. Table
	var tableBuf bytes.Buffer
	if err := RenderTable(&tableBuf, report, true); err != nil {
		t.Fatalf("RenderTable failed: %v", err)
	}
	tableOut := tableBuf.String()
	if !strings.Contains(tableOut, "HISTORICAL TREND") {
		t.Errorf("expected 'HISTORICAL TREND' in title")
	}
	if !strings.Contains(tableOut, "12 Months") {
		t.Errorf("expected '12 Months' in metadata")
	}
	if !strings.Contains(tableOut, "Oct 25") || !strings.Contains(tableOut, "Sep 26") {
		t.Errorf("expected month column headers in table, got:\n%s", tableOut)
	}
	if !strings.Contains(tableOut, "Amazon DynamoDB") {
		t.Errorf("expected DynamoDB in monthly table")
	}

	// 2. Markdown
	var mdBuf bytes.Buffer
	if err := RenderMarkdown(&mdBuf, report, false, 0.00); err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}
	mdOut := mdBuf.String()
	if !strings.Contains(mdOut, "### 📅 Monthly Cost Breakdown (12 Months)") {
		t.Errorf("expected breakdown section in markdown, got:\n%s", mdOut)
	}
	if !strings.Contains(mdOut, "| Amazon DynamoDB |") {
		t.Errorf("expected DynamoDB row in markdown table")
	}

	// 3. CSV
	var csvBuf bytes.Buffer
	if err := RenderCSV(&csvBuf, report); err != nil {
		t.Fatalf("RenderCSV failed: %v", err)
	}
	csvOut := csvBuf.String()
	if !strings.Contains(csvOut, "Service,2025-10,2025-11,2025-12,2026-01,2026-02,2026-03,2026-04,2026-05,2026-06,2026-07,2026-08,2026-09,Total,Average,Currency") {
		t.Errorf("expected CSV monthly header, got:\n%s", csvOut)
	}
	if !strings.Contains(csvOut, "Amazon DynamoDB,") {
		t.Errorf("expected DynamoDB record in CSV")
	}

	// 4. JSON
	var jsonBuf bytes.Buffer
	if err := RenderJSON(&jsonBuf, report); err != nil {
		t.Fatalf("RenderJSON failed: %v", err)
	}
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(jsonBuf.Bytes(), &jsonMap); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if _, ok := jsonMap["monthly_breakdown"]; !ok {
		t.Errorf("expected 'monthly_breakdown' key in JSON")
	}
}

func TestRenderPrometheus(t *testing.T) {
	report := getTestReport(true)
	report.Metadata.AccountName = "Production Core"

	var buf bytes.Buffer
	err := RenderPrometheus(&buf, report, 20.00, "v0.9.0", "abcdef1", "2026-10-29")
	if err != nil {
		t.Fatalf("RenderPrometheus failed: %v", err)
	}

	out := buf.String()

	// Verify build info
	if !strings.Contains(out, `aws_cost_report_build_info{version="v0.9.0",commit="abcdef1",build_date="2026-10-29"} 1`) {
		t.Errorf("expected build_info metric, got:\n%s", out)
	}

	// Verify total unblended
	if !strings.Contains(out, `aws_cost_report_total_unblended_dollars{account="715195480427",account_name="Production Core",currency="USD",billing_month="September 2026"} 21.58`) {
		t.Errorf("expected total_unblended_dollars, got:\n%s", out)
	}

	// Verify MoM delta
	if !strings.Contains(out, `aws_cost_report_mom_delta_dollars{`) || !strings.Contains(out, ` 0.45`) {
		t.Errorf("expected mom_delta_dollars in output, got:\n%s", out)
	}

	// Verify budget threshold breach (21.58 > 20.00 -> 1)
	if !strings.Contains(out, `aws_cost_report_budget_threshold_breached{`) || !strings.Contains(out, ` 1`) {
		t.Errorf("expected budget_threshold_breached = 1, got:\n%s", out)
	}

	// Verify service item
	if !strings.Contains(out, `aws_cost_report_service_unblended_dollars{account="715195480427",account_name="Production Core",currency="USD",billing_month="September 2026",dimension="SERVICE",item="Amazon DynamoDB"} 5.51`) {
		t.Errorf("expected service_unblended_dollars for DynamoDB, got:\n%s", out)
	}
}
