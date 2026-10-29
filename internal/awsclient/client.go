package awsclient

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/smford/aws-cost-report/internal/config"
)

// CostExplorerAPI defines the required Cost Explorer client interface for pagination and mockability.
type CostExplorerAPI interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	GetDimensionValues(ctx context.Context, params *costexplorer.GetDimensionValuesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error)
}

// STSAPI defines the required STS client interface for account identification.
type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// Client orchestrates AWS API queries across STS and Cost Explorer.
type Client struct {
	ceClient  CostExplorerAPI
	stsClient STSAPI
}

// NewClient creates a new AWS client instance.
func NewClient(ce CostExplorerAPI, s STSAPI) *Client {
	return &Client{
		ceClient:  ce,
		stsClient: s,
	}
}

// NewRealClient initializes authentic AWS SDK v2 clients using the default credential chain.
func NewRealClient(ctx context.Context) (*Client, error) {
	// Cost Explorer is a global endpoint hosted in us-east-1
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)
	ceClient := costexplorer.NewFromConfig(cfg)

	return NewClient(ceClient, stsClient), nil
}

// HasSTS returns true if an STS client is configured.
func (c *Client) HasSTS() bool {
	return c.stsClient != nil
}

// VerifySTS queries GetCallerIdentity to verify AWS credential resolution.
func (c *Client) VerifySTS(ctx context.Context) (string, error) {
	if c.stsClient == nil {
		return "", fmt.Errorf("STS client not configured")
	}
	callerID, err := c.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	if callerID.Account != nil {
		return *callerID.Account, nil
	}
	return "unknown", nil
}

// GenerateReport queries AWS Cost Explorer according to the provided configuration and returns a CostReport.
func (c *Client) GenerateReport(ctx context.Context, cfg *config.Config, now time.Time) (*CostReport, error) {
	if cfg.MockMode {
		return GenerateMockReport(cfg, now)
	}

	// 1. Resolve Account ID via STS
	accountID := "Unknown"
	if c.stsClient != nil {
		callerID, err := c.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve AWS caller identity via STS: %w", err)
		}
		if callerID.Account != nil {
			accountID = *callerID.Account
		}
	}

	// 2. Resolve query time periods
	dateRange, err := ResolveTimePeriod(now, cfg.MonthsBack, cfg.StartDate, cfg.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve time period: %w", err)
	}

	// 3. Fetch period costs (concurrently if comparison is enabled)
	var (
		currentTotals TotalCosts
		currentItems  []LineItem
		prevTotals    TotalCosts
		prevItems     []LineItem
		errCurr       error
		errPrev       error
	)

	if cfg.CompareMoM {
		prevDateRange := ResolvePreviousPeriod(dateRange)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			currentTotals, currentItems, errCurr = c.fetchCostData(ctx, cfg, dateRange)
		}()

		go func() {
			defer wg.Done()
			prevTotals, prevItems, errPrev = c.fetchCostData(ctx, cfg, prevDateRange)
		}()

		wg.Wait()

		if errCurr != nil {
			return nil, errCurr
		}
		if errPrev != nil {
			return nil, fmt.Errorf("failed to fetch comparison period: %w", errPrev)
		}
	} else {
		currentTotals, currentItems, errCurr = c.fetchCostData(ctx, cfg, dateRange)
		if errCurr != nil {
			return nil, errCurr
		}
	}

	report := &CostReport{
		Metadata: ReportMetadata{
			AccountID:        accountID,
			BillingMonth:     dateRange.BillingMonthDesc,
			PeriodStart:      dateRange.Start.Format(DateFormat),
			PeriodEnd:        dateRange.EndInclusive.Format(DateFormat),
			AWSPeriodStart:   dateRange.Start.Format(DateFormat),
			AWSPeriodEnd:     dateRange.EndExclusiveAWS.Format(DateFormat),
			Currency:         cfg.Defaults.Currency,
			EstimatedOrFinal: "Finalized",
			ActiveServices:   len(currentItems),
			GroupKey:         cfg.Grouping.Key,
		},
		Totals:        currentTotals,
		Items:         currentItems,
		HasComparison: false,
	}

	if dateRange.IsEstimated {
		report.Metadata.EstimatedOrFinal = "Estimated"
	}

	// 4. Handle Month-over-Month comparison data mapping
	if cfg.CompareMoM {
		report.HasComparison = true

		// Calculate Totals Delta
		report.Totals.PrevUnblendedCost = prevTotals.UnblendedCost
		report.Totals.PrevBlendedCost = prevTotals.BlendedCost
		report.Totals.PrevAmortizedCost = prevTotals.AmortizedCost
		report.Totals.HasPreviousCost = true
		report.Totals.MoMDeltaAmount = roundToTwo(report.Totals.UnblendedCost - prevTotals.UnblendedCost)
		if prevTotals.UnblendedCost > 0 {
			report.Totals.MoMDeltaPercent = roundToTwo(((report.Totals.UnblendedCost - prevTotals.UnblendedCost) / prevTotals.UnblendedCost) * 100.0)
		}

		// Map previous items for fast lookup
		prevMap := make(map[string]LineItem, len(prevItems))
		for _, item := range prevItems {
			prevMap[item.Name] = item
		}

		for i := range report.Items {
			name := report.Items[i].Name
			if prev, exists := prevMap[name]; exists {
				report.Items[i].PrevUnblendedCost = prev.UnblendedCost
				report.Items[i].PrevBlendedCost = prev.BlendedCost
				report.Items[i].PrevAmortizedCost = prev.AmortizedCost
				report.Items[i].HasPreviousCost = true
				report.Items[i].MoMDeltaAmount = roundToTwo(report.Items[i].UnblendedCost - prev.UnblendedCost)
				if prev.UnblendedCost > 0 {
					report.Items[i].MoMDeltaPercent = roundToTwo(((report.Items[i].UnblendedCost - prev.UnblendedCost) / prev.UnblendedCost) * 100.0)
				}
				delete(prevMap, name)
			} else {
				report.Items[i].MoMDeltaAmount = roundToTwo(report.Items[i].UnblendedCost)
				report.Items[i].MoMDeltaPercent = 100.0
			}
		}

		// Include items that existed in previous month but had $0 in current month
		for name, prev := range prevMap {
			if prev.UnblendedCost >= cfg.Defaults.MinDisplayCost {
				report.Items = append(report.Items, LineItem{
					Name:              name,
					UnblendedCost:     0.0,
					BlendedCost:       0.0,
					AmortizedCost:     0.0,
					PrevUnblendedCost: prev.UnblendedCost,
					PrevBlendedCost:   prev.BlendedCost,
					PrevAmortizedCost: prev.AmortizedCost,
					HasPreviousCost:   true,
					MoMDeltaAmount:    roundToTwo(-prev.UnblendedCost),
					MoMDeltaPercent:   -100.0,
				})
			}
		}
	}

	// 5. Filter items by min_display_cost and user-specified name filter
	var filtered []LineItem
	for _, item := range report.Items {
		if len(cfg.Filters) > 0 && !matchesFilter(item.Name, cfg.Filters) {
			continue
		}
		maxCost := math.Max(item.UnblendedCost, math.Max(item.BlendedCost, item.AmortizedCost))
		if item.HasPreviousCost {
			maxCost = math.Max(maxCost, item.PrevUnblendedCost)
		}
		if maxCost >= cfg.Defaults.MinDisplayCost {
			filtered = append(filtered, item)
		}
	}

	// Sort by UnblendedCost descending
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].UnblendedCost != filtered[j].UnblendedCost {
			return filtered[i].UnblendedCost > filtered[j].UnblendedCost
		}
		return filtered[i].Name < filtered[j].Name
	})

	// Apply account alias mapping if configured
	if strings.EqualFold(cfg.Grouping.Key, "LINKED_ACCOUNT") {
		for i := range filtered {
			filtered[i].Name = resolveAccountName(filtered[i].Name, cfg.Accounts)
		}
	}
	report.Metadata.AccountID = resolveAccountName(accountID, cfg.Accounts)

	// Apply Top N aggregation if configured
	filtered = applyTopNAggregation(filtered, cfg.Defaults.Top, cfg.Grouping.Key)

	report.Items = filtered
	report.Metadata.ActiveServices = len(filtered)

	return report, nil
}

// fetchCostData executes paginated queries against AWS Cost Explorer.
func (c *Client) fetchCostData(ctx context.Context, cfg *config.Config, dateRange *DateRange) (TotalCosts, []LineItem, error) {
	timePeriod := &cetypes.DateInterval{
		Start: aws.String(dateRange.Start.Format(DateFormat)),
		End:   aws.String(dateRange.EndExclusiveAWS.Format(DateFormat)),
	}

	groupBy := []cetypes.GroupDefinition{}
	if cfg.Grouping.Type == "TAG" {
		groupBy = append(groupBy, cetypes.GroupDefinition{
			Type: cetypes.GroupDefinitionTypeTag,
			Key:  aws.String(cfg.Grouping.Key),
		})
	} else {
		dimKey := cfg.Grouping.Key
		if dimKey == "" {
			dimKey = "SERVICE"
		}
		groupBy = append(groupBy, cetypes.GroupDefinition{
			Type: cetypes.GroupDefinitionTypeDimension,
			Key:  aws.String(dimKey),
		})
	}

	var totals TotalCosts
	itemsMap := make(map[string]*LineItem)
	var nextPageToken *string

	for {
		input := &costexplorer.GetCostAndUsageInput{
			TimePeriod:    timePeriod,
			Granularity:   cetypes.GranularityMonthly,
			Metrics:       cfg.Defaults.Metrics,
			GroupBy:       groupBy,
			NextPageToken: nextPageToken,
		}

		output, err := c.getCostAndUsageWithRetry(ctx, input)
		if err != nil {
			return totals, nil, fmt.Errorf("AWS Cost Explorer GetCostAndUsage failed: %w", err)
		}

		for _, result := range output.ResultsByTime {
			// Aggregate totals
			if result.Total != nil {
				if m, ok := result.Total["UnblendedCost"]; ok && m.Amount != nil {
					totals.UnblendedCost += parseFloat(*m.Amount)
				}
				if m, ok := result.Total["BlendedCost"]; ok && m.Amount != nil {
					totals.BlendedCost += parseFloat(*m.Amount)
				}
				if m, ok := result.Total["AmortizedCost"]; ok && m.Amount != nil {
					totals.AmortizedCost += parseFloat(*m.Amount)
				}
			}

			// Aggregate line items
			for _, grp := range result.Groups {
				name := "No Group Name"
				if len(grp.Keys) > 0 {
					name = grp.Keys[0]
					if strings.HasPrefix(name, cfg.Grouping.Key+"$") {
						name = strings.TrimPrefix(name, cfg.Grouping.Key+"$")
					}
					if name == "" {
						name = "Untagged / Other"
					}
				}

				item, ok := itemsMap[name]
				if !ok {
					item = &LineItem{Name: name}
					itemsMap[name] = item
				}

				if m, exists := grp.Metrics["UnblendedCost"]; exists && m.Amount != nil {
					item.UnblendedCost += parseFloat(*m.Amount)
				}
				if m, exists := grp.Metrics["BlendedCost"]; exists && m.Amount != nil {
					item.BlendedCost += parseFloat(*m.Amount)
				}
				if m, exists := grp.Metrics["AmortizedCost"]; exists && m.Amount != nil {
					item.AmortizedCost += parseFloat(*m.Amount)
				}
			}
		}

		if output.NextPageToken == nil || *output.NextPageToken == "" {
			break
		}
		nextPageToken = output.NextPageToken
	}

	totals.UnblendedCost = roundToTwo(totals.UnblendedCost)
	totals.BlendedCost = roundToTwo(totals.BlendedCost)
	totals.AmortizedCost = roundToTwo(totals.AmortizedCost)

	var items []LineItem
	for _, item := range itemsMap {
		item.UnblendedCost = roundToTwo(item.UnblendedCost)
		item.BlendedCost = roundToTwo(item.BlendedCost)
		item.AmortizedCost = roundToTwo(item.AmortizedCost)
		items = append(items, *item)
	}

	return totals, items, nil
}

// GenerateMockReport generates realistic mock billing data matching specification benchmarks.
func GenerateMockReport(cfg *config.Config, now time.Time) (*CostReport, error) {
	dateRange, err := ResolveTimePeriod(now, cfg.MonthsBack, cfg.StartDate, cfg.EndDate)
	if err != nil {
		return nil, err
	}

	items := []LineItem{
		{Name: "Amazon DynamoDB", UnblendedCost: 5.51, BlendedCost: 5.51, AmortizedCost: 5.51, PrevUnblendedCost: 5.20, HasPreviousCost: true, MoMDeltaAmount: 0.31, MoMDeltaPercent: 5.96},
		{Name: "AWS Config", UnblendedCost: 4.73, BlendedCost: 4.73, AmortizedCost: 4.73, PrevUnblendedCost: 4.80, HasPreviousCost: true, MoMDeltaAmount: -0.07, MoMDeltaPercent: -1.46},
		{Name: "Amazon Cognito", UnblendedCost: 4.17, BlendedCost: 4.17, AmortizedCost: 4.17, PrevUnblendedCost: 4.15, HasPreviousCost: true, MoMDeltaAmount: 0.02, MoMDeltaPercent: 0.48},
		{Name: "Tax", UnblendedCost: 2.89, BlendedCost: 2.89, AmortizedCost: 2.89, PrevUnblendedCost: 2.80, HasPreviousCost: true, MoMDeltaAmount: 0.09, MoMDeltaPercent: 3.21},
		{Name: "AWS Key Management Service", UnblendedCost: 1.00, BlendedCost: 1.00, AmortizedCost: 1.00, PrevUnblendedCost: 1.00, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "Amazon Simple Storage Service", UnblendedCost: 0.94, BlendedCost: 0.94, AmortizedCost: 0.94, PrevUnblendedCost: 0.88, HasPreviousCost: true, MoMDeltaAmount: 0.06, MoMDeltaPercent: 6.82},
		{Name: "Amazon Route 53", UnblendedCost: 0.80, BlendedCost: 0.80, AmortizedCost: 0.80, PrevUnblendedCost: 0.80, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "Amazon CloudWatch", UnblendedCost: 0.65, BlendedCost: 0.65, AmortizedCost: 0.65, PrevUnblendedCost: 0.70, HasPreviousCost: true, MoMDeltaAmount: -0.05, MoMDeltaPercent: -7.14},
		{Name: "AWS Secrets Manager", UnblendedCost: 0.40, BlendedCost: 0.40, AmortizedCost: 0.40, PrevUnblendedCost: 0.40, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "AWS Lambda", UnblendedCost: 0.25, BlendedCost: 0.25, AmortizedCost: 0.25, PrevUnblendedCost: 0.18, HasPreviousCost: true, MoMDeltaAmount: 0.07, MoMDeltaPercent: 38.89},
		{Name: "Amazon Virtual Private Cloud", UnblendedCost: 0.12, BlendedCost: 0.12, AmortizedCost: 0.12, PrevUnblendedCost: 0.12, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "Amazon Simple Notification Service", UnblendedCost: 0.06, BlendedCost: 0.06, AmortizedCost: 0.06, PrevUnblendedCost: 0.05, HasPreviousCost: true, MoMDeltaAmount: 0.01, MoMDeltaPercent: 20.00},
		{Name: "AWS Step Functions", UnblendedCost: 0.04, BlendedCost: 0.04, AmortizedCost: 0.04, PrevUnblendedCost: 0.03, HasPreviousCost: true, MoMDeltaAmount: 0.01, MoMDeltaPercent: 33.33},
		{Name: "AWS CloudTrail", UnblendedCost: 0.02, BlendedCost: 0.02, AmortizedCost: 0.02, PrevUnblendedCost: 0.02, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
	}

	var filtered []LineItem
	for _, item := range items {
		if len(cfg.Filters) > 0 && !matchesFilter(item.Name, cfg.Filters) {
			continue
		}
		if item.UnblendedCost >= cfg.Defaults.MinDisplayCost {
			filtered = append(filtered, item)
		}
	}

	mockAccountID := resolveAccountName("715195480427", cfg.Accounts)
	if strings.EqualFold(cfg.Grouping.Key, "LINKED_ACCOUNT") {
		for i := range filtered {
			filtered[i].Name = resolveAccountName(filtered[i].Name, cfg.Accounts)
		}
	}
	filtered = applyTopNAggregation(filtered, cfg.Defaults.Top, cfg.Grouping.Key)

	report := &CostReport{
		Metadata: ReportMetadata{
			AccountID:        mockAccountID,
			BillingMonth:     dateRange.BillingMonthDesc,
			PeriodStart:      dateRange.Start.Format(DateFormat),
			PeriodEnd:        dateRange.EndInclusive.Format(DateFormat),
			AWSPeriodStart:   dateRange.Start.Format(DateFormat),
			AWSPeriodEnd:     dateRange.EndExclusiveAWS.Format(DateFormat),
			Currency:         cfg.Defaults.Currency,
			EstimatedOrFinal: "Finalized",
			ActiveServices:   len(filtered),
			GroupKey:         cfg.Grouping.Key,
		},
		Totals: TotalCosts{
			BlendedCost:       21.59,
			UnblendedCost:     21.58,
			AmortizedCost:     21.58,
			PrevUnblendedCost: 21.13,
			PrevBlendedCost:   21.14,
			PrevAmortizedCost: 21.13,
			MoMDeltaAmount:    0.45,
			MoMDeltaPercent:   2.13,
			HasPreviousCost:   cfg.CompareMoM,
		},
		Items:         filtered,
		HasComparison: cfg.CompareMoM,
	}

	if dateRange.IsEstimated {
		report.Metadata.EstimatedOrFinal = "Estimated"
	}

	return report, nil
}

func parseFloat(val string) float64 {
	f, _ := strconv.ParseFloat(val, 64)
	return f
}

func roundToTwo(val float64) float64 {
	return math.Round(val*100) / 100
}

// commonAliases maps common AWS service abbreviations to their official Cost Explorer dimension names.
var commonAliases = map[string]string{
	"s3":         "Amazon Simple Storage Service",
	"ec2":        "Amazon Elastic Compute Cloud - Compute",
	"sns":        "Amazon Simple Notification Service",
	"sqs":        "Amazon Simple Queue Service",
	"kms":        "AWS Key Management Service",
	"vpc":        "Amazon Virtual Private Cloud",
	"cw":         "Amazon CloudWatch",
	"cloudwatch": "Amazon CloudWatch",
	"dynamodb":   "Amazon DynamoDB",
	"lambda":     "AWS Lambda",
	"cognito":    "Amazon Cognito",
	"config":     "AWS Config",
	"cloudtrail": "AWS CloudTrail",
	"secrets":    "AWS Secrets Manager",
	"step":       "AWS Step Functions",
}

func matchesFilter(name string, filters []string) bool {
	lowerName := strings.ToLower(name)
	for _, f := range filters {
		cleanFilter := strings.ToLower(strings.TrimSpace(f))
		if cleanFilter == "" {
			continue
		}
		if strings.Contains(lowerName, cleanFilter) {
			return true
		}
		if officialName, hasAlias := commonAliases[cleanFilter]; hasAlias {
			if strings.Contains(lowerName, strings.ToLower(officialName)) {
				return true
			}
		}
	}
	return false
}

// GetMockServices returns the official AWS Cost Explorer names for simulated services.
func GetMockServices() []string {
	return []string{
		"AWS CloudTrail",
		"AWS Config",
		"AWS Key Management Service",
		"AWS Lambda",
		"AWS Secrets Manager",
		"AWS Step Functions",
		"Amazon CloudWatch",
		"Amazon Cognito",
		"Amazon DynamoDB",
		"Amazon Elastic Compute Cloud - Compute",
		"Amazon Route 53",
		"Amazon Simple Notification Service",
		"Amazon Simple Storage Service",
		"Amazon Virtual Private Cloud",
		"Tax",
	}
}

// ListServices retrieves official AWS Cost Explorer service names.
func (c *Client) ListServices(ctx context.Context, cfg *config.Config, now time.Time) ([]string, error) {
	if cfg.MockMode || c.ceClient == nil {
		return GetMockServices(), nil
	}

	dateRange, err := ResolveTimePeriod(now, cfg.MonthsBack, cfg.StartDate, cfg.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve time period: %w", err)
	}

	timePeriod := &cetypes.DateInterval{
		Start: aws.String(dateRange.Start.Format(DateFormat)),
		End:   aws.String(dateRange.EndExclusiveAWS.Format(DateFormat)),
	}

	var services []string
	var nextPageToken *string

	for {
		input := &costexplorer.GetDimensionValuesInput{
			Dimension:     cetypes.DimensionService,
			TimePeriod:    timePeriod,
			NextPageToken: nextPageToken,
		}

		output, err := c.ceClient.GetDimensionValues(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("AWS Cost Explorer GetDimensionValues failed: %w", err)
		}

		for _, item := range output.DimensionValues {
			if item.Value != nil && *item.Value != "" {
				services = append(services, *item.Value)
			}
		}

		if output.NextPageToken == nil || *output.NextPageToken == "" {
			break
		}
		nextPageToken = output.NextPageToken
	}

	sort.Strings(services)
	return services, nil
}

func (c *Client) getCostAndUsageWithRetry(ctx context.Context, input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error) {
	maxRetries := 5
	backoff := 300 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		out, err := c.ceClient.GetCostAndUsage(ctx, input)
		if err == nil {
			return out, nil
		}

		if attempt == maxRetries {
			return nil, err
		}

		errStr := err.Error()
		if strings.Contains(errStr, "LimitExceededException") ||
			strings.Contains(errStr, "Throttling") ||
			strings.Contains(errStr, "TooManyRequests") ||
			strings.Contains(errStr, "429") {
			sleepDur := backoff + time.Duration(rand.Intn(200))*time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDur):
			}
			backoff *= 2
			continue
		}

		return nil, err
	}
	return nil, fmt.Errorf("exhausted retries")
}

func applyTopNAggregation(items []LineItem, top int, groupKey string) []LineItem {
	if top <= 0 || len(items) <= top {
		return items
	}

	topItems := make([]LineItem, top)
	copy(topItems, items[:top])

	otherLabel := "Other"
	if groupKey == "" || strings.EqualFold(groupKey, "SERVICE") {
		otherLabel = "Other Services"
	} else if strings.EqualFold(groupKey, "LINKED_ACCOUNT") {
		otherLabel = "Other Accounts"
	}

	other := LineItem{Name: otherLabel}
	hasPrev := false

	for _, rem := range items[top:] {
		other.UnblendedCost += rem.UnblendedCost
		other.BlendedCost += rem.BlendedCost
		other.AmortizedCost += rem.AmortizedCost
		if rem.HasPreviousCost {
			hasPrev = true
			other.PrevUnblendedCost += rem.PrevUnblendedCost
			other.PrevBlendedCost += rem.PrevBlendedCost
			other.PrevAmortizedCost += rem.PrevAmortizedCost
		}
	}

	other.UnblendedCost = roundToTwo(other.UnblendedCost)
	other.BlendedCost = roundToTwo(other.BlendedCost)
	other.AmortizedCost = roundToTwo(other.AmortizedCost)
	if hasPrev {
		other.HasPreviousCost = true
		other.PrevUnblendedCost = roundToTwo(other.PrevUnblendedCost)
		other.PrevBlendedCost = roundToTwo(other.PrevBlendedCost)
		other.PrevAmortizedCost = roundToTwo(other.PrevAmortizedCost)
		other.MoMDeltaAmount = roundToTwo(other.UnblendedCost - other.PrevUnblendedCost)
		if other.PrevUnblendedCost > 0 {
			other.MoMDeltaPercent = roundToTwo(((other.UnblendedCost - other.PrevUnblendedCost) / other.PrevUnblendedCost) * 100.0)
		}
	}

	return append(topItems, other)
}

func resolveAccountName(account string, accounts map[string]string) string {
	if len(accounts) == 0 {
		return account
	}
	if name, ok := accounts[account]; ok && name != "" {
		return fmt.Sprintf("%s (%s)", account, name)
	}
	return account
}
