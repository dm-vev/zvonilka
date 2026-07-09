package federationbridge

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (a *api) requireBridgeAccess(ctx context.Context) error {
	if a == nil || a.federation == nil || strings.TrimSpace(a.sharedSecret) == "" {
		return status.Error(codes.Unimplemented, "federation bridge service unavailable")
	}

	token, err := bearerToken(ctx)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(strings.TrimSpace(a.sharedSecret))) != 1 {
		return status.Error(codes.Unauthenticated, "authentication failed")
	}

	return nil
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
