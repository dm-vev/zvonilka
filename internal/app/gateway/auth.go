package gateway

import (
	"context"
	"strings"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SubmitJoinRequest accepts a self-service account request.
func (a *api) SubmitJoinRequest(
	ctx context.Context,
	req *authv1.SubmitJoinRequestRequest,
) (*authv1.SubmitJoinRequestResponse, error) {
	joinRequest, err := a.identity.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:       req.GetUsername(),
		DisplayName:    req.GetDisplayName(),
		Email:          req.GetEmail(),
		Phone:          req.GetPhone(),
		Note:           req.GetNote(),
		InviteCode:     req.GetInviteCode(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.SubmitJoinRequestResponse{
		RequestId: joinRequest.ID,
		Status:    joinStatusToProto(joinRequest.Status),
		ExpiresAt: protoTime(joinRequest.ExpiresAt),
		NextStep:  "await_admin_approval",
	}, nil
}

// BeginLogin starts a code-based login challenge for a human account.
func (a *api) BeginLogin(
	ctx context.Context,
	req *authv1.BeginLoginRequest,
) (*authv1.BeginLoginResponse, error) {
	params := identity.BeginLoginParams{
		Delivery:       loginChannelFromProto(req.GetDeliveryChannel()),
		DeviceName:     req.GetDeviceName(),
		Platform:       identityPlatformFromProto(req.GetDevicePlatform()),
		ClientVersion:  req.GetClientVersion(),
		Locale:         req.GetLocale(),
		IdempotencyKey: req.GetIdempotencyKey(),
	}
	switch identifier := req.Identifier.(type) {
	case *authv1.BeginLoginRequest_Username:
		params.Username = identifier.Username
	case *authv1.BeginLoginRequest_Email:
		params.Email = identifier.Email
	case *authv1.BeginLoginRequest_Phone:
		params.Phone = identifier.Phone
	}

	challenge, targets, err := a.identity.BeginLogin(ctx, params)
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.BeginLoginResponse{
		ChallengeId:           challenge.ID,
		Targets:               authTargets(targets),
		ExpiresAt:             protoTime(challenge.ExpiresAt),
		RequiresTwoFactor:     false,
		RequiresPassword:      false,
		RequiresAdminApproval: false,
		MaskedUsername:        "",
	}, nil
}

// VerifyLoginCode completes a login challenge and issues a session.
func (a *api) VerifyLoginCode(
	ctx context.Context,
	req *authv1.VerifyLoginCodeRequest,
) (*authv1.VerifyLoginCodeResponse, error) {
	result, err := a.identity.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:            req.GetChallengeId(),
		Code:                   req.GetCode(),
		TwoFactorCode:          req.GetTwoFactorCode(),
		RecoveryPassword:       req.GetRecoveryPassword(),
		EnablePasswordRecovery: req.GetEnablePasswordRecovery(),
		DeviceName:             req.GetDeviceName(),
		Platform:               identityPlatformFromProto(req.GetDevicePlatform()),
		PublicKey:              publicKeyFromProto(req.GetDeviceKey()),
		IdempotencyKey:         req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.VerifyLoginCodeResponse{
		Tokens:          authTokens(result),
		Session:         authSession(result.Session),
		Device:          authDevice(result.Device),
		RecoveryEnabled: req.GetEnablePasswordRecovery(),
	}, nil
}

// AuthenticateBot logs a bot account in with its issued bot token.
func (a *api) AuthenticateBot(
	ctx context.Context,
	req *authv1.AuthenticateBotRequest,
) (*authv1.AuthenticateBotResponse, error) {
	result, err := a.identity.AuthenticateBot(ctx, identity.AuthenticateBotParams{
		BotToken:       req.GetBotToken(),
		DeviceName:     req.GetDeviceName(),
		Platform:       identityPlatformFromProto(req.GetDevicePlatform()),
		PublicKey:      publicKeyFromProto(req.GetDeviceKey()),
		ClientVersion:  req.GetClientVersion(),
		Locale:         req.GetLocale(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.AuthenticateBotResponse{
		Tokens:  authTokens(result),
		Session: authSession(result.Session),
		Device:  authDevice(result.Device),
	}, nil
}

// RefreshSession rotates the bearer token pair for an existing session.
func (a *api) RefreshSession(
	ctx context.Context,
	req *authv1.RefreshSessionRequest,
) (*authv1.RefreshSessionResponse, error) {
	result, err := a.identity.RefreshSession(ctx, identity.RefreshSessionParams{
		RefreshToken:   req.GetRefreshToken(),
		DeviceID:       req.GetDeviceId(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.RefreshSessionResponse{
		Tokens:  authTokens(result),
		Session: authSession(result.Session),
	}, nil
}

// RegisterDevice attaches a new trusted device to the authenticated session.
func (a *api) RegisterDevice(
	ctx context.Context,
	req *authv1.RegisterDeviceRequest,
) (*authv1.RegisterDeviceResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		sessionID = authContext.Session.ID
	}
	if sessionID != authContext.Session.ID {
		return nil, grpcError(identity.ErrForbidden)
	}

	device, session, err := a.identity.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      sessionID,
		DeviceName:     req.GetDeviceName(),
		Platform:       identityPlatformFromProto(req.GetDevicePlatform()),
		PublicKey:      publicKeyFromProto(req.GetDeviceKey()),
		PushToken:      req.GetPushToken(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.RegisterDeviceResponse{
		Device:  authDevice(device),
		Session: authSession(session),
	}, nil
}

// ListDevices returns the authenticated account's devices.
func (a *api) ListDevices(
	ctx context.Context,
	req *authv1.ListDevicesRequest,
) (*authv1.ListDevicesResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	devices, err := a.identity.ListDevices(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	if !req.GetIncludeRevoked() {
		filtered := devices[:0]
		for _, device := range devices {
			if device.Status == identity.DeviceStatusRevoked {
				continue
			}
			filtered = append(filtered, device)
		}
		devices = filtered
	}

	offset, err := decodeOffset(req.GetPage(), "devices")
	if err != nil {
		return nil, grpcError(identity.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(devices) {
		end = len(devices)
	}

	page := devices
	if offset < len(devices) {
		page = devices[offset:end]
	} else {
		page = nil
	}
	nextToken := ""
	if end < len(devices) {
		nextToken = offsetToken("devices", end)
	}

	result := make([]*authv1.Device, 0, len(page))
	for _, device := range page {
		result = append(result, authDevice(device))
	}

	return &authv1.ListDevicesResponse{
		Devices: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(devices)),
		},
	}, nil
}

// ListSessions returns the authenticated account's sessions.
func (a *api) ListSessions(
	ctx context.Context,
	req *authv1.ListSessionsRequest,
) (*authv1.ListSessionsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := a.identity.ListSessions(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	if !req.GetIncludeRevoked() {
		filtered := sessions[:0]
		for _, session := range sessions {
			if session.Status == identity.SessionStatusRevoked {
				continue
			}
			filtered = append(filtered, session)
		}
		sessions = filtered
	}

	offset, err := decodeOffset(req.GetPage(), "sessions")
	if err != nil {
		return nil, grpcError(identity.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(sessions) {
		end = len(sessions)
	}

	page := sessions
	if offset < len(sessions) {
		page = sessions[offset:end]
	} else {
		page = nil
	}
	nextToken := ""
	if end < len(sessions) {
		nextToken = offsetToken("sessions", end)
	}

	result := make([]*authv1.Session, 0, len(page))
	for _, session := range page {
		result = append(result, authSession(session))
	}

	return &authv1.ListSessionsResponse{
		Sessions: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(sessions)),
		},
	}, nil
}

// RevokeSession revokes one session of the authenticated account.
func (a *api) RevokeSession(
	ctx context.Context,
	req *authv1.RevokeSessionRequest,
) (*authv1.RevokeSessionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := a.identity.ListSessions(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	owned := false
	for _, session := range sessions {
		if session.ID == req.GetSessionId() {
			owned = true
			break
		}
	}
	if !owned {
		return nil, grpcError(identity.ErrForbidden)
	}

	session, err := a.identity.RevokeSession(ctx, identity.RevokeSessionParams{
		SessionID:      req.GetSessionId(),
		Reason:         req.GetReason(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.RevokeSessionResponse{
		Revoked: true,
		Session: authSession(session),
	}, nil
}

// RevokeAllSessions revokes all sessions that belong to the authenticated account.
func (a *api) RevokeAllSessions(
	ctx context.Context,
	req *authv1.RevokeAllSessionsRequest,
) (*authv1.RevokeAllSessionsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	revoked, err := a.identity.RevokeAllSessions(ctx, authContext.Account.ID, identity.RevokeAllSessionsParams{
		Reason:         req.GetReason(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &authv1.RevokeAllSessionsResponse{RevokedSessions: revoked}, nil
}

func authTokens(result identity.LoginResult) *commonv1.AuthTokens {
	return &commonv1.AuthTokens{
		SessionId:        result.Session.ID,
		AccessToken:      result.Tokens.AccessToken,
		RefreshToken:     result.Tokens.RefreshToken,
		TokenType:        result.Tokens.TokenType,
		AccessExpiresAt:  protoTime(result.Tokens.ExpiresAt),
		RefreshExpiresAt: protoTime(result.Tokens.RefreshExpiresAt),
	}
}

func joinStatusToProto(status identity.JoinRequestStatus) commonv1.JoinRequestStatus {
	switch status {
	case identity.JoinRequestStatusPending:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_PENDING
	case identity.JoinRequestStatusApproved:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_APPROVED
	case identity.JoinRequestStatusRejected:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_REJECTED
	case identity.JoinRequestStatusCancelled:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_CANCELLED
	case identity.JoinRequestStatusExpired:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_EXPIRED
	default:
		return commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_UNSPECIFIED
	}
}
