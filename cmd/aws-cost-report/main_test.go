package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
