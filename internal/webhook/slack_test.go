package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

func TestSendSlackNotification(t *testing.T) {
	var receivedPayload SlackPayload

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode received Slack payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.CompareMoM = true

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := awsclient.GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("failed to generate mock report: %v", err)
	}

	ctx := context.Background()
	// Test sending notification with threshold alert
	err = SendSlackNotification(ctx, ts.URL, report, 20.00)
	if err != nil {
		t.Fatalf("SendSlackNotification failed: %v", err)
	}

	if len(receivedPayload.Blocks) == 0 {
		t.Fatalf("expected blocks in received Slack payload")
	}

	// Verify alert title
	if receivedPayload.Text == "" {
		t.Errorf("expected non-empty fallback text")
	}

	// Test empty webhook URL does not error
	if err := SendSlackNotification(ctx, "", report, 0.00); err != nil {
		t.Errorf("expected nil error for empty webhook URL, got %v", err)
	}
}
