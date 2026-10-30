package awsclient

import (
	"context"
	"errors"
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
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/smford/aws-cost-report/internal/config"
)

// CostExplorerAPI defines the required Cost Explorer client interface for pagination and mockability.
type CostExplorerAPI interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	GetDimensionValues(ctx context.Context, params *costexplorer.GetDimensionValuesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error)
	GetCostForecast(ctx context.Context, params *costexplorer.GetCostForecastInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error)
}

// STSAPI defines the required STS client interface for account identification.
type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// IAMAPI defines the required IAM client interface for account alias detection.
type IAMAPI interface {
	ListAccountAliases(ctx context.Context, params *iam.ListAccountAliasesInput, optFns ...func(*iam.Options)) (*iam.ListAccountAliasesOutput, error)
}

// Client orchestrates AWS API queries across STS, IAM, Cost Explorer, and S3.
type Client struct {
	ceClient  CostExplorerAPI
	stsClient STSAPI
	iamClient IAMAPI
	s3Client  S3API
}

// NewClient creates a new AWS client instance.
func NewClient(ce CostExplorerAPI, s STSAPI, i ...IAMAPI) *Client {
	c := &Client{
		ceClient:  ce,
		stsClient: s,
	}
	if len(i) > 0 {
		c.iamClient = i[0]
	}
	return c
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
	iamClient := iam.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)

	client := NewClient(ceClient, stsClient, iamClient)
	client.s3Client = s3Client
	return client, nil
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

	// 1b. Auto-detect account alias via IAM if not explicitly provided
	resolvedAccountName := cfg.AccountName
	if resolvedAccountName == "" && c.iamClient != nil {
		if aliasOut, err := c.iamClient.ListAccountAliases(ctx, &iam.ListAccountAliasesInput{}); err == nil && aliasOut != nil {
			if len(aliasOut.AccountAliases) > 0 && aliasOut.AccountAliases[0] != "" {
				resolvedAccountName = aliasOut.AccountAliases[0]
			}
		}
	}

	// 2. Resolve query time periods
	dateRange, err := ResolveTimePeriod(now, cfg.MonthsBack, cfg.StartDate, cfg.EndDate, cfg.MTD)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve time period: %w", err)
	}

	// 3. Fetch period costs (concurrently if comparison is enabled)
	var (
		currentTotals    TotalCosts
		currentItems     []LineItem
		currentMonthly   *MonthlyBreakdown
		currentDaily     *DailyBreakdown
		detectedCurrency string
		prevTotals       TotalCosts
		prevItems        []LineItem
		errCurr          error
		errPrev          error
	)

	if cfg.CompareMoM {
		prevDateRange := ResolvePreviousPeriod(dateRange)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			currentTotals, currentItems, currentMonthly, currentDaily, detectedCurrency, errCurr = c.fetchCostData(ctx, cfg, dateRange)
		}()

		go func() {
			defer wg.Done()
			prevTotals, prevItems, _, _, _, errPrev = c.fetchCostData(ctx, cfg, prevDateRange)
		}()

		wg.Wait()

		if errCurr != nil {
			return nil, errCurr
		}
		if errPrev != nil {
			return nil, fmt.Errorf("failed to fetch comparison period: %w", errPrev)
		}
	} else {
		currentTotals, currentItems, currentMonthly, currentDaily, detectedCurrency, errCurr = c.fetchCostData(ctx, cfg, dateRange)
		if errCurr != nil {
			return nil, errCurr
		}
	}

	reportCurrency := cfg.Defaults.Currency
	if !cfg.CurrencyExplicitlySet && detectedCurrency != "" {
		reportCurrency = detectedCurrency
	}
	if reportCurrency == "" {
		reportCurrency = "USD"
	}

	report := &CostReport{
		Metadata: ReportMetadata{
			AccountID:        accountID,
			BillingMonth:     dateRange.BillingMonthDesc,
			PeriodStart:      dateRange.Start.Format(DateFormat),
			PeriodEnd:        dateRange.EndInclusive.Format(DateFormat),
			AWSPeriodStart:   dateRange.Start.Format(DateFormat),
			AWSPeriodEnd:     dateRange.EndExclusiveAWS.Format(DateFormat),
			Currency:         reportCurrency,
			EstimatedOrFinal: "Finalized",
			ActiveServices:   len(currentItems),
			GroupKey:         getReportGroupKey(cfg),
			TagFilter:        cfg.TagFilters.Format(),
			ChargeTypeFilter: strings.Join(cfg.ChargeTypes, ", "),
			ExcludeCredits:   cfg.ExcludeCredits,
		},
		Totals:           currentTotals,
		Items:            currentItems,
		HasComparison:    false,
		MonthlyBreakdown: currentMonthly,
		DailyBreakdown:   currentDaily,
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
		report.Totals.PrevNetUnblendedCost = prevTotals.NetUnblendedCost
		report.Totals.PrevNetAmortizedCost = prevTotals.NetAmortizedCost
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
				report.Items[i].PrevNetUnblendedCost = prev.NetUnblendedCost
				report.Items[i].PrevNetAmortizedCost = prev.NetAmortizedCost
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
					Name:                 name,
					UnblendedCost:        0.0,
					BlendedCost:          0.0,
					AmortizedCost:        0.0,
					NetUnblendedCost:     0.0,
					NetAmortizedCost:     0.0,
					PrevUnblendedCost:    prev.UnblendedCost,
					PrevBlendedCost:      prev.BlendedCost,
					PrevAmortizedCost:    prev.AmortizedCost,
					PrevNetUnblendedCost: prev.NetUnblendedCost,
					PrevNetAmortizedCost: prev.NetAmortizedCost,
					HasPreviousCost:      true,
					MoMDeltaAmount:       roundToTwo(-prev.UnblendedCost),
					MoMDeltaPercent:      -100.0,
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

	// Recalculate totals if filters were applied or if initial totals were empty
	if len(cfg.Filters) > 0 || (report.Totals.UnblendedCost == 0.0 && len(filtered) > 0) {
		var uCost, bCost, aCost, nuCost, naCost, prevUCost, prevBCost, prevACost, prevNUCost, prevNACost float64
		for _, it := range filtered {
			uCost += it.UnblendedCost
			bCost += it.BlendedCost
			aCost += it.AmortizedCost
			nuCost += it.NetUnblendedCost
			naCost += it.NetAmortizedCost
			if it.HasPreviousCost {
				prevUCost += it.PrevUnblendedCost
				prevBCost += it.PrevBlendedCost
				prevACost += it.PrevAmortizedCost
				prevNUCost += it.PrevNetUnblendedCost
				prevNACost += it.PrevNetAmortizedCost
			}
		}
		uCost = roundToTwo(uCost)
		bCost = roundToTwo(bCost)
		aCost = roundToTwo(aCost)
		nuCost = roundToTwo(nuCost)
		naCost = roundToTwo(naCost)
		prevUCost = roundToTwo(prevUCost)
		prevBCost = roundToTwo(prevBCost)
		prevACost = roundToTwo(prevACost)
		prevNUCost = roundToTwo(prevNUCost)
		prevNACost = roundToTwo(prevNACost)
		dAmount := roundToTwo(uCost - prevUCost)
		dPct := 0.0
		if prevUCost > 0 {
			dPct = roundToTwo((dAmount / prevUCost) * 100.0)
		}

		report.Totals = TotalCosts{
			UnblendedCost:        uCost,
			BlendedCost:          bCost,
			AmortizedCost:        aCost,
			NetUnblendedCost:     nuCost,
			NetAmortizedCost:     naCost,
			PrevUnblendedCost:    prevUCost,
			PrevBlendedCost:      prevBCost,
			PrevAmortizedCost:    prevACost,
			PrevNetUnblendedCost: prevNUCost,
			PrevNetAmortizedCost: prevNACost,
			MoMDeltaAmount:       dAmount,
			MoMDeltaPercent:      dPct,
			HasPreviousCost:      cfg.CompareMoM,
		}
	}

	if report.Totals.BlendedCost == 0.0 && report.Totals.UnblendedCost > 0.0 {
		report.Totals.BlendedCost = report.Totals.UnblendedCost
	}
	if report.Totals.AmortizedCost == 0.0 && report.Totals.UnblendedCost > 0.0 {
		report.Totals.AmortizedCost = report.Totals.UnblendedCost
	}
	if cfg.CompareMoM {
		if report.Totals.PrevBlendedCost == 0.0 && report.Totals.PrevUnblendedCost > 0.0 {
			report.Totals.PrevBlendedCost = report.Totals.PrevUnblendedCost
		}
		if report.Totals.PrevAmortizedCost == 0.0 && report.Totals.PrevUnblendedCost > 0.0 {
			report.Totals.PrevAmortizedCost = report.Totals.PrevUnblendedCost
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
			itemAccountName := ""
			if filtered[i].Name == accountID {
				itemAccountName = resolvedAccountName
			}
			filtered[i].Name = resolveAccountName(filtered[i].Name, cfg.Accounts, itemAccountName)
		}
	}
	report.Metadata.AccountID = resolveAccountName(accountID, cfg.Accounts, resolvedAccountName)
	report.Metadata.AccountName = resolvedAccountName
	if report.Metadata.AccountName == "" && cfg.Accounts != nil {
		report.Metadata.AccountName = cfg.Accounts[accountID]
	}

	// Apply Top N aggregation if configured
	filtered = applyTopNAggregation(filtered, cfg.Defaults.Top, cfg.Grouping.Key)

	report.Items = filtered
	report.Metadata.ActiveServices = len(filtered)

	if currentMonthly != nil && len(currentMonthly.Months) > 0 {
		report.MonthlyBreakdown = alignMonthlyBreakdown(currentMonthly, filtered, cfg.Grouping.Key)
	}

	if cfg.Forecast {
		forecast, err := c.fetchCostForecast(ctx, cfg, now, report.Totals.UnblendedCost)
		if err != nil {
			return nil, err
		}
		report.Forecast = forecast
	}

	return report, nil
}

// alignMonthlyBreakdown aligns and re-aggregates monthly breakdown items to match the finalized report items.
func alignMonthlyBreakdown(mb *MonthlyBreakdown, targetItems []LineItem, groupKey string) *MonthlyBreakdown {
	if mb == nil {
		return nil
	}
	itemMap := make(map[string]MonthlyLineItem, len(mb.Items))
	for _, it := range mb.Items {
		itemMap[it.Name] = it
	}

	otherName := "Other Services"
	if strings.EqualFold(groupKey, "LINKED_ACCOUNT") {
		otherName = "Other Accounts"
	} else if groupKey != "" && !strings.EqualFold(groupKey, "SERVICE") {
		otherName = "Other (" + groupKey + ")"
	}

	var aligned []MonthlyLineItem
	used := make(map[string]bool)

	for _, tItem := range targetItems {
		if tItem.Name == otherName {
			otherCosts := make(map[string]float64, len(mb.Months))
			var otherTotal float64
			for _, it := range mb.Items {
				if !used[it.Name] {
					for _, m := range mb.Months {
						otherCosts[m.Key] += it.Costs[m.Key]
					}
					otherTotal += it.Total
				}
			}
			for k, v := range otherCosts {
				otherCosts[k] = roundToTwo(v)
			}
			otherTotal = roundToTwo(otherTotal)
			avg := 0.0
			if len(mb.Months) > 0 {
				avg = roundToTwo(otherTotal / float64(len(mb.Months)))
			}
			aligned = append(aligned, MonthlyLineItem{
				Name:    otherName,
				Costs:   otherCosts,
				Total:   otherTotal,
				Average: avg,
			})
		} else if mIt, exists := itemMap[tItem.Name]; exists {
			aligned = append(aligned, mIt)
			used[tItem.Name] = true
		}
	}

	return &MonthlyBreakdown{
		Months:     mb.Months,
		Items:      aligned,
		Totals:     mb.Totals,
		GrandTotal: mb.GrandTotal,
	}
}

// fetchCostData executes paginated queries against AWS Cost Explorer.
func (c *Client) fetchCostData(ctx context.Context, cfg *config.Config, dateRange *DateRange) (TotalCosts, []LineItem, *MonthlyBreakdown, *DailyBreakdown, string, error) {
	timePeriod := &cetypes.DateInterval{
		Start: aws.String(dateRange.Start.Format(DateFormat)),
		End:   aws.String(dateRange.EndExclusiveAWS.Format(DateFormat)),
	}

	granularity := cetypes.GranularityMonthly
	if cfg.Daily {
		granularity = cetypes.GranularityDaily
	}

	groupings := cfg.Groupings
	if len(groupings) == 0 {
		if cfg.Grouping.Key != "" {
			groupings = []config.GroupingConfig{cfg.Grouping}
		} else {
			groupings = []config.GroupingConfig{{Type: "DIMENSION", Key: "SERVICE"}}
		}
	}

	groupBy := []cetypes.GroupDefinition{}
	for _, g := range groupings {
		if strings.EqualFold(g.Type, "TAG") {
			groupBy = append(groupBy, cetypes.GroupDefinition{
				Type: cetypes.GroupDefinitionTypeTag,
				Key:  aws.String(g.Key),
			})
		} else {
			dimKey := g.Key
			if dimKey == "" {
				dimKey = "SERVICE"
			}
			groupBy = append(groupBy, cetypes.GroupDefinition{
				Type: cetypes.GroupDefinitionTypeDimension,
				Key:  aws.String(dimKey),
			})
		}
	}

	var totals TotalCosts
	var detectedCurrency string
	itemsMap := make(map[string]*LineItem)
	var nextPageToken *string

	type topDriverInfo struct {
		name   string
		amount float64
	}
	dayKeysMap := make(map[string]bool)
	var dayKeys []string
	dayTotalsMap := make(map[string]float64)
	dayTopDriver := make(map[string]topDriverInfo)
	dayItemCosts := make(map[string]map[string]float64)

	monthKeysMap := make(map[string]bool)
	var monthKeys []string
	type monthInfo struct {
		label     string
		fullLabel string
	}
	monthInfoMap := make(map[string]monthInfo)
	monthTotalsMap := make(map[string]float64)
	itemMonthlyCosts := make(map[string]map[string]float64)

	filterExpr := buildFilterExpression(cfg)

	for {
		input := &costexplorer.GetCostAndUsageInput{
			TimePeriod:    timePeriod,
			Granularity:   granularity,
			Metrics:       cfg.Defaults.Metrics,
			GroupBy:       groupBy,
			Filter:        filterExpr,
			NextPageToken: nextPageToken,
		}

		output, err := c.getCostAndUsageWithRetry(ctx, input)
		if err != nil {
			return totals, nil, nil, nil, "", fmt.Errorf("AWS Cost Explorer GetCostAndUsage failed: %w", err)
		}

		for _, result := range output.ResultsByTime {
			var mKey string
			var dKey string
			if result.TimePeriod != nil && result.TimePeriod.Start != nil {
				if pt, err := time.Parse(DateFormat, *result.TimePeriod.Start); err == nil {
					mKey = pt.Format("2006-01")
					if !monthKeysMap[mKey] {
						monthKeysMap[mKey] = true
						monthKeys = append(monthKeys, mKey)
						monthInfoMap[mKey] = monthInfo{
							label:     pt.Format("Jan 06"),
							fullLabel: pt.Format("January 2006"),
						}
					}
					if cfg.Daily {
						dKey = pt.Format(DateFormat)
						if !dayKeysMap[dKey] {
							dayKeysMap[dKey] = true
							dayKeys = append(dayKeys, dKey)
						}
					}
				}
			}

			// Aggregate totals
			if result.Total != nil {
				for _, m := range result.Total {
					if m.Unit != nil && *m.Unit != "" && detectedCurrency == "" {
						detectedCurrency = *m.Unit
					}
				}
				if m, ok := result.Total["UnblendedCost"]; ok && m.Amount != nil {
					totals.UnblendedCost += parseFloat(*m.Amount)
				}
				if m, ok := result.Total["BlendedCost"]; ok && m.Amount != nil {
					totals.BlendedCost += parseFloat(*m.Amount)
				}
				if m, ok := result.Total["AmortizedCost"]; ok && m.Amount != nil {
					totals.AmortizedCost += parseFloat(*m.Amount)
				}
				if m, ok := result.Total["NetUnblendedCost"]; ok && m.Amount != nil {
					totals.NetUnblendedCost += parseFloat(*m.Amount)
				}
				if m, ok := result.Total["NetAmortizedCost"]; ok && m.Amount != nil {
					totals.NetAmortizedCost += parseFloat(*m.Amount)
				}
			}

			// Aggregate line items
			for _, grp := range result.Groups {
				for _, m := range grp.Metrics {
					if m.Unit != nil && *m.Unit != "" && detectedCurrency == "" {
						detectedCurrency = *m.Unit
					}
				}

				name := formatGroupKeys(grp.Keys, groupings, cfg.Accounts)

				item, ok := itemsMap[name]
				if !ok {
					item = &LineItem{Name: name}
					itemsMap[name] = item
				}

				var uCost float64
				if m, exists := grp.Metrics["UnblendedCost"]; exists && m.Amount != nil {
					uCost = parseFloat(*m.Amount)
					item.UnblendedCost += uCost
				}
				if m, exists := grp.Metrics["BlendedCost"]; exists && m.Amount != nil {
					item.BlendedCost += parseFloat(*m.Amount)
				}
				if m, exists := grp.Metrics["AmortizedCost"]; exists && m.Amount != nil {
					item.AmortizedCost += parseFloat(*m.Amount)
				}
				if m, exists := grp.Metrics["NetUnblendedCost"]; exists && m.Amount != nil {
					item.NetUnblendedCost += parseFloat(*m.Amount)
				}
				if m, exists := grp.Metrics["NetAmortizedCost"]; exists && m.Amount != nil {
					item.NetAmortizedCost += parseFloat(*m.Amount)
				}

				if mKey != "" {
					if itemMonthlyCosts[name] == nil {
						itemMonthlyCosts[name] = make(map[string]float64)
					}
					itemMonthlyCosts[name][mKey] += uCost
					monthTotalsMap[mKey] += uCost
				}

				if dKey != "" {
					if dayItemCosts[name] == nil {
						dayItemCosts[name] = make(map[string]float64)
					}
					dayItemCosts[name][dKey] += uCost
					dayTotalsMap[dKey] += uCost
					if uCost > dayTopDriver[dKey].amount {
						dayTopDriver[dKey] = topDriverInfo{name: name, amount: uCost}
					}
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
	totals.NetUnblendedCost = roundToTwo(totals.NetUnblendedCost)
	totals.NetAmortizedCost = roundToTwo(totals.NetAmortizedCost)

	var items []LineItem
	var sumUnblended, sumBlended, sumAmortized, sumNetUnblended, sumNetAmortized float64
	for _, item := range itemsMap {
		item.UnblendedCost = roundToTwo(item.UnblendedCost)
		item.BlendedCost = roundToTwo(item.BlendedCost)
		item.AmortizedCost = roundToTwo(item.AmortizedCost)
		item.NetUnblendedCost = roundToTwo(item.NetUnblendedCost)
		item.NetAmortizedCost = roundToTwo(item.NetAmortizedCost)

		// For accounts without consolidated billing or upfront amortization,
		// BlendedCost or AmortizedCost might be returned as 0 by AWS. Fall back to UnblendedCost if so.
		if item.BlendedCost == 0.0 && item.UnblendedCost > 0.0 {
			item.BlendedCost = item.UnblendedCost
		}
		if item.AmortizedCost == 0.0 && item.UnblendedCost > 0.0 {
			item.AmortizedCost = item.UnblendedCost
		}

		sumUnblended += item.UnblendedCost
		sumBlended += item.BlendedCost
		sumAmortized += item.AmortizedCost
		sumNetUnblended += item.NetUnblendedCost
		sumNetAmortized += item.NetAmortizedCost
		items = append(items, *item)
	}

	// When GroupBy is passed to GetCostAndUsage, AWS omits ResultByTime.Total.
	// In that case, compute totals from the aggregated line items.
	if totals.UnblendedCost == 0.0 && sumUnblended > 0.0 {
		totals.UnblendedCost = roundToTwo(sumUnblended)
	}
	if totals.BlendedCost == 0.0 && sumBlended > 0.0 {
		totals.BlendedCost = roundToTwo(sumBlended)
	}
	if totals.AmortizedCost == 0.0 && sumAmortized > 0.0 {
		totals.AmortizedCost = roundToTwo(sumAmortized)
	}
	if totals.NetUnblendedCost == 0.0 && sumNetUnblended > 0.0 {
		totals.NetUnblendedCost = roundToTwo(sumNetUnblended)
	}
	if totals.NetAmortizedCost == 0.0 && sumNetAmortized > 0.0 {
		totals.NetAmortizedCost = roundToTwo(sumNetAmortized)
	}

	// Fallback for totals if Blended or Amortized are still 0.0 but Unblended > 0.0
	if totals.BlendedCost == 0.0 && totals.UnblendedCost > 0.0 {
		totals.BlendedCost = totals.UnblendedCost
	}
	if totals.AmortizedCost == 0.0 && totals.UnblendedCost > 0.0 {
		totals.AmortizedCost = totals.UnblendedCost
	}

	var monthlyBreakdown *MonthlyBreakdown
	if len(monthKeys) > 1 || cfg.Monthly {
		sort.Strings(monthKeys)
		var buckets []MonthlyBucket
		totalsMap := make(map[string]float64, len(monthKeys))
		var grandTotal float64

		for _, mk := range monthKeys {
			info := monthInfoMap[mk]
			tot := roundToTwo(monthTotalsMap[mk])
			buckets = append(buckets, MonthlyBucket{
				Key:       mk,
				Label:     info.label,
				FullLabel: info.fullLabel,
				Total:     tot,
			})
			totalsMap[mk] = tot
			grandTotal += tot
		}

		var monthlyItems []MonthlyLineItem
		for _, item := range items {
			costs := make(map[string]float64, len(monthKeys))
			var itTot float64
			for _, mk := range monthKeys {
				c := roundToTwo(itemMonthlyCosts[item.Name][mk])
				costs[mk] = c
				itTot += c
			}
			itTot = roundToTwo(itTot)
			avg := 0.0
			if len(monthKeys) > 0 {
				avg = roundToTwo(itTot / float64(len(monthKeys)))
			}
			monthlyItems = append(monthlyItems, MonthlyLineItem{
				Name:    item.Name,
				Costs:   costs,
				Total:   itTot,
				Average: avg,
			})
		}

		monthlyBreakdown = &MonthlyBreakdown{
			Months:     buckets,
			Items:      monthlyItems,
			Totals:     totalsMap,
			GrandTotal: roundToTwo(grandTotal),
		}
	}

	var dailyBreakdown *DailyBreakdown
	if cfg.Daily && len(dayKeys) > 0 {
		sort.Strings(dayKeys)
		var buckets []DailyBucket
		var grandTotal float64

		for _, dk := range dayKeys {
			dTotal := roundToTwo(dayTotalsMap[dk])
			grandTotal += dTotal
			dayTime, _ := time.Parse(DateFormat, dk)
			dayOfWeek := dayTime.Format("Mon")
			topDriver := dayTopDriver[dk]

			buckets = append(buckets, DailyBucket{
				Date:            dk,
				DayOfWeek:       dayOfWeek,
				Total:           dTotal,
				TopDriverName:   topDriver.name,
				TopDriverAmount: roundToTwo(topDriver.amount),
			})
		}

		grandTotal = roundToTwo(grandTotal)
		avg := 0.0
		if len(buckets) > 0 {
			avg = roundToTwo(grandTotal / float64(len(buckets)))
		}

		dailyBreakdown = &DailyBreakdown{
			Days:       buckets,
			Totals:     dayTotalsMap,
			GrandTotal: grandTotal,
			Average:    avg,
		}
	}

	return totals, items, monthlyBreakdown, dailyBreakdown, detectedCurrency, nil
}

// GenerateMockReport generates realistic mock billing data matching specification benchmarks.
func GenerateMockReport(cfg *config.Config, now time.Time) (*CostReport, error) {
	dateRange, err := ResolveTimePeriod(now, cfg.MonthsBack, cfg.StartDate, cfg.EndDate, cfg.MTD)
	if err != nil {
		return nil, err
	}

	items := []LineItem{
		{Name: "Amazon DynamoDB", UnblendedCost: 5.51, BlendedCost: 5.51, AmortizedCost: 5.51, PrevUnblendedCost: 5.20, PrevBlendedCost: 5.20, PrevAmortizedCost: 5.20, HasPreviousCost: true, MoMDeltaAmount: 0.31, MoMDeltaPercent: 5.96},
		{Name: "AWS Config", UnblendedCost: 4.73, BlendedCost: 4.73, AmortizedCost: 4.73, PrevUnblendedCost: 4.80, PrevBlendedCost: 4.80, PrevAmortizedCost: 4.80, HasPreviousCost: true, MoMDeltaAmount: -0.07, MoMDeltaPercent: -1.46},
		{Name: "Amazon Cognito", UnblendedCost: 4.17, BlendedCost: 4.17, AmortizedCost: 4.17, PrevUnblendedCost: 4.15, PrevBlendedCost: 4.15, PrevAmortizedCost: 4.15, HasPreviousCost: true, MoMDeltaAmount: 0.02, MoMDeltaPercent: 0.48},
		{Name: "Tax", UnblendedCost: 2.89, BlendedCost: 2.89, AmortizedCost: 2.89, PrevUnblendedCost: 2.80, PrevBlendedCost: 2.80, PrevAmortizedCost: 2.80, HasPreviousCost: true, MoMDeltaAmount: 0.09, MoMDeltaPercent: 3.21},
		{Name: "AWS Key Management Service", UnblendedCost: 1.00, BlendedCost: 1.00, AmortizedCost: 1.00, PrevUnblendedCost: 1.00, PrevBlendedCost: 1.00, PrevAmortizedCost: 1.00, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "Amazon Simple Storage Service", UnblendedCost: 0.94, BlendedCost: 0.94, AmortizedCost: 0.94, PrevUnblendedCost: 0.88, PrevBlendedCost: 0.88, PrevAmortizedCost: 0.88, HasPreviousCost: true, MoMDeltaAmount: 0.06, MoMDeltaPercent: 6.82},
		{Name: "Amazon Route 53", UnblendedCost: 0.80, BlendedCost: 0.80, AmortizedCost: 0.80, PrevUnblendedCost: 0.80, PrevBlendedCost: 0.80, PrevAmortizedCost: 0.80, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "Amazon CloudWatch", UnblendedCost: 0.65, BlendedCost: 0.65, AmortizedCost: 0.65, PrevUnblendedCost: 0.70, PrevBlendedCost: 0.70, PrevAmortizedCost: 0.70, HasPreviousCost: true, MoMDeltaAmount: -0.05, MoMDeltaPercent: -7.14},
		{Name: "AWS Secrets Manager", UnblendedCost: 0.40, BlendedCost: 0.40, AmortizedCost: 0.40, PrevUnblendedCost: 0.40, PrevBlendedCost: 0.40, PrevAmortizedCost: 0.40, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "AWS Lambda", UnblendedCost: 0.25, BlendedCost: 0.25, AmortizedCost: 0.25, PrevUnblendedCost: 0.18, PrevBlendedCost: 0.18, PrevAmortizedCost: 0.18, HasPreviousCost: true, MoMDeltaAmount: 0.07, MoMDeltaPercent: 38.89},
		{Name: "Amazon Virtual Private Cloud", UnblendedCost: 0.12, BlendedCost: 0.12, AmortizedCost: 0.12, PrevUnblendedCost: 0.12, PrevBlendedCost: 0.12, PrevAmortizedCost: 0.12, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
		{Name: "Amazon Simple Notification Service", UnblendedCost: 0.06, BlendedCost: 0.06, AmortizedCost: 0.06, PrevUnblendedCost: 0.05, PrevBlendedCost: 0.05, PrevAmortizedCost: 0.05, HasPreviousCost: true, MoMDeltaAmount: 0.01, MoMDeltaPercent: 20.00},
		{Name: "AWS Step Functions", UnblendedCost: 0.04, BlendedCost: 0.04, AmortizedCost: 0.04, PrevUnblendedCost: 0.03, PrevBlendedCost: 0.03, PrevAmortizedCost: 0.03, HasPreviousCost: true, MoMDeltaAmount: 0.01, MoMDeltaPercent: 33.33},
		{Name: "AWS CloudTrail", UnblendedCost: 0.02, BlendedCost: 0.02, AmortizedCost: 0.02, PrevUnblendedCost: 0.02, PrevBlendedCost: 0.02, PrevAmortizedCost: 0.02, HasPreviousCost: true, MoMDeltaAmount: 0.00, MoMDeltaPercent: 0.00},
	}

	hasRequestedNet := false
	for _, m := range cfg.Defaults.Metrics {
		if strings.HasPrefix(strings.ToLower(m), "net") {
			hasRequestedNet = true
			break
		}
	}

	if hasRequestedNet {
		for i := range items {
			items[i].NetUnblendedCost = roundToTwo(items[i].UnblendedCost * 0.90)
			items[i].NetAmortizedCost = roundToTwo(items[i].AmortizedCost * 0.90)
			items[i].PrevNetUnblendedCost = roundToTwo(items[i].PrevUnblendedCost * 0.90)
			items[i].PrevNetAmortizedCost = roundToTwo(items[i].PrevAmortizedCost * 0.90)
		}
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

	if len(cfg.TagFilters) > 0 {
		var tagFiltered []LineItem
		filterStr := strings.ToLower(cfg.TagFilters.Format())
		for _, item := range filtered {
			if strings.Contains(filterStr, "prod") {
				if item.Name == "Tax" || item.Name == "Amazon Route 53" || item.Name == "Amazon Simple Notification Service" {
					continue
				}
			} else if strings.Contains(filterStr, "stage") || strings.Contains(filterStr, "dev") {
				if item.Name != "Amazon CloudWatch" && item.Name != "AWS Lambda" && item.Name != "AWS Step Functions" {
					continue
				}
			}
			tagFiltered = append(tagFiltered, item)
		}
		if len(tagFiltered) > 0 {
			filtered = tagFiltered
		}
	}

	if cfg.ExcludeCredits {
		var noCredits []LineItem
		for _, item := range filtered {
			if strings.EqualFold(item.Name, "Credit") || strings.EqualFold(item.Name, "Refund") || strings.Contains(strings.ToLower(item.Name), "credit") {
				continue
			}
			noCredits = append(noCredits, item)
		}
		filtered = noCredits
	}

	if len(cfg.ChargeTypes) > 0 {
		var chargeFiltered []LineItem
		for _, item := range filtered {
			matched := false
			for _, ct := range cfg.ChargeTypes {
				if strings.EqualFold(ct, "Tax") && strings.EqualFold(item.Name, "Tax") {
					matched = true
					break
				}
				if strings.EqualFold(ct, "Usage") && !strings.EqualFold(item.Name, "Tax") && !strings.Contains(strings.ToLower(item.Name), "credit") {
					matched = true
					break
				}
				if strings.EqualFold(ct, "Credit") && strings.Contains(strings.ToLower(item.Name), "credit") {
					matched = true
					break
				}
			}
			if matched {
				chargeFiltered = append(chargeFiltered, item)
			}
		}
		filtered = chargeFiltered
	}

	totals := TotalCosts{
		BlendedCost:       21.59,
		UnblendedCost:     21.58,
		AmortizedCost:     21.58,
		PrevUnblendedCost: 21.13,
		PrevBlendedCost:   21.14,
		PrevAmortizedCost: 21.13,
		MoMDeltaAmount:    0.45,
		MoMDeltaPercent:   2.13,
		HasPreviousCost:   cfg.CompareMoM,
	}
	if hasRequestedNet {
		totals.NetUnblendedCost = 19.42
		totals.NetAmortizedCost = 19.42
		totals.PrevNetUnblendedCost = 19.02
		totals.PrevNetAmortizedCost = 19.02
	}

	if len(cfg.Filters) > 0 || len(cfg.TagFilters) > 0 || len(cfg.ChargeTypes) > 0 || cfg.ExcludeCredits {
		var uCost, bCost, aCost, nuCost, naCost, prevUCost, prevBCost, prevACost, prevNUCost, prevNACost float64
		for _, it := range filtered {
			uCost += it.UnblendedCost
			bCost += it.BlendedCost
			aCost += it.AmortizedCost
			nuCost += it.NetUnblendedCost
			naCost += it.NetAmortizedCost
			if it.HasPreviousCost {
				prevUCost += it.PrevUnblendedCost
				prevBCost += it.PrevBlendedCost
				prevACost += it.PrevAmortizedCost
				prevNUCost += it.PrevNetUnblendedCost
				prevNACost += it.PrevNetAmortizedCost
			}
		}
		uCost = roundToTwo(uCost)
		bCost = roundToTwo(bCost)
		aCost = roundToTwo(aCost)
		nuCost = roundToTwo(nuCost)
		naCost = roundToTwo(naCost)
		prevUCost = roundToTwo(prevUCost)
		prevBCost = roundToTwo(prevBCost)
		prevACost = roundToTwo(prevACost)
		prevNUCost = roundToTwo(prevNUCost)
		prevNACost = roundToTwo(prevNACost)
		dAmount := roundToTwo(uCost - prevUCost)
		dPct := 0.0
		if prevUCost > 0 {
			dPct = roundToTwo((dAmount / prevUCost) * 100.0)
		}

		totals = TotalCosts{
			BlendedCost:          bCost,
			UnblendedCost:        uCost,
			AmortizedCost:        aCost,
			NetUnblendedCost:     nuCost,
			NetAmortizedCost:     naCost,
			PrevUnblendedCost:    prevUCost,
			PrevBlendedCost:      prevBCost,
			PrevAmortizedCost:    prevACost,
			PrevNetUnblendedCost: prevNUCost,
			PrevNetAmortizedCost: prevNACost,
			MoMDeltaAmount:       dAmount,
			MoMDeltaPercent:      dPct,
			HasPreviousCost:      cfg.CompareMoM,
		}
	}

	mockAccountID := resolveAccountName("715195480427", cfg.Accounts, cfg.AccountName)
	mockAccountName := cfg.AccountName
	if mockAccountName == "" && cfg.Accounts != nil {
		mockAccountName = cfg.Accounts["715195480427"]
	}
	if len(cfg.Groupings) > 1 {
		g1 := cfg.Groupings[1]
		for i := range filtered {
			k1 := "Production"
			if strings.EqualFold(g1.Type, "TAG") {
				k1 = g1.Key + "$Production"
			}
			keys := []string{filtered[i].Name, k1}
			filtered[i].Name = formatGroupKeys(keys, cfg.Groupings, cfg.Accounts)
		}
	} else if strings.EqualFold(cfg.Grouping.Key, "LINKED_ACCOUNT") {
		for i := range filtered {
			itemAccountName := ""
			if filtered[i].Name == "715195480427" {
				itemAccountName = cfg.AccountName
			}
			filtered[i].Name = resolveAccountName(filtered[i].Name, cfg.Accounts, itemAccountName)
		}
	}
	filtered = applyTopNAggregation(filtered, cfg.Defaults.Top, cfg.Grouping.Key)

	mockCurrency := cfg.Defaults.Currency
	if mockCurrency == "" {
		mockCurrency = "USD"
	}

	report := &CostReport{
		Metadata: ReportMetadata{
			AccountID:        mockAccountID,
			AccountName:      mockAccountName,
			BillingMonth:     dateRange.BillingMonthDesc,
			PeriodStart:      dateRange.Start.Format(DateFormat),
			PeriodEnd:        dateRange.EndInclusive.Format(DateFormat),
			AWSPeriodStart:   dateRange.Start.Format(DateFormat),
			AWSPeriodEnd:     dateRange.EndExclusiveAWS.Format(DateFormat),
			Currency:         mockCurrency,
			EstimatedOrFinal: "Finalized",
			ActiveServices:   len(filtered),
			GroupKey:         getReportGroupKey(cfg),
			TagFilter:        cfg.TagFilters.Format(),
			ChargeTypeFilter: strings.Join(cfg.ChargeTypes, ", "),
			ExcludeCredits:   cfg.ExcludeCredits,
		},
		Totals:        totals,
		Items:         filtered,
		HasComparison: cfg.CompareMoM,
	}

	if dateRange.IsEstimated {
		report.Metadata.EstimatedOrFinal = "Estimated"
	}

	if cfg.Forecast {
		startStr, endStr := ResolveForecastTimePeriod(now)
		remaining := 6.92
		projected := roundToTwo(totals.UnblendedCost + remaining)
		breached := false
		if cfg.Budget.FailOnThreshold > 0 && projected > cfg.Budget.FailOnThreshold {
			breached = true
		}
		report.Forecast = &CostForecast{
			TotalAmount:        projected,
			MeanAmount:         remaining,
			PeriodStart:        startStr,
			PeriodEnd:          endStr,
			PredictionInterval: 80,
			ThresholdBreached:  breached,
		}
	}

	if cfg.Daily {
		var buckets []DailyBucket
		var grandTotal float64
		totalsMap := make(map[string]float64)

		cur := dateRange.Start
		for !cur.After(dateRange.EndInclusive) {
			dk := cur.Format(DateFormat)
			dayTotal := 0.72
			grandTotal += dayTotal
			totalsMap[dk] = dayTotal

			buckets = append(buckets, DailyBucket{
				Date:            dk,
				DayOfWeek:       cur.Format("Mon"),
				Total:           dayTotal,
				TopDriverName:   "Amazon DynamoDB",
				TopDriverAmount: 0.18,
			})
			cur = cur.AddDate(0, 0, 1)
		}

		grandTotal = roundToTwo(grandTotal)
		avg := 0.0
		if len(buckets) > 0 {
			avg = roundToTwo(grandTotal / float64(len(buckets)))
		}

		report.DailyBreakdown = &DailyBreakdown{
			Days:       buckets,
			Totals:     totalsMap,
			GrandTotal: grandTotal,
			Average:    avg,
		}
	}

	// Generate monthly breakdown if multi-month or monthly mode requested
	if cfg.Monthly || cfg.MonthsBack > 1 {
		var months []MonthlyBucket
		curr := dateRange.Start
		for curr.Before(dateRange.EndExclusiveAWS) {
			mKey := curr.Format("2006-01")
			months = append(months, MonthlyBucket{
				Key:       mKey,
				Label:     curr.Format("Jan 06"),
				FullLabel: curr.Format("January 2006"),
			})
			curr = curr.AddDate(0, 1, 0)
		}

		if len(months) > 1 || cfg.Monthly {
			monthlyItems := make([]MonthlyLineItem, 0, len(filtered))
			monthTotals := make(map[string]float64, len(months))
			var grandTotal float64

			for itemIdx, it := range filtered {
				costs := make(map[string]float64, len(months))
				var itemTotal float64
				for mIdx, m := range months {
					variance := (float64(((itemIdx*5 + mIdx*7) % 9) - 4)) / 100.0
					mCost := roundToTwo(it.UnblendedCost * (1.0 + variance))
					costs[m.Key] = mCost
					itemTotal += mCost
					monthTotals[m.Key] += mCost
				}
				itemTotal = roundToTwo(itemTotal)
				avg := 0.0
				if len(months) > 0 {
					avg = roundToTwo(itemTotal / float64(len(months)))
				}
				monthlyItems = append(monthlyItems, MonthlyLineItem{
					Name:    it.Name,
					Costs:   costs,
					Total:   itemTotal,
					Average: avg,
				})
				grandTotal += itemTotal
			}

			for i := range months {
				months[i].Total = roundToTwo(monthTotals[months[i].Key])
			}

			report.MonthlyBreakdown = &MonthlyBreakdown{
				Months:     months,
				Items:      monthlyItems,
				Totals:     monthTotals,
				GrandTotal: roundToTwo(grandTotal),
			}

			if cfg.MonthsBack > 1 {
				for i := range report.Items {
					report.Items[i].UnblendedCost = monthlyItems[i].Total
					report.Items[i].BlendedCost = monthlyItems[i].Total
					report.Items[i].AmortizedCost = monthlyItems[i].Total
				}
				report.Totals.UnblendedCost = report.MonthlyBreakdown.GrandTotal
				report.Totals.BlendedCost = report.MonthlyBreakdown.GrandTotal
				report.Totals.AmortizedCost = report.MonthlyBreakdown.GrandTotal
			}
		}
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

	dateRange, err := ResolveTimePeriod(now, cfg.MonthsBack, cfg.StartDate, cfg.EndDate, cfg.MTD)
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

// isRetryableCostExplorerError checks whether an AWS Cost Explorer error is caused by rate limiting,
// throttling, or transient conditions that should be retried with exponential backoff.
// It prioritizes typed smithy.APIError matching before falling back to string inspection.
func isRetryableCostExplorerError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "LimitExceededException",
			"ThrottlingException",
			"Throttling",
			"RequestThrottledException",
			"RequestLimitExceeded",
			"TooManyRequestsException",
			"ProvisionedThroughputExceededException":
			return true
		}
	}

	errStr := err.Error()
	return strings.Contains(errStr, "LimitExceededException") ||
		strings.Contains(errStr, "Throttling") ||
		strings.Contains(errStr, "TooManyRequests") ||
		strings.Contains(errStr, "429")
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

		if isRetryableCostExplorerError(err) {
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

func (c *Client) getCostForecastWithRetry(ctx context.Context, input *costexplorer.GetCostForecastInput) (*costexplorer.GetCostForecastOutput, error) {
	maxRetries := 5
	backoff := 300 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		out, err := c.ceClient.GetCostForecast(ctx, input)
		if err == nil {
			return out, nil
		}

		if attempt == maxRetries {
			return nil, err
		}

		if isRetryableCostExplorerError(err) {
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

// ResolveForecastTimePeriod determines the future date range for AWS Cost Explorer GetCostForecast.
// The start date must be tomorrow or later, and the end date is the 1st of next month (or month after).
func ResolveForecastTimePeriod(now time.Time) (string, string) {
	tomorrow := now.AddDate(0, 0, 1)
	startStr := tomorrow.Format(DateFormat)

	year, month, _ := now.Date()
	firstOfNextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
	if tomorrow.After(firstOfNextMonth) || tomorrow.Equal(firstOfNextMonth) {
		firstOfNextNextMonth := time.Date(year, month+2, 1, 0, 0, 0, 0, time.UTC)
		return startStr, firstOfNextNextMonth.Format(DateFormat)
	}
	return startStr, firstOfNextMonth.Format(DateFormat)
}

func (c *Client) fetchCostForecast(ctx context.Context, cfg *config.Config, now time.Time, currentSpend float64) (*CostForecast, error) {
	startStr, endStr := ResolveForecastTimePeriod(now)
	predictionInterval := int32(80)

	input := &costexplorer.GetCostForecastInput{
		Granularity: cetypes.GranularityMonthly,
		Metric:      cetypes.MetricUnblendedCost,
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(startStr),
			End:   aws.String(endStr),
		},
		PredictionIntervalLevel: &predictionInterval,
		Filter:                  buildFilterExpression(cfg),
	}

	output, err := c.getCostForecastWithRetry(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("AWS Cost Explorer GetCostForecast failed: %w", err)
	}

	var remainingAmount float64
	if output.Total != nil && output.Total.Amount != nil {
		remainingAmount = parseFloat(*output.Total.Amount)
	}

	projectedTotal := roundToTwo(currentSpend + remainingAmount)
	forecast := &CostForecast{
		TotalAmount:        projectedTotal,
		MeanAmount:         roundToTwo(remainingAmount),
		PeriodStart:        startStr,
		PeriodEnd:          endStr,
		PredictionInterval: predictionInterval,
	}

	if cfg.Budget.FailOnThreshold > 0 && projectedTotal > cfg.Budget.FailOnThreshold {
		forecast.ThresholdBreached = true
	}

	return forecast, nil
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
		other.NetUnblendedCost += rem.NetUnblendedCost
		other.NetAmortizedCost += rem.NetAmortizedCost
		if rem.HasPreviousCost {
			hasPrev = true
			other.PrevUnblendedCost += rem.PrevUnblendedCost
			other.PrevBlendedCost += rem.PrevBlendedCost
			other.PrevAmortizedCost += rem.PrevAmortizedCost
			other.PrevNetUnblendedCost += rem.PrevNetUnblendedCost
			other.PrevNetAmortizedCost += rem.PrevNetAmortizedCost
		}
	}

	other.UnblendedCost = roundToTwo(other.UnblendedCost)
	other.BlendedCost = roundToTwo(other.BlendedCost)
	other.AmortizedCost = roundToTwo(other.AmortizedCost)
	other.NetUnblendedCost = roundToTwo(other.NetUnblendedCost)
	other.NetAmortizedCost = roundToTwo(other.NetAmortizedCost)
	if hasPrev {
		other.HasPreviousCost = true
		other.PrevUnblendedCost = roundToTwo(other.PrevUnblendedCost)
		other.PrevBlendedCost = roundToTwo(other.PrevBlendedCost)
		other.PrevAmortizedCost = roundToTwo(other.PrevAmortizedCost)
		other.PrevNetUnblendedCost = roundToTwo(other.PrevNetUnblendedCost)
		other.PrevNetAmortizedCost = roundToTwo(other.PrevNetAmortizedCost)
		other.MoMDeltaAmount = roundToTwo(other.UnblendedCost - other.PrevUnblendedCost)
		if other.PrevUnblendedCost > 0 {
			other.MoMDeltaPercent = roundToTwo(((other.UnblendedCost - other.PrevUnblendedCost) / other.PrevUnblendedCost) * 100.0)
		}
	}

	return append(topItems, other)
}

func resolveAccountName(account string, accounts map[string]string, explicitName string) string {
	if explicitName != "" {
		if strings.Contains(explicitName, account) {
			return explicitName
		}
		if account != "" {
			return fmt.Sprintf("%s (%s)", account, explicitName)
		}
		return explicitName
	}
	if len(accounts) == 0 {
		return account
	}
	if name, ok := accounts[account]; ok && name != "" {
		return fmt.Sprintf("%s (%s)", account, name)
	}
	return account
}

func getReportGroupKey(cfg *config.Config) string {
	if len(cfg.Groupings) > 1 {
		var gKeys []string
		for _, g := range cfg.Groupings {
			if strings.EqualFold(g.Type, "TAG") {
				gKeys = append(gKeys, "TAG:"+g.Key)
			} else {
				gKeys = append(gKeys, g.Key)
			}
		}
		return strings.Join(gKeys, ",")
	}
	if strings.EqualFold(cfg.Grouping.Type, "TAG") {
		return "TAG:" + cfg.Grouping.Key
	}
	return cfg.Grouping.Key
}

func formatGroupKey(key string, g config.GroupingConfig, accounts map[string]string) (string, bool) {
	if strings.EqualFold(g.Type, "TAG") {
		tagVal := key
		if strings.HasPrefix(tagVal, g.Key+"$") {
			tagVal = strings.TrimPrefix(tagVal, g.Key+"$")
		} else {
			parts := strings.SplitN(key, "$", 2)
			if len(parts) == 2 {
				tagVal = parts[1]
			}
		}
		if tagVal == "" {
			tagVal = "Untagged"
		}
		return tagVal, true
	}

	val := key
	if strings.EqualFold(g.Key, "LINKED_ACCOUNT") && accounts != nil {
		if alias, ok := accounts[key]; ok {
			val = fmt.Sprintf("%s (%s)", alias, key)
		}
	}
	if val == "" {
		val = "Unknown"
	}
	return val, false
}

func formatGroupKeys(keys []string, groupings []config.GroupingConfig, accounts map[string]string) string {
	if len(keys) == 0 {
		return "Unknown"
	}
	if len(keys) == 1 {
		val, isTag := formatGroupKey(keys[0], groupings[0], accounts)
		if isTag && val == "Untagged" {
			return "Untagged / Other"
		}
		return val
	}

	val0, isTag0 := formatGroupKey(keys[0], groupings[0], accounts)
	var val1 string
	var isTag1 bool
	if len(groupings) > 1 {
		val1, isTag1 = formatGroupKey(keys[1], groupings[1], accounts)
	} else {
		val1 = keys[1]
	}

	if !isTag0 && isTag1 {
		return fmt.Sprintf("%s [%s=%s]", val0, groupings[1].Key, val1)
	}
	if isTag0 && !isTag1 {
		return fmt.Sprintf("%s [%s=%s]", val1, groupings[0].Key, val0)
	}
	if isTag0 && isTag1 {
		return fmt.Sprintf("[%s=%s] [%s=%s]", groupings[0].Key, val0, groupings[1].Key, val1)
	}
	return fmt.Sprintf("%s - %s", val0, val1)
}

func buildFilterExpression(cfg *config.Config) *cetypes.Expression {
	var andExpressions []cetypes.Expression

	if len(cfg.TagFilters) > 0 {
		var tagExprs []cetypes.Expression
		var sortedKeys []string
		for k := range cfg.TagFilters {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)

		for _, k := range sortedKeys {
			vals := cfg.TagFilters[k]
			tagExprs = append(tagExprs, cetypes.Expression{
				Tags: &cetypes.TagValues{
					Key:    aws.String(k),
					Values: vals,
				},
			})
		}

		if len(tagExprs) == 1 {
			andExpressions = append(andExpressions, tagExprs[0])
		} else if len(tagExprs) > 1 {
			andExpressions = append(andExpressions, cetypes.Expression{
				And: tagExprs,
			})
		}
	}

	if len(cfg.ChargeTypes) > 0 {
		andExpressions = append(andExpressions, cetypes.Expression{
			Dimensions: &cetypes.DimensionValues{
				Key:    cetypes.DimensionRecordType,
				Values: cfg.ChargeTypes,
			},
		})
	} else if cfg.ExcludeCredits {
		andExpressions = append(andExpressions, cetypes.Expression{
			Not: &cetypes.Expression{
				Dimensions: &cetypes.DimensionValues{
					Key:    cetypes.DimensionRecordType,
					Values: []string{"Credit", "Refund"},
				},
			},
		})
	}

	if len(andExpressions) == 1 {
		return &andExpressions[0]
	} else if len(andExpressions) > 1 {
		return &cetypes.Expression{
			And: andExpressions,
		}
	}
	return nil
}
