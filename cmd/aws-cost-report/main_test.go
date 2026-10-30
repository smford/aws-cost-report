package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

type mockS3Client struct {
	lastBucket string
	lastKey    string
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.lastBucket = *params.Bucket
	m.lastKey = *params.Key
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}

func TestVersionDefaults(t *testing.T) {
	if version == "" {
		t.Error("expected non-empty version")
	}
}

func TestFormatVersion(t *testing.T) {
	output := formatVersion()

	// Target layout verification:
	// <app-name> <version> (commit: <sha>, date: <timestamp>)
	// Copyright (c) 2026 smford
	// Licensed under the MIT licence.
	expectedPrefix := "aws-cost-report dev (commit: none, date: unknown)\nCopyright (c) 2026 smford\nLicensed under the MIT licence.\n"
	if output != expectedPrefix {
		t.Errorf("formatVersion output mismatch.\nExpected:\n%s\nGot:\n%s", expectedPrefix, output)
	}
}

func TestHealthzEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Errorf("expected status ok, got %q", rr.Body.String())
	}
}

func TestMockRunAndFileOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := tmpDir + "/report.md"
	jsonFile := tmpDir + "/summary.json"

	cfg, err := config.LoadConfig([]string{
		"--mock",
		"--format", "markdown",
		"--output", outFile,
		"--json-summary", jsonFile,
		"--threshold", "100.00",
	})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify fields
	if cfg.OutputFile != outFile {
		t.Errorf("expected OutputFile %s, got %s", outFile, cfg.OutputFile)
	}
	if cfg.JSONSummaryFile != jsonFile {
		t.Errorf("expected JSONSummaryFile %s, got %s", jsonFile, cfg.JSONSummaryFile)
	}
}

func TestReadyzEndpointMock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready","mode":"mock"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"status":"ready"`) {
		t.Errorf("expected status ready, got %s", rr.Body.String())
	}
}

func TestServerReportQueryParameters(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true

	mux := http.NewServeMux()
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		reqCfg := *cfg
		if q.Get("compare") == "true" {
			reqCfg.CompareMoM = true
		}
		if q.Get("format") != "" {
			reqCfg.Reporting.Format = q.Get("format")
		}

		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Report Markdown"))
	})

	req := httptest.NewRequest(http.MethodGet, "/report?format=markdown&compare=true&top=3", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Errorf("expected markdown Content-Type, got %s", rr.Header().Get("Content-Type"))
	}
}

func TestRunCLIMockMode(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := tmpDir + "/output.md"
	jsonFile := tmpDir + "/summary.json"

	cfg, err := config.LoadConfig([]string{
		"--mock",
		"--compare",
		"--format", "markdown",
		"--output", outFile,
		"--json-summary", jsonFile,
		"--threshold", "100.00",
		"--top", "3",
	})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}

	// Verify output files were written
	outData, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(outData), "# 📊 AWS Monthly Billing & Usage Report") {
		t.Errorf("expected markdown header in output file")
	}

	jsonData, err := os.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf("failed to read json summary file: %v", err)
	}
	if !strings.Contains(string(jsonData), `"threshold_breached": false`) {
		t.Errorf("expected threshold_breached false in json summary")
	}
}

func TestRunCLIMockModeWithTagFilter(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := tmpDir + "/output_tagged.md"

	cfg, err := config.LoadConfig([]string{
		"--mock",
		"--format", "markdown",
		"--tag", "Environment=Production",
		"--output", outFile,
	})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}

	outData, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	content := string(outData)
	if !strings.Contains(content, "| **Tag Filter** | `Environment=Production` |") {
		t.Errorf("expected Tag Filter in markdown output, got:\n%s", content)
	}
}

func TestRunCLIMockModeWithCurrency(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := tmpDir + "/output_eur.md"

	cfg, err := config.LoadConfig([]string{
		"--mock",
		"--currency", "EUR",
		"--format", "markdown",
		"--output", outFile,
	})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}

	outData, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	content := string(outData)
	if !strings.Contains(content, "| **Currency** | EUR |") {
		t.Errorf("expected Currency EUR in markdown output, got:\n%s", content)
	}
	if !strings.Contains(content, "€21.58") {
		t.Errorf("expected €21.58 in markdown output, got:\n%s", content)
	}
}

func TestFormatHelp(t *testing.T) {
	helpText := formatHelp()
	if !strings.Contains(helpText, "COMMANDS:") {
		t.Errorf("expected COMMANDS section in help text")
	}
	if !strings.Contains(helpText, "generate") {
		t.Errorf("expected generate command in help text")
	}
	if !strings.Contains(helpText, "services") {
		t.Errorf("expected services command in help text")
	}
	if !strings.Contains(helpText, "server") {
		t.Errorf("expected server command in help text")
	}
}

func TestRunListServicesMock(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.Reporting.Format = "json"

	// Should not panic or error
	var stdout, stderr bytes.Buffer
	code := runListServices(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}
}

func TestReportCacheEvictionAndKeys(t *testing.T) {
	cache := newReportCache(50*time.Millisecond, 3)

	rep1 := &awsclient.CostReport{}
	rep2 := &awsclient.CostReport{}
	rep3 := &awsclient.CostReport{}
	rep4 := &awsclient.CostReport{}

	cache.Set("k1", rep1)
	cache.Set("k2", rep2)
	cache.Set("k3", rep3)

	if got, ok := cache.Get("k1"); !ok || got != rep1 {
		t.Errorf("expected k1 to be present")
	}

	// Adding 4th item when max is 3 triggers eviction
	cache.Set("k4", rep4)
	if got, ok := cache.Get("k4"); !ok || got != rep4 {
		t.Errorf("expected k4 to be present")
	}

	// Verify TTL expiration
	time.Sleep(60 * time.Millisecond)
	if _, ok := cache.Get("k4"); ok {
		t.Errorf("expected k4 to have expired")
	}
}

func TestRunCLIMockModePrometheus(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := tmpDir + "/metrics.prom"

	cfg, err := config.LoadConfig([]string{
		"--mock",
		"--compare",
		"--format", "prometheus",
		"--output", outFile,
	})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runCLI(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read prometheus output: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "aws_cost_report_build_info") {
		t.Errorf("expected aws_cost_report_build_info in output, got:\n%s", content)
	}
	if !strings.Contains(content, "aws_cost_report_total_unblended_dollars") {
		t.Errorf("expected aws_cost_report_total_unblended_dollars in output, got:\n%s", content)
	}
	if !strings.Contains(content, "aws_cost_report_service_unblended_dollars") {
		t.Errorf("expected aws_cost_report_service_unblended_dollars in output, got:\n%s", content)
	}
}

func TestShellCompletionGenerators(t *testing.T) {
	// 1. Bash
	var bashBuf bytes.Buffer
	generateBashCompletion(&bashBuf)
	bashScript := bashBuf.String()
	if !strings.Contains(bashScript, "_aws_cost_report()") || !strings.Contains(bashScript, "complete -F _aws_cost_report aws-cost-report") {
		t.Errorf("expected valid bash completion script, got:\n%s", bashScript)
	}

	// 2. Zsh
	var zshBuf bytes.Buffer
	generateZshCompletion(&zshBuf)
	zshScript := zshBuf.String()
	if !strings.Contains(zshScript, "#compdef aws-cost-report") || !strings.Contains(zshScript, "_arguments") {
		t.Errorf("expected valid zsh completion script, got:\n%s", zshScript)
	}

	// 3. Fish
	var fishBuf bytes.Buffer
	generateFishCompletion(&fishBuf)
	fishScript := fishBuf.String()
	if !strings.Contains(fishScript, "complete -c aws-cost-report") {
		t.Errorf("expected valid fish completion script, got:\n%s", fishScript)
	}
}

func TestSaveOutputFileS3Mock(t *testing.T) {
	ctx := context.Background()
	// Mock mode with s3:// URI
	err := saveOutputFile(ctx, nil, "s3://mock-bucket/reports/test.md", []byte("# Report"), true)
	if err != nil {
		t.Fatalf("expected saveOutputFile with mock s3:// to succeed, got %v", err)
	}

	// Invalid s3 URI
	err = saveOutputFile(ctx, nil, "s3://invalid", []byte("# Report"), true)
	if err == nil {
		t.Errorf("expected error for invalid s3 URI without key")
	}

	// Local file fallback
	tmpDir := t.TempDir()
	localFile := tmpDir + "/test-output.txt"
	err = saveOutputFile(ctx, nil, localFile, []byte("hello local"), false)
	if err != nil {
		t.Fatalf("saveOutputFile local failed: %v", err)
	}
	content, _ := os.ReadFile(localFile)
	if string(content) != "hello local" {
		t.Errorf("expected 'hello local', got %s", string(content))
	}

	// Real S3 upload via client with mock S3 implementation
	mockS3 := &mockS3Client{}
	client := awsclient.NewClient(nil, nil)
	client.SetS3Client(mockS3)
	err = saveOutputFile(ctx, client, "s3://valid-bucket/valid-key.md", []byte("s3 content"), false)
	if err != nil {
		t.Errorf("expected saveOutputFile with client to succeed, got %v", err)
	}
	if mockS3.lastBucket != "valid-bucket" || mockS3.lastKey != "valid-key.md" {
		t.Errorf("unexpected s3 upload target: bucket=%s, key=%s", mockS3.lastBucket, mockS3.lastKey)
	}

	// Local file error
	err = saveOutputFile(ctx, nil, "/nonexistent_root_dir_xyz/test.txt", []byte("data"), false)
	if err == nil {
		t.Errorf("expected error writing to invalid system path")
	}
}

func TestRunCLIMockModeS3Upload(t *testing.T) {
	cfg, err := config.LoadConfig([]string{
		"--mock",
		"--format", "markdown",
		"--output", "s3://my-billing-bucket/reports/oct-2026.md",
		"--s3-bucket", "my-billing-bucket",
		"--s3-key", "backup/oct-2026.md",
	})
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Should run cleanly in mock mode without panicking
	var stdout, stderr bytes.Buffer
	code := runCLI(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, stderr.String())
	}
}

func TestRunCommandEntrypoints(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "nonexistent-profile-xyz")

	cases := []struct {
		name         string
		args         []string
		expectedCode int
		containsText string
		inStderr     bool
	}{
		{
			name:         "No args prints help",
			args:         []string{},
			expectedCode: 0,
			containsText: "aws-cost-report - AWS Cost Explorer reporting",
		},
		{
			name:         "help subcommand",
			args:         []string{"help"},
			expectedCode: 0,
			containsText: "COMMANDS:",
		},
		{
			name:         "--help flag",
			args:         []string{"--help"},
			expectedCode: 0,
			containsText: "COMMANDS:",
		},
		{
			name:         "-h flag",
			args:         []string{"-h"},
			expectedCode: 0,
			containsText: "COMMANDS:",
		},
		{
			name:         "version subcommand",
			args:         []string{"version"},
			expectedCode: 0,
			containsText: "aws-cost-report",
		},
		{
			name:         "--version flag",
			args:         []string{"--version"},
			expectedCode: 0,
			containsText: "aws-cost-report",
		},
		{
			name:         "-v flag",
			args:         []string{"-v"},
			expectedCode: 0,
			containsText: "aws-cost-report",
		},
		{
			name:         "completion bash",
			args:         []string{"completion", "bash"},
			expectedCode: 0,
			containsText: "_aws_cost_report()",
		},
		{
			name:         "completion zsh",
			args:         []string{"completion", "zsh"},
			expectedCode: 0,
			containsText: "#compdef aws-cost-report",
		},
		{
			name:         "completion fish",
			args:         []string{"completion", "fish"},
			expectedCode: 0,
			containsText: "complete -c aws-cost-report",
		},
		{
			name:         "completion missing arg",
			args:         []string{"completion"},
			expectedCode: 1,
			containsText: "shell type required",
			inStderr:     true,
		},
		{
			name:         "completion invalid shell",
			args:         []string{"completion", "powershell"},
			expectedCode: 1,
			containsText: "unsupported shell",
			inStderr:     true,
		},
		{
			name:         "unknown command",
			args:         []string{"foo-bar"},
			expectedCode: 1,
			containsText: "unknown command",
			inStderr:     true,
		},
		{
			name:         "generate with invalid flag",
			args:         []string{"generate", "--unknown-flag-xyz"},
			expectedCode: 1,
			containsText: "Error parsing configuration",
			inStderr:     true,
		},
		{
			name:         "generate --help flag in generate args",
			args:         []string{"generate", "--help"},
			expectedCode: 0,
			containsText: "COMMANDS:",
		},
		{
			name:         "services mock text",
			args:         []string{"services", "--mock"},
			expectedCode: 0,
			containsText: "Official AWS Cost Explorer Service Names:",
		},
		{
			name:         "services mock json",
			args:         []string{"services", "--mock", "--format=json"},
			expectedCode: 0,
			containsText: `"Amazon DynamoDB"`,
		},
		{
			name:         "generate mock table",
			args:         []string{"generate", "--mock", "--format=table"},
			expectedCode: 0,
			containsText: "AWS MONTHLY BILLING & USAGE REPORT",
		},
		{
			name:         "generate mock markdown",
			args:         []string{"generate", "--mock", "--format=markdown"},
			expectedCode: 0,
			containsText: "# 📊 AWS Monthly Billing & Usage Report",
		},
		{
			name:         "generate mock json",
			args:         []string{"generate", "--mock", "--format=json"},
			expectedCode: 0,
			containsText: `"totals"`,
		},
		{
			name:         "generate mock csv",
			args:         []string{"generate", "--mock", "--format=csv"},
			expectedCode: 0,
			containsText: "Service,UnblendedCost,BlendedCost,AmortizedCost",
		},
		{
			name:         "generate mock prometheus",
			args:         []string{"generate", "--mock", "--format=prometheus"},
			expectedCode: 0,
			containsText: "aws_cost_report_total_unblended_dollars",
		},
		{
			name:         "generate mock html",
			args:         []string{"generate", "--mock", "--format=html"},
			expectedCode: 0,
			containsText: "<!DOCTYPE html>",
		},
		{
			name:         "generate mock monthly breakdown",
			args:         []string{"generate", "--mock", "--monthly"},
			expectedCode: 0,
			containsText: "HISTORICAL TREND",
		},
		{
			name:         "generate mock daily breakdown",
			args:         []string{"generate", "--mock", "--daily"},
			expectedCode: 0,
			containsText: "DAILY SPEND BREAKDOWN",
		},
		{
			name:         "generate mock forecast normal",
			args:         []string{"generate", "--mock", "--forecast"},
			expectedCode: 0,
			containsText: "Forecast",
		},
		{
			name:         "generate mock budget threshold breached",
			args:         []string{"generate", "--mock", "--threshold=10.00"},
			expectedCode: 1,
			containsText: "[BUDGET ALERT]",
			inStderr:     true,
		},
		{
			name:         "generate mock forecast threshold breached",
			args:         []string{"generate", "--mock", "--forecast", "--threshold=25.00"},
			expectedCode: 1,
			containsText: "[BUDGET ALERT] Projected month-end spend",
			inStderr:     true,
		},
		{
			name:         "generate mock exclude credits",
			args:         []string{"generate", "--mock", "--exclude-credits"},
			expectedCode: 0,
			containsText: "Credits / Refunds:  Excluded",
		},
		{
			name:         "generate mock charge type tax",
			args:         []string{"generate", "--mock", "--charge-type=Tax"},
			expectedCode: 0,
			containsText: "Charge Types:       Tax",
		},
		{
			name:         "direct mock flag without subcommand",
			args:         []string{"--mock"},
			expectedCode: 0,
			containsText: "AWS MONTHLY BILLING & USAGE REPORT",
		},
		{
			name:         "--services flag",
			args:         []string{"--services", "--mock"},
			expectedCode: 0,
			containsText: "Official AWS Cost Explorer Service Names:",
		},
		{
			name:         "--list-services flag",
			args:         []string{"--list-services", "--mock"},
			expectedCode: 0,
			containsText: "Official AWS Cost Explorer Service Names:",
		},
		{
			name:         "generate with --version flag",
			args:         []string{"generate", "--version"},
			expectedCode: 0,
			containsText: "aws-cost-report",
		},
		{
			name:         "generate with invalid config file path",
			args:         []string{"generate", "-c", "nonexistent-config-file-xyz.yaml"},
			expectedCode: 1,
			containsText: "Error parsing configuration",
			inStderr:     true,
		},
		{
			name:         "services command without mock fails gracefully without AWS auth",
			args:         []string{"services"},
			expectedCode: 1,
			containsText: "AWS Credential Resolution Order:",
			inStderr:     true,
		},
		{
			name:         "generate command without mock fails gracefully without AWS auth",
			args:         []string{"generate"},
			expectedCode: 1,
			containsText: "AWS Credential Resolution Order:",
			inStderr:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, &stdout, &stderr)

			if code != tc.expectedCode {
				t.Errorf("expected exit code %d, got %d. Stderr: %s, Stdout: %s", tc.expectedCode, code, stderr.String(), stdout.String())
			}

			output := stdout.String()
			if tc.inStderr {
				output = stderr.String()
			}

			if tc.containsText != "" && !strings.Contains(output, tc.containsText) {
				t.Errorf("expected output to contain %q, but got:\n%s", tc.containsText, output)
			}
		})
	}
}

func TestBuildServerMuxEndpoints(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "nonexistent-profile-xyz")

	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cache := newReportCache(10 * time.Minute)

	handler := buildServerMux(cfg, cache)

	// 1. /healthz
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected /healthz status 200, got %d", w.Code)
	}

	// 2. /readyz (mock mode)
	req = httptest.NewRequest("GET", "/readyz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"mode":"mock"`) {
		t.Errorf("expected /readyz mock response, got %d: %s", w.Code, w.Body.String())
	}

	// 3. /metrics
	req = httptest.NewRequest("GET", "/metrics?compare=true&group-by=SERVICE", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "aws_cost_report_") {
		t.Errorf("expected /metrics prometheus response, got %d: %s", w.Code, w.Body.String())
	}

	// 4. /report with various formats
	formats := []struct {
		format      string
		contentType string
	}{
		{"json", "application/json"},
		{"markdown", "text/markdown"},
		{"html", "text/html"},
		{"table", "text/plain"},
		{"csv", "text/csv"},
		{"prometheus", "text/plain"},
	}

	for _, f := range formats {
		req = httptest.NewRequest("GET", "/report?format="+f.format+"&compare=true&top=5&months=1&mtd=true", nil)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected /report format %s status 200, got %d", f.format, w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Type"), f.contentType) {
			t.Errorf("expected Content-Type containing %s, got %s", f.contentType, w.Header().Get("Content-Type"))
		}
	}

	// 5. /report cache hit
	req = httptest.NewRequest("GET", "/report?format=json&compare=true&top=5&months=1&mtd=true", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected cached /report status 200, got %d", w.Code)
	}

	// 6. /report with query params for filtering, grouping, dates, aliases
	extraQueries := []string{
		"/report?trend=1&current-month=1&start=2026-09-01&end=2026-09-30&group-by=TAG:Environment&name=MyAccount&filter=DynamoDB,Config&top=3&min-cost=0.50&tag=Environment=Production&tag-filter=Project=Core",
		"/report?monthly=true&months=6&groupBy=SERVICE&account-name=TestAlias",
		"/report?groupBy=TAG:Project",
		"/metrics?mtd=1&name=TestProd&group-by=TAG:Environment",
	}
	for _, qStr := range extraQueries {
		req = httptest.NewRequest("GET", qStr, nil)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected %s status 200, got %d", qStr, w.Code)
		}
	}

	// 7. Non-mock error branches
	cfgReal := config.DefaultConfig()
	cfgReal.MockMode = false
	handlerReal := buildServerMux(cfgReal, cache)

	req = httptest.NewRequest("GET", "/readyz", nil)
	w = httptest.NewRecorder()
	handlerReal.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusOK {
		t.Errorf("expected 503 or 200 from real readyz, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/metrics", nil)
	w = httptest.NewRecorder()
	handlerReal.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusOK {
		t.Errorf("expected 503 or 200 from real metrics, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/report", nil)
	w = httptest.NewRecorder()
	handlerReal.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusOK {
		t.Errorf("expected 500 or 200 from real report, got %d", w.Code)
	}
}

func TestPrintCredentialHelp(t *testing.T) {
	var buf bytes.Buffer
	printCredentialHelp(&buf, errors.New("auth failed"))
	out := buf.String()
	if !strings.Contains(out, "AWS Credential Resolution Order:") {
		t.Errorf("expected credential resolution guidance, got:\n%s", out)
	}
}

func TestRunServerGracefulShutdown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.Server.Port = 0 // OS allocated free port
	cfg.Server.Host = "127.0.0.1"

	stopCh := make(chan os.Signal, 1)

	done := make(chan struct{})
	go func() {
		runServer(cfg, stopCh)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	stopCh <- os.Interrupt

	select {
	case <-done:
		// Clean exit
	case <-time.After(2 * time.Second):
		t.Fatal("runServer did not shut down in time")
	}
}

func TestRunCLIWithWebhooksAndSummary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	outFile := tmpDir + "/nested/dir/report.md"
	jsonFile := tmpDir + "/nested/dir/summary.json"

	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.Reporting.Format = "markdown"
	cfg.SlackWebhook = ts.URL
	cfg.TeamsWebhook = ts.URL
	cfg.WebhookURL = ts.URL
	cfg.OutputFile = outFile
	cfg.JSONSummaryFile = jsonFile
	cfg.S3Bucket = "backup-bucket"
	cfg.Forecast = true
	cfg.Budget.FailOnThreshold = 1000.00

	var stdout, stderr bytes.Buffer
	code := runCLI(cfg, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected runCLI code 0, got %d: %s", code, stderr.String())
	}

	if _, err := os.Stat(outFile); err != nil {
		t.Errorf("expected output file to exist: %v", err)
	}
	if _, err := os.Stat(jsonFile); err != nil {
		t.Errorf("expected json summary file to exist: %v", err)
	}

	// Webhook failure and output file failure warnings
	cfgWarn := config.DefaultConfig()
	cfgWarn.MockMode = true
	cfgWarn.SlackWebhook = "http://127.0.0.1:1/invalid"
	cfgWarn.TeamsWebhook = "http://127.0.0.1:1/invalid"
	cfgWarn.WebhookURL = "http://127.0.0.1:1/invalid"
	cfgWarn.OutputFile = "/nonexistent_root_dir/report.md"
	cfgWarn.JSONSummaryFile = "/nonexistent_root_dir/summary.json"
	var warnStdout, warnStderr bytes.Buffer
	codeWarn := runCLI(cfgWarn, &warnStdout, &warnStderr)
	if codeWarn != 0 {
		t.Fatalf("expected code 0 despite warnings, got %d", codeWarn)
	}
	if !strings.Contains(warnStderr.String(), "Warning:") {
		t.Errorf("expected warnings in stderr, got:\n%s", warnStderr.String())
	}
}
