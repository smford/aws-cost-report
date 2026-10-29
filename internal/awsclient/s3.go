package awsclient

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API defines the required S3 client interface for report uploads.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// ParseS3URI parses an s3://bucket/key URI into bucket and key components.
func ParseS3URI(uri string) (bucket, key string, err error) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", fmt.Errorf("invalid S3 URI %q: must begin with s3://", uri)
	}
	trimmed := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q: expected s3://<bucket>/<key>", uri)
	}
	return parts[0], parts[1], nil
}

// GuessContentType determines standard Content-Type header based on file extension.
func GuessContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".prom", ".prometheus":
		return "text/plain; version=0.0.4; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

// SetS3Client sets or overrides the S3 client used for uploads.
func (c *Client) SetS3Client(s3Client S3API) {
	c.s3Client = s3Client
}

// UploadToS3 uploads content bytes to the specified S3 bucket and key.
func (c *Client) UploadToS3(ctx context.Context, bucket, key string, data []byte, contentType ...string) error {
	if c.s3Client == nil {
		return fmt.Errorf("s3 client is not initialized")
	}

	ct := GuessContentType(key)
	if len(contentType) > 0 && contentType[0] != "" {
		ct = contentType[0]
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(ct),
	}

	_, err := c.s3Client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}
