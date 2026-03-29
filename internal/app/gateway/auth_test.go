package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	e2eetest "github.com/dm-vev/zvonilka/internal/domain/e2ee/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	presencetest "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/search"
	searchtest "github.com/dm-vev/zvonilka/internal/domain/search/teststore"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	usertest "github.com/dm-vev/zvonilka/internal/domain/user/teststore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type recordingSender struct {
	mu    sync.Mutex
	codes map[string]string
}

func (s *recordingSender) SendLoginCode(_ context.Context, target identity.LoginTarget, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.codes == nil {
		s.codes = make(map[string]string)
	}
	s.codes[target.DestinationMask] = code
	return nil
}

func (s *recordingSender) code(mask string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.codes[mask]
}

func newTestAPI(t *testing.T) (*api, *recordingSender) {
	t.Helper()

	identityStore := identitytest.NewMemoryStore()
	sender := &recordingSender{}
	searchService, err := search.NewService(searchtest.New())
	if err != nil {
		t.Fatalf("new search service: %v", err)
	}
	identityService, err := identity.NewService(identityStore, sender, identity.WithNow(func() time.Time {
		return time.Date(2026, time.March, 26, 15, 0, 0, 0, time.UTC)
	}), identity.WithIndexer(searchService))
	if err != nil {
		t.Fatalf("new identity service: %v", err)
	}

	presenceStore := presencetest.NewMemoryStore()
	presenceService, err := presence.NewService(presenceStore, identityStore)
	if err != nil {
		t.Fatalf("new presence service: %v", err)
	}
	conversationService, err := conversation.NewService(
		conversationtest.NewMemoryStore(),
		conversation.WithIndexer(searchService),
	)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	userService, err := domainuser.NewService(usertest.NewMemoryStore(), identityService)
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}
	e2eeService, err := domaine2ee.NewService(e2eetest.NewMemoryStore(), identityStore)
	if err != nil {
		t.Fatalf("new e2ee service: %v", err)
	}

	return &api{
		e2ee:         e2eeService,
		identity:     identityService,
		conversation: conversationService,
		presence:     presenceService,
		search:       searchService,
		user:         userService,
		syncNotifier: newSyncNotifier(),
	}, sender
}

func TestGetMyProfileRejectsMissingBearer(t *testing.T) {
	t.Parallel()

	api, _ := newTestAPI(t)

	_, err := api.GetMyProfile(context.Background(), &usersv1.GetMyProfileRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestRefreshSessionRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	account, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "grpc-user",
		DisplayName: "gRPC User",
		Email:       "grpc@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if len(begin.Targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(begin.Targets))
	}

	verify, err := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     "phone",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	refreshed, err := api.RefreshSession(ctx, &authv1.RefreshSessionRequest{
		RefreshToken: verify.Tokens.RefreshToken,
		DeviceId:     verify.Device.DeviceId,
	})
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if refreshed.Tokens.AccessToken == verify.Tokens.AccessToken {
		t.Fatalf("expected rotated access token")
	}
	if refreshed.Tokens.RefreshToken == verify.Tokens.RefreshToken {
		t.Fatalf("expected rotated refresh token")
	}

	authCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(
		"authorization",
		"Bearer "+refreshed.Tokens.AccessToken,
	))
	profile, err := api.GetMyProfile(authCtx, &usersv1.GetMyProfileRequest{})
	if err != nil {
		t.Fatalf("get my profile: %v", err)
	}
	if profile.Profile.UserId != account.ID {
		t.Fatalf("expected profile for %s, got %s", account.ID, profile.Profile.UserId)
	}
}

func TestGetLoginOptionsRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	account, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "login-options",
		DisplayName: "Login Options",
		Email:       "login-options@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	_, err = api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:            begin.ChallengeId,
		Code:                   sender.code(begin.Targets[0].DestinationMask),
		EnablePasswordRecovery: true,
		RecoveryPassword:       "recovery-pass",
		DeviceName:             "phone",
		DevicePlatform:         commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:              &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login with recovery: %v", err)
	}

	options, err := api.GetLoginOptions(ctx, &authv1.GetLoginOptionsRequest{
		Identifier: &authv1.GetLoginOptionsRequest_Username{Username: account.Username},
	})
	if err != nil {
		t.Fatalf("get login options: %v", err)
	}
	if !options.PasswordRecoveryEnabled {
		t.Fatal("expected password recovery to be enabled")
	}
	if len(options.Options) == 0 {
		t.Fatal("expected at least one login option")
	}
}

func TestRotateDeviceKeyRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	account, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "rotate-device",
		DisplayName: "Rotate Device",
		Email:       "rotate@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	verify, err := api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     "phone",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	authCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(
		"authorization",
		"Bearer "+verify.Tokens.AccessToken,
	))
	rotated, err := api.RotateDeviceKey(authCtx, &authv1.RotateDeviceKeyRequest{
		DeviceId:  verify.Device.DeviceId,
		DeviceKey: &commonv1.PublicKeyBundle{PublicKey: []byte("new-key")},
	})
	if err != nil {
		t.Fatalf("rotate device key: %v", err)
	}
	if string(rotated.Device.PublicKey.PublicKey) != "new-key" {
		t.Fatalf("expected rotated key, got %q", string(rotated.Device.PublicKey.PublicKey))
	}
}

func TestPasswordRecoveryRPC(t *testing.T) {
	t.Parallel()

	api, sender := newTestAPI(t)
	ctx := context.Background()

	account, _, err := api.identity.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "recovery-user",
		DisplayName: "Recovery User",
		Email:       "recovery@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := api.BeginLogin(ctx, &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "phone",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	_, err = api.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:            begin.ChallengeId,
		Code:                   sender.code(begin.Targets[0].DestinationMask),
		EnablePasswordRecovery: true,
		RecoveryPassword:       "recovery-pass",
		DeviceName:             "phone",
		DevicePlatform:         commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:              &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login with recovery: %v", err)
	}

	recoveryStart, err := api.BeginPasswordRecovery(ctx, &authv1.BeginPasswordRecoveryRequest{
		Identifier:      &authv1.BeginPasswordRecoveryRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
	})
	if err != nil {
		t.Fatalf("begin password recovery: %v", err)
	}
	recovered, err := api.CompletePasswordRecovery(ctx, &authv1.CompletePasswordRecoveryRequest{
		RecoveryChallengeId: recoveryStart.RecoveryChallengeId,
		Code:                sender.code(recoveryStart.Targets[0].DestinationMask),
		NewPassword:         "new-password",
		NewRecoveryPassword: "new-recovery-password",
		DeviceName:          "restored",
		DevicePlatform:      commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP,
		DeviceKey:           &commonv1.PublicKeyBundle{PublicKey: []byte("restored-key")},
	})
	if err != nil {
		t.Fatalf("complete password recovery: %v", err)
	}
	if recovered.Tokens.AccessToken == "" || !recovered.RecoveryEnabled {
		t.Fatalf("unexpected recovery response: %+v", recovered)
	}
}
