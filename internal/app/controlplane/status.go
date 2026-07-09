package controlplane

import (
	"context"
	"errors"
	"fmt"

	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func grpcError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	case errors.Is(err, domainidentity.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "authentication failed")
	case errors.Is(err, domainfederation.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "authentication failed")
	case errors.Is(err, domainidentity.ErrExpiredToken):
		return status.Error(codes.Unauthenticated, "token expired")
	case errors.Is(err, domainidentity.ErrInvalidCode):
		return status.Error(codes.Unauthenticated, "invalid login code")
	case errors.Is(err, domainidentity.ErrExpiredChallenge):
		return status.Error(codes.FailedPrecondition, "login challenge expired")
	case errors.Is(err, domainidentity.ErrExpiredJoinRequest):
		return status.Error(codes.FailedPrecondition, "join request expired")
	case errors.Is(err, domainidentity.ErrForbidden):
		return status.Error(codes.PermissionDenied, "operation forbidden")
	case errors.Is(err, domainfederation.ErrForbidden):
		return status.Error(codes.PermissionDenied, "operation forbidden")
	case errors.Is(err, domainidentity.ErrNotFound),
		errors.Is(err, domainfederation.ErrNotFound),
		errors.Is(err, domainpresence.ErrNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, domainidentity.ErrConflict),
		errors.Is(err, domainfederation.ErrConflict):
		return status.Error(codes.FailedPrecondition, "state conflict")
	case errors.Is(err, domainidentity.ErrInvalidInput),
		errors.Is(err, domainfederation.ErrInvalidInput),
		errors.Is(err, domainpresence.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid request")
	default:
		return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
	}
}
