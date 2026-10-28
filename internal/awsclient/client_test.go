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
