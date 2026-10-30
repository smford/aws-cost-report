package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestGenericWebhookConfig(t *testing.T) {
	// Flag
	cfg, err := LoadConfig([]string{"--webhook-url", "https://api.example.com/webhook/costs"})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.WebhookURL != "https://api.example.com/webhook/costs" {
		t.Errorf("expected WebhookURL from flag, got %s", cfg.WebhookURL)
	}

	// YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "generic-webhook.yaml")
	yamlContent := `
version: "1"
webhook_url: "https://api.example.com/webhook/yaml"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg2, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg2.WebhookURL != "https://api.example.com/webhook/yaml" {
		t.Errorf("expected WebhookURL from YAML, got %s", cfg2.WebhookURL)
	}

	// Environment variable fallback
	t.Setenv("WEBHOOK_URL", "https://api.example.com/webhook/env")
	cfg3, err := LoadConfig([]string{})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg3.WebhookURL != "https://api.example.com/webhook/env" {
		t.Errorf("expected WebhookURL from env, got %s", cfg3.WebhookURL)
	}
}

func TestS3ConfigAndFlags(t *testing.T) {
	// Flags
	cfg, err := LoadConfig([]string{
		"--s3-bucket", "my-reports-bucket",
		"--s3-key", "finops/monthly.json",
	})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.S3Bucket != "my-reports-bucket" {
		t.Errorf("expected S3Bucket 'my-reports-bucket', got %s", cfg.S3Bucket)
	}
	if cfg.S3Key != "finops/monthly.json" {
		t.Errorf("expected S3Key 'finops/monthly.json', got %s", cfg.S3Key)
	}

	// YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "s3-config.yaml")
	yamlContent := `
version: "1"
s3_bucket: "yaml-bucket"
s3_key: "reports/yaml.md"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg2, err := LoadConfig([]string{"--config", configPath})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg2.S3Bucket != "yaml-bucket" {
		t.Errorf("expected S3Bucket 'yaml-bucket', got %s", cfg2.S3Bucket)
	}
	if cfg2.S3Key != "reports/yaml.md" {
		t.Errorf("expected S3Key 'reports/yaml.md', got %s", cfg2.S3Key)
	}
}

func TestTwoDimensionalGroupByConfig(t *testing.T) {
	// Single dimension
	cfg1, err := LoadConfig([]string{"--group-by", "SERVICE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg1.Groupings) != 1 || cfg1.Groupings[0].Key != "SERVICE" || cfg1.Groupings[0].Type != "DIMENSION" {
		t.Errorf("expected 1 dimension SERVICE, got %+v", cfg1.Groupings)
	}

	// Two dimensions: Dimension + Tag
	cfg2, err := LoadConfig([]string{"--group-by", "SERVICE,TAG:Environment"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg2.Groupings) != 2 {
		t.Fatalf("expected 2 groupings, got %d", len(cfg2.Groupings))
	}
	if cfg2.Groupings[0].Type != "DIMENSION" || cfg2.Groupings[0].Key != "SERVICE" {
		t.Errorf("unexpected grouping 0: %+v", cfg2.Groupings[0])
	}
	if cfg2.Groupings[1].Type != "TAG" || cfg2.Groupings[1].Key != "Environment" {
		t.Errorf("unexpected grouping 1: %+v", cfg2.Groupings[1])
	}

	// Two dimensions: Linked Account + Service
	cfg3, err := LoadConfig([]string{"--group-by", "LINKED_ACCOUNT,SERVICE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg3.Groupings) != 2 || cfg3.Groupings[0].Key != "LINKED_ACCOUNT" || cfg3.Groupings[1].Key != "SERVICE" {
		t.Errorf("unexpected groupings: %+v", cfg3.Groupings)
	}

	// More than 2 dimensions should fail
	_, err = LoadConfig([]string{"--group-by", "SERVICE,TAG:Environment,TAG:Team"})
	if err == nil {
		t.Error("expected error when > 2 dimensions specified, got nil")
	}
}

func TestConfigNetMetrics(t *testing.T) {
	// 1. Explicit net metrics
	cfg, err := LoadConfig([]string{"--metrics", "NetUnblendedCost,NetAmortizedCost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Defaults.Metrics) != 2 || cfg.Defaults.Metrics[0] != "NetUnblendedCost" || cfg.Defaults.Metrics[1] != "NetAmortizedCost" {
		t.Errorf("unexpected metrics: %+v", cfg.Defaults.Metrics)
	}

	// 2. Short aliases
	cfg2, err := LoadConfig([]string{"--metrics", "net_unblended,net_amortized"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg2.Defaults.Metrics) != 2 || cfg2.Defaults.Metrics[0] != "NetUnblendedCost" || cfg2.Defaults.Metrics[1] != "NetAmortizedCost" {
		t.Errorf("unexpected metrics: %+v", cfg2.Defaults.Metrics)
	}

	// 3. "all" keyword includes net metrics
	cfg3, err := LoadConfig([]string{"--metrics", "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg3.Defaults.Metrics) != 5 {
		t.Errorf("expected 5 metrics for 'all', got %d: %+v", len(cfg3.Defaults.Metrics), cfg3.Defaults.Metrics)
	}
}

func TestConfigDaily(t *testing.T) {
	// 1. --daily flag
	cfg1, err := LoadConfig([]string{"--daily"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg1.Daily || cfg1.Granularity != "DAILY" {
		t.Errorf("expected Daily=true and Granularity=DAILY, got Daily=%v, Granularity=%s", cfg1.Daily, cfg1.Granularity)
	}

	// 2. --granularity=DAILY flag
	cfg2, err := LoadConfig([]string{"--granularity", "DAILY"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg2.Daily || cfg2.Granularity != "DAILY" {
		t.Errorf("expected Daily=true and Granularity=DAILY, got Daily=%v, Granularity=%s", cfg2.Daily, cfg2.Granularity)
	}
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_REGION", "eu-central-1")
	t.Setenv("TEST_CURRENCY", "EUR")

	cases := []struct {
		input    string
		expected string
	}{
		{"${TEST_REGION}", "eu-central-1"},
		{"$TEST_REGION", "eu-central-1"},
		{"${TEST_UNSET:-us-east-1}", "us-east-1"},
		{"${TEST_REGION:-us-west-2}", "eu-central-1"},
		{"region: ${TEST_REGION}, curr: ${TEST_CURRENCY}", "region: eu-central-1, curr: EUR"},
		{"no vars", "no vars"},
	}

	for _, c := range cases {
		actual := ExpandEnv(c.input)
		if actual != c.expected {
			t.Errorf("ExpandEnv(%q) = %q; want %q", c.input, actual, c.expected)
		}
	}
}

func TestConfigFileEnvExpansion(t *testing.T) {
	t.Setenv("TEST_REPORT_FORMAT", "markdown")
	t.Setenv("TEST_WEBHOOK_URL", "https://hooks.slack.com/services/TEST/EXPAND")

	yamlContent := `version: "1"
defaults:
  currency: ${TEST_CONFIG_CURRENCY:-GBP}
reporting:
  format: ${TEST_REPORT_FORMAT}
slack_webhook: ${TEST_WEBHOOK_URL}
`
	tmpFile, err := os.CreateTemp("", "aws-cost-report-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig([]string{"--config", tmpFile.Name()})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Defaults.Currency != "GBP" {
		t.Errorf("expected expanded default currency GBP, got %s", cfg.Defaults.Currency)
	}
	if cfg.Reporting.Format != "markdown" {
		t.Errorf("expected expanded format markdown, got %s", cfg.Reporting.Format)
	}
	if cfg.SlackWebhook != "https://hooks.slack.com/services/TEST/EXPAND" {
		t.Errorf("expected expanded Slack webhook URL, got %s", cfg.SlackWebhook)
	}
}

func TestConfigChargeTypeAndExcludeCredits(t *testing.T) {
	// 1. --exclude-credits flag
	cfg1, err := LoadConfig([]string{"--exclude-credits"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg1.ExcludeCredits {
		t.Error("expected ExcludeCredits=true, got false")
	}

	// 2. --charge-type flag
	cfg2, err := LoadConfig([]string{"--charge-type", "usage,tax,refund"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg2.ChargeTypes) != 3 || cfg2.ChargeTypes[0] != "Usage" || cfg2.ChargeTypes[1] != "Tax" || cfg2.ChargeTypes[2] != "Refund" {
		t.Errorf("expected [Usage Tax Refund], got %+v", cfg2.ChargeTypes)
	}

	// 3. --record-type alias
	cfg3, err := LoadConfig([]string{"--record-type", "Credit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg3.ChargeTypes) != 1 || cfg3.ChargeTypes[0] != "Credit" {
		t.Errorf("expected [Credit], got %+v", cfg3.ChargeTypes)
	}
}

func TestConfigRegionsAndAssumeRole(t *testing.T) {
	// 1. Region flag
	cfg1, err := LoadConfig([]string{"--region", "us-east-1,eu-west-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg1.Regions) != 2 || cfg1.Regions[0] != "us-east-1" || cfg1.Regions[1] != "eu-west-1" {
		t.Errorf("expected [us-east-1 eu-west-1], got %+v", cfg1.Regions)
	}

	// 2. Assume role flags
	cfg2, err := LoadConfig([]string{
		"--assume-role-arn", "arn:aws:iam::123456789012:role/BillingAudit",
		"--role-session-name", "my-audit-session",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.AssumeRoleARN != "arn:aws:iam::123456789012:role/BillingAudit" {
		t.Errorf("expected assume role ARN set, got %s", cfg2.AssumeRoleARN)
	}
	if cfg2.RoleSessionName != "my-audit-session" {
		t.Errorf("expected role session name 'my-audit-session', got %s", cfg2.RoleSessionName)
	}

	// 3. Environment variable fallback
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::999888777666:role/EnvRole")
	t.Setenv("AWS_ROLE_SESSION_NAME", "env-session")
	cfg3, err := LoadConfig([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg3.AssumeRoleARN != "arn:aws:iam::999888777666:role/EnvRole" {
		t.Errorf("expected ARN from env, got %s", cfg3.AssumeRoleARN)
	}
	if cfg3.RoleSessionName != "env-session" {
		t.Errorf("expected session name from env, got %s", cfg3.RoleSessionName)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError string
	}{
		{
			name:        "invalid format",
			args:        []string{"--format", "xml"},
			expectError: "invalid format",
		},
		{
			name:        "invalid granularity",
			args:        []string{"--granularity", "HOURLY"},
			expectError: "invalid granularity",
		},
		{
			name:        "negative months",
			args:        []string{"--months", "-1"},
			expectError: "months back cannot be negative",
		},
		{
			name:        "negative top",
			args:        []string{"--top", "-5"},
			expectError: "top cannot be negative",
		},
		{
			name:        "negative threshold",
			args:        []string{"--threshold", "-10.0"},
			expectError: "threshold cannot be negative",
		},
		{
			name:        "negative min-cost",
			args:        []string{"--min-cost", "-2.5"},
			expectError: "min-cost cannot be negative",
		},
		{
			name:        "invalid start date",
			args:        []string{"--start", "not-a-date"},
			expectError: "invalid start date",
		},
		{
			name:        "invalid end date",
			args:        []string{"--end", "2026-99-99"},
			expectError: "invalid end date",
		},
		{
			name:        "start after end date",
			args:        []string{"--start", "2026-10-15", "--end", "2026-10-01"},
			expectError: "start date (2026-10-15) must be before or equal to end date (2026-10-01)",
		},
		{
			name:        "invalid group-by dimension",
			args:        []string{"--group-by", "INVALID_DIM"},
			expectError: "invalid grouping dimension",
		},
		{
			name:        "valid configuration",
			args:        []string{"--format", "markdown", "--granularity", "DAILY", "--months", "3", "--top", "5", "--threshold", "100.0", "--min-cost", "0.05", "--start", "2026-09-01", "--end", "2026-10-01", "--group-by", "SERVICE,TAG:Environment"},
			expectError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadConfig(tt.args)
			if tt.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectError)
				}
				if !strings.Contains(err.Error(), tt.expectError) {
					t.Fatalf("expected error containing %q, got %q", tt.expectError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for valid config: %v", err)
				}
				if cfg == nil {
					t.Fatalf("expected non-nil config")
				}
			}
		})
	}
}
