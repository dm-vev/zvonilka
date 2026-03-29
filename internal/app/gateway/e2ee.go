package gateway

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	e2eev1 "github.com/dm-vev/zvonilka/gen/proto/contracts/e2ee/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
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
	a.publishE2EEUpdates(domaine2ee.Update{
		ID:              newGatewayEventID("e2ee"),
		Type:            domaine2ee.UpdateTypeDevicePreKeysUpdated,
		ActorAccountID:  authContext.Account.ID,
		ActorDeviceID:   deviceID,
		TargetAccountID: authContext.Account.ID,
		TargetDeviceID:  deviceID,
		CreatedAt:       timeNowUTC(),
	})

	return &e2eev1.UploadDevicePreKeysResponse{
		Bundle: deviceBundleProto(bundle),
	}, nil
}

func (a *api) RotateE2EEKeys(
	ctx context.Context,
	req *e2eev1.RotateE2EEKeysRequest,
) (*e2eev1.RotateE2EEKeysResponse, error) {
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

	bundle, expiredDirect, expiredGroup, err := a.e2ee.RotateDeviceKeys(ctx, domaine2ee.RotateDeviceKeysParams{
		AccountID:                    authContext.Account.ID,
		DeviceID:                     deviceID,
		SignedPreKey:                 signedPreKeyFromProto(req.GetSignedPrekey()),
		OneTimePreKeys:               oneTimePreKeysFromProto(req.GetOneTimePrekeys()),
		ReplaceOneTimePreKey:         req.GetReplaceOneTimePrekeys(),
		ExpirePendingDirectSessions:  req.GetExpirePendingDirectSessions(),
		ExpirePendingGroupSenderKeys: req.GetExpirePendingGroupSenderKeys(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishE2EEUpdates(
		domaine2ee.Update{
			ID:              newGatewayEventID("e2ee"),
			Type:            domaine2ee.UpdateTypeDevicePreKeysUpdated,
			ActorAccountID:  authContext.Account.ID,
			ActorDeviceID:   deviceID,
			TargetAccountID: authContext.Account.ID,
			TargetDeviceID:  deviceID,
			CreatedAt:       timeNowUTC(),
		},
		domaine2ee.Update{
			ID:              newGatewayEventID("e2ee"),
			Type:            domaine2ee.UpdateTypeConversationCoverageChanged,
			ActorAccountID:  authContext.Account.ID,
			ActorDeviceID:   deviceID,
			TargetAccountID: authContext.Account.ID,
			TargetDeviceID:  deviceID,
			Metadata: map[string]string{
				"expired_pending_direct_sessions":   strconv.FormatUint(uint64(expiredDirect), 10),
				"expired_pending_group_sender_keys": strconv.FormatUint(uint64(expiredGroup), 10),
			},
			CreatedAt: timeNowUTC(),
		},
	)
	return &e2eev1.RotateE2EEKeysResponse{
		Bundle:                        deviceBundleProto(bundle),
		ExpiredPendingDirectSessions:  expiredDirect,
		ExpiredPendingGroupSenderKeys: expiredGroup,
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

func (a *api) GetDeviceVerificationCode(
	ctx context.Context,
	req *e2eev1.GetDeviceVerificationCodeRequest,
) (*e2eev1.GetDeviceVerificationCodeResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	code, err := a.e2ee.GetDeviceVerificationCode(ctx, domaine2ee.GetDeviceVerificationCodeParams{
		ObserverAccountID: authContext.Account.ID,
		ObserverDeviceID:  authContext.Session.DeviceID,
		TargetAccountID:   req.GetTargetUserId(),
		TargetDeviceID:    req.GetTargetDeviceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &e2eev1.GetDeviceVerificationCodeResponse{Verification: deviceVerificationCodeProto(code)}, nil
}

func (a *api) VerifyDeviceSafetyNumber(
	ctx context.Context,
	req *e2eev1.VerifyDeviceSafetyNumberRequest,
) (*e2eev1.VerifyDeviceSafetyNumberResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	trust, err := a.e2ee.VerifyDeviceSafetyNumber(ctx, domaine2ee.VerifyDeviceSafetyNumberParams{
		ObserverAccountID: authContext.Account.ID,
		ObserverDeviceID:  authContext.Session.DeviceID,
		TargetAccountID:   req.GetTargetUserId(),
		TargetDeviceID:    req.GetTargetDeviceId(),
		SafetyNumber:      req.GetSafetyNumber(),
		Note:              req.GetNote(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	conversationIDs, err := a.e2ee.SharedConversationIDs(ctx, authContext.Account.ID, trust.TargetAccountID)
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishE2EEUpdates(domaine2ee.Update{
		ID:                   newGatewayEventID("e2ee"),
		Type:                 domaine2ee.UpdateTypeDeviceVerified,
		ActorAccountID:       authContext.Account.ID,
		ActorDeviceID:        authContext.Session.DeviceID,
		TargetAccountID:      trust.TargetAccountID,
		TargetDeviceID:       trust.TargetDeviceID,
		CurrentTrustState:    trust.State,
		TargetKeyFingerprint: trust.KeyFingerprint,
		VerificationRequired: false,
		ConversationIDs:      conversationIDs,
		Metadata: map[string]string{
			"state": string(trust.State),
		},
		CreatedAt: timeNowUTC(),
	})
	a.publishConversationE2EERequiredActionEvents(ctx, authContext.Account.ID, authContext.Session.DeviceID, conversationIDs)
	return &e2eev1.VerifyDeviceSafetyNumberResponse{Trust: deviceTrustProto(trust)}, nil
}

func (a *api) SetDeviceTrust(
	ctx context.Context,
	req *e2eev1.SetDeviceTrustRequest,
) (*e2eev1.SetDeviceTrustResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	trust, err := a.e2ee.SetDeviceTrust(ctx, domaine2ee.SetDeviceTrustParams{
		ObserverAccountID: authContext.Account.ID,
		ObserverDeviceID:  authContext.Session.DeviceID,
		TargetAccountID:   req.GetTargetUserId(),
		TargetDeviceID:    req.GetTargetDeviceId(),
		State:             deviceTrustStateFromProto(req.GetState()),
		Note:              req.GetNote(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	conversationIDs, err := a.e2ee.SharedConversationIDs(ctx, authContext.Account.ID, trust.TargetAccountID)
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishE2EEUpdates(domaine2ee.Update{
		ID:                   newGatewayEventID("e2ee"),
		Type:                 domaine2ee.UpdateTypeDeviceTrustUpdated,
		ActorAccountID:       authContext.Account.ID,
		ActorDeviceID:        authContext.Session.DeviceID,
		TargetAccountID:      trust.TargetAccountID,
		TargetDeviceID:       trust.TargetDeviceID,
		CurrentTrustState:    trust.State,
		TargetKeyFingerprint: trust.KeyFingerprint,
		VerificationRequired: trust.State != domaine2ee.DeviceTrustStateTrusted,
		ConversationIDs:      conversationIDs,
		Metadata: map[string]string{
			"state": string(trust.State),
		},
		CreatedAt: timeNowUTC(),
	})
	a.publishConversationE2EERequiredActionEvents(ctx, authContext.Account.ID, authContext.Session.DeviceID, conversationIDs)
	return &e2eev1.SetDeviceTrustResponse{Trust: deviceTrustProto(trust)}, nil
}

func (a *api) ListDeviceTrusts(
	ctx context.Context,
	req *e2eev1.ListDeviceTrustsRequest,
) (*e2eev1.ListDeviceTrustsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	trusts, err := a.e2ee.ListDeviceTrusts(ctx, domaine2ee.ListDeviceTrustsParams{
		ObserverAccountID: authContext.Account.ID,
		ObserverDeviceID:  authContext.Session.DeviceID,
		TargetAccountID:   req.GetTargetUserId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	result := make([]*e2eev1.DeviceTrust, 0, len(trusts))
	for _, item := range trusts {
		result = append(result, deviceTrustProto(item))
	}
	return &e2eev1.ListDeviceTrustsResponse{Trusts: result}, nil
}

func (a *api) ListVerificationRequiredDevices(
	ctx context.Context,
	_ *e2eev1.ListVerificationRequiredDevicesRequest,
) (*e2eev1.ListVerificationRequiredDevicesResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	devices, err := a.e2ee.ListVerificationRequiredDevices(ctx, domaine2ee.ListVerificationRequiredDevicesParams{
		ObserverAccountID: authContext.Account.ID,
		ObserverDeviceID:  authContext.Session.DeviceID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	result := make([]*e2eev1.VerificationRequiredDevice, 0, len(devices))
	for _, item := range devices {
		result = append(result, verificationRequiredDeviceProto(item))
	}
	return &e2eev1.ListVerificationRequiredDevicesResponse{Devices: result}, nil
}

func (a *api) GetConversationKeyCoverage(
	ctx context.Context,
	req *e2eev1.GetConversationKeyCoverageRequest,
) (*e2eev1.GetConversationKeyCoverageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := a.e2ee.GetConversationKeyCoverage(ctx, domaine2ee.GetConversationKeyCoverageParams{
		ConversationID:  req.GetConversationId(),
		SenderAccountID: authContext.Account.ID,
		SenderDeviceID:  authContext.Session.DeviceID,
		SenderKeyID:     req.GetSenderKeyId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	result := make([]*e2eev1.ConversationKeyCoverageEntry, 0, len(entries))
	for _, item := range entries {
		result = append(result, conversationKeyCoverageEntryProto(item))
	}
	return &e2eev1.GetConversationKeyCoverageResponse{Entries: result}, nil
}

func (a *api) SubscribeE2EEUpdates(
	req *e2eev1.SubscribeE2EEUpdatesRequest,
	stream e2eev1.E2EEService_SubscribeE2EEUpdatesServer,
) error {
	authContext, err := a.requireAuth(stream.Context())
	if err != nil {
		return err
	}
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if deviceID == "" {
		deviceID = authContext.Session.DeviceID
	}
	if deviceID != authContext.Session.DeviceID {
		return grpcError(domaine2ee.ErrForbidden)
	}

	updates, unsubscribe := a.subscribeE2EEUpdates()
	defer unsubscribe()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case signal := <-updates:
			if signal.update == nil {
				continue
			}
			if !e2eeUpdateVisible(*signal.update, authContext.Account.ID, deviceID) {
				continue
			}
			if err := stream.Send(&e2eev1.SubscribeE2EEUpdatesResponse{Update: e2eeUpdateProto(*signal.update)}); err != nil {
				return err
			}
		}
	}
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
		a.publishE2EEUpdates(domaine2ee.Update{
			ID:              newGatewayEventID("e2ee"),
			Type:            domaine2ee.UpdateTypeDirectSessionCreated,
			ActorAccountID:  authContext.Account.ID,
			ActorDeviceID:   authContext.Session.DeviceID,
			TargetAccountID: session.RecipientAccountID,
			TargetDeviceID:  session.RecipientDeviceID,
			SessionID:       session.ID,
			CreatedAt:       timeNowUTC(),
		})
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
	a.publishE2EEUpdates(domaine2ee.Update{
		ID:              newGatewayEventID("e2ee"),
		Type:            domaine2ee.UpdateTypeDirectSessionAcknowledged,
		ActorAccountID:  authContext.Account.ID,
		ActorDeviceID:   authContext.Session.DeviceID,
		TargetAccountID: session.InitiatorAccountID,
		TargetDeviceID:  session.InitiatorDeviceID,
		SessionID:       session.ID,
		CreatedAt:       timeNowUTC(),
	})

	return &e2eev1.AcknowledgeDirectSessionResponse{Session: directSessionProto(session)}, nil
}

func (a *api) PublishGroupSenderKeys(
	ctx context.Context,
	req *e2eev1.PublishGroupSenderKeysRequest,
) (*e2eev1.PublishGroupSenderKeysResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	distributions, err := a.e2ee.PublishGroupSenderKeys(ctx, domaine2ee.PublishGroupSenderKeysParams{
		ConversationID:  req.GetConversationId(),
		SenderAccountID: authContext.Account.ID,
		SenderDeviceID:  authContext.Session.DeviceID,
		SenderKeyID:     req.GetSenderKeyId(),
		Recipients:      recipientSenderKeysFromProto(req.GetRecipients()),
		ExpiresAt:       zeroTime(req.GetExpiresAt()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	result := make([]*e2eev1.GroupSenderKeyDistribution, 0, len(distributions))
	for _, item := range distributions {
		a.publishE2EEUpdates(domaine2ee.Update{
			ID:              newGatewayEventID("e2ee"),
			Type:            domaine2ee.UpdateTypeGroupSenderKeyPublished,
			ActorAccountID:  authContext.Account.ID,
			ActorDeviceID:   authContext.Session.DeviceID,
			TargetAccountID: item.RecipientAccountID,
			TargetDeviceID:  item.RecipientDeviceID,
			ConversationID:  item.ConversationID,
			DistributionID:  item.ID,
			SenderKeyID:     item.SenderKeyID,
			CreatedAt:       timeNowUTC(),
		})
		result = append(result, groupSenderKeyDistributionProto(item))
	}
	return &e2eev1.PublishGroupSenderKeysResponse{Distributions: result}, nil
}

func (a *api) ListGroupSenderKeys(
	ctx context.Context,
	req *e2eev1.ListGroupSenderKeysRequest,
) (*e2eev1.ListGroupSenderKeysResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	distributions, err := a.e2ee.ListGroupSenderKeys(ctx, domaine2ee.ListGroupSenderKeysParams{
		ConversationID:      req.GetConversationId(),
		RecipientAccountID:  authContext.Account.ID,
		RecipientDeviceID:   authContext.Session.DeviceID,
		IncludeAcknowledged: req.GetIncludeAcknowledged(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	result := make([]*e2eev1.GroupSenderKeyDistribution, 0, len(distributions))
	for _, item := range distributions {
		result = append(result, groupSenderKeyDistributionProto(item))
	}
	return &e2eev1.ListGroupSenderKeysResponse{Distributions: result}, nil
}

func (a *api) AcknowledgeGroupSenderKey(
	ctx context.Context,
	req *e2eev1.AcknowledgeGroupSenderKeyRequest,
) (*e2eev1.AcknowledgeGroupSenderKeyResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	distribution, err := a.e2ee.AcknowledgeGroupSenderKey(ctx, domaine2ee.AcknowledgeGroupSenderKeyParams{
		DistributionID:     req.GetDistributionId(),
		RecipientAccountID: authContext.Account.ID,
		RecipientDeviceID:  authContext.Session.DeviceID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishE2EEUpdates(domaine2ee.Update{
		ID:              newGatewayEventID("e2ee"),
		Type:            domaine2ee.UpdateTypeGroupSenderKeyAcknowledged,
		ActorAccountID:  authContext.Account.ID,
		ActorDeviceID:   authContext.Session.DeviceID,
		TargetAccountID: distribution.SenderAccountID,
		TargetDeviceID:  distribution.SenderDeviceID,
		ConversationID:  distribution.ConversationID,
		DistributionID:  distribution.ID,
		SenderKeyID:     distribution.SenderKeyID,
		CreatedAt:       timeNowUTC(),
	})
	return &e2eev1.AcknowledgeGroupSenderKeyResponse{Distribution: groupSenderKeyDistributionProto(distribution)}, nil
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

func deviceTrustProto(value domaine2ee.DeviceTrust) *e2eev1.DeviceTrust {
	return &e2eev1.DeviceTrust{
		ObserverUserId:   value.ObserverAccountID,
		ObserverDeviceId: value.ObserverDeviceID,
		TargetUserId:     value.TargetAccountID,
		TargetDeviceId:   value.TargetDeviceID,
		State:            deviceTrustStateProto(value.State),
		KeyFingerprint:   value.KeyFingerprint,
		Note:             value.Note,
		CreatedAt:        protoTime(value.CreatedAt),
		UpdatedAt:        protoTime(value.UpdatedAt),
	}
}

func deviceVerificationCodeProto(value domaine2ee.DeviceVerificationCode) *e2eev1.DeviceVerificationCode {
	return &e2eev1.DeviceVerificationCode{
		ObserverUserId:       value.ObserverAccountID,
		ObserverDeviceId:     value.ObserverDeviceID,
		TargetUserId:         value.TargetAccountID,
		TargetDeviceId:       value.TargetDeviceID,
		TargetKeyFingerprint: value.TargetKeyFingerprint,
		SafetyNumber:         value.SafetyNumber,
		CurrentTrustState:    deviceTrustStateProto(value.CurrentTrustState),
	}
}

func verificationRequiredDeviceProto(value domaine2ee.VerificationRequiredDevice) *e2eev1.VerificationRequiredDevice {
	return &e2eev1.VerificationRequiredDevice{
		UserId:             value.AccountID,
		DeviceId:           value.DeviceID,
		TrustState:         deviceTrustStateProto(value.TrustState),
		KeyFingerprint:     value.KeyFingerprint,
		ConversationIds:    append([]string(nil), value.ConversationIDs...),
		DirectConversation: value.DirectConversation,
	}
}

func (a *api) publishConversationE2EERequiredActionEvents(
	ctx context.Context,
	accountID string,
	deviceID string,
	conversationIDs []string,
) {
	if a == nil || len(conversationIDs) == 0 {
		return
	}

	overlays, err := a.conversationE2EEOverlays(ctx, accountID, deviceID)
	if err != nil {
		return
	}

	for _, conversationID := range conversationIDs {
		overlay := overlays[conversationID]
		a.publishSyntheticSyncEvent(domainconversation.EventEnvelope{
			EventID:        newGatewayEventID("sync"),
			EventType:      domainconversation.EventTypeConversationUpdated,
			ConversationID: conversationID,
			ActorAccountID: accountID,
			ActorDeviceID:  deviceID,
			PayloadType:    "e2ee_required_action",
			Metadata: map[string]string{
				"verification_required_devices": strconv.FormatUint(uint64(overlay.VerificationRequiredDevices), 10),
				"e2ee_required_action":          overlay.RequiredAction.String(),
			},
			CreatedAt: timeNowUTC(),
		})
	}
}

func e2eeUpdateProto(value domaine2ee.Update) *e2eev1.E2EEUpdate {
	return &e2eev1.E2EEUpdate{
		UpdateId:             value.ID,
		UpdateType:           e2eeUpdateTypeProto(value.Type),
		ActorUserId:          value.ActorAccountID,
		ActorDeviceId:        value.ActorDeviceID,
		TargetUserId:         value.TargetAccountID,
		TargetDeviceId:       value.TargetDeviceID,
		ConversationId:       value.ConversationID,
		SessionId:            value.SessionID,
		DistributionId:       value.DistributionID,
		SenderKeyId:          value.SenderKeyID,
		Metadata:             cloneStringMap(value.Metadata),
		CreatedAt:            protoTime(value.CreatedAt),
		CurrentTrustState:    deviceTrustStateProto(value.CurrentTrustState),
		TargetKeyFingerprint: value.TargetKeyFingerprint,
		VerificationRequired: value.VerificationRequired,
		ConversationIds:      append([]string(nil), value.ConversationIDs...),
	}
}

func conversationKeyCoverageEntryProto(value domaine2ee.ConversationKeyCoverageEntry) *e2eev1.ConversationKeyCoverageEntry {
	return &e2eev1.ConversationKeyCoverageEntry{
		UserId:               value.AccountID,
		DeviceId:             value.DeviceID,
		State:                conversationKeyCoverageStateProto(value.State),
		ReferenceId:          value.ReferenceID,
		ExpiresAt:            protoTime(value.ExpiresAt),
		TrustState:           deviceTrustStateProto(value.TrustState),
		KeyFingerprint:       value.KeyFingerprint,
		VerificationRequired: value.VerificationRequired,
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
		SessionId:             value.ID,
		InitiatorUserId:       value.InitiatorAccountID,
		InitiatorDeviceId:     value.InitiatorDeviceID,
		RecipientUserId:       value.RecipientAccountID,
		RecipientDeviceId:     value.RecipientDeviceID,
		InitiatorEphemeralKey: publicKeyBundleProto(value.InitiatorEphemeral),
		IdentityKey:           publicKeyBundleProto(value.IdentityKey),
		SignedPrekey:          signedPreKeyProto(value.SignedPreKey),
		OneTimePrekey:         oneTimePreKeyProto(value.OneTimePreKey),
		Bootstrap:             bootstrapPayloadProto(value.Bootstrap),
		State:                 directSessionStateProto(value.State),
		CreatedAt:             protoTime(value.CreatedAt),
		AcknowledgedAt:        protoTime(value.AcknowledgedAt),
		ExpiresAt:             protoTime(value.ExpiresAt),
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

func groupSenderKeyDistributionProto(value domaine2ee.GroupSenderKeyDistribution) *e2eev1.GroupSenderKeyDistribution {
	return &e2eev1.GroupSenderKeyDistribution{
		DistributionId:    value.ID,
		ConversationId:    value.ConversationID,
		SenderUserId:      value.SenderAccountID,
		SenderDeviceId:    value.SenderDeviceID,
		RecipientUserId:   value.RecipientAccountID,
		RecipientDeviceId: value.RecipientDeviceID,
		SenderKeyId:       value.SenderKeyID,
		Payload:           senderKeyPayloadProto(value.Payload),
		State:             groupSenderKeyStateProto(value.State),
		CreatedAt:         protoTime(value.CreatedAt),
		AcknowledgedAt:    protoTime(value.AcknowledgedAt),
		ExpiresAt:         protoTime(value.ExpiresAt),
	}
}

func senderKeyPayloadProto(value domaine2ee.SenderKeyPayload) *e2eev1.SenderKeyPayload {
	if strings.TrimSpace(value.Algorithm) == "" && len(value.Ciphertext) == 0 {
		return nil
	}
	result := &e2eev1.SenderKeyPayload{
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

func senderKeyPayloadFromProto(value *e2eev1.SenderKeyPayload) domaine2ee.SenderKeyPayload {
	if value == nil {
		return domaine2ee.SenderKeyPayload{}
	}
	result := domaine2ee.SenderKeyPayload{
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

func recipientSenderKeysFromProto(values []*e2eev1.RecipientSenderKey) []domaine2ee.RecipientSenderKey {
	if len(values) == 0 {
		return nil
	}
	result := make([]domaine2ee.RecipientSenderKey, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		result = append(result, domaine2ee.RecipientSenderKey{
			RecipientAccountID: strings.TrimSpace(value.GetRecipientUserId()),
			RecipientDeviceID:  strings.TrimSpace(value.GetRecipientDeviceId()),
			Payload:            senderKeyPayloadFromProto(value.GetPayload()),
		})
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

func groupSenderKeyStateProto(value domaine2ee.GroupSenderKeyState) e2eev1.GroupSenderKeyState {
	switch value {
	case domaine2ee.GroupSenderKeyStatePending:
		return e2eev1.GroupSenderKeyState_GROUP_SENDER_KEY_STATE_PENDING
	case domaine2ee.GroupSenderKeyStateAcknowledged:
		return e2eev1.GroupSenderKeyState_GROUP_SENDER_KEY_STATE_ACKNOWLEDGED
	default:
		return e2eev1.GroupSenderKeyState_GROUP_SENDER_KEY_STATE_UNSPECIFIED
	}
}

func deviceTrustStateProto(value domaine2ee.DeviceTrustState) e2eev1.DeviceTrustState {
	switch value {
	case domaine2ee.DeviceTrustStateTrusted:
		return e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED
	case domaine2ee.DeviceTrustStateUntrusted:
		return e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_UNTRUSTED
	case domaine2ee.DeviceTrustStateCompromised:
		return e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_COMPROMISED
	default:
		return e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_UNSPECIFIED
	}
}

func e2eeUpdateTypeProto(value domaine2ee.UpdateType) e2eev1.E2EEUpdateType {
	switch value {
	case domaine2ee.UpdateTypeDevicePreKeysUpdated:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_DEVICE_PREKEYS_UPDATED
	case domaine2ee.UpdateTypeDeviceTrustUpdated:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_DEVICE_TRUST_UPDATED
	case domaine2ee.UpdateTypeDirectSessionCreated:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_DIRECT_SESSION_CREATED
	case domaine2ee.UpdateTypeDirectSessionAcknowledged:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_DIRECT_SESSION_ACKNOWLEDGED
	case domaine2ee.UpdateTypeGroupSenderKeyPublished:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_GROUP_SENDER_KEY_PUBLISHED
	case domaine2ee.UpdateTypeGroupSenderKeyAcknowledged:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_GROUP_SENDER_KEY_ACKNOWLEDGED
	case domaine2ee.UpdateTypeConversationCoverageChanged:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_CONVERSATION_KEY_COVERAGE_CHANGED
	case domaine2ee.UpdateTypeDeviceVerified:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_DEVICE_VERIFIED
	default:
		return e2eev1.E2EEUpdateType_E2EE_UPDATE_TYPE_UNSPECIFIED
	}
}

func deviceTrustStateFromProto(value e2eev1.DeviceTrustState) domaine2ee.DeviceTrustState {
	switch value {
	case e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_TRUSTED:
		return domaine2ee.DeviceTrustStateTrusted
	case e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_UNTRUSTED:
		return domaine2ee.DeviceTrustStateUntrusted
	case e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_COMPROMISED:
		return domaine2ee.DeviceTrustStateCompromised
	default:
		return domaine2ee.DeviceTrustStateUnspecified
	}
}

func conversationKeyCoverageStateProto(value domaine2ee.ConversationKeyCoverageState) e2eev1.ConversationKeyCoverageState {
	switch value {
	case domaine2ee.ConversationKeyCoverageStateReady:
		return e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_READY
	case domaine2ee.ConversationKeyCoverageStatePending:
		return e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_PENDING
	case domaine2ee.ConversationKeyCoverageStateExpired:
		return e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_EXPIRED
	case domaine2ee.ConversationKeyCoverageStateMissing:
		return e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_MISSING
	default:
		return e2eev1.ConversationKeyCoverageState_CONVERSATION_KEY_COVERAGE_STATE_UNSPECIFIED
	}
}

func e2eeUpdateVisible(value domaine2ee.Update, accountID string, deviceID string) bool {
	if value.TargetAccountID == "" && value.TargetDeviceID == "" {
		return value.ActorAccountID == accountID && value.ActorDeviceID == deviceID
	}
	if value.TargetAccountID == accountID && (value.TargetDeviceID == "" || value.TargetDeviceID == deviceID) {
		return true
	}
	return value.ActorAccountID == accountID && value.ActorDeviceID == deviceID
}

func newGatewayEventID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}
