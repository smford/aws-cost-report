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
	if cfg.Defaults.Currency != "EUR" {
		t.Errorf("expected Currency EUR, got %s", cfg.Defaults.Currency)
	}
	if !cfg.CurrencyExplicitlySet {
		t.Errorf("expected CurrencyExplicitlySet true when set in YAML")
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
		"--currency", "GBP",
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
	if cfgWithFlags.Defaults.Currency != "GBP" {
		t.Errorf("expected Currency GBP overridden by flag, got %s", cfgWithFlags.Defaults.Currency)
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

	// Test 4: Test tag-filter and tag flags
	tagArgs := []string{
		"--tag", "Environment=Production,Staging",
		"--tag", "Project=FinOps",
		"--tag-filter", "Team=Core; Tier=Backend",
	}
	cfgTags, err := LoadConfig(tagArgs)
	if err != nil {
		t.Fatalf("LoadConfig with tag flags failed: %v", err)
	}
	if len(cfgTags.TagFilters["Environment"]) != 2 {
		t.Errorf("expected 2 values for Environment, got %v", cfgTags.TagFilters["Environment"])
	}
	if len(cfgTags.TagFilters["Project"]) != 1 || cfgTags.TagFilters["Project"][0] != "FinOps" {
		t.Errorf("expected Project=FinOps, got %v", cfgTags.TagFilters["Project"])
	}
	if len(cfgTags.TagFilters["Team"]) != 1 || cfgTags.TagFilters["Team"][0] != "Core" {
		t.Errorf("expected Team=Core, got %v", cfgTags.TagFilters["Team"])
	}
	formattedTags := cfgTags.TagFilters.Format()
	if formattedTags != "Environment in (Production, Staging); Project=FinOps; Team=Core; Tier=Backend" {
		t.Errorf("unexpected formatted tags: %s", formattedTags)
	}
}

func TestTagFilterYAML(t *testing.T) {
	yamlData := `
version: "1"
tag_filters:
  Environment:
    - "Production"
    - "Staging"
  Owner: "FinOps"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tags.yaml")
	if err := os.WriteFile(configPath, []byte(yamlData), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.TagFilters["Environment"]) != 2 {
		t.Errorf("expected 2 Environment tag values, got %v", cfg.TagFilters["Environment"])
	}
	if len(cfg.TagFilters["Owner"]) != 1 || cfg.TagFilters["Owner"][0] != "FinOps" {
		t.Errorf("expected Owner=FinOps, got %v", cfg.TagFilters["Owner"])
	}
}

func TestAccountNameConfigAndFlags(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. YAML with name
	yamlContent1 := `
version: "1"
name: "Production Core"
`
	configPath1 := filepath.Join(tmpDir, "config1.yaml")
	if err := os.WriteFile(configPath1, []byte(yamlContent1), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg1, err := LoadConfig([]string{"--config", configPath1})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg1.AccountName != "Production Core" {
		t.Errorf("expected AccountName 'Production Core', got %q", cfg1.AccountName)
	}

	// 2. YAML with account_name alias
	yamlContent2 := `
version: "1"
account_name: "Staging Workloads"
`
	configPath2 := filepath.Join(tmpDir, "config2.yaml")
	if err := os.WriteFile(configPath2, []byte(yamlContent2), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg2, err := LoadConfig([]string{"--config", configPath2})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg2.AccountName != "Staging Workloads" {
		t.Errorf("expected AccountName 'Staging Workloads', got %q", cfg2.AccountName)
	}

	// 3. CLI flag --name overrides YAML
	cfg3, err := LoadConfig([]string{"--config", configPath1, "--name", "CLI Override Name"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg3.AccountName != "CLI Override Name" {
		t.Errorf("expected CLI override 'CLI Override Name', got %q", cfg3.AccountName)
	}

	// 4. CLI flag --account-name alias
	cfg4, err := LoadConfig([]string{"--account-name", "Alias Name"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg4.AccountName != "Alias Name" {
		t.Errorf("expected AccountName 'Alias Name', got %q", cfg4.AccountName)
	}
}

func TestLoadConfigMonthly(t *testing.T) {
	// 1. --monthly flag defaults to 12 months when months not specified
	cfg1, err := LoadConfig([]string{"--monthly"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg1.Monthly {
		t.Errorf("expected Monthly true, got false")
	}
	if cfg1.MonthsBack != 12 {
		t.Errorf("expected MonthsBack 12, got %d", cfg1.MonthsBack)
	}

	// 2. --trend alias
	cfg2, err := LoadConfig([]string{"--trend", "--months", "6"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg2.Monthly {
		t.Errorf("expected Monthly true, got false")
	}
	if cfg2.MonthsBack != 6 {
		t.Errorf("expected MonthsBack 6, got %d", cfg2.MonthsBack)
	}

	// 3. YAML config with monthly: true
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "monthly-config.yaml")
	yamlContent := `
version: "1"
defaults:
  monthly: true
  months_back: 12
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg3, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg3.Monthly {
		t.Errorf("expected Monthly true from YAML, got false")
	}
	if cfg3.MonthsBack != 12 {
		t.Errorf("expected MonthsBack 12, got %d", cfg3.MonthsBack)
	}
}

func TestLoadConfigMTD(t *testing.T) {
	// 1. --mtd flag
	cfg1, err := LoadConfig([]string{"--mtd"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg1.MTD {
		t.Errorf("expected MTD true, got false")
	}

	// 2. --current-month alias
	cfg2, err := LoadConfig([]string{"--current-month"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg2.MTD {
		t.Errorf("expected MTD true from alias, got false")
	}

	// 3. YAML config with mtd: true
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mtd-config.yaml")
	yamlContent := `
version: "1"
defaults:
  mtd: true
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg3, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg3.MTD {
		t.Errorf("expected MTD true from YAML, got false")
	}
}

func TestTeamsWebhookConfig(t *testing.T) {
	// Flag
	cfg, err := LoadConfig([]string{"--teams-webhook", "https://outlook.office.com/webhook/test"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.TeamsWebhook != "https://outlook.office.com/webhook/test" {
		t.Errorf("expected TeamsWebhook to be set from flag, got %s", cfg.TeamsWebhook)
	}

	// YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "teams-config.yaml")
	yamlContent := `
version: "1"
teams_webhook: "https://outlook.office.com/webhook/yaml"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg2, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg2.TeamsWebhook != "https://outlook.office.com/webhook/yaml" {
		t.Errorf("expected TeamsWebhook to be set from YAML, got %s", cfg2.TeamsWebhook)
	}
}
