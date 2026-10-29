package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration.
type Config struct {
	Version      string            `yaml:"version"`
	Defaults     DefaultsConfig    `yaml:"defaults"`
	Grouping     GroupingConfig    `yaml:"grouping"`
	Budget       BudgetConfig      `yaml:"budget"`
	Reporting    ReportingConfig   `yaml:"reporting"`
	Server       ServerConfig      `yaml:"server"`
	Accounts     map[string]string `yaml:"accounts"`
	SlackWebhook string            `yaml:"slack_webhook"`

	// Runtime options resolved from CLI flags or env
	ConfigFile      string   `yaml:"-"`
	ShowVersion     bool     `yaml:"-"`
	RunServer       bool     `yaml:"-"`
	MonthsBack      int      `yaml:"-"`
	StartDate       string   `yaml:"-"`
	EndDate         string   `yaml:"-"`
	MetricsList     []string `yaml:"-"`
	CompareMoM      bool     `yaml:"-"`
	MockMode        bool     `yaml:"-"`
	Filters         []string `yaml:"filters"`
	NoColor         bool     `yaml:"no_color"`
	OutputFile      string   `yaml:"output_file"`
	JSONSummaryFile string   `yaml:"json_summary_file"`
}

// DefaultsConfig represents default billing query parameters.
type DefaultsConfig struct {
	Currency       string   `yaml:"currency"`
	MonthsBack     int      `yaml:"months_back"`
	MinDisplayCost float64  `yaml:"min_display_cost"`
	Top            int      `yaml:"top"`
	Metrics        []string `yaml:"metrics"`
}

// GroupingConfig defines how Cost Explorer aggregates line items.
type GroupingConfig struct {
	Type string `yaml:"type"` // DIMENSION or TAG
	Key  string `yaml:"key"`  // SERVICE, LINKED_ACCOUNT, or Tag Key
}

// BudgetConfig defines threshold guardrail options.
type BudgetConfig struct {
	FailOnThreshold float64 `yaml:"fail_on_threshold"` // 0.00 disables exit-1 behavior
}

// ReportingConfig defines output formats and GitHub Actions integration.
type ReportingConfig struct {
	Format          string `yaml:"format"` // table, markdown, json, csv
	WriteJobSummary bool   `yaml:"write_job_summary"`
}

// ServerConfig defines optional HTTP daemon options for health checks and reports.
type ServerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

// DefaultConfig returns a Config populated with sensible production defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: "1",
		Defaults: DefaultsConfig{
			Currency:       "USD",
			MonthsBack:     1,
			MinDisplayCost: 0.01,
			Metrics: []string{
				"UnblendedCost",
				"BlendedCost",
				"AmortizedCost",
			},
		},
		Grouping: GroupingConfig{
			Type: "DIMENSION",
			Key:  "SERVICE",
		},
		Budget: BudgetConfig{
			FailOnThreshold: 0.00,
		},
		Reporting: ReportingConfig{
			Format:          "table",
			WriteJobSummary: true,
		},
	}
}

// LoadConfig parses CLI arguments, loads YAML configuration if provided,
// and applies CLI flag overrides with highest precedence.
func LoadConfig(args []string) (*Config, error) {
	cfg := DefaultConfig()

	fs := flag.NewFlagSet("aws-cost-report", flag.ContinueOnError)

	var (
		configPath   = fs.String("config", "", "Path to YAML configuration file")
		configPathSh = fs.String("c", "", "Path to YAML configuration file (shorthand)")
		showVersion  = fs.Bool("version", false, "Print version information and exit")
		showVerSh    = fs.Bool("v", false, "Print version information and exit (shorthand)")
		months       = fs.Int("months", 0, "Query historical full months back (default: 1)")
		start        = fs.String("start", "", "Explicit start date (YYYY-MM-01)")
		end          = fs.String("end", "", "Explicit end date (YYYY-MM-01 or YYYY-MM-DD)")
		metricsFlag  = fs.String("metrics", "", "Comma-separated metrics (UnblendedCost,BlendedCost,AmortizedCost or all)")
		groupBy      = fs.String("group-by", "", "Grouping dimension (SERVICE, LINKED_ACCOUNT) or TAG:<Key>")
		minCost      = fs.Float64("min-cost", -1.0, "Minimum display cost threshold (e.g. 0.01)")
		compareMoM   = fs.Bool("compare", false, "Compare Month-over-Month with preceding period")
		failThresh   = fs.Float64("fail-on-threshold", -1.0, "Terminate with exit code 1 if total cost exceeds this threshold")
		threshAlias  = fs.Float64("threshold", -1.0, "Alias for --fail-on-threshold")
		format       = fs.String("format", "", "Output format: table, markdown, json, csv")
		writeSummary = fs.Bool("write-summary", true, "Write markdown report to $GITHUB_STEP_SUMMARY when set")
		mockMode     = fs.Bool("mock", false, "Run in offline mock mode with simulated billing data")
		serverMode   = fs.Bool("server", false, "Run as an HTTP service daemon exposing /healthz and /report")
		filterFlag   = fs.String("filter", "", "Filter services by comma-separated substring (e.g. DynamoDB,Lambda)")
		serviceFlag  = fs.String("service", "", "Alias for --filter")
		noColorFlag  = fs.Bool("no-color", false, "Disable ANSI color output in terminal tables")
		outputFlag   = fs.String("output", "", "Optional file path to write report output")
		outputSh     = fs.String("o", "", "Optional file path to write report output (shorthand)")
		jsonSumFlag  = fs.String("json-summary", "", "Optional file path to write machine-readable summary JSON")
		topFlag      = fs.Int("top", 0, "Show top N highest cost items and aggregate remainder into Other")
		slackFlag    = fs.String("slack-webhook", "", "Incoming Slack webhook URL to post formatted cost report")
	)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg.ShowVersion = *showVersion || *showVerSh
	if cfg.ShowVersion {
		return cfg, nil
	}

	selectedConfig := *configPath
	if selectedConfig == "" {
		selectedConfig = *configPathSh
	}

	if selectedConfig == "" {
		// Look for default config.yaml in working directory
		if _, err := os.Stat("config.yaml"); err == nil {
			selectedConfig = "config.yaml"
		}
	}

	if selectedConfig != "" {
		cfg.ConfigFile = selectedConfig
		data, err := os.ReadFile(selectedConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %q: %w", selectedConfig, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config YAML: %w", err)
		}
	}

	// Apply CLI flag overrides (highest precedence)
	if *months > 0 {
		cfg.Defaults.MonthsBack = *months
	}
	cfg.MonthsBack = cfg.Defaults.MonthsBack

	if *start != "" {
		cfg.StartDate = *start
	}
	if *end != "" {
		cfg.EndDate = *end
	}

	if *metricsFlag != "" {
		if strings.EqualFold(*metricsFlag, "all") {
			cfg.Defaults.Metrics = []string{"UnblendedCost", "BlendedCost", "AmortizedCost"}
		} else {
			parts := strings.Split(*metricsFlag, ",")
			var cleaned []string
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					cleaned = append(cleaned, trimmed)
				}
			}
			if len(cleaned) > 0 {
				cfg.Defaults.Metrics = cleaned
			}
		}
	}
	cfg.MetricsList = cfg.Defaults.Metrics

	if *groupBy != "" {
		if strings.HasPrefix(strings.ToUpper(*groupBy), "TAG:") {
			cfg.Grouping.Type = "TAG"
			cfg.Grouping.Key = (*groupBy)[4:]
		} else {
			cfg.Grouping.Type = "DIMENSION"
			cfg.Grouping.Key = strings.ToUpper(*groupBy)
		}
	}

	if *minCost >= 0.0 {
		cfg.Defaults.MinDisplayCost = *minCost
	}

	if *topFlag > 0 {
		cfg.Defaults.Top = *topFlag
	}

	if *slackFlag != "" {
		cfg.SlackWebhook = *slackFlag
	}

	if *compareMoM {
		cfg.CompareMoM = true
	}

	if *threshAlias >= 0.0 && *failThresh < 0.0 {
		*failThresh = *threshAlias
	}

	if *failThresh >= 0.0 {
		cfg.Budget.FailOnThreshold = *failThresh
	}

	if *outputFlag != "" {
		cfg.OutputFile = *outputFlag
	} else if *outputSh != "" {
		cfg.OutputFile = *outputSh
	}

	if *jsonSumFlag != "" {
		cfg.JSONSummaryFile = *jsonSumFlag
	}

	if *format != "" {
		cfg.Reporting.Format = strings.ToLower(*format)
	}

	cfg.Reporting.WriteJobSummary = *writeSummary
	cfg.MockMode = *mockMode
	if *serverMode || cfg.Server.Enabled || cfg.Server.Port > 0 {
		cfg.RunServer = true
	}

	rawFilter := *filterFlag
	if rawFilter == "" {
		rawFilter = *serviceFlag
	}
	if rawFilter != "" {
		parts := strings.Split(rawFilter, ",")
		var cleaned []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) > 0 {
			cfg.Filters = cleaned
		}
	}

	if *noColorFlag || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		cfg.NoColor = true
	}

	return cfg, nil
}
