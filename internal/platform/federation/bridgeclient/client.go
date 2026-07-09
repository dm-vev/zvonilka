package bridgeclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/domain/federation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config defines how a bridge adapter connects to the local federationbridge service.
type Config struct {
	Endpoint     string
	SharedSecret string
	DialTimeout  time.Duration
}

// Client is a typed FederationBridgeService client with auth handling.
type Client struct {
	conn         *grpc.ClientConn
	client       federationv1.FederationBridgeServiceClient
	sharedSecret string
}

// NewGRPC dials the bridge service and returns an authenticated client wrapper.
func NewGRPC(ctx context.Context, cfg Config) (*Client, error) {
	if ctx == nil {
		return nil, federation.ErrInvalidInput
	}
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.SharedSecret = strings.TrimSpace(cfg.SharedSecret)
	if cfg.Endpoint == "" || cfg.SharedSecret == "" {
		return nil, federation.ErrInvalidInput
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}

	target, transportCredentials, err := dialTarget(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	dialCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		target,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial federation bridge %s: %w", cfg.Endpoint, err)
	}

	return &Client{
		conn:         conn,
		client:       federationv1.NewFederationBridgeServiceClient(conn),
		sharedSecret: cfg.SharedSecret,
	}, nil
}

// PullFragments fetches queued outbound fragments for one bridge-managed link.
func (c *Client) PullFragments(
	ctx context.Context,
	peerServerName string,
	linkName string,
	limit int,
) (*federationv1.PullBridgeFragmentsResponse, error) {
	if c == nil || c.client == nil {
		return nil, federation.ErrInvalidInput
	}

	return c.client.PullBridgeFragments(
		withBridgeCredentials(ctx, c.sharedSecret),
		&federationv1.PullBridgeFragmentsRequest{
			PeerServerName: peerServerName,
			LinkName:       linkName,
			Limit:          uint32(maxInt(0, limit)),
		},
	)
}

// SubmitFragments stores inbound fragments received from the transport.
func (c *Client) SubmitFragments(
	ctx context.Context,
	peerServerName string,
	linkName string,
	fragments []*federationv1.BundleFragment,
) (*federationv1.SubmitBridgeFragmentsResponse, error) {
	if c == nil || c.client == nil {
		return nil, federation.ErrInvalidInput
	}

	return c.client.SubmitBridgeFragments(
		withBridgeCredentials(ctx, c.sharedSecret),
		&federationv1.SubmitBridgeFragmentsRequest{
			PeerServerName: peerServerName,
			LinkName:       linkName,
			Fragments:      fragments,
		},
	)
}

// AcknowledgeFragments marks locally transmitted fragments as handed off to the transport.
func (c *Client) AcknowledgeFragments(
	ctx context.Context,
	peerServerName string,
	linkName string,
	fragmentIDs []string,
	leaseToken string,
) (*federationv1.AcknowledgeBridgeFragmentsResponse, error) {
	if c == nil || c.client == nil {
		return nil, federation.ErrInvalidInput
	}

	return c.client.AcknowledgeBridgeFragments(
		withBridgeCredentials(ctx, c.sharedSecret),
		&federationv1.AcknowledgeBridgeFragmentsRequest{
			PeerServerName: peerServerName,
			LinkName:       linkName,
			FragmentIds:    append([]string(nil), fragmentIDs...),
			LeaseToken:     strings.TrimSpace(leaseToken),
		},
	)
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func withBridgeCredentials(ctx context.Context, sharedSecret string) context.Context {
	return metadata.NewOutgoingContext(
		ctx,
		metadata.Pairs("authorization", "Bearer "+strings.TrimSpace(sharedSecret)),
	)
}

func dialTarget(endpoint string) (string, credentials.TransportCredentials, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", nil, federation.ErrInvalidInput
	}

	if !strings.Contains(endpoint, "://") {
		return endpoint, insecure.NewCredentials(), nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, err
	}
	target := parsed.Host
	if target == "" {
		return "", nil, federation.ErrInvalidInput
	}

	switch normalizedDialScheme(parsed.Scheme) {
	case "https", "grpcs":
		if _, _, err := net.SplitHostPort(target); err != nil {
			target = net.JoinHostPort(parsed.Hostname(), "443")
		}
		return target, credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: parsed.Hostname(),
		}), nil
	case "http", "grpc", "bridge":
		if _, _, err := net.SplitHostPort(target); err != nil {
			target = net.JoinHostPort(parsed.Hostname(), "80")
		}
		return target, insecure.NewCredentials(), nil
	default:
		return "", nil, federation.ErrInvalidInput
	}
}

func normalizedDialScheme(scheme string) string {
	switch strings.TrimSpace(strings.ToLower(scheme)) {
	case "https", "grpcs":
		return "https"
	case "http", "grpc", "bridge":
		return "http"
	default:
		return strings.TrimSpace(strings.ToLower(scheme))
	}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
