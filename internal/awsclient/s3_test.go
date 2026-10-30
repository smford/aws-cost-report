package awsclient

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type mockS3Client struct {
	putCalls      int
	headCalls     int
	lastBucket    string
	lastKey       string
	lastBody      string
	lastType      string
	lastRegion    string
	bucketRegions map[string]string
	putErr        error
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.putCalls++
	m.lastBucket = *params.Bucket
	m.lastKey = *params.Key
	if params.ContentType != nil {
		m.lastType = *params.ContentType
	}
	if params.Body != nil {
		data, _ := io.ReadAll(params.Body)
		m.lastBody = string(data)
	}
	var opts s3.Options
	for _, fn := range optFns {
		fn(&opts)
	}
	m.lastRegion = opts.Region

	if m.putErr != nil {
		return nil, m.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	m.headCalls++
	reg := "us-east-1"
	if m.bucketRegions != nil && m.bucketRegions[*params.Bucket] != "" {
		reg = m.bucketRegions[*params.Bucket]
	}
	return &s3.HeadBucketOutput{
		BucketRegion: aws.String(reg),
	}, nil
}

func TestParseS3URI(t *testing.T) {
	// Valid
	b, k, err := ParseS3URI("s3://my-billing-bucket/reports/oct-2026.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != "my-billing-bucket" {
		t.Errorf("expected bucket 'my-billing-bucket', got %s", b)
	}
	if k != "reports/oct-2026.md" {
		t.Errorf("expected key 'reports/oct-2026.md', got %s", k)
	}

	// Invalid
	_, _, err = ParseS3URI("https://my-bucket/key")
	if err == nil {
		t.Errorf("expected error for non-s3 URI")
	}

	_, _, err = ParseS3URI("s3://my-bucket")
	if err == nil {
		t.Errorf("expected error for s3 URI without key")
	}
}

func TestGuessContentType(t *testing.T) {
	cases := map[string]string{
		"report.md":     "text/markdown; charset=utf-8",
		"report.json":   "application/json",
		"report.csv":    "text/csv; charset=utf-8",
		"report.html":   "text/html; charset=utf-8",
		"report.prom":   "text/plain; version=0.0.4; charset=utf-8",
		"report.txt":    "text/plain; charset=utf-8",
		"report-no-ext": "text/plain; charset=utf-8",
	}

	for filename, expected := range cases {
		got := GuessContentType(filename)
		if got != expected {
			t.Errorf("for %s: expected %s, got %s", filename, expected, got)
		}
	}
}

func TestUploadToS3WithMock(t *testing.T) {
	mockS3 := &mockS3Client{}
	client := NewClient(nil, nil)
	client.SetS3Client(mockS3)

	ctx := context.Background()
	data := []byte("# Cost Report\nTotal: $21.58")
	err := client.UploadToS3(ctx, "billing-reports-bucket", "daily/2026-10-29.md", data)
	if err != nil {
		t.Fatalf("UploadToS3 failed: %v", err)
	}

	if mockS3.putCalls != 1 {
		t.Errorf("expected 1 PutObject call, got %d", mockS3.putCalls)
	}
	if mockS3.lastBucket != "billing-reports-bucket" {
		t.Errorf("expected bucket billing-reports-bucket, got %s", mockS3.lastBucket)
	}
	if mockS3.lastKey != "daily/2026-10-29.md" {
		t.Errorf("expected key daily/2026-10-29.md, got %s", mockS3.lastKey)
	}
	if mockS3.lastType != "text/markdown; charset=utf-8" {
		t.Errorf("expected text/markdown Content-Type, got %s", mockS3.lastType)
	}
	if mockS3.lastBody != string(data) {
		t.Errorf("expected body matching uploaded data")
	}
}

func TestUploadToS3MultiRegion(t *testing.T) {
	mockS3 := &mockS3Client{
		bucketRegions: map[string]string{
			"eu-central-bucket": "eu-central-1",
			"ap-se-bucket":      "ap-southeast-2",
		},
	}
	client := NewClient(nil, nil)
	client.SetS3Client(mockS3)

	ctx := context.Background()

	// 1. Upload to eu-central-1 bucket
	err := client.UploadToS3(ctx, "eu-central-bucket", "report.md", []byte("# Central Report"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockS3.lastRegion != "eu-central-1" {
		t.Errorf("expected region eu-central-1, got %q", mockS3.lastRegion)
	}

	// 2. Upload to ap-southeast-2 bucket
	err = client.UploadToS3(ctx, "ap-se-bucket", "report.json", []byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockS3.lastRegion != "ap-southeast-2" {
		t.Errorf("expected region ap-southeast-2, got %q", mockS3.lastRegion)
	}
}

func TestExtractBucketRegionFromError(t *testing.T) {
	err := errors.New("The authorization header is malformed; expecting 'eu-west-1'")
	reg := extractBucketRegionFromError(err)
	if reg != "eu-west-1" {
		t.Errorf("expected eu-west-1, got %q", reg)
	}

	errNil := extractBucketRegionFromError(nil)
	if errNil != "" {
		t.Errorf("expected empty string for nil error, got %q", errNil)
	}
}
