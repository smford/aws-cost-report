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
