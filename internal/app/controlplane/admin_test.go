package controlplane

import (
	"context"
	"sync"
	"testing"
	"time"

	adminv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/admin/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	presencetest "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type recordingSender struct {
	mu    sync.Mutex
	codes map[string]string
}

func (s *recordingSender) SendLoginCode(
	_ context.Context,
	target domainidentity.LoginTarget,
	code string,
) error {
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

func newTestAdminAPI(t *testing.T) (*api, *recordingSender) {
	t.Helper()

	identityStore := identitytest.NewMemoryStore()
	sender := &recordingSender{}
	identityService, err := domainidentity.NewService(
		identityStore,
		sender,
		domainidentity.WithNow(func() time.Time {
			return time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	presenceService, err := domainpresence.NewService(presencetest.NewMemoryStore(), identityStore)
	require.NoError(t, err)

	return &api{
		identity: identityService,
		presence: presenceService,
	}, sender
}

func mustCreateAccount(
	t *testing.T,
	api *api,
	params domainidentity.CreateAccountParams,
) domainidentity.Account {
	t.Helper()

	account, _, err := api.identity.CreateAccount(context.Background(), params)
	require.NoError(t, err)

	return account
}

func mustLoginAccount(
	t *testing.T,
	api *api,
	sender *recordingSender,
	username string,
) context.Context {
	t.Helper()

	challenge, targets, err := api.identity.BeginLogin(context.Background(), domainidentity.BeginLoginParams{
		Username:   username,
		Delivery:   domainidentity.LoginDeliveryChannelEmail,
		DeviceName: "admin-device",
		Platform:   domainidentity.DevicePlatformDesktop,
	})
	require.NoError(t, err)
	require.NotEmpty(t, targets)

	verify, err := api.identity.VerifyLoginCode(context.Background(), domainidentity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        sender.code(targets[0].DestinationMask),
		DeviceName:  "admin-device",
		Platform:    domainidentity.DevicePlatformDesktop,
		PublicKey:   "device-key-" + username,
	})
	require.NoError(t, err)

	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("authorization", "Bearer "+verify.Tokens.AccessToken),
	)
}

func TestAdminServiceJoinRequestLifecycle(t *testing.T) {
	t.Parallel()

	api, sender := newTestAdminAPI(t)
	admin := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "admin-user",
		DisplayName: "Admin User",
		Email:       "admin@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleAdmin},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})
	adminCtx := mustLoginAccount(t, api, sender, admin.Username)

	first, err := api.identity.SubmitJoinRequest(context.Background(), domainidentity.SubmitJoinRequestParams{
		Username:    "alpha",
		DisplayName: "Alpha",
		Email:       "alpha@example.com",
		RequestedAt: time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	second, err := api.identity.SubmitJoinRequest(context.Background(), domainidentity.SubmitJoinRequestParams{
		Username:    "beta",
		DisplayName: "Beta",
		Email:       "beta@example.com",
		RequestedAt: time.Date(2026, time.April, 6, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	pageOne, err := api.ListJoinRequests(adminCtx, &adminv1.ListJoinRequestsRequest{
		StatusFilter: commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_PENDING,
		Page:         &commonv1.PageRequest{PageSize: 1},
	})
	require.NoError(t, err)
	require.Len(t, pageOne.JoinRequests, 1)
	require.Equal(t, first.ID, pageOne.JoinRequests[0].JoinRequestId)
	require.Equal(t, uint64(2), pageOne.GetPage().GetTotalSize())
	require.NotEmpty(t, pageOne.GetPage().GetNextPageToken())

	pageTwo, err := api.ListJoinRequests(adminCtx, &adminv1.ListJoinRequestsRequest{
		StatusFilter: commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_PENDING,
		Page: &commonv1.PageRequest{
			PageSize:  1,
			PageToken: pageOne.GetPage().GetNextPageToken(),
		},
	})
	require.NoError(t, err)
	require.Len(t, pageTwo.JoinRequests, 1)
	require.Equal(t, second.ID, pageTwo.JoinRequests[0].JoinRequestId)

	approved, err := api.ApproveJoinRequest(adminCtx, &adminv1.ApproveJoinRequestRequest{
		JoinRequestId:  first.ID,
		Roles:          []adminv1.AccountRole{adminv1.AccountRole_ACCOUNT_ROLE_MODERATOR},
		Note:           "approved after review",
		IdempotencyKey: "approve-alpha",
	})
	require.NoError(t, err)
	require.Equal(
		t,
		commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_APPROVED,
		approved.GetJoinRequest().GetStatus(),
	)
	require.Equal(t, admin.ID, approved.GetJoinRequest().GetReviewedByUserId())
	require.Equal(t, commonv1.AccountKind_ACCOUNT_KIND_USER, approved.GetProfile().GetAccountKind())
	require.Equal(t, "alpha", approved.GetProfile().GetUsername())

	rejected, err := api.RejectJoinRequest(adminCtx, &adminv1.RejectJoinRequestRequest{
		JoinRequestId:  second.ID,
		Reason:         "duplicate submission",
		IdempotencyKey: "reject-beta",
	})
	require.NoError(t, err)
	require.Equal(
		t,
		commonv1.JoinRequestStatus_JOIN_REQUEST_STATUS_REJECTED,
		rejected.GetJoinRequest().GetStatus(),
	)
	require.Equal(t, admin.ID, rejected.GetJoinRequest().GetReviewedByUserId())

	all, err := api.ListJoinRequests(adminCtx, &adminv1.ListJoinRequestsRequest{})
	require.NoError(t, err)
	require.Len(t, all.JoinRequests, 2)
	require.Equal(t, uint64(2), all.GetPage().GetTotalSize())
	require.Equal(t, first.ID, all.JoinRequests[0].GetJoinRequestId())
	require.Equal(t, second.ID, all.JoinRequests[1].GetJoinRequestId())
}

func TestAdminServiceCreateAccountPermissions(t *testing.T) {
	t.Parallel()

	api, sender := newTestAdminAPI(t)
	admin := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "admin-user",
		DisplayName: "Admin User",
		Email:       "admin@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleAdmin},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})
	support := mustCreateAccount(t, api, domainidentity.CreateAccountParams{
		Username:    "support-user",
		DisplayName: "Support User",
		Email:       "support@example.com",
		Roles:       []domainidentity.Role{domainidentity.RoleSupport},
		AccountKind: domainidentity.AccountKindUser,
		CreatedBy:   "bootstrap",
	})

	adminCtx := mustLoginAccount(t, api, sender, admin.Username)
	supportCtx := mustLoginAccount(t, api, sender, support.Username)

	_, err := api.CreateAccount(supportCtx, &adminv1.CreateAccountRequest{
		Username:    "should-fail",
		DisplayName: "Should Fail",
		Email:       "fail@example.com",
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	bot, err := api.CreateAccount(adminCtx, &adminv1.CreateAccountRequest{
		Username:       "build-bot",
		DisplayName:    "Build Bot",
		AccountKind:    commonv1.AccountKind_ACCOUNT_KIND_BOT,
		Roles:          []adminv1.AccountRole{adminv1.AccountRole_ACCOUNT_ROLE_SUPPORT},
		IdempotencyKey: "create-bot",
	})
	require.NoError(t, err)
	require.NotEmpty(t, bot.GetBotToken())
	require.Equal(t, commonv1.AccountKind_ACCOUNT_KIND_BOT, bot.GetProfile().GetAccountKind())
	require.Equal(t, "build-bot", bot.GetProfile().GetUsername())

	user, err := api.CreateAccount(adminCtx, &adminv1.CreateAccountRequest{
		Username:       "plain-user",
		DisplayName:    "Plain User",
		Email:          "plain@example.com",
		IdempotencyKey: "create-user",
	})
	require.NoError(t, err)
	require.Empty(t, user.GetBotToken())
	require.Equal(t, commonv1.AccountKind_ACCOUNT_KIND_USER, user.GetProfile().GetAccountKind())
}

func TestAppRegisterGRPCRegistersAdminService(t *testing.T) {
	t.Parallel()

	server := grpc.NewServer()
	(&app{api: &api{}}).registerGRPC(server)

	services := server.GetServiceInfo()
	_, ok := services["zvonilka.admin.v1.AdminService"]
	require.True(t, ok)
}
