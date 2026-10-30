package config

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

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
	WebhookURL       string            `yaml:"webhook_url"`
	S3Bucket         string            `yaml:"s3_bucket"`
	S3Key            string            `yaml:"s3_key"`
	Monthly          bool              `yaml:"monthly"`
	MTD              bool              `yaml:"mtd"`
	Forecast         bool              `yaml:"forecast"`
	Daily            bool              `yaml:"daily"`
	Granularity      string            `yaml:"granularity"`

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
	ChargeTypes           []string     `yaml:"charge_types"`
	ExcludeCredits        bool         `yaml:"exclude_credits"`
	Regions               []string     `yaml:"regions"`
	AssumeRoleARN         string       `yaml:"assume_role_arn"`
	RoleSessionName       string       `yaml:"role_session_name"`
	ListTags              bool         `yaml:"-"`
	TagKeyToQuery         string       `yaml:"-"`
	ListAnomalies         bool         `yaml:"-"`
	NoColor               bool         `yaml:"no_color"`
	NoCache               bool         `yaml:"no_cache"`
	CacheDir              string       `yaml:"cache_dir"`
	ClearCache            bool         `yaml:"-"`
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
		configPath         = fs.String("config", "", "Path to YAML configuration file")
		configPathSh       = fs.String("c", "", "Path to YAML configuration file (shorthand)")
		showVersion        = fs.Bool("version", false, "Print version information and exit")
		showVerSh          = fs.Bool("v", false, "Print version information and exit (shorthand)")
		months             = fs.Int("months", 0, "Query historical full months back (default: 1)")
		start              = fs.String("start", "", "Explicit start date (YYYY-MM-01)")
		end                = fs.String("end", "", "Explicit end date (YYYY-MM-01 or YYYY-MM-DD)")
		metricsFlag        = fs.String("metrics", "", "Comma-separated metrics (UnblendedCost, BlendedCost, AmortizedCost, NetUnblendedCost, NetAmortizedCost, or all)")
		groupBy            = fs.String("group-by", "", "Grouping dimension(s), up to 2 comma-separated (SERVICE, LINKED_ACCOUNT, TAG:<Key>)")
		minCost            = fs.Float64("min-cost", -1.0, "Minimum display cost threshold (e.g. 0.01)")
		compareMoM         = fs.Bool("compare", false, "Compare Month-over-Month with preceding period")
		failThresh         = fs.Float64("fail-on-threshold", -1.0, "Terminate with exit code 1 if total cost exceeds this threshold")
		threshAlias        = fs.Float64("threshold", -1.0, "Alias for --fail-on-threshold")
		format             = fs.String("format", "", "Output format: table, markdown, json, csv")
		writeSummary       = fs.Bool("write-summary", true, "Write markdown report to $GITHUB_STEP_SUMMARY when set")
		mockMode           = fs.Bool("mock", false, "Run in offline mock mode with simulated billing data")
		serverMode         = fs.Bool("server", false, "Run as an HTTP service daemon exposing /healthz and /report")
		filterFlag         = fs.String("filter", "", "Filter services by comma-separated substring (e.g. DynamoDB,Lambda)")
		serviceFlag        = fs.String("service", "", "Alias for --filter")
		noColorFlag        = fs.Bool("no-color", false, "Disable ANSI color output in terminal tables")
		noCacheFlag        = fs.Bool("no-cache", false, "Disable local query response caching")
		cacheDirFlag       = fs.String("cache-dir", "", "Custom directory to store local query cache files")
		clearCacheFlag     = fs.Bool("clear-cache", false, "Purge local query response cache files and exit")
		outputFlag         = fs.String("output", "", "Optional file path to write report output")
		outputSh           = fs.String("o", "", "Optional file path to write report output (shorthand)")
		jsonSumFlag        = fs.String("json-summary", "", "Optional file path to write machine-readable summary JSON")
		topFlag            = fs.Int("top", 0, "Show top N highest cost items and aggregate remainder into Other")
		slackFlag          = fs.String("slack-webhook", "", "Incoming Slack webhook URL to post formatted cost report")
		teamsFlag          = fs.String("teams-webhook", "", "Incoming Microsoft Teams webhook URL to post formatted cost report")
		webhookFlag        = fs.String("webhook-url", "", "Generic HTTP/HTTPS webhook URL to post formatted cost report JSON")
		s3BucketFlag       = fs.String("s3-bucket", "", "Optional S3 bucket name to upload cost report")
		s3KeyFlag          = fs.String("s3-key", "", "Optional S3 object key when using --s3-bucket (e.g. 'reports/cost-report.md')")
		currencyFlag       = fs.String("currency", "", "Currency code (e.g. USD, EUR, GBP) to override auto-detected AWS billing currency")
		nameFlag           = fs.String("name", "", "Human-readable name or alias for the AWS account (e.g. 'Production Core')")
		nameAlias          = fs.String("account-name", "", "Alias for --name")
		monthlyFlag        = fs.Bool("monthly", false, "Display multi-month historical breakdown with each month as a separate column")
		trendFlag          = fs.Bool("trend", false, "Alias for --monthly")
		monthlyBrk         = fs.Bool("monthly-breakdown", false, "Alias for --monthly")
		mtdFlag            = fs.Bool("mtd", false, "Query Month-to-Date spend from the 1st of the current month up to today")
		currMonthFlg       = fs.Bool("current-month", false, "Alias for --mtd")
		forecastFlag       = fs.Bool("forecast", false, "Query AWS Cost Explorer projected month-end spend forecast and guardrail")
		dailyFlag          = fs.Bool("daily", false, "Query daily spend granularity over the billing period")
		granFlag           = fs.String("granularity", "", "Spend granularity: MONTHLY (default) or DAILY")
		servicesFlag       = fs.Bool("services", false, "List official AWS Cost Explorer service names")
		listServFlag       = fs.Bool("list-services", false, "Alias for --services")
		excludeCreditsFlag = fs.Bool("exclude-credits", false, "Exclude credits and refunds from billing calculations")
		chargeTypeFlag     = fs.String("charge-type", "", "Comma-separated list of charge/record types (e.g. Usage,Tax,Credit,Refund)")
		recordTypeFlag     = fs.String("record-type", "", "Alias for --charge-type")
		regionFlag         = fs.String("region", "", "Filter costs by comma-separated AWS regions (e.g. us-east-1,eu-west-1)")
		filterRegionFlag   = fs.String("filter-region", "", "Alias for --region")
		assumeRoleFlag     = fs.String("assume-role-arn", "", "AWS IAM Role ARN to assume via STS for cross-account cost auditing")
		sessionNameFlag    = fs.String("role-session-name", "", "Optional role session name when assuming an IAM role")
		tagsFlag           = fs.Bool("tags", false, "List available Cost Allocation Tag keys")
		listTagsFlag       = fs.Bool("list-tags", false, "Alias for --tags")
		tagKeyFlag         = fs.String("tag-key", "", "Query tag values for a specific tag key when using --tags")
		anomaliesFlag      = fs.Bool("anomalies", false, "List recent AWS Cost Anomalies detected by Cost Explorer")
		listAnomFlag       = fs.Bool("list-anomalies", false, "Alias for --anomalies")
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
		expanded := []byte(ExpandEnv(string(data)))
		if err := yaml.Unmarshal(expanded, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config YAML: %w", err)
		}
		var raw map[string]interface{}
		if err := yaml.Unmarshal(expanded, &raw); err == nil {
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
	if *months != 0 {
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
			cfg.Defaults.Metrics = []string{"UnblendedCost", "BlendedCost", "AmortizedCost", "NetUnblendedCost", "NetAmortizedCost"}
		} else {
			parts := strings.Split(*metricsFlag, ",")
			var cleaned []string
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed == "" {
					continue
				}
				switch strings.ToLower(trimmed) {
				case "unblendedcost", "unblended":
					cleaned = append(cleaned, "UnblendedCost")
				case "blendedcost", "blended":
					cleaned = append(cleaned, "BlendedCost")
				case "amortizedcost", "amortized":
					cleaned = append(cleaned, "AmortizedCost")
				case "netunblendedcost", "netunblended", "net_unblended":
					cleaned = append(cleaned, "NetUnblendedCost")
				case "netamortizedcost", "netamortized", "net_amortized":
					cleaned = append(cleaned, "NetAmortizedCost")
				default:
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

	if *minCost != -1.0 {
		cfg.Defaults.MinDisplayCost = *minCost
	}

	if *topFlag != 0 {
		cfg.Defaults.Top = *topFlag
	}

	if *slackFlag != "" {
		cfg.SlackWebhook = *slackFlag
	}

	if *teamsFlag != "" {
		cfg.TeamsWebhook = *teamsFlag
	}

	if *webhookFlag != "" {
		cfg.WebhookURL = *webhookFlag
	}
	if cfg.WebhookURL == "" {
		cfg.WebhookURL = os.Getenv("WEBHOOK_URL")
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

	if *forecastFlag {
		cfg.Forecast = true
	}

	if *granFlag != "" {
		cfg.Granularity = *granFlag
	}
	if *dailyFlag || strings.EqualFold(cfg.Granularity, "DAILY") {
		cfg.Daily = true
		cfg.Granularity = "DAILY"
	}

	if *excludeCreditsFlag {
		cfg.ExcludeCredits = true
	}

	rawChargeType := *chargeTypeFlag
	if rawChargeType == "" {
		rawChargeType = *recordTypeFlag
	}
	if rawChargeType != "" {
		parts := strings.Split(rawChargeType, ",")
		var cleaned []string
		for _, p := range parts {
			t := normalizeChargeType(strings.TrimSpace(p))
			if t != "" {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) > 0 {
			cfg.ChargeTypes = cleaned
		}
	} else if len(cfg.ChargeTypes) > 0 {
		var cleaned []string
		for _, p := range cfg.ChargeTypes {
			t := normalizeChargeType(strings.TrimSpace(p))
			if t != "" {
				cleaned = append(cleaned, t)
			}
		}
		cfg.ChargeTypes = cleaned
	}

	if cfg.Monthly && cfg.MonthsBack <= 1 && *months == 0 {
		cfg.MonthsBack = 12
		cfg.Defaults.MonthsBack = 12
	}

	if *threshAlias != -1.0 && *failThresh == -1.0 {
		*failThresh = *threshAlias
	}

	if *failThresh != -1.0 {
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
	if *tagsFlag || *listTagsFlag {
		cfg.ListTags = true
	}
	if *tagKeyFlag != "" {
		cfg.TagKeyToQuery = strings.TrimSpace(*tagKeyFlag)
	}
	if *anomaliesFlag || *listAnomFlag {
		cfg.ListAnomalies = true
	}

	rawRegion := *regionFlag
	if rawRegion == "" {
		rawRegion = *filterRegionFlag
	}
	if rawRegion != "" {
		parts := strings.Split(rawRegion, ",")
		var cleaned []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) > 0 {
			cfg.Regions = cleaned
		}
	}

	if *assumeRoleFlag != "" {
		cfg.AssumeRoleARN = strings.TrimSpace(*assumeRoleFlag)
	}
	if cfg.AssumeRoleARN == "" {
		cfg.AssumeRoleARN = os.Getenv("AWS_ROLE_ARN")
	}

	if *sessionNameFlag != "" {
		cfg.RoleSessionName = strings.TrimSpace(*sessionNameFlag)
	}
	if cfg.RoleSessionName == "" {
		cfg.RoleSessionName = os.Getenv("AWS_ROLE_SESSION_NAME")
	}
	if cfg.RoleSessionName == "" {
		cfg.RoleSessionName = "aws-cost-report"
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

	if *noCacheFlag || os.Getenv("AWS_COST_REPORT_NO_CACHE") == "1" || os.Getenv("AWS_COST_REPORT_NO_CACHE") == "true" {
		cfg.NoCache = true
	}
	if *cacheDirFlag != "" {
		cfg.CacheDir = *cacheDirFlag
	}
	if *clearCacheFlag {
		cfg.ClearCache = true
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate performs strict validation on configuration options and returns an error describing any violations.
func (c *Config) Validate() error {
	// 1. Format validation
	if c.Reporting.Format != "" {
		switch strings.ToLower(c.Reporting.Format) {
		case "table", "markdown", "md", "html", "json", "csv", "prometheus", "prom":
			// valid
		default:
			return fmt.Errorf("invalid format %q: supported formats are table, markdown, html, json, csv, prometheus", c.Reporting.Format)
		}
	}

	// 2. Months validation
	if c.MonthsBack < 0 {
		return fmt.Errorf("months back cannot be negative (got %d)", c.MonthsBack)
	}

	// 3. Top validation
	if c.Defaults.Top < 0 {
		return fmt.Errorf("top cannot be negative (got %d)", c.Defaults.Top)
	}

	// 4. Threshold validation
	if c.Budget.FailOnThreshold < 0 && c.Budget.FailOnThreshold != -1.0 {
		return fmt.Errorf("threshold cannot be negative (got %.2f)", c.Budget.FailOnThreshold)
	}

	// 5. MinDisplayCost validation
	if c.Defaults.MinDisplayCost < 0 && c.Defaults.MinDisplayCost != -1.0 {
		return fmt.Errorf("min-cost cannot be negative (got %.2f)", c.Defaults.MinDisplayCost)
	}

	// 6. Granularity validation
	if c.Granularity != "" {
		upper := strings.ToUpper(c.Granularity)
		if upper != "MONTHLY" && upper != "DAILY" {
			return fmt.Errorf("invalid granularity %q: must be MONTHLY or DAILY", c.Granularity)
		}
	}

	// 7. Date format and ordering validation
	if c.StartDate != "" {
		if _, err := time.Parse("2006-01-02", c.StartDate); err != nil {
			return fmt.Errorf("invalid start date %q: expected YYYY-MM-DD: %w", c.StartDate, err)
		}
	}
	if c.EndDate != "" {
		if _, err := time.Parse("2006-01-02", c.EndDate); err != nil {
			return fmt.Errorf("invalid end date %q: expected YYYY-MM-DD: %w", c.EndDate, err)
		}
	}
	if c.StartDate != "" && c.EndDate != "" {
		s, _ := time.Parse("2006-01-02", c.StartDate)
		e, _ := time.Parse("2006-01-02", c.EndDate)
		if e.Before(s) {
			return fmt.Errorf("start date (%s) must be before or equal to end date (%s)", c.StartDate, c.EndDate)
		}
	}

	// 8. Grouping dimension validation
	if len(c.Groupings) > 2 {
		return fmt.Errorf("AWS Cost Explorer supports a maximum of 2 group-by dimensions, received %d", len(c.Groupings))
	}
	for _, g := range c.Groupings {
		if g.Type == "TAG" {
			if strings.TrimSpace(g.Key) == "" {
				return fmt.Errorf("tag grouping key cannot be empty")
			}
		} else {
			upperKey := strings.ToUpper(strings.TrimSpace(g.Key))
			switch upperKey {
			case "SERVICE", "LINKED_ACCOUNT", "REGION", "RECORD_TYPE":
				// valid
			default:
				return fmt.Errorf("invalid grouping dimension %q: supported dimensions are SERVICE, LINKED_ACCOUNT, REGION, RECORD_TYPE, or TAG:<Key>", g.Key)
			}
		}
	}

	return nil
}

func normalizeChargeType(s string) string {
	lower := strings.ToLower(s)
	switch lower {
	case "usage":
		return "Usage"
	case "credit", "credits":
		return "Credit"
	case "refund", "refunds":
		return "Refund"
	case "tax", "taxes":
		return "Tax"
	case "upfront", "upfrontreservationfee":
		return "UpfrontReservationFee"
	case "recurring", "recurringreservationfee":
		return "RecurringReservationFee"
	case "support":
		return "Support"
	case "other":
		return "Other"
	default:
		return s
	}
}

// ExpandEnv replaces ${VAR} or ${VAR:-default} with values from the environment.
func ExpandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		if idx := strings.Index(key, ":-"); idx != -1 {
			k := key[:idx]
			def := key[idx+2:]
			if val, ok := os.LookupEnv(k); ok && val != "" {
				return val
			}
			return def
		}
		return os.Getenv(key)
	})
}
