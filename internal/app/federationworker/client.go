package federationworker

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
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const federationServerNameMetadataKey = "x-zvonilka-federation-server-name"
const federationTransportKindMetadataKey = "x-zvonilka-federation-transport-kind"

type grpcClientFactory struct {
	dialTimeout time.Duration
}

func newGRPCClientFactory(cfg config.FederationConfig) federation.ReplicationClientFactory {
	timeout := cfg.DialTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	factory := grpcClientFactory{dialTimeout: timeout}
	return factory.newClient
}

func (f grpcClientFactory) newClient(
	ctx context.Context,
	peer federation.Peer,
	link federation.Link,
) (federation.ReplicationClient, error) {
	target, transportCredentials, err := dialTarget(link.Endpoint)
	if err != nil {
		return nil, err
	}

	dialCtx, cancel := context.WithTimeout(ctx, f.dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		target,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial federation peer %s via %s: %w", peer.ServerName, link.Endpoint, err)
	}

	return &grpcReplicationClient{
		conn:          conn,
		client:        federationv1.NewFederationServiceClient(conn),
		sharedSecret:  peer.SharedSecret,
		transportKind: link.TransportKind,
	}, nil
}

type grpcReplicationClient struct {
	conn          *grpc.ClientConn
	client        federationv1.FederationServiceClient
	sharedSecret  string
	transportKind federation.TransportKind
}

func (c *grpcReplicationClient) PushBundles(
	ctx context.Context,
	serverName string,
	linkName string,
	bundles []federation.Bundle,
) error {
	reqBundles := make([]*federationv1.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		reqBundles = append(reqBundles, &federationv1.Bundle{
			BundleId:      bundle.ID,
			DedupKey:      bundle.DedupKey,
			Direction:     federationv1.BundleDirection_BUNDLE_DIRECTION_OUTBOUND,
			CursorFrom:    bundle.CursorFrom,
			CursorTo:      bundle.CursorTo,
			EventCount:    uint32(maxInt(0, bundle.EventCount)),
			PayloadType:   bundle.PayloadType,
			Payload:       append([]byte(nil), bundle.Payload...),
			Compression:   compressionKindProto(bundle.Compression),
			IntegrityHash: bundle.IntegrityHash,
			AuthTag:       bundle.AuthTag,
			AvailableAt:   timestampOrNil(bundle.AvailableAt),
			ExpiresAt:     timestampOrNil(bundle.ExpiresAt),
		})
	}

	_, err := c.client.PushBundles(
		withFederationCredentials(ctx, serverName, c.sharedSecret, c.transportKind),
		&federationv1.PushBundlesRequest{
			ServerName: serverName,
			LinkName:   linkName,
			Bundles:    reqBundles,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *grpcReplicationClient) PullBundles(
	ctx context.Context,
	serverName string,
	linkName string,
	afterCursor uint64,
	limit int,
) ([]federation.Bundle, bool, error) {
	resp, err := c.client.PullBundles(
		withFederationCredentials(ctx, serverName, c.sharedSecret, c.transportKind),
		&federationv1.PullBundlesRequest{
			ServerName:  serverName,
			LinkName:    linkName,
			AfterCursor: afterCursor,
			Limit:       uint32(maxInt(0, limit)),
		},
	)
	if err != nil {
		return nil, false, err
	}

	bundles := make([]federation.Bundle, 0, len(resp.GetBundles()))
	for _, bundle := range resp.GetBundles() {
		if bundle == nil {
			continue
		}

		bundles = append(bundles, federation.Bundle{
			ID:            bundle.GetBundleId(),
			DedupKey:      bundle.GetDedupKey(),
			Direction:     bundleDirectionFromProto(bundle.GetDirection()),
			CursorFrom:    bundle.GetCursorFrom(),
			CursorTo:      bundle.GetCursorTo(),
			EventCount:    int(bundle.GetEventCount()),
			PayloadType:   bundle.GetPayloadType(),
			Payload:       append([]byte(nil), bundle.GetPayload()...),
			Compression:   compressionKindFromProto(bundle.GetCompression()),
			IntegrityHash: bundle.GetIntegrityHash(),
			AuthTag:       bundle.GetAuthTag(),
			State:         bundleStateFromProto(bundle.GetState()),
			CreatedAt:     timestampValue(bundle.GetCreatedAt()),
			AvailableAt:   timestampValue(bundle.GetAvailableAt()),
			ExpiresAt:     timestampValue(bundle.GetExpiresAt()),
			AckedAt:       timestampValue(bundle.GetAckedAt()),
		})
	}

	return bundles, resp.GetHasMore(), nil
}

func (c *grpcReplicationClient) AcknowledgeBundles(
	ctx context.Context,
	serverName string,
	linkName string,
	upToCursor uint64,
	bundleIDs []string,
) error {
	_, err := c.client.AcknowledgeBundles(
		withFederationCredentials(ctx, serverName, c.sharedSecret, c.transportKind),
		&federationv1.AcknowledgeBundlesRequest{
			ServerName: serverName,
			LinkName:   linkName,
			UpToCursor: upToCursor,
			BundleIds:  append([]string(nil), bundleIDs...),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (c *grpcReplicationClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func withFederationCredentials(
	ctx context.Context,
	serverName string,
	sharedSecret string,
	transportKind federation.TransportKind,
) context.Context {
	pairs := []string{
		"authorization", "Bearer " + sharedSecret,
		federationServerNameMetadataKey, strings.TrimSpace(strings.ToLower(serverName)),
	}
	if transportValue := strings.TrimSpace(strings.ToLower(string(transportKind))); transportValue != "" {
		pairs = append(pairs, federationTransportKindMetadataKey, transportValue)
	}
	values := metadata.Pairs(pairs...)

	return metadata.NewOutgoingContext(ctx, values)
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
		return target, credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}), nil
	case "http", "grpc", "bridge":
		if _, _, err := net.SplitHostPort(target); err != nil {
			target = net.JoinHostPort(parsed.Hostname(), "80")
		}
		return target, insecure.NewCredentials(), nil
	default:
		if _, _, err := net.SplitHostPort(target); err != nil {
			target = net.JoinHostPort(parsed.Hostname(), "80")
		}
		return target, insecure.NewCredentials(), nil
	}
}

func normalizedDialScheme(scheme string) string {
	scheme = strings.TrimSpace(strings.ToLower(scheme))
	switch scheme {
	case "meshtastic", "meshcore", "dtn", "custom_dtn", "bridge",
		"meshtastic+grpc", "meshcore+grpc", "dtn+grpc", "custom_dtn+grpc", "bridge+grpc":
		return "bridge"
	case "meshtastic+grpcs", "meshcore+grpcs", "dtn+grpcs", "custom_dtn+grpcs", "bridge+grpcs":
		return "grpcs"
	default:
		return scheme
	}
}

func compressionKindProto(value federation.CompressionKind) federationv1.CompressionKind {
	switch value {
	case federation.CompressionKindGzip:
		return federationv1.CompressionKind_COMPRESSION_KIND_GZIP
	default:
		return federationv1.CompressionKind_COMPRESSION_KIND_NONE
	}
}

func compressionKindFromProto(value federationv1.CompressionKind) federation.CompressionKind {
	switch value {
	case federationv1.CompressionKind_COMPRESSION_KIND_GZIP:
		return federation.CompressionKindGzip
	default:
		return federation.CompressionKindNone
	}
}

func bundleDirectionFromProto(value federationv1.BundleDirection) federation.BundleDirection {
	switch value {
	case federationv1.BundleDirection_BUNDLE_DIRECTION_INBOUND:
		return federation.BundleDirectionInbound
	case federationv1.BundleDirection_BUNDLE_DIRECTION_OUTBOUND:
		return federation.BundleDirectionOutbound
	default:
		return federation.BundleDirectionUnspecified
	}
}

func bundleStateFromProto(value federationv1.BundleState) federation.BundleState {
	switch value {
	case federationv1.BundleState_BUNDLE_STATE_QUEUED:
		return federation.BundleStateQueued
	case federationv1.BundleState_BUNDLE_STATE_ACCEPTED:
		return federation.BundleStateAccepted
	case federationv1.BundleState_BUNDLE_STATE_ACKNOWLEDGED:
		return federation.BundleStateAcknowledged
	case federationv1.BundleState_BUNDLE_STATE_FAILED:
		return federation.BundleStateFailed
	default:
		return federation.BundleStateUnspecified
	}
}

func timestampOrNil(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}

	return timestamppb.New(value.UTC())
}

func timestampValue(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}

	return b
}
