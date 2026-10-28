package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		configPath  string
		showVersion bool
		listenAddr  string
	)

	flag.StringVar(&configPath, "config", "/etc/aws-cost-report/aws-cost-report.yaml", "Path to configuration file")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version information and exit (shorthand)")
	flag.StringVar(&listenAddr, "addr", ":8080", "Server listen address")
	flag.Parse()

	if showVersion {
		fmt.Printf("aws-cost-report version %s (%s, %s)\n", version, commit, date)
		os.Exit(0)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	log.Printf("Starting aws-cost-report %s on %s (config: %s)", version, listenAddr, configPath)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
