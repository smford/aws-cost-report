package awsclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/smford/aws-cost-report/internal/config"
)

// DefaultCacheDir returns the platform-specific standard cache directory for aws-cost-report.
func DefaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			return filepath.Join(home, ".cache", "aws-cost-report")
		}
		return filepath.Join(os.TempDir(), "aws-cost-report-cache")
	}
	return filepath.Join(base, "aws-cost-report")
}

type CachedEntry struct {
	Report    CostReport `json:"report"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// ComputeCacheKey produces a deterministic SHA-256 hash representing all query parameters.
func ComputeCacheKey(accountID string, cfg *config.Config, periodStart, periodEnd string) string {
	var b strings.Builder
	b.WriteString(accountID)
	b.WriteString("|")
	b.WriteString(periodStart)
	b.WriteString("|")
	b.WriteString(periodEnd)
	b.WriteString("|")
	b.WriteString(cfg.Granularity)
	b.WriteString("|")
	b.WriteString(fmt.Sprintf("months:%d|daily:%t|mtd:%t|compare:%t|forecast:%t|top:%d", cfg.MonthsBack, cfg.Daily, cfg.MTD, cfg.CompareMoM, cfg.Forecast, cfg.Defaults.Top))
	b.WriteString("|")
	b.WriteString(cfg.AccountName)
	b.WriteString("|")
	b.WriteString(fmt.Sprintf("ex_credits:%t|charge_types:%s", cfg.ExcludeCredits, strings.Join(cfg.ChargeTypes, ",")))
	b.WriteString("|")

	for _, g := range cfg.Groupings {
		b.WriteString(fmt.Sprintf("%s:%s;", g.Type, g.Key))
	}
	b.WriteString("|")

	var sortedFilters []string
	sortedFilters = append(sortedFilters, cfg.Filters...)
	sort.Strings(sortedFilters)
	b.WriteString(strings.Join(sortedFilters, ","))
	b.WriteString("|")

	if len(cfg.TagFilters) > 0 {
		var tagKeys []string
		for k := range cfg.TagFilters {
			tagKeys = append(tagKeys, k)
		}
		sort.Strings(tagKeys)
		for _, k := range tagKeys {
			vals := cfg.TagFilters[k]
			sort.Strings(vals)
			b.WriteString(fmt.Sprintf("%s=%s;", k, strings.Join(vals, ",")))
		}
	}
	b.WriteString("|")
	b.WriteString(strings.Join(cfg.Defaults.Metrics, ","))
	b.WriteString("|")
	b.WriteString(cfg.Defaults.Currency)

	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// GetCachedReport reads a cached report from disk if present and not expired.
func GetCachedReport(dir, key string) (*CostReport, bool, error) {
	if dir == "" || key == "" {
		return nil, false, nil
	}
	path := filepath.Join(dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var entry CachedEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false, err
	}

	if time.Now().UTC().After(entry.ExpiresAt) {
		_ = os.Remove(path)
		return nil, false, nil
	}

	return &entry.Report, true, nil
}

// SaveCachedReport atomically writes a CostReport to disk with an expiration TTL.
func SaveCachedReport(dir, key string, report *CostReport, ttl time.Duration) error {
	if dir == "" || key == "" || report == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	now := time.Now().UTC()
	entry := CachedEntry{
		Report:    *report,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cached report: %w", err)
	}

	tmpFile := filepath.Join(dir, fmt.Sprintf(".%s-%d.tmp", key, now.UnixNano()))
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary cache file: %w", err)
	}

	destFile := filepath.Join(dir, key+".json")
	return os.Rename(tmpFile, destFile)
}

// ClearCache removes all cached cost report JSON files from the specified cache directory.
func ClearCache(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			p := filepath.Join(dir, e.Name())
			if err := os.Remove(p); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
