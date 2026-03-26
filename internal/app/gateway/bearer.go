package gateway

import (
	"context"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (a *api) requireAuth(ctx context.Context) (identity.AuthContext, error) {
	token, err := bearerToken(ctx)
	if err != nil {
		return identity.AuthContext{}, err
	}

	authContext, err := a.identity.AuthenticateAccessToken(ctx, token)
	if err != nil {
		return identity.AuthContext{}, grpcError(err)
	}

	return authContext, nil
}

func bearerToken(ctx context.Context) (string, error) {
	metadataValues, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing authorization metadata")
	}

	values := metadataValues.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization metadata")
	}

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts := strings.SplitN(value, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			continue
		}

		token := strings.TrimSpace(parts[1])
		if token != "" {
			return token, nil
		}
	}

	return "", status.Error(codes.Unauthenticated, "invalid authorization metadata")
}
