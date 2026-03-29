package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/dm-vev/zvonilka/internal/domain/e2ee"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanDirectSession(ctx context.Context, query string, args ...any) (e2ee.DirectSession, error) {
	value, err := scanDirectSession(s.conn().QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return e2ee.DirectSession{}, e2ee.ErrNotFound
	}
	return value, err
}

func scanDirectSession(row rowScanner) (e2ee.DirectSession, error) {
	var (
		value                 e2ee.DirectSession
		metadata              []byte
		oneTimeKeyID          sql.NullString
		oneTimeAlgorithm      sql.NullString
		oneTimePublicKey      []byte
		acknowledgedAt        sql.NullTime
		expiresAt             sql.NullTime
	)
	err := row.Scan(
		&value.ID,
		&value.InitiatorAccountID,
		&value.InitiatorDeviceID,
		&value.RecipientAccountID,
		&value.RecipientDeviceID,
		&value.InitiatorEphemeral.KeyID,
		&value.InitiatorEphemeral.Algorithm,
		&value.InitiatorEphemeral.PublicKey,
		&value.IdentityKey.KeyID,
		&value.IdentityKey.Algorithm,
		&value.IdentityKey.PublicKey,
		&value.SignedPreKey.Key.KeyID,
		&value.SignedPreKey.Key.Algorithm,
		&value.SignedPreKey.Key.PublicKey,
		&value.SignedPreKey.Signature,
		&oneTimeKeyID,
		&oneTimeAlgorithm,
		&oneTimePublicKey,
		&value.Bootstrap.Algorithm,
		&value.Bootstrap.Nonce,
		&value.Bootstrap.Ciphertext,
		&metadata,
		&value.State,
		&value.CreatedAt,
		&acknowledgedAt,
		&expiresAt,
	)
	if err != nil {
		return e2ee.DirectSession{}, err
	}
	value.OneTimePreKey.Key.KeyID = oneTimeKeyID.String
	value.OneTimePreKey.Key.Algorithm = oneTimeAlgorithm.String
	value.OneTimePreKey.Key.PublicKey = append([]byte(nil), oneTimePublicKey...)
	value.Bootstrap.Metadata = unmarshalMetadata(metadata)
	value.AcknowledgedAt = acknowledgedAt.Time
	value.ExpiresAt = expiresAt.Time
	return value, nil
}

func marshalMetadata(value map[string]string) []byte {
	if len(value) == 0 {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func unmarshalMetadata(value []byte) map[string]string {
	if len(value) == 0 {
		return nil
	}
	result := make(map[string]string)
	if err := json.Unmarshal(value, &result); err != nil {
		return nil
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (s *Store) scanGroupSenderKeyDistribution(ctx context.Context, query string, args ...any) (e2ee.GroupSenderKeyDistribution, error) {
	value, err := scanGroupSenderKeyDistribution(s.conn().QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return e2ee.GroupSenderKeyDistribution{}, e2ee.ErrNotFound
	}
	return value, err
}

func scanGroupSenderKeyDistribution(row rowScanner) (e2ee.GroupSenderKeyDistribution, error) {
	var (
		value          e2ee.GroupSenderKeyDistribution
		metadata       []byte
		acknowledgedAt sql.NullTime
		expiresAt      sql.NullTime
	)
	err := row.Scan(
		&value.ID,
		&value.ConversationID,
		&value.SenderAccountID,
		&value.SenderDeviceID,
		&value.RecipientAccountID,
		&value.RecipientDeviceID,
		&value.SenderKeyID,
		&value.Payload.Algorithm,
		&value.Payload.Nonce,
		&value.Payload.Ciphertext,
		&metadata,
		&value.State,
		&value.CreatedAt,
		&acknowledgedAt,
		&expiresAt,
	)
	if err != nil {
		return e2ee.GroupSenderKeyDistribution{}, err
	}
	value.Payload.Metadata = unmarshalMetadata(metadata)
	value.AcknowledgedAt = acknowledgedAt.Time
	value.ExpiresAt = expiresAt.Time
	return value, nil
}
