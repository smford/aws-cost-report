package awsclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

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
