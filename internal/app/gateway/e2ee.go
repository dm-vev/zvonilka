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
