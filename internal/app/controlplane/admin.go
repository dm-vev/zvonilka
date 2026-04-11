package controlplane

import (
	"context"
	"errors"
	"sort"
	"time"

	adminv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/admin/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const joinRequestsPagePrefix = "admin_join_requests"

// ListJoinRequests returns the requested join-request page for an authenticated admin actor.
func (a *api) ListJoinRequests(
	ctx context.Context,
	req *adminv1.ListJoinRequestsRequest,
) (*adminv1.ListJoinRequestsResponse, error) {
	_, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleModerator,
		domainidentity.RoleSupport,
		domainidentity.RoleAuditor,
	)
	if err != nil {
		return nil, err
	}

	joinRequests, err := a.listJoinRequests(ctx, req.GetStatusFilter())
	if err != nil {
		return nil, err
	}

	offset, err := decodeOffset(req.GetPage(), joinRequestsPagePrefix)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(joinRequests) {
		end = len(joinRequests)
	}

	pageCap := 0
	if offset < end {
		pageCap = end - offset
	}
	page := make([]*adminv1.JoinRequest, 0, pageCap)
	if offset < len(joinRequests) {
		for _, joinRequest := range joinRequests[offset:end] {
			page = append(page, joinRequestProto(joinRequest))
		}
	}

	nextToken := ""
	if end < len(joinRequests) {
		nextToken = offsetToken(joinRequestsPagePrefix, end)
	}

	return &adminv1.ListJoinRequestsResponse{
		JoinRequests: page,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(joinRequests)),
		},
	}, nil
}

// ApproveJoinRequest approves one pending join request and returns the created account profile.
func (a *api) ApproveJoinRequest(
	ctx context.Context,
	req *adminv1.ApproveJoinRequestRequest,
) (*adminv1.ApproveJoinRequestResponse, error) {
	authContext, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleModerator,
	)
	if err != nil {
		return nil, err
	}

	roles, err := rolesFromProto(req.GetRoles())
	if err != nil {
		return nil, err
	}

	joinRequest, account, err := a.identity.ApproveJoinRequest(ctx, domainidentity.ApproveJoinRequestParams{
		JoinRequestID:  req.GetJoinRequestId(),
		Roles:          roles,
		Note:           req.GetNote(),
		ReviewedBy:     authContext.Account.ID,
		DecisionReason: req.GetNote(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	profile, err := a.profileForAccount(ctx, account)
	if err != nil {
		return nil, err
	}

	return &adminv1.ApproveJoinRequestResponse{
		JoinRequest: joinRequestProto(joinRequest),
		Profile:     profile,
	}, nil
}

// RejectJoinRequest rejects one pending join request.
func (a *api) RejectJoinRequest(
	ctx context.Context,
	req *adminv1.RejectJoinRequestRequest,
) (*adminv1.RejectJoinRequestResponse, error) {
	authContext, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleModerator,
	)
	if err != nil {
		return nil, err
	}

	joinRequest, err := a.identity.RejectJoinRequest(ctx, domainidentity.RejectJoinRequestParams{
		JoinRequestID:  req.GetJoinRequestId(),
		Reason:         req.GetReason(),
		ReviewedBy:     authContext.Account.ID,
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &adminv1.RejectJoinRequestResponse{
		JoinRequest: joinRequestProto(joinRequest),
	}, nil
}

// CreateAccount creates one direct user or bot account.
func (a *api) CreateAccount(
	ctx context.Context,
	req *adminv1.CreateAccountRequest,
) (*adminv1.CreateAccountResponse, error) {
	authContext, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin)
	if err != nil {
		return nil, err
	}

	roles, err := rolesFromProto(req.GetRoles())
	if err != nil {
		return nil, err
	}

	accountKind, err := accountKindFromProto(req.GetAccountKind())
	if err != nil {
		return nil, err
	}

	account, botToken, err := a.identity.CreateAccount(ctx, domainidentity.CreateAccountParams{
		Username:       req.GetUsername(),
		DisplayName:    req.GetDisplayName(),
		Email:          req.GetEmail(),
		Phone:          req.GetPhone(),
		Password:       req.GetPassword(),
		Roles:          roles,
		Note:           req.GetNote(),
		InviteCode:     req.GetInviteCode(),
		AccountKind:    accountKind,
		CreatedBy:      authContext.Account.ID,
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	profile, err := a.profileForAccount(ctx, account)
	if err != nil {
		return nil, err
	}

	return &adminv1.CreateAccountResponse{
		Profile:  profile,
		BotToken: botToken,
	}, nil
}

func (a *api) listJoinRequests(
	ctx context.Context,
	statusFilter commonv1.JoinRequestStatus,
) ([]domainidentity.JoinRequest, error) {
	statuses, listAll, err := joinStatusesFromProto(statusFilter)
	if err != nil {
		return nil, err
	}
	if !listAll {
		joinRequests, err := a.identity.ListJoinRequestsByStatus(ctx, statuses[0])
		if err != nil {
			return nil, grpcError(err)
		}

		return joinRequests, nil
	}

	joinRequests := make([]domainidentity.JoinRequest, 0)
	for _, statusValue := range statuses {
		rows, err := a.identity.ListJoinRequestsByStatus(ctx, statusValue)
		if err != nil {
			return nil, grpcError(err)
		}
		joinRequests = append(joinRequests, rows...)
	}

	sort.Slice(joinRequests, func(i, j int) bool {
		if joinRequests[i].RequestedAt.Equal(joinRequests[j].RequestedAt) {
			return joinRequests[i].ID < joinRequests[j].ID
		}

		return joinRequests[i].RequestedAt.Before(joinRequests[j].RequestedAt)
	})

	return joinRequests, nil
}

func (a *api) profileForAccount(
	ctx context.Context,
	account domainidentity.Account,
) (*usersv1.UserProfile, error) {
	snapshot := domainpresence.Snapshot{}
	if a != nil && a.presence != nil {
		resolved, err := a.presence.GetPresence(ctx, domainpresence.GetParams{
			AccountID:       account.ID,
			ViewerAccountID: account.ID,
		})
		if err != nil && !errors.Is(err, domainpresence.ErrNotFound) {
			return nil, grpcError(err)
		}
		snapshot = resolved
	}

	return &usersv1.UserProfile{
		UserId:           account.ID,
		Username:         account.Username,
		DisplayName:      account.DisplayName,
		Bio:              account.Bio,
		Phone:            account.Phone,
		Email:            account.Email,
		Verified:         false,
		CustomBadgeEmoji: account.CustomBadgeEmoji,
		Presence:         presenceStateToProto(snapshot.State),
		CustomStatus:     snapshot.CustomStatus,
		LastSeenAt:       protoTime(snapshot.LastSeenAt),
		CreatedAt:        protoTime(account.CreatedAt),
		UpdatedAt:        protoTime(account.UpdatedAt),
		AccountKind:      accountKindToProto(account.Kind),
	}, nil
}

func joinRequestProto(joinRequest domainidentity.JoinRequest) *adminv1.JoinRequest {
	return &adminv1.JoinRequest{
		JoinRequestId:    joinRequest.ID,
		Username:         joinRequest.Username,
		DisplayName:      joinRequest.DisplayName,
		Email:            joinRequest.Email,
		Phone:            joinRequest.Phone,
		Note:             joinRequest.Note,
		Status:           joinStatusToProto(joinRequest.Status),
		ReviewedByUserId: joinRequest.ReviewedBy,
		DecisionReason:   joinRequest.DecisionReason,
		RequestedAt:      protoTime(joinRequest.RequestedAt),
		ReviewedAt:       protoTime(joinRequest.ReviewedAt),
		ExpiresAt:        protoTime(joinRequest.ExpiresAt),
	}
}

func joinStatusToProto(status domainidentity.JoinRequestStatus) commonv1.JoinRequestStatus {
	switch status {
	case domainidentity.JoinRequestStatusPending:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_PENDING
	case domainidentity.JoinRequestStatusApproved:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_APPROVED
	case domainidentity.JoinRequestStatusRejected:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_REJECTED
	case domainidentity.JoinRequestStatusCancelled:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_CANCELLED
	case domainidentity.JoinRequestStatusExpired:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_EXPIRED
	default:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_UNSPECIFIED
	}
}

func joinStatusesFromProto(
	status commonv1.JoinRequestStatus,
) ([]domainidentity.JoinRequestStatus, bool, error) {
	switch status {
	case commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_UNSPECIFIED:
		return []domainidentity.JoinRequestStatus{
			domainidentity.JoinRequestStatusPending,
			domainidentity.JoinRequestStatusApproved,
			domainidentity.JoinRequestStatusRejected,
			domainidentity.JoinRequestStatusCancelled,
			domainidentity.JoinRequestStatusExpired,
		}, true, nil
	case commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_PENDING:
		return []domainidentity.JoinRequestStatus{domainidentity.JoinRequestStatusPending}, false, nil
	case commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_APPROVED:
		return []domainidentity.JoinRequestStatus{domainidentity.JoinRequestStatusApproved}, false, nil
	case commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_REJECTED:
		return []domainidentity.JoinRequestStatus{domainidentity.JoinRequestStatusRejected}, false, nil
	case commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_CANCELLED:
		return []domainidentity.JoinRequestStatus{domainidentity.JoinRequestStatusCancelled}, false, nil
	case commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_EXPIRED:
		return []domainidentity.JoinRequestStatus{domainidentity.JoinRequestStatusExpired}, false, nil
	default:
		return nil, false, grpcError(domainidentity.ErrInvalidInput)
	}
}

func rolesFromProto(values []adminv1.AccountRole) ([]domainidentity.Role, error) {
	if len(values) == 0 {
		return nil, nil
	}

	roles := make([]domainidentity.Role, 0, len(values))
	seen := make(map[domainidentity.Role]struct{}, len(values))
	for _, value := range values {
		role, err := roleFromProto(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[role]; ok {
			continue
		}

		seen[role] = struct{}{}
		roles = append(roles, role)
	}

	return roles, nil
}

func roleFromProto(value adminv1.AccountRole) (domainidentity.Role, error) {
	switch value {
	case adminv1.AccountRole_ACCOUNT_ROLE_OWNER:
		return domainidentity.RoleOwner, nil
	case adminv1.AccountRole_ACCOUNT_ROLE_ADMIN:
		return domainidentity.RoleAdmin, nil
	case adminv1.AccountRole_ACCOUNT_ROLE_MODERATOR:
		return domainidentity.RoleModerator, nil
	case adminv1.AccountRole_ACCOUNT_ROLE_SUPPORT:
		return domainidentity.RoleSupport, nil
	case adminv1.AccountRole_ACCOUNT_ROLE_AUDITOR:
		return domainidentity.RoleAuditor, nil
	default:
		return "", grpcError(domainidentity.ErrInvalidInput)
	}
}

func accountKindFromProto(kind commonv1.AccountKind) (domainidentity.AccountKind, error) {
	switch kind {
	case commonv1.AccountKind_ACCOUNT_KIND_UNSPECIFIED:
		return domainidentity.AccountKindUser, nil
	case commonv1.AccountKind_ACCOUNT_KIND_USER:
		return domainidentity.AccountKindUser, nil
	case commonv1.AccountKind_ACCOUNT_KIND_BOT:
		return domainidentity.AccountKindBot, nil
	default:
		return domainidentity.AccountKindUnspecified, grpcError(domainidentity.ErrInvalidInput)
	}
}

func accountKindToProto(kind domainidentity.AccountKind) commonv1.AccountKind {
	switch kind {
	case domainidentity.AccountKindUser:
		return commonv1.AccountKind_ACCOUNT_KIND_USER
	case domainidentity.AccountKindBot:
		return commonv1.AccountKind_ACCOUNT_KIND_BOT
	default:
		return commonv1.AccountKind_ACCOUNT_KIND_UNSPECIFIED
	}
}

func presenceStateToProto(state domainpresence.PresenceState) commonv1.PresenceState {
	switch state {
	case domainpresence.PresenceStateOffline:
		return commonv1.PresenceState_PRESENCE_STATE_OFFLINE
	case domainpresence.PresenceStateOnline:
		return commonv1.PresenceState_PRESENCE_STATE_ONLINE
	case domainpresence.PresenceStateAway:
		return commonv1.PresenceState_PRESENCE_STATE_AWAY
	case domainpresence.PresenceStateBusy:
		return commonv1.PresenceState_PRESENCE_STATE_BUSY
	case domainpresence.PresenceStateInvisible:
		return commonv1.PresenceState_PRESENCE_STATE_INVISIBLE
	default:
		return commonv1.PresenceState_PRESENCE_STATE_UNSPECIFIED
	}
}

func protoTime(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}

	return timestamppb.New(value)
}
