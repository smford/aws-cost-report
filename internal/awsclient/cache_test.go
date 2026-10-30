package awsclient

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/smford/aws-cost-report/internal/config"
)

func TestCacheSaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()

	report, err := GenerateMockReport(cfg, time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	key := ComputeCacheKey(report.Metadata.AccountID, cfg, report.Metadata.PeriodStart, report.Metadata.PeriodEnd)
	if key == "" {
		t.Fatalf("expected non-empty cache key")
	}

	// 1. Get non-existent
	cached, hit, err := GetCachedReport(tmpDir, key)
	if err != nil {
		t.Fatalf("unexpected error getting non-existent cache: %v", err)
	}
	if hit || cached != nil {
		t.Fatalf("expected cache miss, got hit")
	}

	// 2. Save to cache
	err = SaveCachedReport(tmpDir, key, report, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to save cached report: %v", err)
	}

	// 3. Get cached hit
	cached, hit, err = GetCachedReport(tmpDir, key)
	if err != nil {
		t.Fatalf("failed to get cached report: %v", err)
	}
	if !hit || cached == nil {
		t.Fatalf("expected cache hit, got miss")
	}
	if cached.Totals.UnblendedCost != report.Totals.UnblendedCost {
		t.Errorf("expected cost %.2f, got %.2f", report.Totals.UnblendedCost, cached.Totals.UnblendedCost)
	}

	// 4. Test expired cache entry
	expiredKey := "expired-test-key"
	err = SaveCachedReport(tmpDir, expiredKey, report, -1*time.Minute)
	if err != nil {
		t.Fatalf("failed to save expired cache: %v", err)
	}
	cached, hit, err = GetCachedReport(tmpDir, expiredKey)
	if err != nil {
		t.Fatalf("unexpected error on expired cache: %v", err)
	}
	if hit || cached != nil {
		t.Fatalf("expected expired entry to be treated as cache miss")
	}

	// 5. Clear cache
	removed, err := ClearCache(tmpDir)
	if err != nil {
		t.Fatalf("failed to clear cache: %v", err)
	}
	if removed < 1 {
		t.Errorf("expected at least 1 removed file, got %d", removed)
	}
}

func TestDefaultCacheDir(t *testing.T) {
	dir := DefaultCacheDir()
	if dir == "" {
		t.Errorf("expected non-empty default cache dir")
	}
	if filepath.Base(dir) != "aws-cost-report" {
		t.Errorf("expected base directory 'aws-cost-report', got %s", filepath.Base(dir))
	}
}
