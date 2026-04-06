package controlplane

import (
	"context"
	"strings"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (a *api) requireAuth(ctx context.Context) (domainidentity.AuthContext, error) {
	if a == nil || a.identity == nil {
		return domainidentity.AuthContext{}, status.Error(codes.Internal, "admin service unavailable")
	}

	token, err := bearerToken(ctx)
	if err != nil {
		return domainidentity.AuthContext{}, err
	}

	authContext, err := a.identity.AuthenticateAccessToken(ctx, token)
	if err != nil {
		return domainidentity.AuthContext{}, grpcError(err)
	}
	if authContext.Device.Status == domainidentity.DeviceStatusUnverified {
		return domainidentity.AuthContext{}, grpcError(domainidentity.ErrForbidden)
	}

	return authContext, nil
}

func (a *api) requireRoles(
	ctx context.Context,
	allowed ...domainidentity.Role,
) (domainidentity.AuthContext, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return domainidentity.AuthContext{}, err
	}
	if hasAnyRole(authContext.Account.Roles, allowed...) {
		return authContext, nil
	}

	return domainidentity.AuthContext{}, grpcError(domainidentity.ErrForbidden)
}

func hasAnyRole(roles []domainidentity.Role, allowed ...domainidentity.Role) bool {
	if len(roles) == 0 || len(allowed) == 0 {
		return false
	}

	for _, role := range roles {
		for _, candidate := range allowed {
			if role == candidate {
				return true
			}
		}
	}

	return false
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
