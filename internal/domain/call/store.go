package call

import "context"

// Store persists call state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SaveCall(ctx context.Context, call Call) (Call, error)
	CallByID(ctx context.Context, callID string) (Call, error)
	ActiveCallByConversation(ctx context.Context, conversationID string) (Call, error)
	ActiveCalls(ctx context.Context, limit int) ([]Call, error)
	CallsByConversation(ctx context.Context, conversationID string, includeEnded bool) ([]Call, error)

	SaveInvite(ctx context.Context, invite Invite) (Invite, error)
	InviteByCallAndAccount(ctx context.Context, callID string, accountID string) (Invite, error)
	InvitesByCall(ctx context.Context, callID string) ([]Invite, error)

	SaveParticipant(ctx context.Context, participant Participant) (Participant, error)
	ParticipantByCallAndDevice(ctx context.Context, callID string, deviceID string) (Participant, error)
	ParticipantsByCall(ctx context.Context, callID string) ([]Participant, error)

	SaveEvent(ctx context.Context, event Event) (Event, error)
	EventsAfterSequence(
		ctx context.Context,
		fromSequence uint64,
		callID string,
		conversationID string,
		limit int,
	) ([]Event, error)

	SaveWorkerCursor(ctx context.Context, cursor WorkerCursor) (WorkerCursor, error)
	WorkerCursorByName(ctx context.Context, name string) (WorkerCursor, error)
}
