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

func printVersion() {
	fmt.Print(formatVersion())
}

func formatHelp() string {
	return `aws-cost-report - AWS Cost Explorer reporting, Month-over-Month deltas, and CI/CD budget guardrails

USAGE:
  aws-cost-report <command> [flags]

COMMANDS:
  generate          Generate monthly billing and usage cost report (default)
  services          List official AWS Cost Explorer service names available in your account
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
  --format <string>       Output format: table (default), markdown, json, csv, prometheus
  --months <int>          Number of historical complete months back to query (default: 1)
  --start <YYYY-MM-DD>    Explicit billing start date (inclusive)
  --end <YYYY-MM-DD>      Explicit billing end date (inclusive)
  --group-by <dim>        Grouping dimension: SERVICE (default), LINKED_ACCOUNT, or TAG:<Key>
  --name <string>         Human-readable name or alias for the AWS account (e.g. 'Production Core')
  --account-name <name>   Alias for --name
  --filter <names>        Filter services by official name, alias, or substring (e.g. 'Amazon DynamoDB', 'S3', 'Lambda')
  --tag <Key=Value>       Filter costs by AWS Cost Allocation Tag (e.g. 'Environment=Production', 'Project=Billing')
  --tag-filter <Key=Val>  Alias for --tag. Supports multiple tags or comma-separated values (e.g. 'Env=Prod,Stage')
  --threshold <float>     SRE budget alert threshold dollar amount. Exits with code 1 if exceeded
  --top <int>             Display top N highest cost drivers and aggregate remainder into "Other"
  --min-cost <float>      Minimum cost floor to display in itemized breakdown (default: 0.01)
  --output, -o <path>     Write formatted report output to specified file
  --json-summary <path>   Write machine-readable summary metrics to JSON file
  --slack-webhook <url>   Post formatted Block Kit report to incoming Slack webhook
  --teams-webhook <url>   Post formatted MessageCard report to incoming Microsoft Teams webhook
  --currency <code>       Override currency code and symbol (e.g. USD, EUR, GBP). Auto-detected by default
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

func printHelp() {
	fmt.Print(formatHelp())
}

func main() {
	args := os.Args[1:]

	// If no arguments are passed, immediately display help and exit cleanly (no AWS auth pause!)
	if len(args) == 0 {
		printHelp()
		os.Exit(0)
	}

	cmd := args[0]
	var cmdArgs []string

	switch cmd {
	case "help", "-h", "--help":
		printHelp()
		os.Exit(0)
	case "version", "-v", "--version":
		printVersion()
		os.Exit(0)
	case "completion":
		runCompletion(args[1:])
		os.Exit(0)
	case "generate":
		cmdArgs = args[1:]
	case "services", "list-services", "--services", "--list-services":
		cmd = "services"
		cmdArgs = args[1:]
	case "server":
		cmdArgs = args[1:]
	default:
		if strings.HasPrefix(cmd, "-") {
			cmdArgs = args
		} else {
			fmt.Fprintf(os.Stderr, "Error: unknown command %q\nRun 'aws-cost-report help' for usage.\n", cmd)
			os.Exit(1)
		}
	}

	cfg, err := config.LoadConfig(cmdArgs)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printHelp()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error parsing configuration: %v\n", err)
		os.Exit(1)
	}

	if cfg.ShowVersion {
		printVersion()
		os.Exit(0)
	}

	if cmd == "server" || cfg.RunServer {
		runServer(cfg)
		return
	}

	if cmd == "services" || cfg.ListServices {
		runListServices(cfg)
		return
	}

	runCLI(cfg)
}

func runListServices(cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	var client *awsclient.Client
	if cfg.MockMode {
		client = awsclient.NewClient(nil, nil)
	} else {
		var err error
		client, err = awsclient.NewRealClient(ctx)
		if err != nil {
			printCredentialHelp(err)
			os.Exit(1)
		}
	}

	services, err := client.ListServices(ctx, cfg, time.Now())
	if err != nil {
		printCredentialHelp(err)
		os.Exit(1)
	}

	if cfg.Reporting.Format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(services)
		return
	}

	fmt.Println("Official AWS Cost Explorer Service Names:")
	for _, s := range services {
		fmt.Printf("  • %s\n", s)
	}
	fmt.Printf("\nTotal: %d services available.\nUse these exact names, aliases, or substrings with --filter (e.g. --filter=%q)\n", len(services), services[0])
}

func runCLI(cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var client *awsclient.Client
	if cfg.MockMode {
		client = awsclient.NewClient(nil, nil)
	} else {
		var err error
		client, err = awsclient.NewRealClient(ctx)
		if err != nil {
			printCredentialHelp(err)
			os.Exit(1)
		}
	}

	report, err := client.GenerateReport(ctx, cfg, time.Now())
	if err != nil {
		printCredentialHelp(err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	outWriter := io.MultiWriter(os.Stdout, &buf)

	switch cfg.Reporting.Format {
	case "markdown", "md":
		if err := renderer.RenderMarkdown(outWriter, report, cfg.Reporting.WriteJobSummary, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering markdown: %v\n", err)
			os.Exit(1)
		}
	case "json":
		if err := renderer.RenderJSON(outWriter, report); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering JSON: %v\n", err)
			os.Exit(1)
		}
	case "csv":
		if err := renderer.RenderCSV(outWriter, report); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering CSV: %v\n", err)
			os.Exit(1)
		}
	case "prometheus", "prom":
		if err := renderer.RenderPrometheus(outWriter, report, cfg.Budget.FailOnThreshold, version, commit, date); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering prometheus: %v\n", err)
			os.Exit(1)
		}
	default:
		if err := renderer.RenderTable(outWriter, report, cfg.NoColor); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering table: %v\n", err)
			os.Exit(1)
		}
	}

	if cfg.OutputFile != "" {
		if err := os.WriteFile(cfg.OutputFile, buf.Bytes(), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write output file %s: %v\n", cfg.OutputFile, err)
		}
	}

	thresholdBreached := cfg.Budget.FailOnThreshold > 0.00 && report.Totals.UnblendedCost > cfg.Budget.FailOnThreshold

	if cfg.JSONSummaryFile != "" {
		summary := struct {
			TotalCost         float64 `json:"total_cost"`
			PreviousCost      float64 `json:"previous_cost"`
			CostDelta         float64 `json:"cost_delta"`
			CostDeltaPct      float64 `json:"cost_delta_pct"`
			Currency          string  `json:"currency"`
			Threshold         float64 `json:"threshold"`
			ThresholdBreached bool    `json:"threshold_breached"`
		}{
			TotalCost:         report.Totals.UnblendedCost,
			PreviousCost:      report.Totals.PrevUnblendedCost,
			CostDelta:         report.Totals.MoMDeltaAmount,
			CostDeltaPct:      report.Totals.MoMDeltaPercent,
			Currency:          report.Metadata.Currency,
			Threshold:         cfg.Budget.FailOnThreshold,
			ThresholdBreached: thresholdBreached,
		}

		if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
			_ = os.WriteFile(cfg.JSONSummaryFile, append(data, '\n'), 0644)
		}
	}

	// Dispatch Slack notification if configured
	if cfg.SlackWebhook != "" {
		if err := webhook.SendSlackNotification(ctx, cfg.SlackWebhook, report, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to dispatch Slack notification: %v\n", err)
		}
	}

	// Dispatch Microsoft Teams notification if configured
	if cfg.TeamsWebhook != "" {
		if err := webhook.SendTeamsNotification(ctx, cfg.TeamsWebhook, report, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to dispatch Microsoft Teams notification: %v\n", err)
		}
	}

	// Budget guardrail check
	if thresholdBreached {
		sym := report.Metadata.CurrencySymbol()
		fmt.Fprintf(os.Stderr, "\n[BUDGET ALERT] Total cost (%s%.2f %s) exceeds configured threshold (%s%.2f %s)!\n",
			sym, report.Totals.UnblendedCost, report.Metadata.Currency, sym, cfg.Budget.FailOnThreshold, report.Metadata.Currency)
		os.Exit(1)
	}
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

func runServer(cfg *config.Config) {
	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	host := cfg.Server.Host
	addr := fmt.Sprintf("%s:%d", host, port)

	cache := newReportCache(15 * time.Minute)

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

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting aws-cost-report service on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
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

func printCredentialHelp(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
	fmt.Fprintln(os.Stderr, "AWS Credential Resolution Order:")
	fmt.Fprintln(os.Stderr, "  1. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN)")
	fmt.Fprintln(os.Stderr, "  2. Shared credentials/config file (~/.aws/credentials, ~/.aws/config, AWS_PROFILE)")
	fmt.Fprintln(os.Stderr, "  3. IAM Identity Center (AWS SSO token cache)")
	fmt.Fprintln(os.Stderr, "  4. Web Identity / OIDC token (AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE in CI)")
	fmt.Fprintln(os.Stderr, "  5. Amazon ECS / EC2 Instance Metadata Service (IMDSv2)")
	fmt.Fprintln(os.Stderr, "\nTip: Run with --mock to test output rendering and budget alerts with simulated billing data.")
}
