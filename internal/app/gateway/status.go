package gateway

import (
	"context"
	"errors"
	"fmt"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
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
	case errors.Is(err, domainidentity.ErrExpiredToken):
		return status.Error(codes.Unauthenticated, "token expired")
	case errors.Is(err, domainidentity.ErrInvalidCode):
		return status.Error(codes.Unauthenticated, "invalid login code")
	case errors.Is(err, domainidentity.ErrExpiredChallenge):
		return status.Error(codes.FailedPrecondition, "login challenge expired")
	case errors.Is(err, domainidentity.ErrExpiredJoinRequest):
		return status.Error(codes.FailedPrecondition, "join request expired")
	case errors.Is(err, domainconversation.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "rate limited")
	case errors.Is(err, domainidentity.ErrForbidden),
		errors.Is(err, domaincall.ErrForbidden),
		errors.Is(err, domainconversation.ErrForbidden),
		errors.Is(err, domaine2ee.ErrForbidden),
		errors.Is(err, domainmedia.ErrForbidden),
		errors.Is(err, domainuser.ErrForbidden):
		return status.Error(codes.PermissionDenied, "operation forbidden")
	case errors.Is(err, domainidentity.ErrNotFound),
		errors.Is(err, domaincall.ErrNotFound),
		errors.Is(err, domainconversation.ErrNotFound),
		errors.Is(err, domaine2ee.ErrNotFound),
		errors.Is(err, domainmedia.ErrNotFound),
		errors.Is(err, domainpresence.ErrNotFound),
		errors.Is(err, domainuser.ErrNotFound):
		return status.Error(codes.NotFound, "resource not found")
	case errors.Is(err, domainidentity.ErrConflict),
		errors.Is(err, domaincall.ErrConflict),
		errors.Is(err, domainconversation.ErrConflict),
		errors.Is(err, domaine2ee.ErrConflict),
		errors.Is(err, domainmedia.ErrConflict),
		errors.Is(err, domainuser.ErrConflict):
		return status.Error(codes.FailedPrecondition, "state conflict")
	case errors.Is(err, domainidentity.ErrInvalidInput),
		errors.Is(err, domaincall.ErrInvalidInput),
		errors.Is(err, domainconversation.ErrInvalidInput),
		errors.Is(err, domaine2ee.ErrInvalidInput),
		errors.Is(err, domainmedia.ErrInvalidInput),
		errors.Is(err, domainpresence.ErrInvalidInput),
		errors.Is(err, domainsearch.ErrInvalidInput),
		errors.Is(err, domainuser.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, "invalid request")
	default:
		return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
	}
}
