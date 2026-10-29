package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Defaults.Currency != "USD" {
		t.Errorf("expected default currency USD, got %s", cfg.Defaults.Currency)
	}
	if cfg.Defaults.MonthsBack != 1 {
		t.Errorf("expected default MonthsBack 1, got %d", cfg.Defaults.MonthsBack)
	}
	if cfg.Grouping.Key != "SERVICE" {
		t.Errorf("expected default grouping SERVICE, got %s", cfg.Grouping.Key)
	}
	if cfg.Reporting.Format != "table" {
		t.Errorf("expected default format table, got %s", cfg.Reporting.Format)
	}
}

func TestLoadConfigFlagPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	yamlContent := `
version: "1"
defaults:
  currency: "EUR"
  months_back: 3
  min_display_cost: 0.50
  metrics:
    - "UnblendedCost"
grouping:
  type: "DIMENSION"
  key: "LINKED_ACCOUNT"
budget:
  fail_on_threshold: 100.00
reporting:
  format: "json"
  write_job_summary: false
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	// Test 1: Config file loading without flags
	cfg, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Defaults.MonthsBack != 3 {
		t.Errorf("expected MonthsBack 3, got %d", cfg.Defaults.MonthsBack)
	}
	if cfg.Defaults.MinDisplayCost != 0.50 {
		t.Errorf("expected MinDisplayCost 0.50, got %f", cfg.Defaults.MinDisplayCost)
	}
	if cfg.Budget.FailOnThreshold != 100.00 {
		t.Errorf("expected threshold 100, got %f", cfg.Budget.FailOnThreshold)
	}
	if cfg.Reporting.Format != "json" {
		t.Errorf("expected format json, got %s", cfg.Reporting.Format)
	}

	// Test 2: Flags override config file
	args := []string{
		"--config", configPath,
		"--months", "6",
		"--min-cost", "0.05",
		"--group-by", "TAG:Environment",
		"--format", "markdown",
		"--fail-on-threshold", "250.00",
		"--compare",
	}
	cfgWithFlags, err := LoadConfig(args)
	if err != nil {
		t.Fatalf("LoadConfig with flags failed: %v", err)
	}
	if cfgWithFlags.Defaults.MonthsBack != 6 {
		t.Errorf("expected MonthsBack 6, got %d", cfgWithFlags.Defaults.MonthsBack)
	}
	if cfgWithFlags.Defaults.MinDisplayCost != 0.05 {
		t.Errorf("expected MinDisplayCost 0.05, got %f", cfgWithFlags.Defaults.MinDisplayCost)
	}
	if cfgWithFlags.Grouping.Type != "TAG" || cfgWithFlags.Grouping.Key != "Environment" {
		t.Errorf("expected TAG:Environment, got %s:%s", cfgWithFlags.Grouping.Type, cfgWithFlags.Grouping.Key)
	}
	if cfgWithFlags.Reporting.Format != "markdown" {
		t.Errorf("expected format markdown, got %s", cfgWithFlags.Reporting.Format)
	}
	if cfgWithFlags.Budget.FailOnThreshold != 250.00 {
		t.Errorf("expected threshold 250, got %f", cfgWithFlags.Budget.FailOnThreshold)
	}
	if !cfgWithFlags.CompareMoM {
		t.Errorf("expected CompareMoM true")
	}

	// Test 3: Test threshold alias, output, json-summary, top, and slack-webhook flags
	extraArgs := []string{
		"--threshold", "750.50",
		"--output", "/tmp/report.md",
		"--json-summary", "/tmp/summary.json",
		"--top", "5",
		"--slack-webhook", "https://hooks.slack.com/services/test",
	}
	cfgExtra, err := LoadConfig(extraArgs)
	if err != nil {
		t.Fatalf("LoadConfig with extra flags failed: %v", err)
	}
	if cfgExtra.Budget.FailOnThreshold != 750.50 {
		t.Errorf("expected threshold 750.50, got %f", cfgExtra.Budget.FailOnThreshold)
	}
	if cfgExtra.OutputFile != "/tmp/report.md" {
		t.Errorf("expected OutputFile /tmp/report.md, got %s", cfgExtra.OutputFile)
	}
	if cfgExtra.JSONSummaryFile != "/tmp/summary.json" {
		t.Errorf("expected JSONSummaryFile /tmp/summary.json, got %s", cfgExtra.JSONSummaryFile)
	}
	if cfgExtra.Defaults.Top != 5 {
		t.Errorf("expected Top 5, got %d", cfgExtra.Defaults.Top)
	}
	if cfgExtra.SlackWebhook != "https://hooks.slack.com/services/test" {
		t.Errorf("expected SlackWebhook, got %s", cfgExtra.SlackWebhook)
	}
}
