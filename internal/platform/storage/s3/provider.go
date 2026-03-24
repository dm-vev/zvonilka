package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// Provider is a logical S3-compatible storage provider.
type Provider struct {
	client       *s3.Client
	presign      *s3.PresignClient
	bucket       string
	name         string
	kind         domainstorage.Kind
	purpose      domainstorage.Purpose
	capabilities domainstorage.Capability
}

// Name returns the configured provider name.
func (p *Provider) Name() string {
	if p == nil {
		return ""
	}

	return p.name
}

// Kind returns the logical provider kind.
func (p *Provider) Kind() domainstorage.Kind {
	if p == nil {
		return domainstorage.KindUnspecified
	}

	return p.kind
}

// Purpose returns the logical provider purpose.
func (p *Provider) Purpose() domainstorage.Purpose {
	if p == nil {
		return domainstorage.PurposeUnspecified
	}

	return p.purpose
}

// Capabilities returns the supported capabilities.
func (p *Provider) Capabilities() domainstorage.Capability {
	if p == nil {
		return 0
	}

	return p.capabilities
}

// Bucket returns the backing bucket name.
func (p *Provider) Bucket() string {
	if p == nil {
		return ""
	}

	return p.bucket
}

// Close is a no-op for the AWS SDK client.
func (p *Provider) Close(context.Context) error {
	return nil
}

// PutObject uploads a blob to the configured bucket.
func (p *Provider) PutObject(
	ctx context.Context,
	key string,
	body io.Reader,
	size int64,
	options domainstorage.PutObjectOptions,
) (domainstorage.BlobObject, error) {
	if ctx == nil {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	if p == nil || p.client == nil {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	if body == nil {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}

	input := &s3.PutObjectInput{
		Bucket:             aws.String(p.bucket),
		Key:                aws.String(key),
		Body:               body,
		ContentType:        aws.String(strings.TrimSpace(options.ContentType)),
		ContentDisposition: aws.String(strings.TrimSpace(options.ContentDisposition)),
		CacheControl:       aws.String(strings.TrimSpace(options.CacheControl)),
		Metadata:           copyStringMap(options.Metadata),
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}

	output, err := p.client.PutObject(ctx, input)
	if err != nil {
		return domainstorage.BlobObject{}, mapObjectError(err)
	}

	head, err := p.HeadObject(ctx, key)
	if err != nil {
		return domainstorage.BlobObject{}, err
	}
	if output.ETag != nil {
		head.ETag = aws.ToString(output.ETag)
	}

	return head, nil
}

// GetObject downloads a blob from the configured bucket.
func (p *Provider) GetObject(ctx context.Context, key string) (io.ReadCloser, domainstorage.BlobObject, error) {
	if ctx == nil {
		return nil, domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	if p == nil || p.client == nil {
		return nil, domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}

	output, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, domainstorage.BlobObject{}, mapObjectError(err)
	}

	object := domainstorage.BlobObject{
		Bucket:             p.bucket,
		Key:                key,
		ETag:               aws.ToString(output.ETag),
		ContentLength:      aws.ToInt64(output.ContentLength),
		ContentType:        aws.ToString(output.ContentType),
		ContentDisposition: aws.ToString(output.ContentDisposition),
		CacheControl:       aws.ToString(output.CacheControl),
		Metadata:           copyStringMap(output.Metadata),
		VersionID:          aws.ToString(output.VersionId),
	}
	if output.LastModified != nil {
		object.LastModified = output.LastModified.UTC()
	}

	return output.Body, object, nil
}

// HeadObject resolves blob metadata without downloading the object.
func (p *Provider) HeadObject(ctx context.Context, key string) (domainstorage.BlobObject, error) {
	if ctx == nil {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	if p == nil || p.client == nil {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
	}

	output, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return domainstorage.BlobObject{}, mapObjectError(err)
	}

	object := domainstorage.BlobObject{
		Bucket:             p.bucket,
		Key:                key,
		ETag:               aws.ToString(output.ETag),
		ContentLength:      aws.ToInt64(output.ContentLength),
		ContentType:        aws.ToString(output.ContentType),
		ContentDisposition: aws.ToString(output.ContentDisposition),
		CacheControl:       aws.ToString(output.CacheControl),
		Metadata:           copyStringMap(output.Metadata),
		VersionID:          aws.ToString(output.VersionId),
	}
	if output.LastModified != nil {
		object.LastModified = output.LastModified.UTC()
	}

	return object, nil
}

// DeleteObject deletes a blob from the configured bucket.
func (p *Provider) DeleteObject(ctx context.Context, key string) error {
	if ctx == nil {
		return domainstorage.ErrInvalidInput
	}
	if p == nil || p.client == nil {
		return domainstorage.ErrInvalidInput
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return domainstorage.ErrInvalidInput
	}

	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return mapObjectError(err)
	}

	return nil
}

// PresignPutObject returns a presigned PUT request for a blob key.
func (p *Provider) PresignPutObject(
	ctx context.Context,
	key string,
	expires time.Duration,
	options domainstorage.PutObjectOptions,
) (domainstorage.PresignedRequest, error) {
	if ctx == nil {
		return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
	}
	if p == nil || p.presign == nil {
		return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
	}
	key = strings.TrimSpace(key)
	if key == "" || expires <= 0 {
		return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
	}

	request, err := p.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:             aws.String(p.bucket),
		Key:                aws.String(key),
		ContentType:        aws.String(strings.TrimSpace(options.ContentType)),
		ContentDisposition: aws.String(strings.TrimSpace(options.ContentDisposition)),
		CacheControl:       aws.String(strings.TrimSpace(options.CacheControl)),
		Metadata:           copyStringMap(options.Metadata),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return domainstorage.PresignedRequest{}, mapObjectError(err)
	}

	return toPresignedRequest(request, expires, p.bucket, key), nil
}

// PresignGetObject returns a presigned GET request for a blob key.
func (p *Provider) PresignGetObject(
	ctx context.Context,
	key string,
	expires time.Duration,
) (domainstorage.PresignedRequest, error) {
	if ctx == nil {
		return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
	}
	if p == nil || p.presign == nil {
		return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
	}
	key = strings.TrimSpace(key)
	if key == "" || expires <= 0 {
		return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
	}

	request, err := p.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return domainstorage.PresignedRequest{}, mapObjectError(err)
	}

	return toPresignedRequest(request, expires, p.bucket, key), nil
}

func toPresignedRequest(request *v4.PresignedHTTPRequest, expires time.Duration, bucket string, key string) domainstorage.PresignedRequest {
	if request == nil {
		return domainstorage.PresignedRequest{
			ExpiresAt: time.Now().UTC().Add(expires),
			Bucket:    bucket,
			ObjectKey: key,
		}
	}

	headers := make(map[string]string, len(request.SignedHeader))
	for key, values := range request.SignedHeader {
		if len(values) == 0 {
			continue
		}
		headers[key] = strings.Join(values, ",")
	}

	return domainstorage.PresignedRequest{
		URL:       request.URL,
		Method:    request.Method,
		Headers:   headers,
		ExpiresAt: presignedExpiresAt(request, expires),
		Bucket:    bucket,
		ObjectKey: key,
	}
}

func presignedExpiresAt(request *v4.PresignedHTTPRequest, expires time.Duration) time.Time {
	if request == nil {
		return time.Now().UTC().Add(expires)
	}

	parsedURL, err := url.Parse(request.URL)
	if err != nil {
		return time.Now().UTC().Add(expires)
	}

	signedAtRaw := parsedURL.Query().Get("X-Amz-Date")
	if signedAtRaw == "" {
		return time.Now().UTC().Add(expires)
	}

	signedAt, err := time.Parse("20060102T150405Z", signedAtRaw)
	if err != nil {
		return time.Now().UTC().Add(expires)
	}

	return signedAt.UTC().Add(expires)
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}

	return result
}

func mapObjectError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket":
			return domainstorage.ErrNotFound
		case "AccessDenied", "Forbidden", "Unauthorized":
			return domainstorage.ErrForbidden
		}
	}

	return fmt.Errorf("s3 operation failed: %w", err)
}

type bucketClient interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
}

func mapBucketCreateError(err error) error {
	if err == nil {
		return nil
	}

	var owned *s3types.BucketAlreadyOwnedByYou
	if errors.As(err, &owned) {
		return nil
	}

	var exists *s3types.BucketAlreadyExists
	if errors.As(err, &exists) {
		return domainstorage.ErrConflict
	}

	return mapObjectError(err)
}

// ensureBucket creates the bucket when it does not already exist.
func ensureBucket(ctx context.Context, client bucketClient, bucket string, region string) error {
	if ctx == nil {
		return domainstorage.ErrInvalidInput
	}
	if client == nil {
		return domainstorage.ErrInvalidInput
	}
	if strings.TrimSpace(bucket) == "" {
		return domainstorage.ErrInvalidInput
	}

	input := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if normalizedRegion := strings.TrimSpace(region); normalizedRegion != "" && normalizedRegion != "us-east-1" {
		input.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(normalizedRegion),
		}
	}

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ensure bucket %s: %w", bucket, err)
		}

		if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if _, err := client.CreateBucket(ctx, input); err != nil {
			if mapped := mapBucketCreateError(err); mapped == nil {
				if _, headErr := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); headErr == nil {
					return nil
				} else {
					lastErr = headErr
				}
			} else if errors.Is(mapped, domainstorage.ErrForbidden) {
				return fmt.Errorf("ensure bucket %s: %w", bucket, mapped)
			} else if errors.Is(mapped, domainstorage.ErrConflict) {
				if _, headErr := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); headErr == nil {
					return nil
				} else {
					lastErr = mapped
				}
			} else {
				lastErr = mapped
			}
		} else if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err == nil {
			return nil
		} else {
			lastErr = err
		}

		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("ensure bucket %s: %w", bucket, ctx.Err())
		case <-timer.C:
		}
	}

	if lastErr == nil {
		lastErr = domainstorage.ErrNotFound
	}

	return fmt.Errorf("ensure bucket %s: %w", bucket, lastErr)
}

var _ domainstorage.BlobStore = (*Provider)(nil)
