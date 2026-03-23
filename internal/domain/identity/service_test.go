package identity_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

type recordingCodeSender struct {
	mu    sync.Mutex
	codes map[string]string
}

func (s *recordingCodeSender) SendLoginCode(_ context.Context, target identity.LoginTarget, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.codes == nil {
		s.codes = make(map[string]string)
	}

	s.codes[target.DestinationMask] = code
	return nil
}

func (s *recordingCodeSender) codeFor(mask string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.codes[mask]
}

func TestUserAccountLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	fixedNow := time.Date(2026, time.March, 23, 18, 30, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "alice",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		Phone:       "+1 555 0100",
		Note:        "invite request",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	approvedJoinRequest, account, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID: joinRequest.ID,
		ReviewedBy:    "admin-1",
		Roles:         []identity.Role{identity.RoleAdmin},
		Note:          "approved",
	})
	if err != nil {
		t.Fatalf("approve join request: %v", err)
	}
	if approvedJoinRequest.Status != identity.JoinRequestStatusApproved {
		t.Fatalf("expected approved join request, got %s", approvedJoinRequest.Status)
	}
	if account.Kind != identity.AccountKindUser {
		t.Fatalf("expected user account, got %s", account.Kind)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:      "alice",
		Delivery:      identity.LoginDeliveryChannelEmail,
		DeviceName:    "Alice iPhone",
		Platform:      identity.DevicePlatformIOS,
		ClientVersion: "1.0.0",
		Locale:        "en",
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}

	code := sender.codeFor(targets[0].DestinationMask)
	if code == "" {
		t.Fatalf("expected recorded login code")
	}

	loginResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        code,
		DeviceName:  "Alice iPhone",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "alice-device-key",
		PushToken:   "push-token-1",
	})
	if err != nil {
		t.Fatalf("verify login code: %v", err)
	}
	if loginResult.Session.Status != identity.SessionStatusActive {
		t.Fatalf("expected active session, got %s", loginResult.Session.Status)
	}
	if loginResult.Device.Status != identity.DeviceStatusActive {
		t.Fatalf("expected active device, got %s", loginResult.Device.Status)
	}
	if loginResult.Tokens.AccessToken == "" || loginResult.Tokens.RefreshToken == "" {
		t.Fatalf("expected issued tokens")
	}

	extraDevice, updatedSession, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:  loginResult.Session.ID,
		DeviceName: "Alice Mac",
		Platform:   identity.DevicePlatformDesktop,
		PublicKey:  "alice-mac-key",
		PushToken:  "push-token-2",
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}
	if extraDevice.SessionID != loginResult.Session.ID {
		t.Fatalf("expected device to attach to session %s, got %s", loginResult.Session.ID, extraDevice.SessionID)
	}
	if updatedSession.ID != loginResult.Session.ID {
		t.Fatalf("expected session %s, got %s", loginResult.Session.ID, updatedSession.ID)
	}

	devices, err := svc.ListDevices(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	revokedSession, err := svc.RevokeSession(ctx, identity.RevokeSessionParams{
		SessionID: loginResult.Session.ID,
		Reason:    "logout",
	})
	if err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if revokedSession.Status != identity.SessionStatusRevoked {
		t.Fatalf("expected revoked session, got %s", revokedSession.Status)
	}
}

func TestBotAccountAuthentication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 23, 18, 45, 0, 0, time.UTC)
	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, botToken, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "notifier-bot",
		DisplayName: "Notifier",
		AccountKind: identity.AccountKindBot,
		CreatedBy:   "admin-1",
		Roles:       []identity.Role{identity.RoleSupport},
	})
	if err != nil {
		t.Fatalf("create bot account: %v", err)
	}
	if account.Kind != identity.AccountKindBot {
		t.Fatalf("expected bot account, got %s", account.Kind)
	}
	if botToken == "" {
		t.Fatalf("expected bot token")
	}

	loginResult, err := svc.AuthenticateBot(ctx, identity.AuthenticateBotParams{
		BotToken:      botToken,
		DeviceName:    "bot-worker-1",
		Platform:      identity.DevicePlatformServer,
		PublicKey:     "bot-public-key",
		ClientVersion: "1.0.0",
		Locale:        "en",
	})
	if err != nil {
		t.Fatalf("authenticate bot: %v", err)
	}
	if loginResult.Session.Status != identity.SessionStatusActive {
		t.Fatalf("expected active session, got %s", loginResult.Session.Status)
	}
	if loginResult.Device.AccountID != account.ID {
		t.Fatalf("expected device for account %s, got %s", account.ID, loginResult.Device.AccountID)
	}
}
