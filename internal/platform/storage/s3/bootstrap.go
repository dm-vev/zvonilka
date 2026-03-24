package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// Bootstrap opens and prepares an S3-compatible object provider.
type Bootstrap struct {
	mu         sync.Mutex
	cfg        config.Configuration
	httpClient *http.Client
	provider   *Provider
}

// NewBootstrap constructs an object-storage bootstrap for the current config.
func NewBootstrap(cfg config.Configuration, opts ...Option) *Bootstrap {
	bootstrap := &Bootstrap{cfg: cfg}
	for _, opt := range opts {
		if opt != nil {
			opt(bootstrap)
		}
	}

	return bootstrap
}

// Option configures bootstrap behavior.
type Option func(*Bootstrap)

// WithHTTPClient overrides the HTTP client used by the AWS SDK.
func WithHTTPClient(client *http.Client) Option {
	return func(bootstrap *Bootstrap) {
		if bootstrap == nil {
			return
		}
		bootstrap.httpClient = client
	}
}

// Open returns a ready-to-use S3-compatible provider.
func (b *Bootstrap) Open(ctx context.Context) (*Provider, error) {
	if b == nil {
		return nil, fmt.Errorf("open object storage: %w", domainstorage.ErrInvalidInput)
	}
	if ctx == nil {
		return nil, domainstorage.ErrInvalidInput
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.provider != nil {
		return b.provider, nil
	}

	provider, err := b.open(ctx)
	if err != nil {
		return nil, err
	}

	b.provider = provider
	return b.provider, nil
}

// Close releases bootstrap resources. The AWS client itself is stateless.
func (b *Bootstrap) Close(context.Context) error {
	if b == nil {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.provider = nil
	return nil
}

func (b *Bootstrap) open(ctx context.Context) (*Provider, error) {
	if ctx == nil {
		return nil, domainstorage.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	objectCfg := b.cfg.Infrastructure.ObjectStore
	endpoint, err := normalizeEndpoint(objectCfg.Endpoint, objectCfg.UseSSL)
	if err != nil {
		return nil, fmt.Errorf("normalize object storage endpoint: %w", err)
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(objectCfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			objectCfg.AccessKeyID,
			objectCfg.SecretAccessKey,
			"",
		)),
	}
	if b.httpClient != nil {
		loadOpts = append(loadOpts, awsconfig.WithHTTPClient(b.httpClient))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load s3 config: %w", err)
	}

	bootstrapClient := s3.NewFromConfig(awsCfg, func(opts *s3.Options) {
		opts.BaseEndpoint = &endpoint
		opts.UsePathStyle = objectCfg.ForcePathStyle
		opts.Retryer = aws.NopRetryer{}
	})
	client := s3.NewFromConfig(awsCfg, func(opts *s3.Options) {
		opts.BaseEndpoint = &endpoint
		opts.UsePathStyle = objectCfg.ForcePathStyle
	})

	if err := ensureBucket(ctx, bootstrapClient, objectCfg.Bucket, objectCfg.Region); err != nil {
		return nil, err
	}

	return &Provider{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  objectCfg.Bucket,
	}, nil
}

func normalizeEndpoint(raw string, useSSL bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", domainstorage.ErrInvalidInput
	}

	if !strings.Contains(raw, "://") {
		if useSSL {
			raw = "https://" + raw
		} else {
			raw = "http://" + raw
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse endpoint %q: %w", raw, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("parse endpoint %q: %w", raw, domainstorage.ErrInvalidInput)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("parse endpoint %q: %w", raw, domainstorage.ErrInvalidInput)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("parse endpoint %q: %w", raw, domainstorage.ErrInvalidInput)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("parse endpoint %q: %w", raw, domainstorage.ErrInvalidInput)
	}

	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
