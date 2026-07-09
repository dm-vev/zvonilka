package controlplane

import (
	"context"
	"strings"

	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const federationServerNameMetadataKey = "x-zvonilka-federation-server-name"

func (a *api) requireFederationPeer(ctx context.Context) (domainfederation.Peer, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return domainfederation.Peer{}, err
	}

	metadataValues, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return domainfederation.Peer{}, status.Error(codes.Unauthenticated, "missing federation metadata")
	}

	serverName := firstMetadataValue(metadataValues, federationServerNameMetadataKey)
	if serverName == "" {
		return domainfederation.Peer{}, status.Error(codes.Unauthenticated, "missing federation server name")
	}

	secret, err := bearerToken(ctx)
	if err != nil {
		return domainfederation.Peer{}, err
	}

	peer, err := service.AuthenticatePeerByServerName(ctx, serverName, secret)
	if err != nil {
		return domainfederation.Peer{}, grpcError(err)
	}

	return peer, nil
}

func firstMetadataValue(values metadata.MD, key string) string {
	items := values.Get(strings.ToLower(strings.TrimSpace(key)))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}

	return ""
}
