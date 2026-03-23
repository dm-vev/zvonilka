package identity

import "context"

// Store persists identity state for the service.
type Store interface {
	SaveJoinRequest(ctx context.Context, joinRequest JoinRequest) (JoinRequest, error)
	JoinRequestByID(ctx context.Context, joinRequestID string) (JoinRequest, error)
	JoinRequestsByStatus(ctx context.Context, status JoinRequestStatus) ([]JoinRequest, error)

	SaveAccount(ctx context.Context, account Account) (Account, error)
	DeleteAccount(ctx context.Context, accountID string) error
	AccountByID(ctx context.Context, accountID string) (Account, error)
	AccountByUsername(ctx context.Context, username string) (Account, error)
	AccountByEmail(ctx context.Context, email string) (Account, error)
	AccountByPhone(ctx context.Context, phone string) (Account, error)
	AccountByBotTokenHash(ctx context.Context, tokenHash string) (Account, error)

	SaveLoginChallenge(ctx context.Context, challenge LoginChallenge) (LoginChallenge, error)
	LoginChallengeByID(ctx context.Context, challengeID string) (LoginChallenge, error)
	DeleteLoginChallenge(ctx context.Context, challengeID string) error

	SaveDevice(ctx context.Context, device Device) (Device, error)
	DeviceByID(ctx context.Context, deviceID string) (Device, error)
	DevicesByAccountID(ctx context.Context, accountID string) ([]Device, error)

	SaveSession(ctx context.Context, session Session) (Session, error)
	SessionByID(ctx context.Context, sessionID string) (Session, error)
	SessionsByAccountID(ctx context.Context, accountID string) ([]Session, error)
	UpdateSession(ctx context.Context, session Session) (Session, error)
}
