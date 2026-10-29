package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
)

func TestSendTeamsNotification(t *testing.T) {
	var receivedPayload TeamsPayload

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode received Teams payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1"))
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.CompareMoM = true
	cfg.AccountName = "Production Core"

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := awsclient.GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("failed to generate mock report: %v", err)
	}

	ctx := context.Background()

	// 1. Test sending notification with threshold alert breached (21.58 > 20.00)
	err = SendTeamsNotification(ctx, ts.URL, report, 20.00)
	if err != nil {
		t.Fatalf("SendTeamsNotification failed: %v", err)
	}

	if receivedPayload.Type != "MessageCard" {
		t.Errorf("expected @type MessageCard, got %s", receivedPayload.Type)
	}
	if receivedPayload.ThemeColor != "E81123" {
		t.Errorf("expected alert red themeColor E81123, got %s", receivedPayload.ThemeColor)
	}
	if !strings.Contains(receivedPayload.Title, "Budget Alert") {
		t.Errorf("expected Budget Alert in title, got %s", receivedPayload.Title)
	}
	if len(receivedPayload.Sections) < 2 {
		t.Fatalf("expected at least 2 sections, got %d", len(receivedPayload.Sections))
	}

	// Verify facts contain account name
	foundAccount := false
	for _, f := range receivedPayload.Sections[0].Facts {
		if f.Name == "Account:" && strings.Contains(f.Value, "Production Core") {
			foundAccount = true
			break
		}
	}
	if !foundAccount {
		t.Errorf("expected account fact with 'Production Core'")
	}

	// 2. Test empty webhook URL is no-op
	if err := SendTeamsNotification(ctx, "", report, 0.00); err != nil {
		t.Errorf("expected no error for empty webhook URL, got %v", err)
	}

	// 3. Test non-breached threshold
	err = SendTeamsNotification(ctx, ts.URL, report, 100.00)
	if err != nil {
		t.Fatalf("SendTeamsNotification with high threshold failed: %v", err)
	}
	if receivedPayload.ThemeColor != "0076D7" {
		t.Errorf("expected standard blue themeColor 0076D7, got %s", receivedPayload.ThemeColor)
	}
}
