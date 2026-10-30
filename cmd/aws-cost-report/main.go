package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
	"github.com/smford/aws-cost-report/internal/renderer"
	"github.com/smford/aws-cost-report/internal/webhook"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func formatVersion() string {
	return fmt.Sprintf("aws-cost-report %s (commit: %s, date: %s)\nCopyright (c) 2026 smford\nLicensed under the MIT licence.\n", version, commit, date)
}

func printVersion(w io.Writer) {
	fmt.Fprint(w, formatVersion())
}

func formatHelp() string {
	return `aws-cost-report - AWS Cost Explorer reporting, Month-over-Month deltas, and CI/CD budget guardrails

USAGE:
  aws-cost-report <command> [flags]

COMMANDS:
  generate          Generate monthly billing and usage cost report (default)
  services          List official AWS Cost Explorer service names available in your account
  tags              List active AWS Cost Allocation Tag keys or tag values
  anomalies         List recent AWS Cost Anomalies detected by Cost Explorer
  server            Run as an HTTP service daemon exposing /healthz, /readyz, /report, and /metrics
  completion        Generate shell autocompletion script (bash, zsh, fish)
  version           Print application version metadata and exit
  help              Display this help message

GENERATE FLAGS:
  --compare               Compare Month-over-Month with preceding billing period
  --mtd                   Query Month-to-Date spend from the 1st of the current month up to today
  --current-month         Alias for --mtd
  --monthly               Display multi-month historical breakdown with each month as a separate column (default: 12 months)
  --trend                 Alias for --monthly
  --format <string>       Output format: table (default), markdown, html, json, csv, prometheus
  --months <int>          Number of historical complete months back to query (default: 1)
  --start <YYYY-MM-DD>    Explicit billing start date (inclusive)
  --end <YYYY-MM-DD>      Explicit billing end date (inclusive)
  --group-by <dim>        Grouping dimension: SERVICE (default), LINKED_ACCOUNT, REGION, or TAG:<Key>
  --region <regions>      Filter costs by comma-separated AWS regions (e.g. us-east-1,eu-west-1)
  --filter-region <reg>   Alias for --region
  --assume-role-arn <arn> AWS IAM Role ARN to assume via STS for cross-account cost auditing
  --role-session-name <n> Optional session name when assuming an IAM role (default: aws-cost-report)
  --tag-key <key>         Query tag values for a specific tag key when running tags command
  --name <string>         Human-readable name or alias for the AWS account (e.g. 'Production Core')
  --account-name <name>   Alias for --name
  --filter <names>        Filter services by official name, alias, or substring (e.g. 'Amazon DynamoDB', 'S3', 'Lambda')
  --tag <Key=Value>       Filter costs by AWS Cost Allocation Tag (e.g. 'Environment=Production', 'Project=Billing')
  --tag-filter <Key=Val>  Alias for --tag. Supports multiple tags or comma-separated values (e.g. 'Env=Prod,Stage')
  --exclude-credits       Exclude credits and refunds from billing calculations
  --charge-type <types>   Filter costs by charge/record type (e.g. 'Usage,Tax', 'Credit')
  --threshold <float>     SRE budget alert threshold dollar amount. Exits with code 1 if exceeded
  --top <int>             Display top N highest cost drivers and aggregate remainder into "Other"
  --min-cost <float>      Minimum cost floor to display in itemized breakdown (default: 0.01)
  --output, -o <path|s3>  Write formatted report output to local file path or S3 URI (s3://bucket/key)
  --s3-bucket <name>      S3 bucket to automatically upload report
  --s3-key <key>          S3 object key (e.g. 'reports/cost-report.md')
  --json-summary <path>   Write machine-readable summary metrics to JSON file (supports local path or s3://)
  --slack-webhook <url>   Post formatted Block Kit report to incoming Slack webhook
  --teams-webhook <url>   Post formatted MessageCard report to incoming Microsoft Teams webhook
  --webhook-url <url>     Post formatted cost report JSON payload to generic HTTP/HTTPS webhook
  --currency <code>       Override currency code and symbol (e.g. USD, EUR, GBP). Auto-detected by default
  --no-cache              Disable local query response caching
  --cache-dir <dir>       Custom local cache directory path (defaults to user cache dir)
  --clear-cache           Purge local query response cache files and exit
  --no-color              Disable ANSI colors in terminal tables (honors NO_COLOR=1)
  --mock                  Run in offline mode with simulated AWS billing data
  --config, -c <path>     Path to YAML configuration file (default: config.yaml)

EXAMPLES:
  # 1. Print standard billing report for previous month
  aws-cost-report generate

  # 2. Compare Month-over-Month spend with colorized delta indicators
  aws-cost-report generate --compare

  # 3. Label report with a human-readable AWS account name
  aws-cost-report generate --compare --name="Production Core"

  # 4. Filter specific services using official AWS Cost Explorer names or substrings
  aws-cost-report generate --filter="Amazon DynamoDB,AWS Lambda"

  # 5. Filter using common service aliases (e.g. S3, EC2, KMS)
  aws-cost-report generate --filter="S3,EC2"

  # 6. Filter spend scoped to a specific Cost Allocation Tag (e.g. Environment=Production)
  aws-cost-report generate --tag="Environment=Production"

  # 7. Filter by multiple tag values or multiple tags with Month-over-Month comparison
  aws-cost-report generate --tag="Environment=Production,Staging" --tag="Project=FinOps" --compare

  # 8. Group tag-filtered costs by linked account or sub-tag
  aws-cost-report generate --tag="Environment=Production" --group-by=LINKED_ACCOUNT

  # 9. Identify costs of untagged resources for a specific tag key
  aws-cost-report generate --group-by="TAG:Environment" --filter="Untagged" --compare

  # 10. List all official AWS service names available in Cost Explorer
  aws-cost-report services

  # 11. Top 5 cost drivers with remainder aggregated into "Other Services"
  aws-cost-report generate --compare --top=5

  # 12. CI/CD budget guardrail in GitHub Actions with Step Summary
  aws-cost-report generate --compare --format=markdown --threshold=500.00

  # 13. Offline test mode with tag filtering simulation
  aws-cost-report generate --mock --tag="Environment=Production" --compare

  # 14. Start HTTP microservice daemon
  aws-cost-report server --port=8080

  # 15. View previous 12 months of bills in multi-column table format
  aws-cost-report generate --months=12 --monthly

  # 16. Query Month-to-Date spend up to today with Month-over-Month comparison
  aws-cost-report generate --mtd --compare

  # 17. Generate shell completion script for zsh or bash
  source <(aws-cost-report completion zsh)
  source <(aws-cost-report completion bash)

Use "aws-cost-report <command> --help" for more information about a command.
`
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, formatHelp())
}

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run executes the CLI command and returns the process exit code (0 for success, 1 for failure).
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	cmd := args[0]
	var cmdArgs []string

	switch cmd {
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	case "version", "-v", "--version":
		printVersion(stdout)
		return 0
	case "completion":
		return runCompletion(args[1:], stdout, stderr)
	case "generate":
		cmdArgs = args[1:]
	case "services", "list-services", "--services", "--list-services":
		cmd = "services"
		cmdArgs = args[1:]
	case "tags", "list-tags", "--tags", "--list-tags":
		cmd = "tags"
		cmdArgs = args[1:]
	case "anomalies", "list-anomalies", "--anomalies", "--list-anomalies":
		cmd = "anomalies"
		cmdArgs = args[1:]
	case "server":
		cmdArgs = args[1:]
	default:
		if strings.HasPrefix(cmd, "-") {
			cmdArgs = args
		} else {
			fmt.Fprintf(stderr, "Error: unknown command %q\nRun 'aws-cost-report help' for usage.\n", cmd)
			return 1
		}
	}

	cfg, err := config.LoadConfig(cmdArgs)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printHelp(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "Error parsing configuration: %v\n", err)
		return 1
	}

	if cfg.ShowVersion {
		printVersion(stdout)
		return 0
	}

	if cfg.ClearCache {
		dir := cfg.CacheDir
		if dir == "" {
			dir = awsclient.DefaultCacheDir()
		}
		count, err := awsclient.ClearCache(dir)
		if err != nil {
			fmt.Fprintf(stderr, "Error clearing cache directory %s: %v\n", dir, err)
			return 1
		}
		fmt.Fprintf(stdout, "Successfully purged %d cached report(s) from %s\n", count, dir)
		return 0
	}

	if cmd == "server" || cfg.RunServer {
		runServer(cfg)
		return 0
	}

	if cmd == "services" || cfg.ListServices {
		return runListServices(cfg, stdout, stderr)
	}

	if cmd == "tags" || cfg.ListTags {
		return runListTags(cfg, stdout, stderr)
	}

	if cmd == "anomalies" || cfg.ListAnomalies {
		return runListAnomalies(cfg, stdout, stderr)
	}

	if !cfg.NoCache && cfg.CacheDir == "" {
		cfg.CacheDir = awsclient.DefaultCacheDir()
	}

	return runCLI(cfg, stdout, stderr)
}

func runListServices(cfg *config.Config, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var client *awsclient.Client
	if cfg.MockMode {
		client = awsclient.NewClient(nil, nil)
	} else {
		var err error
		client, err = awsclient.NewRealClient(ctx, cfg.AssumeRoleARN, cfg.RoleSessionName)
		if err != nil {
			printCredentialHelp(stderr, err)
			return 1
		}
	}

	services, err := client.ListServices(ctx, cfg, time.Now())
	if err != nil {
		printCredentialHelp(stderr, err)
		return 1
	}

	if cfg.Reporting.Format == "json" {
		_ = json.NewEncoder(stdout).Encode(services)
		return 0
	}

	fmt.Fprintln(stdout, "Official AWS Cost Explorer Service Names:")
	for _, s := range services {
		fmt.Fprintf(stdout, "  • %s\n", s)
	}
	fmt.Fprintf(stdout, "\nTotal: %d services available.\nUse these exact names, aliases, or substrings with --filter (e.g. --filter=%q)\n", len(services), services[0])
	return 0
}

func runListTags(cfg *config.Config, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var client *awsclient.Client
	if cfg.MockMode {
		client = awsclient.NewClient(nil, nil)
	} else {
		var err error
		client, err = awsclient.NewRealClient(ctx, cfg.AssumeRoleARN, cfg.RoleSessionName)
		if err != nil {
			printCredentialHelp(stderr, err)
			return 1
		}
	}

	tags, err := client.ListTags(ctx, cfg, time.Now(), cfg.TagKeyToQuery)
	if err != nil {
		printCredentialHelp(stderr, err)
		return 1
	}

	if cfg.Reporting.Format == "json" {
		_ = json.NewEncoder(stdout).Encode(tags)
		return 0
	}

	if cfg.TagKeyToQuery != "" {
		fmt.Fprintf(stdout, "AWS Cost Allocation Tag Values for %q:\n", cfg.TagKeyToQuery)
	} else {
		fmt.Fprintln(stdout, "Official AWS Cost Allocation Tag Keys:")
	}
	for _, t := range tags {
		fmt.Fprintf(stdout, "  • %s\n", t)
	}
	return 0
}

func runListAnomalies(cfg *config.Config, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var client *awsclient.Client
	if cfg.MockMode {
		client = awsclient.NewClient(nil, nil)
	} else {
		var err error
		client, err = awsclient.NewRealClient(ctx, cfg.AssumeRoleARN, cfg.RoleSessionName)
		if err != nil {
			printCredentialHelp(stderr, err)
			return 1
		}
	}

	report, err := client.ListAnomalies(ctx, cfg, time.Now())
	if err != nil {
		printCredentialHelp(stderr, err)
		return 1
	}

	if cfg.Reporting.Format == "json" {
		_ = json.NewEncoder(stdout).Encode(report)
		return 0
	}

	sym := awsclient.CurrencySymbol(report.Currency)
	fmt.Fprintf(stdout, "AWS COST ANOMALY DETECTION REPORT\n")
	fmt.Fprintf(stdout, "Account ID: %s | Period: %s to %s | Total Impact: %s%.2f\n\n",
		report.AccountID, report.PeriodStart, report.PeriodEnd, sym, report.TotalImpact)

	if len(report.Anomalies) == 0 {
		fmt.Fprintln(stdout, "No cost anomalies detected during this billing period.")
		return 0
	}

	for _, a := range report.Anomalies {
		svc := a.Service
		if svc == "" {
			svc = a.Dimension
		}
		reg := a.Region
		if reg == "" {
			reg = "global"
		}
		fmt.Fprintf(stdout, "  • [%s] %s (%s): Impact %s%.2f (Actual: %s%.2f, Expected: %s%.2f)\n",
			a.ID, svc, reg, sym, a.ImpactAmount, sym, a.ActualSpend, sym, a.ExpectedSpend)
	}
	return 0
}

func runCLI(cfg *config.Config, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var client *awsclient.Client
	if cfg.MockMode {
		client = awsclient.NewClient(nil, nil)
	} else {
		var err error
		client, err = awsclient.NewRealClient(ctx, cfg.AssumeRoleARN, cfg.RoleSessionName)
		if err != nil {
			printCredentialHelp(stderr, err)
			return 1
		}
	}

	report, err := client.GenerateReport(ctx, cfg, time.Now())
	if err != nil {
		printCredentialHelp(stderr, err)
		return 1
	}

	var buf bytes.Buffer
	outWriter := io.MultiWriter(stdout, &buf)

	switch cfg.Reporting.Format {
	case "markdown", "md":
		if err := renderer.RenderMarkdown(outWriter, report, cfg.Reporting.WriteJobSummary, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(stderr, "Error rendering markdown: %v\n", err)
			return 1
		}
	case "html":
		if err := renderer.RenderHTML(outWriter, report, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(stderr, "Error rendering HTML: %v\n", err)
			return 1
		}
	case "json":
		if err := renderer.RenderJSON(outWriter, report); err != nil {
			fmt.Fprintf(stderr, "Error rendering JSON: %v\n", err)
			return 1
		}
	case "csv":
		if err := renderer.RenderCSV(outWriter, report); err != nil {
			fmt.Fprintf(stderr, "Error rendering CSV: %v\n", err)
			return 1
		}
	case "prometheus", "prom":
		if err := renderer.RenderPrometheus(outWriter, report, cfg.Budget.FailOnThreshold, version, commit, date); err != nil {
			fmt.Fprintf(stderr, "Error rendering prometheus: %v\n", err)
			return 1
		}
	default:
		if err := renderer.RenderTable(outWriter, report, cfg.NoColor); err != nil {
			fmt.Fprintf(stderr, "Error rendering table: %v\n", err)
			return 1
		}
	}

	if cfg.OutputFile != "" {
		if err := saveOutputFile(ctx, client, cfg.OutputFile, buf.Bytes(), cfg.MockMode); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to write output file %s: %v\n", cfg.OutputFile, err)
		}
	}

	if cfg.S3Bucket != "" && !strings.HasPrefix(cfg.OutputFile, "s3://") {
		s3Key := cfg.S3Key
		if s3Key == "" {
			ext := ".txt"
			switch cfg.Reporting.Format {
			case "markdown", "md":
				ext = ".md"
			case "html":
				ext = ".html"
			case "json":
				ext = ".json"
			case "csv":
				ext = ".csv"
			case "prometheus", "prom":
				ext = ".prom"
			}
			s3Key = fmt.Sprintf("cost-reports/aws-cost-report-%s%s", report.Metadata.PeriodStart, ext)
		}
		s3URI := fmt.Sprintf("s3://%s/%s", cfg.S3Bucket, s3Key)
		if err := saveOutputFile(ctx, client, s3URI, buf.Bytes(), cfg.MockMode); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to upload report to %s: %v\n", s3URI, err)
		}
	}

	thresholdBreached := cfg.Budget.FailOnThreshold > 0.00 && report.Totals.UnblendedCost > cfg.Budget.FailOnThreshold
	if cfg.Forecast && report.Forecast != nil && cfg.Budget.FailOnThreshold > 0.00 && report.Forecast.TotalAmount > cfg.Budget.FailOnThreshold {
		thresholdBreached = true
		report.Forecast.ThresholdBreached = true
	}

	if cfg.JSONSummaryFile != "" {
		forecastAmount := 0.0
		forecastBreached := false
		if report.Forecast != nil {
			forecastAmount = report.Forecast.TotalAmount
			forecastBreached = report.Forecast.ThresholdBreached
		}

		summary := struct {
			TotalCost         float64 `json:"total_cost"`
			PreviousCost      float64 `json:"previous_cost"`
			CostDelta         float64 `json:"cost_delta"`
			CostDeltaPct      float64 `json:"cost_delta_pct"`
			Currency          string  `json:"currency"`
			Threshold         float64 `json:"threshold"`
			ThresholdBreached bool    `json:"threshold_breached"`
			ForecastAmount    float64 `json:"forecast_amount,omitempty"`
			ForecastBreached  bool    `json:"forecast_breached,omitempty"`
		}{
			TotalCost:         report.Totals.UnblendedCost,
			PreviousCost:      report.Totals.PrevUnblendedCost,
			CostDelta:         report.Totals.MoMDeltaAmount,
			CostDeltaPct:      report.Totals.MoMDeltaPercent,
			Currency:          report.Metadata.Currency,
			Threshold:         cfg.Budget.FailOnThreshold,
			ThresholdBreached: thresholdBreached,
			ForecastAmount:    forecastAmount,
			ForecastBreached:  forecastBreached,
		}

		if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
			if err := saveOutputFile(ctx, client, cfg.JSONSummaryFile, append(data, '\n'), cfg.MockMode); err != nil {
				fmt.Fprintf(stderr, "Warning: failed to write JSON summary file %s: %v\n", cfg.JSONSummaryFile, err)
			}
		}
	}

	// Dispatch Slack notification if configured
	if cfg.SlackWebhook != "" {
		if err := webhook.SendSlackNotification(ctx, cfg.SlackWebhook, report, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to dispatch Slack notification: %v\n", err)
		}
	}

	// Dispatch Microsoft Teams notification if configured
	if cfg.TeamsWebhook != "" {
		if err := webhook.SendTeamsNotification(ctx, cfg.TeamsWebhook, report, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to dispatch Microsoft Teams notification: %v\n", err)
		}
	}

	// Dispatch generic webhook notification if configured
	if cfg.WebhookURL != "" {
		if err := webhook.SendGenericWebhook(ctx, cfg.WebhookURL, report, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to dispatch webhook: %v\n", err)
		}
	}

	// Budget guardrail check
	if thresholdBreached {
		sym := report.Metadata.CurrencySymbol()
		if report.Totals.UnblendedCost > cfg.Budget.FailOnThreshold {
			fmt.Fprintf(stderr, "\n[BUDGET ALERT] Total cost (%s%.2f %s) exceeds configured threshold (%s%.2f %s)!\n",
				sym, report.Totals.UnblendedCost, report.Metadata.Currency, sym, cfg.Budget.FailOnThreshold, report.Metadata.Currency)
		} else if report.Forecast != nil && report.Forecast.ThresholdBreached {
			fmt.Fprintf(stderr, "\n[BUDGET ALERT] Projected month-end spend (%s%.2f %s) is on track to breach configured threshold (%s%.2f %s)!\n",
				sym, report.Forecast.TotalAmount, report.Metadata.Currency, sym, cfg.Budget.FailOnThreshold, report.Metadata.Currency)
		}
		return 1
	}

	return 0
}

func saveOutputFile(ctx context.Context, client *awsclient.Client, pathOrURI string, data []byte, mock bool) error {
	if strings.HasPrefix(pathOrURI, "s3://") {
		bucket, key, err := awsclient.ParseS3URI(pathOrURI)
		if err != nil {
			return err
		}
		if mock {
			fmt.Printf("[MOCK] Uploaded %d bytes to s3://%s/%s\n", len(data), bucket, key)
			return nil
		}
		if client == nil {
			var err error
			client, err = awsclient.NewRealClient(ctx)
			if err != nil {
				return fmt.Errorf("failed to initialize AWS client for S3 upload: %w", err)
			}
		}
		return client.UploadToS3(ctx, bucket, key, data)
	}

	dir := filepath.Dir(pathOrURI)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return os.WriteFile(pathOrURI, data, 0644)
}

type reportCache struct {
	mu         sync.RWMutex
	entries    map[string]cacheEntry
	ttl        time.Duration
	maxEntries int
}

type cacheEntry struct {
	report    *awsclient.CostReport
	expiresAt time.Time
}

func newReportCache(ttl time.Duration, maxEntries ...int) *reportCache {
	limit := 500
	if len(maxEntries) > 0 && maxEntries[0] > 0 {
		limit = maxEntries[0]
	}
	return &reportCache{
		entries:    make(map[string]cacheEntry),
		ttl:        ttl,
		maxEntries: limit,
	}
}

func (c *reportCache) Get(key string) (*awsclient.CostReport, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.report, true
}

func (c *reportCache) Set(key string, report *awsclient.CostReport) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	// Prune expired entries if map reaches capacity
	if len(c.entries) >= c.maxEntries {
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
			}
		}
		// If still at capacity, evict an entry to bound memory
		if len(c.entries) >= c.maxEntries {
			for k := range c.entries {
				delete(c.entries, k)
				break
			}
		}
	}

	c.entries[key] = cacheEntry{
		report:    report,
		expiresAt: now.Add(c.ttl),
	}
}

func runServer(cfg *config.Config, stopCh ...chan os.Signal) {
	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	host := cfg.Server.Host
	addr := fmt.Sprintf("%s:%d", host, port)

	cache := newReportCache(15 * time.Minute)
	mux := buildServerMux(cfg, cache)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	var stop chan os.Signal
	if len(stopCh) > 0 && stopCh[0] != nil {
		stop = stopCh[0]
	} else {
		stop = make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	}

	go func() {
		log.Printf("Starting aws-cost-report service on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server stopped: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown error: %v", err)
	}
}

func buildServerMux(cfg *config.Config, cache *reportCache) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if cfg.MockMode {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ready","mode":"mock"}`))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		client, err := awsclient.NewRealClient(ctx)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "error": err.Error()})
			return
		}

		accountID, err := client.VerifySTS(ctx)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "error": err.Error()})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready", "account": accountID})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Minute)
		defer cancel()

		reqCfg := *cfg
		q := r.URL.Query()

		if cmp := q.Get("compare"); cmp != "" {
			reqCfg.CompareMoM = (cmp == "true" || cmp == "1")
		} else {
			reqCfg.CompareMoM = true
		}
		if gb := q.Get("group-by"); gb != "" {
			if strings.HasPrefix(strings.ToUpper(gb), "TAG:") {
				reqCfg.Grouping.Type = "TAG"
				reqCfg.Grouping.Key = gb[4:]
			} else {
				reqCfg.Grouping.Type = "DIMENSION"
				reqCfg.Grouping.Key = strings.ToUpper(gb)
			}
		}
		if n := q.Get("name"); n != "" {
			reqCfg.AccountName = n
		}
		if mtd := q.Get("mtd"); mtd == "true" || mtd == "1" {
			reqCfg.MTD = true
		}

		cacheKey := fmt.Sprintf("metrics:%v:%s:%s:%d:%s:%v:%s:%d:%.2f:%v:%s:%s:%v",
			reqCfg.CompareMoM, reqCfg.StartDate, reqCfg.EndDate,
			reqCfg.MonthsBack, reqCfg.Grouping.Key, reqCfg.Filters,
			reqCfg.TagFilters.Format(),
			reqCfg.Defaults.Top, reqCfg.Defaults.MinDisplayCost,
			reqCfg.Monthly, reqCfg.AccountName, reqCfg.Defaults.Currency, reqCfg.MTD)

		report, ok := cache.Get(cacheKey)
		if !ok {
			var client *awsclient.Client
			if reqCfg.MockMode {
				client = awsclient.NewClient(nil, nil)
			} else {
				var err error
				client, err = awsclient.NewRealClient(ctx)
				if err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprintf(w, "# Error connecting to AWS: %v\n", err)
					return
				}
			}

			var err error
			report, err = client.GenerateReport(ctx, &reqCfg, time.Now())
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "# Error generating report: %v\n", err)
				return
			}
			cache.Set(cacheKey, report)
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = renderer.RenderPrometheus(w, report, reqCfg.Budget.FailOnThreshold, version, commit, date)
	})

	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Minute)
		defer cancel()

		reqCfg := *cfg
		q := r.URL.Query()

		if cmp := q.Get("compare"); cmp != "" {
			reqCfg.CompareMoM = (cmp == "true" || cmp == "1")
		}
		if m := q.Get("months"); m != "" {
			if monthsVal, err := strconv.Atoi(m); err == nil && monthsVal > 0 {
				reqCfg.MonthsBack = monthsVal
				reqCfg.Defaults.MonthsBack = monthsVal
			}
		}
		if mon := q.Get("monthly"); mon == "true" || mon == "1" {
			reqCfg.Monthly = true
			if reqCfg.MonthsBack <= 1 {
				reqCfg.MonthsBack = 12
				reqCfg.Defaults.MonthsBack = 12
			}
		} else if tr := q.Get("trend"); tr == "true" || tr == "1" {
			reqCfg.Monthly = true
			if reqCfg.MonthsBack <= 1 {
				reqCfg.MonthsBack = 12
				reqCfg.Defaults.MonthsBack = 12
			}
		}
		if mtd := q.Get("mtd"); mtd == "true" || mtd == "1" {
			reqCfg.MTD = true
		} else if cm := q.Get("current-month"); cm == "true" || cm == "1" {
			reqCfg.MTD = true
		}
		if s := q.Get("start"); s != "" {
			reqCfg.StartDate = s
		}
		if e := q.Get("end"); e != "" {
			reqCfg.EndDate = e
		}
		if gb := q.Get("group-by"); gb != "" {
			if strings.HasPrefix(strings.ToUpper(gb), "TAG:") {
				reqCfg.Grouping.Type = "TAG"
				reqCfg.Grouping.Key = gb[4:]
			} else {
				reqCfg.Grouping.Type = "DIMENSION"
				reqCfg.Grouping.Key = strings.ToUpper(gb)
			}
		} else if gb := q.Get("groupBy"); gb != "" {
			if strings.HasPrefix(strings.ToUpper(gb), "TAG:") {
				reqCfg.Grouping.Type = "TAG"
				reqCfg.Grouping.Key = gb[4:]
			} else {
				reqCfg.Grouping.Type = "DIMENSION"
				reqCfg.Grouping.Key = strings.ToUpper(gb)
			}
		}
		if n := q.Get("name"); n != "" {
			reqCfg.AccountName = n
		} else if an := q.Get("account-name"); an != "" {
			reqCfg.AccountName = an
		}
		if flt := q.Get("filter"); flt != "" {
			parts := strings.Split(flt, ",")
			var cleaned []string
			for _, p := range parts {
				if t := strings.TrimSpace(p); t != "" {
					cleaned = append(cleaned, t)
				}
			}
			reqCfg.Filters = cleaned
		}
		if t := q.Get("top"); t != "" {
			if topVal, err := strconv.Atoi(t); err == nil && topVal > 0 {
				reqCfg.Defaults.Top = topVal
			}
		}
		if mc := q.Get("min-cost"); mc != "" {
			if minVal, err := strconv.ParseFloat(mc, 64); err == nil && minVal >= 0 {
				reqCfg.Defaults.MinDisplayCost = minVal
			}
		}

		if reqCfg.TagFilters == nil {
			reqCfg.TagFilters = make(config.TagFilterMap)
		} else {
			clonedTags := make(config.TagFilterMap, len(reqCfg.TagFilters))
			for k, v := range reqCfg.TagFilters {
				vals := make([]string, len(v))
				copy(vals, v)
				clonedTags[k] = vals
			}
			reqCfg.TagFilters = clonedTags
		}
		for _, tagVal := range q["tag"] {
			config.ParseTagFilterString(tagVal, reqCfg.TagFilters)
		}
		for _, tagVal := range q["tag-filter"] {
			config.ParseTagFilterString(tagVal, reqCfg.TagFilters)
		}

		cacheKey := fmt.Sprintf("%v:%s:%s:%d:%s:%v:%s:%d:%.2f:%v:%s:%s:%v",
			reqCfg.CompareMoM, reqCfg.StartDate, reqCfg.EndDate,
			reqCfg.MonthsBack, reqCfg.Grouping.Key, reqCfg.Filters,
			reqCfg.TagFilters.Format(),
			reqCfg.Defaults.Top, reqCfg.Defaults.MinDisplayCost,
			reqCfg.Monthly, reqCfg.AccountName, reqCfg.Defaults.Currency, reqCfg.MTD)

		report, ok := cache.Get(cacheKey)
		if !ok {
			var client *awsclient.Client
			if reqCfg.MockMode {
				client = awsclient.NewClient(nil, nil)
			} else {
				var err error
				client, err = awsclient.NewRealClient(ctx)
				if err != nil {
					http.Error(w, fmt.Sprintf("AWS client error: %v", err), http.StatusInternalServerError)
					return
				}
			}

			var err error
			report, err = client.GenerateReport(ctx, &reqCfg, time.Now())
			if err != nil {
				http.Error(w, fmt.Sprintf("Report generation error: %v", err), http.StatusInternalServerError)
				return
			}
			cache.Set(cacheKey, report)
		}

		formatParam := q.Get("format")
		if formatParam == "" {
			formatParam = reqCfg.Reporting.Format
		}

		switch formatParam {
		case "markdown", "md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_ = renderer.RenderMarkdown(w, report, false, 0.00)
		case "html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = renderer.RenderHTML(w, report, reqCfg.Budget.FailOnThreshold)
		case "table":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_ = renderer.RenderTable(w, report, true)
		case "csv":
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			_ = renderer.RenderCSV(w, report)
		case "prometheus", "prom":
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_ = renderer.RenderPrometheus(w, report, reqCfg.Budget.FailOnThreshold, version, commit, date)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = renderer.RenderJSON(w, report)
		}
	})

	return mux
}

func printCredentialHelp(w io.Writer, err error) {
	fmt.Fprintf(w, "Error: %v\n\n", err)
	fmt.Fprintln(w, "AWS Credential Resolution Order:")
	fmt.Fprintln(w, "  1. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN)")
	fmt.Fprintln(w, "  2. Shared credentials/config file (~/.aws/credentials, ~/.aws/config, AWS_PROFILE)")
	fmt.Fprintln(w, "  3. IAM Identity Center (AWS SSO token cache)")
	fmt.Fprintln(w, "  4. Web Identity / OIDC token (AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE in CI)")
	fmt.Fprintln(w, "  5. Amazon ECS / EC2 Instance Metadata Service (IMDSv2)")
	fmt.Fprintln(w, "\nTip: Run with --mock to test output rendering and budget alerts with simulated billing data.")
}
