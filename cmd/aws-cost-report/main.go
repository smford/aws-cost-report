package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smford/aws-cost-report/internal/awsclient"
	"github.com/smford/aws-cost-report/internal/config"
	"github.com/smford/aws-cost-report/internal/renderer"
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

func main() {
	cfg, err := config.LoadConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing configuration: %v\n", err)
		os.Exit(1)
	}

	if cfg.ShowVersion {
		printVersion()
		os.Exit(0)
	}

	if cfg.RunServer {
		runServer(cfg)
		return
	}

	runCLI(cfg)
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

	switch cfg.Reporting.Format {
	case "markdown", "md":
		if err := renderer.RenderMarkdown(os.Stdout, report, cfg.Reporting.WriteJobSummary, cfg.Budget.FailOnThreshold); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering markdown: %v\n", err)
			os.Exit(1)
		}
	case "json":
		if err := renderer.RenderJSON(os.Stdout, report); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering JSON: %v\n", err)
			os.Exit(1)
		}
	case "csv":
		if err := renderer.RenderCSV(os.Stdout, report); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering CSV: %v\n", err)
			os.Exit(1)
		}
	default:
		if err := renderer.RenderTable(os.Stdout, report, cfg.NoColor); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering table: %v\n", err)
			os.Exit(1)
		}
	}

	// Budget guardrail check
	if cfg.Budget.FailOnThreshold > 0.00 {
		if report.Totals.UnblendedCost > cfg.Budget.FailOnThreshold {
			fmt.Fprintf(os.Stderr, "\n[BUDGET ALERT] Total cost (%.2f %s) exceeds configured threshold (%.2f %s)!\n",
				report.Totals.UnblendedCost, cfg.Defaults.Currency, cfg.Budget.FailOnThreshold, cfg.Defaults.Currency)
			os.Exit(1)
		}
	}
}

func runServer(cfg *config.Config) {
	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	host := cfg.Server.Host
	addr := fmt.Sprintf("%s:%d", host, port)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
	})

	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Minute)
		defer cancel()

		var client *awsclient.Client
		if cfg.MockMode {
			client = awsclient.NewClient(nil, nil)
		} else {
			var err error
			client, err = awsclient.NewRealClient(ctx)
			if err != nil {
				http.Error(w, fmt.Sprintf("AWS client error: %v", err), http.StatusInternalServerError)
				return
			}
		}

		report, err := client.GenerateReport(ctx, cfg, time.Now())
		if err != nil {
			http.Error(w, fmt.Sprintf("Report generation error: %v", err), http.StatusInternalServerError)
			return
		}

		formatParam := r.URL.Query().Get("format")
		if formatParam == "" {
			formatParam = cfg.Reporting.Format
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
