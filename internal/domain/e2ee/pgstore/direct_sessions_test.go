package pgstore

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/domain/e2ee"
)

func TestDirectSessionRoundTripQueries(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(7 * 24 * time.Hour)
	buildRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{
			"id", "initiator_account_id", "initiator_device_id", "recipient_account_id", "recipient_device_id",
			"initiator_ephemeral_key_id", "initiator_ephemeral_algorithm", "initiator_ephemeral_public_key",
			"identity_key_id", "identity_key_algorithm", "identity_key_public_key",
			"signed_prekey_id", "signed_prekey_algorithm", "signed_prekey_public_key", "signed_prekey_signature",
			"one_time_prekey_id", "one_time_prekey_algorithm", "one_time_prekey_public_key",
			"bootstrap_algorithm", "bootstrap_nonce", "bootstrap_ciphertext", "bootstrap_metadata",
			"state", "created_at", "acknowledged_at", "expires_at",
		}).AddRow(
			"dse-1", "acc-a", "dev-a", "acc-b", "dev-b",
			"eph-1", "x25519", []byte("eph"),
			"dev-b", "device_public_key", []byte("device"),
			"spk-1", "x25519", []byte("signed"), []byte("sig"),
			"otk-1", "x25519", []byte("otk"),
			"x3dh-v1", []byte("nonce"), []byte("ciphertext"), []byte(`{"conversation_id":"conv-1"}`),
			"pending", now, nil, expiresAt,
		)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "tenant"."e2ee_direct_sessions" (`)).
		WithArgs(
			"dse-1", "acc-a", "dev-a", "acc-b", "dev-b",
			"eph-1", "x25519", []byte("eph"),
			"dev-b", "device_public_key", []byte("device"),
			"spk-1", "x25519", []byte("signed"), []byte("sig"),
			"otk-1", "x25519", []byte("otk"),
			"x3dh-v1", []byte("nonce"), []byte("ciphertext"), []byte(`{"conversation_id":"conv-1"}`),
			e2ee.DirectSessionStatePending, now, nil, expiresAt,
		).
		WillReturnRows(buildRows())

	saved, err := store.SaveDirectSession(context.Background(), e2ee.DirectSession{
		ID:                 "dse-1",
		InitiatorAccountID: "acc-a",
		InitiatorDeviceID:  "dev-a",
		RecipientAccountID: "acc-b",
		RecipientDeviceID:  "dev-b",
		InitiatorEphemeral: e2ee.PublicKey{KeyID: "eph-1", Algorithm: "x25519", PublicKey: []byte("eph")},
		IdentityKey:        e2ee.PublicKey{KeyID: "dev-b", Algorithm: "device_public_key", PublicKey: []byte("device")},
		SignedPreKey: e2ee.SignedPreKey{
			Key:       e2ee.PublicKey{KeyID: "spk-1", Algorithm: "x25519", PublicKey: []byte("signed")},
			Signature: []byte("sig"),
		},
		OneTimePreKey: e2ee.OneTimePreKey{
			Key: e2ee.PublicKey{KeyID: "otk-1", Algorithm: "x25519", PublicKey: []byte("otk")},
		},
		Bootstrap: e2ee.BootstrapPayload{
			Algorithm:  "x3dh-v1",
			Nonce:      []byte("nonce"),
			Ciphertext: []byte("ciphertext"),
			Metadata:   map[string]string{"conversation_id": "conv-1"},
		},
		State:     e2ee.DirectSessionStatePending,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("save direct session: %v", err)
	}
	if saved.Bootstrap.Metadata["conversation_id"] != "conv-1" {
		t.Fatalf("unexpected metadata: %+v", saved.Bootstrap.Metadata)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
	id, initiator_account_id, initiator_device_id, recipient_account_id, recipient_device_id,
	initiator_ephemeral_key_id, initiator_ephemeral_algorithm, initiator_ephemeral_public_key,
	identity_key_id, identity_key_algorithm, identity_key_public_key,
	signed_prekey_id, signed_prekey_algorithm, signed_prekey_public_key, signed_prekey_signature,
	one_time_prekey_id, one_time_prekey_algorithm, one_time_prekey_public_key,
	bootstrap_algorithm, bootstrap_nonce, bootstrap_ciphertext, bootstrap_metadata,
	state, created_at, acknowledged_at, expires_at
FROM "tenant"."e2ee_direct_sessions"
WHERE id = $1`)).
		WithArgs("dse-1").
		WillReturnRows(buildRows())

	loaded, err := store.DirectSessionByID(context.Background(), "dse-1")
	if err != nil {
		t.Fatalf("load direct session: %v", err)
	}
	if loaded.ID != "dse-1" || loaded.Bootstrap.Algorithm != "x3dh-v1" {
		t.Fatalf("unexpected loaded session: %+v", loaded)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
	id, initiator_account_id, initiator_device_id, recipient_account_id, recipient_device_id,
	initiator_ephemeral_key_id, initiator_ephemeral_algorithm, initiator_ephemeral_public_key,
	identity_key_id, identity_key_algorithm, identity_key_public_key,
	signed_prekey_id, signed_prekey_algorithm, signed_prekey_public_key, signed_prekey_signature,
	one_time_prekey_id, one_time_prekey_algorithm, one_time_prekey_public_key,
	bootstrap_algorithm, bootstrap_nonce, bootstrap_ciphertext, bootstrap_metadata,
	state, created_at, acknowledged_at, expires_at
FROM "tenant"."e2ee_direct_sessions"
WHERE recipient_account_id = $1 AND recipient_device_id = $2
ORDER BY created_at DESC, id DESC`)).
		WithArgs("acc-b", "dev-b").
		WillReturnRows(buildRows())

	listed, err := store.DirectSessionsByRecipientDevice(context.Background(), "acc-b", "dev-b")
	if err != nil {
		t.Fatalf("list direct sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "dse-1" {
		t.Fatalf("unexpected listed sessions: %+v", listed)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
