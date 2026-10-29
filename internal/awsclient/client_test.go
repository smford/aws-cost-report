package awsclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/smford/aws-cost-report/internal/config"
)

type mockCEClient struct {
	pages []*costexplorer.GetCostAndUsageOutput
	calls int
}

func (m *mockCEClient) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	if m.calls < len(m.pages) {
		out := m.pages[m.calls]
		m.calls++
		return out, nil
	}
	return &costexplorer.GetCostAndUsageOutput{}, nil
}

func (m *mockCEClient) GetDimensionValues(ctx context.Context, params *costexplorer.GetDimensionValuesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	return &costexplorer.GetDimensionValuesOutput{
		DimensionValues: []cetypes.DimensionValuesWithAttributes{
			{Value: aws.String("Amazon DynamoDB")},
			{Value: aws.String("AWS Lambda")},
		},
	}, nil
}

type mockSTSClient struct {
	accountID string
}

func (m *mockSTSClient) GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Account: aws.String(m.accountID),
	}, nil
}

type mockIAMClient struct {
	aliases []string
	err     error
}

func (m *mockIAMClient) ListAccountAliases(ctx context.Context, params *iam.ListAccountAliasesInput, optFns ...func(*iam.Options)) (*iam.ListAccountAliasesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &iam.ListAccountAliasesOutput{
		AccountAliases: m.aliases,
	}, nil
}

func TestClientPaginationAndAggregation(t *testing.T) {
	page1Token := "token-page-2"
	page1 := &costexplorer.GetCostAndUsageOutput{
		NextPageToken: &page1Token,
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
					"BlendedCost":   {Amount: aws.String("10.05"), Unit: aws.String("USD")},
					"AmortizedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
				},
				Groups: []cetypes.Group{
					{
						Keys: []string{"Amazon DynamoDB"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("6.00")},
							"BlendedCost":   {Amount: aws.String("6.02")},
							"AmortizedCost": {Amount: aws.String("6.00")},
						},
					},
				},
			},
		},
	}

	page2 := &costexplorer.GetCostAndUsageOutput{
		NextPageToken: nil,
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("5.50"), Unit: aws.String("USD")},
					"BlendedCost":   {Amount: aws.String("5.50"), Unit: aws.String("USD")},
					"AmortizedCost": {Amount: aws.String("5.50"), Unit: aws.String("USD")},
				},
				Groups: []cetypes.Group{
					{
						Keys: []string{"AWS Config"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("5.50")},
							"BlendedCost":   {Amount: aws.String("5.50")},
							"AmortizedCost": {Amount: aws.String("5.50")},
						},
					},
					{
						// Same service across pages should aggregate
						Keys: []string{"Amazon DynamoDB"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("4.00")},
							"BlendedCost":   {Amount: aws.String("4.03")},
							"AmortizedCost": {Amount: aws.String("4.00")},
						},
					},
				},
			},
		},
	}

	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page1, page2}}
	stsMock := &mockSTSClient{accountID: "123456789012"}

	client := NewClient(ceMock, stsMock)
	cfg := config.DefaultConfig()

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if report.Metadata.AccountID != "123456789012" {
		t.Errorf("expected account ID 123456789012, got %s", report.Metadata.AccountID)
	}
	if report.Totals.UnblendedCost != 15.50 {
		t.Errorf("expected total unblended 15.50, got %f", report.Totals.UnblendedCost)
	}
	if len(report.Items) != 2 {
		t.Fatalf("expected 2 aggregated line items, got %d", len(report.Items))
	}
	if report.Items[0].Name != "Amazon DynamoDB" || report.Items[0].UnblendedCost != 10.00 {
		t.Errorf("expected first item DynamoDB with 10.00, got %s: %f", report.Items[0].Name, report.Items[0].UnblendedCost)
	}
	if report.Items[1].Name != "AWS Config" || report.Items[1].UnblendedCost != 5.50 {
		t.Errorf("expected second item AWS Config with 5.50, got %s: %f", report.Items[1].Name, report.Items[1].UnblendedCost)
	}
}

// TestClientAggregationWhenTotalIsNil verifies that when AWS Cost Explorer omits ResultByTime.Total
// (which AWS does whenever GroupBy is specified), totals are computed accurately from groups.
func TestClientAggregationWhenTotalIsNil(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		NextPageToken: nil,
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: nil, // Total omitted by AWS
				Groups: []cetypes.Group{
					{
						Keys: []string{"Amazon DynamoDB"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("12.50")},
							"BlendedCost":   {Amount: aws.String("12.50")},
							"AmortizedCost": {Amount: aws.String("12.50")},
						},
					},
					{
						Keys: []string{"AWS Lambda"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("3.25")},
							"BlendedCost":   {Amount: aws.String("3.25")},
							"AmortizedCost": {Amount: aws.String("3.25")},
						},
					},
				},
			},
		},
	}

	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "123456789012"}

	client := NewClient(ceMock, stsMock)
	cfg := config.DefaultConfig()

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	expectedTotal := 15.75
	if report.Totals.UnblendedCost != expectedTotal {
		t.Errorf("expected UnblendedCost %f, got %f", expectedTotal, report.Totals.UnblendedCost)
	}
	if report.Totals.BlendedCost != expectedTotal {
		t.Errorf("expected BlendedCost %f, got %f", expectedTotal, report.Totals.BlendedCost)
	}
	if report.Totals.AmortizedCost != expectedTotal {
		t.Errorf("expected AmortizedCost %f, got %f", expectedTotal, report.Totals.AmortizedCost)
	}
}

// TestGenerateReportFilteredTotals verifies that when filters are specified,
// report.Totals reflects only the filtered items rather than the entire account total.
func TestGenerateReportFilteredTotals(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		NextPageToken: nil,
		ResultsByTime: []cetypes.ResultByTime{
			{
				Groups: []cetypes.Group{
					{
						Keys: []string{"Amazon DynamoDB"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("10.00")},
							"BlendedCost":   {Amount: aws.String("10.00")},
							"AmortizedCost": {Amount: aws.String("10.00")},
						},
					},
					{
						Keys: []string{"AWS Lambda"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("5.00")},
							"BlendedCost":   {Amount: aws.String("5.00")},
							"AmortizedCost": {Amount: aws.String("5.00")},
						},
					},
				},
			},
		},
	}

	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "123456789012"}

	client := NewClient(ceMock, stsMock)
	cfg := config.DefaultConfig()
	cfg.Filters = []string{"Lambda"}

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if len(report.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(report.Items))
	}
	if report.Totals.UnblendedCost != 5.00 {
		t.Errorf("expected filtered UnblendedCost 5.00, got %f", report.Totals.UnblendedCost)
	}
	if report.Totals.BlendedCost != 5.00 {
		t.Errorf("expected filtered BlendedCost 5.00, got %f", report.Totals.BlendedCost)
	}
	if report.Totals.AmortizedCost != 5.00 {
		t.Errorf("expected filtered AmortizedCost 5.00, got %f", report.Totals.AmortizedCost)
	}
}

// TestCurrencyAutoDetectionFromAWS verifies that when AWS Cost Explorer returns a non-USD currency
// in the MetricValue.Unit field (e.g. EUR), the tool automatically adopts it without manual config.
func TestCurrencyAutoDetectionFromAWS(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		NextPageToken: nil,
		ResultsByTime: []cetypes.ResultByTime{
			{
				Groups: []cetypes.Group{
					{
						Keys: []string{"Amazon DynamoDB"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("EUR")},
							"BlendedCost":   {Amount: aws.String("10.00"), Unit: aws.String("EUR")},
							"AmortizedCost": {Amount: aws.String("10.00"), Unit: aws.String("EUR")},
						},
					},
				},
			},
		},
	}

	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "123456789012"}

	client := NewClient(ceMock, stsMock)
	cfg := config.DefaultConfig() // Defaults.Currency is "USD", but CurrencyExplicitlySet is false

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if report.Metadata.Currency != "EUR" {
		t.Errorf("expected auto-detected currency EUR, got %s", report.Metadata.Currency)
	}
	if report.Metadata.CurrencySymbol() != "€" {
		t.Errorf("expected currency symbol €, got %s", report.Metadata.CurrencySymbol())
	}
}

// TestCurrencyExplicitOverride verifies that an explicit user override takes precedence over auto-detection.
func TestCurrencyExplicitOverride(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		NextPageToken: nil,
		ResultsByTime: []cetypes.ResultByTime{
			{
				Groups: []cetypes.Group{
					{
						Keys: []string{"Amazon DynamoDB"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("EUR")},
						},
					},
				},
			},
		},
	}

	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "123456789012"}

	client := NewClient(ceMock, stsMock)
	cfg := config.DefaultConfig()
	cfg.Defaults.Currency = "GBP"
	cfg.CurrencyExplicitlySet = true

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if report.Metadata.Currency != "GBP" {
		t.Errorf("expected explicit currency GBP, got %s", report.Metadata.Currency)
	}
	if report.Metadata.CurrencySymbol() != "£" {
		t.Errorf("expected currency symbol £, got %s", report.Metadata.CurrencySymbol())
	}
}

// TestCurrencySymbolMapping checks standard ISO symbol lookups.
func TestCurrencySymbolMapping(t *testing.T) {
	cases := map[string]string{
		"USD": "$",
		"EUR": "€",
		"GBP": "£",
		"JPY": "¥",
		"AUD": "A$",
		"CAD": "CA$",
		"CHF": "CHF",
		"BRL": "R$",
		"SEK": "kr",
		"XYZ": "XYZ",
	}
	for code, expected := range cases {
		got := CurrencySymbol(code)
		if got != expected {
			t.Errorf("CurrencySymbol(%q) = %q; expected %q", code, got, expected)
		}
	}
}

func TestGenerateMockReport(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.CompareMoM = true

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	if report.Metadata.AccountID != "715195480427" {
		t.Errorf("expected mock account 715195480427, got %s", report.Metadata.AccountID)
	}
	if report.Totals.UnblendedCost != 21.58 {
		t.Errorf("expected 21.58, got %f", report.Totals.UnblendedCost)
	}
	if !report.HasComparison {
		t.Errorf("expected comparison enabled")
	}
}

func TestFilterByName(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.Filters = []string{"DynamoDB", "Cognito"}

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport with filter failed: %v", err)
	}

	if len(report.Items) != 2 {
		t.Fatalf("expected exactly 2 filtered items, got %d", len(report.Items))
	}
	if report.Items[0].Name != "Amazon DynamoDB" {
		t.Errorf("expected Amazon DynamoDB, got %s", report.Items[0].Name)
	}
	if report.Items[1].Name != "Amazon Cognito" {
		t.Errorf("expected Amazon Cognito, got %s", report.Items[1].Name)
	}
	if report.Totals.UnblendedCost != 9.68 {
		t.Errorf("expected filtered mock total 9.68, got %f", report.Totals.UnblendedCost)
	}
}

func TestFilterByOfficialNameAndAlias(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	// Test full official name "Amazon DynamoDB" and alias "s3"
	cfg.Filters = []string{"Amazon DynamoDB", "s3"}

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport with filter failed: %v", err)
	}

	if len(report.Items) != 2 {
		t.Fatalf("expected exactly 2 filtered items, got %d", len(report.Items))
	}
	if report.Items[0].Name != "Amazon DynamoDB" {
		t.Errorf("expected Amazon DynamoDB, got %s", report.Items[0].Name)
	}
	if report.Items[1].Name != "Amazon Simple Storage Service" {
		t.Errorf("expected Amazon Simple Storage Service (matched via s3 alias), got %s", report.Items[1].Name)
	}
}

func TestFilterByTag(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.CompareMoM = true
	cfg.TagFilters["Environment"] = []string{"Production"}

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport with tag filter failed: %v", err)
	}

	if report.Metadata.TagFilter != "Environment=Production" {
		t.Errorf("expected TagFilter Environment=Production, got %s", report.Metadata.TagFilter)
	}
	if report.Totals.UnblendedCost <= 0 {
		t.Errorf("expected positive total unblended cost, got %f", report.Totals.UnblendedCost)
	}
	if len(report.Items) == 0 {
		t.Errorf("expected filtered items for tag filter")
	}
}

func TestListServices(t *testing.T) {
	client := NewClient(nil, nil)
	cfg := config.DefaultConfig()
	cfg.MockMode = true

	services, err := client.ListServices(context.Background(), cfg, time.Now())
	if err != nil {
		t.Fatalf("ListServices failed: %v", err)
	}
	if len(services) == 0 {
		t.Fatalf("expected non-empty services list")
	}

	// Verify key official AWS Cost Explorer names exist
	foundDynamo := false
	foundS3 := false
	for _, s := range services {
		if s == "Amazon DynamoDB" {
			foundDynamo = true
		}
		if s == "Amazon Simple Storage Service" {
			foundS3 = true
		}
	}
	if !foundDynamo {
		t.Errorf("expected 'Amazon DynamoDB' in services list")
	}
	if !foundS3 {
		t.Errorf("expected 'Amazon Simple Storage Service' in services list")
	}
}

type throttlingCEClient struct {
	attempts int
}

func (t *throttlingCEClient) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	t.attempts++
	if t.attempts < 2 {
		return nil, errors.New("LimitExceededException: Rate exceeded")
	}
	return &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("5.00"), Unit: aws.String("USD")},
				},
			},
		},
	}, nil
}

func (t *throttlingCEClient) GetDimensionValues(ctx context.Context, params *costexplorer.GetDimensionValuesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	return &costexplorer.GetDimensionValuesOutput{}, nil
}

func TestThrottlingRetry(t *testing.T) {
	ceMock := &throttlingCEClient{}
	stsMock := &mockSTSClient{accountID: "111122223333"}
	client := NewClient(ceMock, stsMock)

	input := &costexplorer.GetCostAndUsageInput{}
	out, err := client.getCostAndUsageWithRetry(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful retry after throttling, got error: %v", err)
	}
	if out == nil || len(out.ResultsByTime) == 0 {
		t.Fatalf("expected valid results from retry output")
	}
	if ceMock.attempts != 2 {
		t.Errorf("expected exactly 2 attempts, got %d", ceMock.attempts)
	}
}

type smithyThrottlingCEClient struct {
	attempts int
}

func (s *smithyThrottlingCEClient) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	s.attempts++
	if s.attempts < 2 {
		return nil, &smithy.GenericAPIError{
			Code:    "LimitExceededException",
			Message: "Rate exceeded by AWS Cost Explorer",
			Fault:   smithy.FaultClient,
		}
	}
	return &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
				},
			},
		},
	}, nil
}

func (s *smithyThrottlingCEClient) GetDimensionValues(ctx context.Context, params *costexplorer.GetDimensionValuesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	return &costexplorer.GetDimensionValuesOutput{}, nil
}

func TestSmithyAPIErrorRetry(t *testing.T) {
	ceMock := &smithyThrottlingCEClient{}
	stsMock := &mockSTSClient{accountID: "111122223333"}
	client := NewClient(ceMock, stsMock)

	input := &costexplorer.GetCostAndUsageInput{}
	out, err := client.getCostAndUsageWithRetry(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful retry with typed smithy error, got error: %v", err)
	}
	if out == nil || len(out.ResultsByTime) == 0 {
		t.Fatalf("expected valid results from retry output")
	}
	if ceMock.attempts != 2 {
		t.Errorf("expected exactly 2 attempts, got %d", ceMock.attempts)
	}
}

func TestIsRetryableCostExplorerError(t *testing.T) {
	if isRetryableCostExplorerError(nil) {
		t.Errorf("nil error should not be retryable")
	}

	// Typed smithy errors
	retryableCodes := []string{
		"LimitExceededException",
		"ThrottlingException",
		"Throttling",
		"RequestThrottledException",
		"RequestLimitExceeded",
		"TooManyRequestsException",
		"ProvisionedThroughputExceededException",
	}
	for _, code := range retryableCodes {
		err := &smithy.GenericAPIError{Code: code, Message: "error message"}
		if !isRetryableCostExplorerError(err) {
			t.Errorf("expected code %s to be retryable", code)
		}
	}

	// Non-retryable typed error
	nonRetryableErr := &smithy.GenericAPIError{Code: "ValidationException", Message: "invalid param"}
	if isRetryableCostExplorerError(nonRetryableErr) {
		t.Errorf("ValidationException should not be retryable")
	}

	// String fallback error
	fallbackErr := errors.New("HTTP 429 TooManyRequests")
	if !isRetryableCostExplorerError(fallbackErr) {
		t.Errorf("fallback 429 error should be retryable")
	}
}

func TestTopNAggregation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.Defaults.Top = 3

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	// 3 top items + 1 "Other Services" item = 4
	if len(report.Items) != 4 {
		t.Fatalf("expected 4 items (3 top + Other), got %d", len(report.Items))
	}

	if report.Items[3].Name != "Other Services" {
		t.Errorf("expected last item to be 'Other Services', got %s", report.Items[3].Name)
	}

	// Sum of all items should match unblended total cost
	var sum float64
	for _, item := range report.Items {
		sum += item.UnblendedCost
	}
	sum = roundToTwo(sum)
	if sum != report.Totals.UnblendedCost {
		t.Errorf("expected sum of items (%.2f) to equal total unblended cost (%.2f)", sum, report.Totals.UnblendedCost)
	}
}

func TestAccountMapping(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.Accounts = map[string]string{
		"715195480427": "Production Core",
	}

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	expected := "715195480427 (Production Core)"
	if report.Metadata.AccountID != expected {
		t.Errorf("expected account ID %q, got %q", expected, report.Metadata.AccountID)
	}
}

func TestAccountNameDirectInMockReport(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MockMode = true
	cfg.AccountName = "FinOps Analytics"

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	expected := "715195480427 (FinOps Analytics)"
	if report.Metadata.AccountID != expected {
		t.Errorf("expected account ID %q, got %q", expected, report.Metadata.AccountID)
	}
	if report.Metadata.AccountName != "FinOps Analytics" {
		t.Errorf("expected AccountName 'FinOps Analytics', got %q", report.Metadata.AccountName)
	}
}

func TestAutoDetectAccountAliasFromIAM(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
				},
			},
		},
	}
	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "111222333444"}
	iamMock := &mockIAMClient{aliases: []string{"acme-corp-prod"}}

	client := NewClient(ceMock, stsMock, iamMock)
	cfg := config.DefaultConfig()

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	expectedID := "111222333444 (acme-corp-prod)"
	if report.Metadata.AccountID != expectedID {
		t.Errorf("expected AccountID %q, got %q", expectedID, report.Metadata.AccountID)
	}
	if report.Metadata.AccountName != "acme-corp-prod" {
		t.Errorf("expected AccountName 'acme-corp-prod', got %q", report.Metadata.AccountName)
	}
}

func TestIAMListAccountAliasesErrorGraceful(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
				},
			},
		},
	}
	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "111222333444"}
	iamMock := &mockIAMClient{err: errors.New("AccessDeniedException: not authorized to perform iam:ListAccountAliases")}

	client := NewClient(ceMock, stsMock, iamMock)
	cfg := config.DefaultConfig()

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport should not fail when IAM errors: %v", err)
	}

	expectedID := "111222333444"
	if report.Metadata.AccountID != expectedID {
		t.Errorf("expected AccountID %q, got %q", expectedID, report.Metadata.AccountID)
	}
	if report.Metadata.AccountName != "" {
		t.Errorf("expected empty AccountName, got %q", report.Metadata.AccountName)
	}
}

func TestExplicitNameOverridesIAMAlias(t *testing.T) {
	page := &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				Total: map[string]cetypes.MetricValue{
					"UnblendedCost": {Amount: aws.String("10.00"), Unit: aws.String("USD")},
				},
			},
		},
	}
	ceMock := &mockCEClient{pages: []*costexplorer.GetCostAndUsageOutput{page}}
	stsMock := &mockSTSClient{accountID: "111222333444"}
	iamMock := &mockIAMClient{aliases: []string{"iam-alias-ignored"}}

	client := NewClient(ceMock, stsMock, iamMock)
	cfg := config.DefaultConfig()
	cfg.AccountName = "Explicit CLI Name"

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := client.GenerateReport(context.Background(), cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	expectedID := "111222333444 (Explicit CLI Name)"
	if report.Metadata.AccountID != expectedID {
		t.Errorf("expected AccountID %q, got %q", expectedID, report.Metadata.AccountID)
	}
	if report.Metadata.AccountName != "Explicit CLI Name" {
		t.Errorf("expected AccountName 'Explicit CLI Name', got %q", report.Metadata.AccountName)
	}
}

func TestMonthlyBreakdownWithTopNAggregation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Monthly = true
	cfg.MonthsBack = 6
	cfg.Defaults.Top = 3

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	if report.MonthlyBreakdown == nil {
		t.Fatalf("expected MonthlyBreakdown to be populated")
	}
	if len(report.MonthlyBreakdown.Months) != 6 {
		t.Errorf("expected 6 months, got %d", len(report.MonthlyBreakdown.Months))
	}

	// With Top 3, items should be Top 3 + "Other Services"
	if len(report.MonthlyBreakdown.Items) != 4 {
		t.Fatalf("expected 4 monthly items (3 top + 1 Other Services), got %d", len(report.MonthlyBreakdown.Items))
	}
	lastItem := report.MonthlyBreakdown.Items[len(report.MonthlyBreakdown.Items)-1]
	if lastItem.Name != "Other Services" {
		t.Errorf("expected last item to be 'Other Services', got %q", lastItem.Name)
	}
	if lastItem.Total <= 0 {
		t.Errorf("expected Other Services total > 0, got %f", lastItem.Total)
	}
}

func TestFormatGroupKeys(t *testing.T) {
	accounts := map[string]string{
		"123456789012": "Prod Core",
	}

	// 1. Single dimension (Service)
	serviceGrouping := []config.GroupingConfig{{Type: "DIMENSION", Key: "SERVICE"}}
	if got := formatGroupKeys([]string{"Amazon DynamoDB"}, serviceGrouping, accounts); got != "Amazon DynamoDB" {
		t.Errorf("expected 'Amazon DynamoDB', got %q", got)
	}

	// 2. Single tag (Tagged and Untagged)
	tagGrouping := []config.GroupingConfig{{Type: "TAG", Key: "Environment"}}
	if got := formatGroupKeys([]string{"Environment$Production"}, tagGrouping, accounts); got != "Production" {
		t.Errorf("expected 'Production', got %q", got)
	}
	if got := formatGroupKeys([]string{"Environment$"}, tagGrouping, accounts); got != "Untagged / Other" {
		t.Errorf("expected 'Untagged / Other', got %q", got)
	}

	// 3. Two dimensions: Service + Tag
	twoDimGrouping := []config.GroupingConfig{
		{Type: "DIMENSION", Key: "SERVICE"},
		{Type: "TAG", Key: "Environment"},
	}
	if got := formatGroupKeys([]string{"Amazon EC2", "Environment$Production"}, twoDimGrouping, accounts); got != "Amazon EC2 [Environment=Production]" {
		t.Errorf("expected 'Amazon EC2 [Environment=Production]', got %q", got)
	}
	if got := formatGroupKeys([]string{"Amazon EC2", "Environment$"}, twoDimGrouping, accounts); got != "Amazon EC2 [Environment=Untagged]" {
		t.Errorf("expected 'Amazon EC2 [Environment=Untagged]', got %q", got)
	}

	// 4. Two dimensions: Linked Account + Service
	accountServiceGrouping := []config.GroupingConfig{
		{Type: "DIMENSION", Key: "LINKED_ACCOUNT"},
		{Type: "DIMENSION", Key: "SERVICE"},
	}
	if got := formatGroupKeys([]string{"123456789012", "Amazon S3"}, accountServiceGrouping, accounts); got != "Prod Core (123456789012) - Amazon S3" {
		t.Errorf("expected 'Prod Core (123456789012) - Amazon S3', got %q", got)
	}
	if got := formatGroupKeys([]string{"999888777666", "Amazon S3"}, accountServiceGrouping, accounts); got != "999888777666 - Amazon S3" {
		t.Errorf("expected '999888777666 - Amazon S3', got %q", got)
	}

	// 5. Two tags: Environment + Team
	twoTagsGrouping := []config.GroupingConfig{
		{Type: "TAG", Key: "Environment"},
		{Type: "TAG", Key: "Team"},
	}
	if got := formatGroupKeys([]string{"Environment$Production", "Team$Core"}, twoTagsGrouping, accounts); got != "[Environment=Production] [Team=Core]" {
		t.Errorf("expected '[Environment=Production] [Team=Core]', got %q", got)
	}
}

func TestTwoDimensionalGroupByMockReport(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Groupings = []config.GroupingConfig{
		{Type: "DIMENSION", Key: "SERVICE"},
		{Type: "TAG", Key: "Environment"},
	}
	cfg.Grouping = cfg.Groupings[0]

	refTime := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	report, err := GenerateMockReport(cfg, refTime)
	if err != nil {
		t.Fatalf("GenerateMockReport failed: %v", err)
	}

	if report.Metadata.GroupKey != "SERVICE,TAG:Environment" {
		t.Errorf("expected GroupKey 'SERVICE,TAG:Environment', got %q", report.Metadata.GroupKey)
	}
	if len(report.Items) == 0 {
		t.Fatalf("expected items in report")
	}
	firstItem := report.Items[0].Name
	if !strings.Contains(firstItem, "[Environment=Production]") {
		t.Errorf("expected item to contain [Environment=Production], got %q", firstItem)
	}
}
