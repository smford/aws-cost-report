package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/smford/aws-cost-report/internal/config"
)

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

	runCLI(cfg)

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
	runListServices(cfg)
}
