package s3

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsretry "github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// Bootstrap opens and prepares an S3-compatible object provider.
type Bootstrap struct {
	mu       sync.Mutex
	cfg      config.Configuration
	provider *Provider
}

// NewBootstrap constructs an object-storage bootstrap for the current config.
func NewBootstrap(cfg config.Configuration) *Bootstrap {
	return &Bootstrap{cfg: cfg}
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

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(objectCfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			objectCfg.AccessKeyID,
			objectCfg.SecretAccessKey,
			"",
		)),
		awsconfig.WithRetryer(func() aws.Retryer {
			return awsretry.NewStandard(func(options *awsretry.StandardOptions) {
				options.MaxAttempts = 2
				options.MaxBackoff = time.Second
			})
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("load s3 config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(opts *s3.Options) {
		opts.BaseEndpoint = &endpoint
		opts.UsePathStyle = objectCfg.ForcePathStyle
	})

	if err := ensureBucket(ctx, client, objectCfg.Bucket, objectCfg.Region); err != nil {
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

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse endpoint %q: %w", raw, err)
		}
		return parsed.String(), nil
	}

	if useSSL {
		return "https://" + raw, nil
	}

	return "http://" + raw, nil
}
