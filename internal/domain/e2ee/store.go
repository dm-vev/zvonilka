package e2ee

import (
	"context"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error
	SaveSignedPreKey(ctx context.Context, accountID string, deviceID string, value SignedPreKey) (SignedPreKey, error)
	SignedPreKeyByDevice(ctx context.Context, accountID string, deviceID string) (SignedPreKey, error)
	DeleteOneTimePreKeysByDevice(ctx context.Context, accountID string, deviceID string) error
	SaveOneTimePreKeys(ctx context.Context, accountID string, deviceID string, values []OneTimePreKey) error
	ClaimOneTimePreKey(
		ctx context.Context,
		accountID string,
		deviceID string,
		claimedByAccountID string,
		claimedByDeviceID string,
	) (OneTimePreKey, error)
	CountAvailableOneTimePreKeys(ctx context.Context, accountID string, deviceID string) (uint32, error)
	ExpirePendingDirectSessionsByDevice(ctx context.Context, accountID string, deviceID string, expiresAt time.Time) (uint32, error)
	SaveDirectSession(ctx context.Context, value DirectSession) (DirectSession, error)
	DirectSessionByID(ctx context.Context, sessionID string) (DirectSession, error)
	DirectSessionsByRecipientDevice(ctx context.Context, accountID string, deviceID string) ([]DirectSession, error)
	DirectSessionsByParticipantDevice(ctx context.Context, accountID string, deviceID string) ([]DirectSession, error)
	ExpirePendingGroupSenderKeysBySenderDevice(ctx context.Context, accountID string, deviceID string, expiresAt time.Time) (uint32, error)
	SaveGroupSenderKeyDistribution(ctx context.Context, value GroupSenderKeyDistribution) (GroupSenderKeyDistribution, error)
	GroupSenderKeyDistributionByID(ctx context.Context, distributionID string) (GroupSenderKeyDistribution, error)
	GroupSenderKeyDistributionsByRecipientDevice(ctx context.Context, conversationID string, accountID string, deviceID string) ([]GroupSenderKeyDistribution, error)
	GroupSenderKeyDistributionsBySenderKey(ctx context.Context, conversationID string, senderAccountID string, senderDeviceID string, senderKeyID string) ([]GroupSenderKeyDistribution, error)
	GroupSenderKeyDistributionsBySenderDevice(ctx context.Context, conversationID string, senderAccountID string, senderDeviceID string) ([]GroupSenderKeyDistribution, error)
	SaveDeviceTrust(ctx context.Context, value DeviceTrust) (DeviceTrust, error)
	DeviceTrustsByObserverDevice(ctx context.Context, observerAccountID string, observerDeviceID string, targetAccountID string) ([]DeviceTrust, error)
}

type Directory interface {
	AccountByID(ctx context.Context, accountID string) (identity.Account, error)
	DeviceByID(ctx context.Context, deviceID string) (identity.Device, error)
	DevicesByAccountID(ctx context.Context, accountID string) ([]identity.Device, error)
}

type Conversations interface {
	ConversationByID(ctx context.Context, conversationID string) (conversation.Conversation, error)
	ConversationMemberByConversationAndAccount(ctx context.Context, conversationID string, accountID string) (conversation.ConversationMember, error)
	ConversationMembersByConversationID(ctx context.Context, conversationID string) ([]conversation.ConversationMember, error)
}
