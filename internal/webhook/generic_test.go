package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

func TestSendGenericWebhookSuccess(t *testing.T) {
	var receivedPayload GenericWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json Content-Type, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Errorf("failed to unmarshal received body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"received"}`))
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := awsclient.GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("failed to generate mock report: %v", err)
	}

	ctx := context.Background()
	err = SendGenericWebhook(ctx, server.URL, report, 100.0)
	if err != nil {
		t.Fatalf("expected no error sending generic webhook, got: %v", err)
	}

	if receivedPayload.Event != "cost_report" {
		t.Errorf("expected event 'cost_report', got %s", receivedPayload.Event)
	}
	if receivedPayload.Alert {
		t.Errorf("expected alert to be false when cost is below threshold")
	}
	if receivedPayload.Totals.UnblendedCost <= 0 {
		t.Errorf("expected positive total cost in payload")
	}
}

func TestSendGenericWebhookAlert(t *testing.T) {
	var receivedPayload GenericWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, _ := awsclient.GenerateMockReport(cfg, refTime)

	ctx := context.Background()
	// Threshold set lower than total spend
	err := SendGenericWebhook(ctx, server.URL, report, 5.00)
	if err != nil {
		t.Fatalf("expected no error sending generic webhook: %v", err)
	}

	if !receivedPayload.Alert {
		t.Errorf("expected alert to be true when cost exceeds threshold")
	}
	if !receivedPayload.ThresholdExceeded {
		t.Errorf("expected threshold_exceeded to be true")
	}
}

func TestSendGenericWebhookHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, _ := awsclient.GenerateMockReport(cfg, refTime)

	ctx := context.Background()
	err := SendGenericWebhook(ctx, server.URL, report, 0)
	if err == nil {
		t.Fatalf("expected error on 500 response, got nil")
	}
}

func TestSendGenericWebhookEmptyURL(t *testing.T) {
	err := SendGenericWebhook(context.Background(), "", nil, 0)
	if err != nil {
		t.Fatalf("expected nil when webhookURL is empty, got %v", err)
	}
}
