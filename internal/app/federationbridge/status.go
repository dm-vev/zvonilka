package federationbridge

import (
	"context"
	"errors"
	"fmt"

	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
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
	case errors.Is(err, domainfederation.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "authentication failed")
	case errors.Is(err, domainfederation.ErrForbidden):
		return status.Error(codes.PermissionDenied, "operation forbidden")
	case errors.Is(err, domainfederation.ErrNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, domainfederation.ErrConflict):
		return status.Error(codes.FailedPrecondition, "state conflict")
	case errors.Is(err, domainfederation.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid request")
	default:
		return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
	}
}
