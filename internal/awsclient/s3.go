package awsclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// S3API defines the required S3 client interface for report uploads and region resolution.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
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

// ResolveBucketRegion attempts to determine the AWS region for an S3 bucket using HeadBucket or error headers.
func (c *Client) ResolveBucketRegion(ctx context.Context, bucket string) string {
	if c.s3Client == nil || bucket == "" {
		return ""
	}

	headOut, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil && headOut != nil && headOut.BucketRegion != nil && *headOut.BucketRegion != "" {
		return *headOut.BucketRegion
	}
	if err != nil {
		if r := extractBucketRegionFromError(err); r != "" {
			return r
		}
	}
	return ""
}

func extractBucketRegionFromError(err error) string {
	if err == nil {
		return ""
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil {
		if r := respErr.Response.Header.Get("X-Amz-Bucket-Region"); r != "" {
			return r
		}
	}
	errStr := err.Error()
	if idx := strings.Index(errStr, "expecting '"); idx != -1 {
		rest := errStr[idx+len("expecting '"):]
		if end := strings.Index(rest, "'"); end != -1 {
			return rest[:end]
		}
	}
	return ""
}

// UploadToS3 uploads content bytes to the specified S3 bucket and key with automatic multi-region resolution.
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

	var optFns []func(*s3.Options)
	region := c.ResolveBucketRegion(ctx, bucket)
	if region != "" {
		optFns = append(optFns, func(o *s3.Options) {
			o.Region = region
		})
	}

	_, err := c.s3Client.PutObject(ctx, input, optFns...)
	if err != nil {
		if reg := extractBucketRegionFromError(err); reg != "" && reg != region {
			retryOpts := []func(*s3.Options){
				func(o *s3.Options) {
					o.Region = reg
				},
			}
			input.Body = bytes.NewReader(data)
			_, retryErr := c.s3Client.PutObject(ctx, input, retryOpts...)
			if retryErr == nil {
				return nil
			}
			err = retryErr
		}
		return fmt.Errorf("failed to upload s3://%s/%s: %w", bucket, key, err)
	}
	return nil
}
