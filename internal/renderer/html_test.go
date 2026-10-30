package renderer

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

func TestRenderHTML(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CompareMoM = true
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := awsclient.GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderHTML(&buf, report, 50.0); err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "<!DOCTYPE html>") {
		t.Errorf("expected HTML doctype")
	}
	if !strings.Contains(out, "AWS Monthly Billing & Usage Report") {
		t.Errorf("expected AWS Monthly Billing & Usage Report header")
	}
	if !strings.Contains(out, report.Metadata.AccountID) {
		t.Errorf("expected account ID in HTML output")
	}
	if !strings.Contains(out, "Previous Month") {
		t.Errorf("expected Previous Month card in HTML output")
	}
	if !strings.Contains(out, "Amazon DynamoDB") {
		t.Errorf("expected DynamoDB item in table")
	}
}

func TestRenderHTMLThresholdAlert(t *testing.T) {
	cfg := config.DefaultConfig()
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, _ := awsclient.GenerateMockReport(cfg, refTime)

	var buf bytes.Buffer
	// Threshold 1.0 is lower than mock total ($21.58)
	if err := RenderHTML(&buf, report, 1.0); err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "BUDGET ALERT:") {
		t.Errorf("expected BUDGET ALERT banner in HTML output")
	}
}

func TestRenderHTMLDailyAndMonthly(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Daily = true
	cfg.Monthly = true
	cfg.MonthsBack = 12
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, _ := awsclient.GenerateMockReport(cfg, refTime)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, report, 0); err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Daily Spend Breakdown") {
		t.Errorf("expected Daily Spend Breakdown section")
	}
	if !strings.Contains(out, "Historical Multi-Month Trend") {
		t.Errorf("expected Historical Multi-Month Trend section")
	}
}
