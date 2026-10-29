package config

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TagFilterMap maps cost allocation tag keys to one or more allowed tag values.
type TagFilterMap map[string][]string

// Format returns a human-readable representation of active tag filters.
func (m TagFilterMap) Format() string {
	if len(m) == 0 {
		return ""
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := m[k]
		if len(vals) == 0 {
			parts = append(parts, k)
		} else if len(vals) == 1 {
			parts = append(parts, fmt.Sprintf("%s=%s", k, vals[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%s in (%s)", k, strings.Join(vals, ", ")))
		}
	}
	return strings.Join(parts, "; ")
}

// UnmarshalYAML handles parsing tag filters from YAML mappings, sequences, or scalar strings.
func (m *TagFilterMap) UnmarshalYAML(node *yaml.Node) error {
	if *m == nil {
		*m = make(TagFilterMap)
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			valNode := node.Content[i+1]
			switch valNode.Kind {
			case yaml.ScalarNode:
				if valNode.Value != "" {
					(*m)[key] = []string{valNode.Value}
				}
			case yaml.SequenceNode:
				var vals []string
				for _, item := range valNode.Content {
					if item.Value != "" {
						vals = append(vals, item.Value)
					}
				}
				(*m)[key] = vals
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			ParseTagFilterString(item.Value, *m)
		}
	case yaml.ScalarNode:
		ParseTagFilterString(node.Value, *m)
	}
	return nil
}

// ParseTagFilterString parses tag filter representations such as:
// "Environment=Production", "Env=Prod,Stage", "Project=FinOps; Team=Core", or "Tier:Backend"
func ParseTagFilterString(raw string, target TagFilterMap) {
	if target == nil {
		return
	}
	pairs := strings.Split(raw, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		var key, valPart string
		if idx := strings.Index(pair, "="); idx != -1 {
			key = strings.TrimSpace(pair[:idx])
			valPart = strings.TrimSpace(pair[idx+1:])
		} else if idx := strings.Index(pair, ":"); idx != -1 {
			key = strings.TrimSpace(pair[:idx])
			valPart = strings.TrimSpace(pair[idx+1:])
		} else {
			key = pair
			valPart = ""
		}
		if key == "" {
			continue
		}
		if valPart == "" {
			if _, exists := target[key]; !exists {
				target[key] = []string{}
			}
			continue
		}
		vals := strings.Split(valPart, ",")
		for _, v := range vals {
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				target[key] = append(target[key], trimmed)
			}
		}
	}
}

type tagFlagSlice []string

func (t *tagFlagSlice) String() string {
	return strings.Join(*t, ", ")
}

func (t *tagFlagSlice) Set(val string) error {
	*t = append(*t, val)
	return nil
}

// Config represents the complete application configuration.
type Config struct {
	Version          string            `yaml:"version"`
	Defaults         DefaultsConfig    `yaml:"defaults"`
	Grouping         GroupingConfig    `yaml:"grouping"`
	Groupings        []GroupingConfig  `yaml:"groupings"`
	Budget           BudgetConfig      `yaml:"budget"`
	Reporting        ReportingConfig   `yaml:"reporting"`
	Server           ServerConfig      `yaml:"server"`
	Accounts         map[string]string `yaml:"accounts"`
	AccountName      string            `yaml:"name"`
	AccountNameAlias string            `yaml:"account_name"`
	SlackWebhook     string            `yaml:"slack_webhook"`
	TeamsWebhook     string            `yaml:"teams_webhook"`
	S3Bucket         string            `yaml:"s3_bucket"`
	S3Key            string            `yaml:"s3_key"`
	Monthly          bool              `yaml:"monthly"`
	MTD              bool              `yaml:"mtd"`

	// Runtime options resolved from CLI flags or env
	ConfigFile            string       `yaml:"-"`
	ShowVersion           bool         `yaml:"-"`
	RunServer             bool         `yaml:"-"`
	ListServices          bool         `yaml:"-"`
	MonthsBack            int          `yaml:"-"`
	StartDate             string       `yaml:"-"`
	EndDate               string       `yaml:"-"`
	MetricsList           []string     `yaml:"-"`
	CompareMoM            bool         `yaml:"-"`
	MockMode              bool         `yaml:"-"`
	Filters               []string     `yaml:"filters"`
	TagFilters            TagFilterMap `yaml:"tag_filters"`
	NoColor               bool         `yaml:"no_color"`
	OutputFile            string       `yaml:"output_file"`
	JSONSummaryFile       string       `yaml:"json_summary_file"`
	CurrencyExplicitlySet bool         `yaml:"-"`
}

// DefaultsConfig represents default billing query parameters.
type DefaultsConfig struct {
	Currency       string   `yaml:"currency"`
	MonthsBack     int      `yaml:"months_back"`
	Monthly        bool     `yaml:"monthly"`
	MTD            bool     `yaml:"mtd"`
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
		Groupings: []GroupingConfig{
			{
				Type: "DIMENSION",
				Key:  "SERVICE",
			},
		},
		Budget: BudgetConfig{
			FailOnThreshold: 0.00,
		},
		Reporting: ReportingConfig{
			Format:          "table",
			WriteJobSummary: true,
		},
		TagFilters: make(TagFilterMap),
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
		groupBy      = fs.String("group-by", "", "Grouping dimension(s), up to 2 comma-separated (SERVICE, LINKED_ACCOUNT, TAG:<Key>)")
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
		teamsFlag    = fs.String("teams-webhook", "", "Incoming Microsoft Teams webhook URL to post formatted cost report")
		s3BucketFlag = fs.String("s3-bucket", "", "Optional S3 bucket name to upload cost report")
		s3KeyFlag    = fs.String("s3-key", "", "Optional S3 object key when using --s3-bucket (e.g. 'reports/cost-report.md')")
		currencyFlag = fs.String("currency", "", "Currency code (e.g. USD, EUR, GBP) to override auto-detected AWS billing currency")
		nameFlag     = fs.String("name", "", "Human-readable name or alias for the AWS account (e.g. 'Production Core')")
		nameAlias    = fs.String("account-name", "", "Alias for --name")
		monthlyFlag  = fs.Bool("monthly", false, "Display multi-month historical breakdown with each month as a separate column")
		trendFlag    = fs.Bool("trend", false, "Alias for --monthly")
		monthlyBrk   = fs.Bool("monthly-breakdown", false, "Alias for --monthly")
		mtdFlag      = fs.Bool("mtd", false, "Query Month-to-Date spend from the 1st of the current month up to today")
		currMonthFlg = fs.Bool("current-month", false, "Alias for --mtd")
		servicesFlag = fs.Bool("services", false, "List official AWS Cost Explorer service names")
		listServFlag = fs.Bool("list-services", false, "Alias for --services")
	)

	var tagFlags tagFlagSlice
	fs.Var(&tagFlags, "tag-filter", "Filter costs by AWS Cost Allocation Tag (e.g. 'Environment=Production' or 'Team=Core,API')")
	fs.Var(&tagFlags, "tag", "Alias for --tag-filter")

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
		var raw map[string]interface{}
		if err := yaml.Unmarshal(data, &raw); err == nil {
			if defs, ok := raw["defaults"].(map[string]interface{}); ok {
				if _, set := defs["currency"]; set {
					cfg.CurrencyExplicitlySet = true
				}
			}
		}
	}

	// Apply CLI flag overrides (highest precedence)
	if *nameFlag != "" {
		cfg.AccountName = strings.TrimSpace(*nameFlag)
	} else if *nameAlias != "" {
		cfg.AccountName = strings.TrimSpace(*nameAlias)
	} else if cfg.AccountName == "" && cfg.AccountNameAlias != "" {
		cfg.AccountName = strings.TrimSpace(cfg.AccountNameAlias)
	} else if cfg.AccountName == "" {
		if envName := strings.TrimSpace(os.Getenv("AWS_ACCOUNT_NAME")); envName != "" {
			cfg.AccountName = envName
		}
	}

	if *currencyFlag != "" {
		cfg.Defaults.Currency = strings.ToUpper(strings.TrimSpace(*currencyFlag))
		cfg.CurrencyExplicitlySet = true
	}
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
		parts := strings.Split(*groupBy, ",")
		var parsedGroupings []GroupingConfig
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.HasPrefix(strings.ToUpper(part), "TAG:") {
				parsedGroupings = append(parsedGroupings, GroupingConfig{
					Type: "TAG",
					Key:  part[4:],
				})
			} else {
				parsedGroupings = append(parsedGroupings, GroupingConfig{
					Type: "DIMENSION",
					Key:  strings.ToUpper(part),
				})
			}
		}
		if len(parsedGroupings) > 2 {
			return nil, fmt.Errorf("AWS Cost Explorer supports a maximum of 2 group-by dimensions, received %d", len(parsedGroupings))
		}
		if len(parsedGroupings) > 0 {
			cfg.Groupings = parsedGroupings
			cfg.Grouping = parsedGroupings[0]
		}
	} else if len(cfg.Groupings) > 0 {
		if len(cfg.Groupings) > 2 {
			return nil, fmt.Errorf("AWS Cost Explorer supports a maximum of 2 group-by dimensions, received %d", len(cfg.Groupings))
		}
		cfg.Grouping = cfg.Groupings[0]
	} else if cfg.Grouping.Key != "" {
		cfg.Groupings = []GroupingConfig{cfg.Grouping}
	} else {
		cfg.Grouping = GroupingConfig{Type: "DIMENSION", Key: "SERVICE"}
		cfg.Groupings = []GroupingConfig{cfg.Grouping}
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

	if *teamsFlag != "" {
		cfg.TeamsWebhook = *teamsFlag
	}

	if *s3BucketFlag != "" {
		cfg.S3Bucket = *s3BucketFlag
	}
	if *s3KeyFlag != "" {
		cfg.S3Key = *s3KeyFlag
	}

	if *compareMoM {
		cfg.CompareMoM = true
	}

	if *monthlyFlag || *trendFlag || *monthlyBrk {
		cfg.Monthly = true
	} else if cfg.Defaults.Monthly {
		cfg.Monthly = true
	}

	if *mtdFlag || *currMonthFlg {
		cfg.MTD = true
	} else if cfg.Defaults.MTD {
		cfg.MTD = true
	}

	if cfg.Monthly && cfg.MonthsBack <= 1 && *months == 0 {
		cfg.MonthsBack = 12
		cfg.Defaults.MonthsBack = 12
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
	if *servicesFlag || *listServFlag {
		cfg.ListServices = true
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

	if cfg.TagFilters == nil {
		cfg.TagFilters = make(TagFilterMap)
	}
	for _, rawTag := range tagFlags {
		ParseTagFilterString(rawTag, cfg.TagFilters)
	}

	if *noColorFlag || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		cfg.NoColor = true
	}

	return cfg, nil
}
