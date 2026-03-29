package gateway

import (
	"context"
	"encoding/base64"
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	e2eev1 "github.com/dm-vev/zvonilka/gen/proto/contracts/e2ee/v1"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
)

func (a *api) UploadDevicePreKeys(
	ctx context.Context,
	req *e2eev1.UploadDevicePreKeysRequest,
) (*e2eev1.UploadDevicePreKeysResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		deviceID = authContext.Session.DeviceID
	}
	if deviceID != authContext.Session.DeviceID {
		return nil, grpcError(domaine2ee.ErrForbidden)
	}

	bundle, err := a.e2ee.UploadDevicePreKeys(ctx, domaine2ee.UploadDevicePreKeysParams{
		AccountID:            authContext.Account.ID,
		DeviceID:             deviceID,
		SignedPreKey:         signedPreKeyFromProto(req.GetSignedPrekey()),
		OneTimePreKeys:       oneTimePreKeysFromProto(req.GetOneTimePrekeys()),
		ReplaceOneTimePreKey: req.GetReplaceOneTimePrekeys(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &e2eev1.UploadDevicePreKeysResponse{
		Bundle: deviceBundleProto(bundle),
	}, nil
}

func (a *api) GetAccountPreKeyBundles(
	ctx context.Context,
	req *e2eev1.GetAccountPreKeyBundlesRequest,
) (*e2eev1.GetAccountPreKeyBundlesResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	bundles, err := a.e2ee.GetAccountBundles(ctx, domaine2ee.FetchAccountBundlesParams{
		RequesterAccountID:   authContext.Account.ID,
		RequesterDeviceID:    authContext.Session.DeviceID,
		TargetAccountID:      req.GetUserId(),
		ConsumeOneTimePreKey: req.GetConsumeOneTimePrekeys(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*e2eev1.DevicePreKeyBundle, 0, len(bundles))
	for _, bundle := range bundles {
		result = append(result, deviceBundleProto(bundle))
	}
	return &e2eev1.GetAccountPreKeyBundlesResponse{Bundles: result}, nil
}

func (a *api) CreateDirectSessions(
	ctx context.Context,
	req *e2eev1.CreateDirectSessionsRequest,
) (*e2eev1.CreateDirectSessionsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := a.e2ee.CreateDirectSessions(ctx, domaine2ee.CreateDirectSessionsParams{
		InitiatorAccountID: authContext.Account.ID,
		InitiatorDeviceID:  authContext.Session.DeviceID,
		TargetAccountID:    req.GetUserId(),
		TargetDeviceID:     req.GetDeviceId(),
		InitiatorEphemeral: publicKeyBundleFromProto(req.GetInitiatorEphemeralKey()),
		Bootstrap:          bootstrapPayloadFromProto(req.GetBootstrap()),
		ExpiresAt:          zeroTime(req.GetExpiresAt()),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*e2eev1.DirectSession, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, directSessionProto(session))
	}
	return &e2eev1.CreateDirectSessionsResponse{Sessions: result}, nil
}

func (a *api) ListDeviceSessions(
	ctx context.Context,
	req *e2eev1.ListDeviceSessionsRequest,
) (*e2eev1.ListDeviceSessionsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := a.e2ee.ListDeviceSessions(ctx, domaine2ee.ListDeviceSessionsParams{
		AccountID:           authContext.Account.ID,
		DeviceID:            authContext.Session.DeviceID,
		IncludeAcknowledged: req.GetIncludeAcknowledged(),
		PeerAccountID:       req.GetPeerUserId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*e2eev1.DirectSession, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, directSessionProto(session))
	}
	return &e2eev1.ListDeviceSessionsResponse{Sessions: result}, nil
}

func (a *api) AcknowledgeDirectSession(
	ctx context.Context,
	req *e2eev1.AcknowledgeDirectSessionRequest,
) (*e2eev1.AcknowledgeDirectSessionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	session, err := a.e2ee.AcknowledgeDirectSession(ctx, domaine2ee.AcknowledgeDirectSessionParams{
		SessionID:          req.GetSessionId(),
		RecipientAccountID: authContext.Account.ID,
		RecipientDeviceID:  authContext.Session.DeviceID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &e2eev1.AcknowledgeDirectSessionResponse{Session: directSessionProto(session)}, nil
}

func deviceBundleProto(value domaine2ee.DeviceBundle) *e2eev1.DevicePreKeyBundle {
	return &e2eev1.DevicePreKeyBundle{
		UserId:                  value.AccountID,
		DeviceId:                value.DeviceID,
		IdentityKey:             publicKeyBundleProto(value.IdentityKey),
		SignedPrekey:            signedPreKeyProto(value.SignedPreKey),
		OneTimePrekey:           oneTimePreKeyProto(value.OneTimePreKey),
		OneTimePrekeysAvailable: value.OneTimePreKeysAvail,
		DeviceLastSeenAt:        protoTime(value.DeviceLastSeenAt),
	}
}

func signedPreKeyProto(value domaine2ee.SignedPreKey) *e2eev1.SignedPreKey {
	if value.Key.KeyID == "" && len(value.Key.PublicKey) == 0 && len(value.Signature) == 0 {
		return nil
	}
	return &e2eev1.SignedPreKey{
		Key:       publicKeyBundleProto(value.Key),
		Signature: append([]byte(nil), value.Signature...),
	}
}

func oneTimePreKeyProto(value domaine2ee.OneTimePreKey) *e2eev1.PreKey {
	if value.Key.KeyID == "" && len(value.Key.PublicKey) == 0 {
		return nil
	}
	return &e2eev1.PreKey{Key: publicKeyBundleProto(value.Key)}
}

func publicKeyBundleProto(value domaine2ee.PublicKey) *commonv1.PublicKeyBundle {
	if strings.TrimSpace(value.KeyID) == "" && strings.TrimSpace(value.Algorithm) == "" && len(value.PublicKey) == 0 {
		return nil
	}
	return &commonv1.PublicKeyBundle{
		KeyId:     value.KeyID,
		Algorithm: value.Algorithm,
		PublicKey: append([]byte(nil), value.PublicKey...),
		CreatedAt: protoTime(value.CreatedAt),
		RotatedAt: protoTime(value.RotatedAt),
		ExpiresAt: protoTime(value.ExpiresAt),
	}
}

func directSessionProto(value domaine2ee.DirectSession) *e2eev1.DirectSession {
	return &e2eev1.DirectSession{
		SessionId:            value.ID,
		InitiatorUserId:      value.InitiatorAccountID,
		InitiatorDeviceId:    value.InitiatorDeviceID,
		RecipientUserId:      value.RecipientAccountID,
		RecipientDeviceId:    value.RecipientDeviceID,
		InitiatorEphemeralKey: publicKeyBundleProto(value.InitiatorEphemeral),
		IdentityKey:          publicKeyBundleProto(value.IdentityKey),
		SignedPrekey:         signedPreKeyProto(value.SignedPreKey),
		OneTimePrekey:        oneTimePreKeyProto(value.OneTimePreKey),
		Bootstrap:            bootstrapPayloadProto(value.Bootstrap),
		State:                directSessionStateProto(value.State),
		CreatedAt:            protoTime(value.CreatedAt),
		AcknowledgedAt:       protoTime(value.AcknowledgedAt),
		ExpiresAt:            protoTime(value.ExpiresAt),
	}
}

func bootstrapPayloadProto(value domaine2ee.BootstrapPayload) *e2eev1.SessionBootstrapPayload {
	if strings.TrimSpace(value.Algorithm) == "" && len(value.Ciphertext) == 0 {
		return nil
	}
	result := &e2eev1.SessionBootstrapPayload{
		Algorithm:  value.Algorithm,
		Nonce:      append([]byte(nil), value.Nonce...),
		Ciphertext: append([]byte(nil), value.Ciphertext...),
	}
	if len(value.Metadata) > 0 {
		result.Metadata = make(map[string]string, len(value.Metadata))
		for key, item := range value.Metadata {
			result.Metadata[key] = item
		}
	}
	return result
}

func signedPreKeyFromProto(value *e2eev1.SignedPreKey) domaine2ee.SignedPreKey {
	if value == nil {
		return domaine2ee.SignedPreKey{}
	}
	return domaine2ee.SignedPreKey{
		Key:       publicKeyBundleFromProto(value.GetKey()),
		Signature: append([]byte(nil), value.GetSignature()...),
	}
}

func oneTimePreKeysFromProto(values []*e2eev1.PreKey) []domaine2ee.OneTimePreKey {
	if len(values) == 0 {
		return nil
	}
	result := make([]domaine2ee.OneTimePreKey, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		result = append(result, domaine2ee.OneTimePreKey{
			Key: publicKeyBundleFromProto(value.GetKey()),
		})
	}
	return result
}

func publicKeyBundleFromProto(value *commonv1.PublicKeyBundle) domaine2ee.PublicKey {
	if value == nil {
		return domaine2ee.PublicKey{}
	}
	keyBytes := append([]byte(nil), value.GetPublicKey()...)
	if len(keyBytes) == 0 {
		raw := strings.TrimSpace(base64.RawStdEncoding.EncodeToString(value.GetPublicKey()))
		if raw != "" {
			keyBytes = []byte(raw)
		}
	}
	return domaine2ee.PublicKey{
		KeyID:     strings.TrimSpace(value.GetKeyId()),
		Algorithm: strings.TrimSpace(value.GetAlgorithm()),
		PublicKey: keyBytes,
		CreatedAt: zeroTime(value.GetCreatedAt()),
		RotatedAt: zeroTime(value.GetRotatedAt()),
		ExpiresAt: zeroTime(value.GetExpiresAt()),
	}
}

func bootstrapPayloadFromProto(value *e2eev1.SessionBootstrapPayload) domaine2ee.BootstrapPayload {
	if value == nil {
		return domaine2ee.BootstrapPayload{}
	}
	result := domaine2ee.BootstrapPayload{
		Algorithm:  strings.TrimSpace(value.GetAlgorithm()),
		Nonce:      append([]byte(nil), value.GetNonce()...),
		Ciphertext: append([]byte(nil), value.GetCiphertext()...),
	}
	if len(value.GetMetadata()) > 0 {
		result.Metadata = make(map[string]string, len(value.GetMetadata()))
		for key, item := range value.GetMetadata() {
			result.Metadata[key] = item
		}
	}
	return result
}

func directSessionStateProto(value domaine2ee.DirectSessionState) e2eev1.DirectSessionState {
	switch value {
	case domaine2ee.DirectSessionStatePending:
		return e2eev1.DirectSessionState_DIRECT_SESSION_STATE_PENDING
	case domaine2ee.DirectSessionStateAcknowledged:
		return e2eev1.DirectSessionState_DIRECT_SESSION_STATE_ACKNOWLEDGED
	default:
		return e2eev1.DirectSessionState_DIRECT_SESSION_STATE_UNSPECIFIED
	}
}
