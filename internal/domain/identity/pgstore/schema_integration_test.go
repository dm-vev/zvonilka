package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestIdentitySchemaHardening(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.March, 23, 23, 45, 0, 0, time.UTC)

	account, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-1",
		Kind:        identity.AccountKindUser,
		Username:    "alice",
		DisplayName: "Alice",
		Status:      identity.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save valid account: %v", err)
	}
	peerAccount, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-2",
		Kind:        identity.AccountKindUser,
		Username:    "bob",
		DisplayName: "Bob",
		Status:      identity.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save peer account: %v", err)
	}
	const peerAccountSessionID = "sess-peer-account"
	const peerAccountDeviceID = "dev-peer-account"

	if err := store.WithinTx(ctx, func(tx identity.Store) error {
		if _, err := tx.SaveDevice(ctx, identity.Device{
			ID:         peerAccountDeviceID,
			AccountID:  peerAccount.ID,
			SessionID:  peerAccountSessionID,
			Name:       "Bob phone",
			Platform:   identity.DevicePlatformIOS,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-peer-account-1",
			CreatedAt:  now,
			LastSeenAt: now,
		}); err != nil {
			return err
		}
		if _, err := tx.SaveSession(ctx, identity.Session{
			ID:             peerAccountSessionID,
			AccountID:      peerAccount.ID,
			DeviceID:       peerAccountDeviceID,
			DeviceName:     "Bob phone",
			DevicePlatform: identity.DevicePlatformIOS,
			Status:         identity.SessionStatusActive,
			Current:        true,
			CreatedAt:      now,
			LastSeenAt:     now,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("save peer account device/session pair: %v", err)
	}

	t.Run("rejects invalid account kind", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:          "acc-kind",
			Kind:        identity.AccountKind("alien"),
			Username:    "alien",
			DisplayName: "Alien",
			Status:      identity.AccountStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects bot without token hash", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:          "acc-bot",
			Kind:        identity.AccountKindBot,
			Username:    "bot",
			DisplayName: "Bot",
			Status:      identity.AccountStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects user with bot token hash", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:           "acc-user-token",
			Kind:         identity.AccountKindUser,
			Username:     "user-token",
			DisplayName:  "User Token",
			Status:       identity.AccountStatusActive,
			BotTokenHash: "token-hash",
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects invalid account status", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:          "acc-status",
			Kind:        identity.AccountKindUser,
			Username:    "status",
			DisplayName: "Status",
			Status:      identity.AccountStatus("ghost"),
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects invalid join request status", func(t *testing.T) {
		_, err := store.SaveJoinRequest(ctx, identity.JoinRequest{
			ID:          "join-1",
			Username:    "alice-join",
			DisplayName: "Alice Join",
			Status:      identity.JoinRequestStatus("bogus"),
			RequestedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects invalid login challenge delivery channel", func(t *testing.T) {
		_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
			ID:              "chal-1",
			AccountID:       account.ID,
			AccountKind:     identity.AccountKindUser,
			CodeHash:        "hash",
			DeliveryChannel: identity.LoginDeliveryChannel("fax"),
			ExpiresAt:       now.Add(time.Hour),
			CreatedAt:       now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects invalid login challenge account kind", func(t *testing.T) {
		_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
			ID:              "chal-kind",
			AccountID:       account.ID,
			AccountKind:     identity.AccountKind("alien"),
			CodeHash:        "hash",
			DeliveryChannel: identity.LoginDeliveryChannelEmail,
			ExpiresAt:       now.Add(time.Hour),
			CreatedAt:       now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects mismatched login challenge used pair", func(t *testing.T) {
		_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
			ID:              "chal-2",
			AccountID:       account.ID,
			AccountKind:     identity.AccountKindUser,
			CodeHash:        "hash",
			DeliveryChannel: identity.LoginDeliveryChannelEmail,
			ExpiresAt:       now.Add(time.Hour),
			CreatedAt:       now,
			Used:            true,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects login challenge used_at without used flag", func(t *testing.T) {
		_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
			ID:              "chal-3",
			AccountID:       account.ID,
			AccountKind:     identity.AccountKindUser,
			CodeHash:        "hash",
			DeliveryChannel: identity.LoginDeliveryChannelEmail,
			ExpiresAt:       now.Add(time.Hour),
			CreatedAt:       now,
			UsedAt:          now,
		})
		requireInvalidInput(t, err)
	})

	const sessionID = "sess-1"
	const primaryDeviceID = "dev-1"
	const peerSessionID = "sess-2"
	const peerDeviceID = "dev-2"

	t.Run("allows deferred device then session insert", func(t *testing.T) {
		err := store.WithinTx(ctx, func(tx identity.Store) error {
			if _, err := tx.SaveDevice(ctx, identity.Device{
				ID:         primaryDeviceID,
				AccountID:  account.ID,
				SessionID:  sessionID,
				Name:       "Alice laptop",
				Platform:   identity.DevicePlatformWeb,
				Status:     identity.DeviceStatusActive,
				PublicKey:  "pk-1",
				CreatedAt:  now,
				LastSeenAt: now,
			}); err != nil {
				return err
			}
			if _, err := tx.SaveSession(ctx, identity.Session{
				ID:             sessionID,
				AccountID:      account.ID,
				DeviceID:       primaryDeviceID,
				DeviceName:     "Alice laptop",
				DevicePlatform: identity.DevicePlatformWeb,
				Status:         identity.SessionStatusActive,
				Current:        true,
				CreatedAt:      now,
				LastSeenAt:     now,
			}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("save deferred device/session pair: %v", err)
		}

		device, err := store.DeviceByID(ctx, primaryDeviceID)
		if err != nil {
			t.Fatalf("load deferred device: %v", err)
		}
		if device.SessionID != sessionID {
			t.Fatalf("expected session %q, got %q", sessionID, device.SessionID)
		}
	})

	t.Run("allows deferred session then device insert", func(t *testing.T) {
		const reversedSessionID = "sess-reversed"
		const reversedDeviceID = "dev-reversed"

		err := store.WithinTx(ctx, func(tx identity.Store) error {
			if _, err := tx.SaveSession(ctx, identity.Session{
				ID:             reversedSessionID,
				AccountID:      account.ID,
				DeviceID:       reversedDeviceID,
				DeviceName:     "Alice desktop",
				DevicePlatform: identity.DevicePlatformDesktop,
				Status:         identity.SessionStatusActive,
				Current:        false,
				CreatedAt:      now,
				LastSeenAt:     now,
			}); err != nil {
				return err
			}
			if _, err := tx.SaveDevice(ctx, identity.Device{
				ID:         reversedDeviceID,
				AccountID:  account.ID,
				SessionID:  reversedSessionID,
				Name:       "Alice desktop",
				Platform:   identity.DevicePlatformDesktop,
				Status:     identity.DeviceStatusActive,
				PublicKey:  "pk-reversed",
				CreatedAt:  now,
				LastSeenAt: now,
			}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("save reversed session/device pair: %v", err)
		}

		session, err := store.SessionByID(ctx, reversedSessionID)
		if err != nil {
			t.Fatalf("load reversed session: %v", err)
		}
		if session.DeviceID != reversedDeviceID {
			t.Fatalf("expected device %q, got %q", reversedDeviceID, session.DeviceID)
		}
	})

	t.Run("rejects invalid device session reference", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-orphan",
			AccountID:  account.ID,
			SessionID:  "sess-missing",
			Name:       "Orphan",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-orphan",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		if !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("rejects cross-account device session reference", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-cross",
			AccountID:  account.ID,
			SessionID:  peerAccountSessionID,
			Name:       "Cross-account device",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-cross",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		if !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("rejects invalid device platform", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-platform",
			AccountID:  account.ID,
			SessionID:  sessionID,
			Name:       "Tablet",
			Platform:   identity.DevicePlatform("tvos"),
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-platform",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("rejects invalid device status", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-status",
			AccountID:  account.ID,
			SessionID:  sessionID,
			Name:       "Tablet",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatus("ghost"),
			PublicKey:  "pk-status",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		requireInvalidInput(t, err)
	})

	t.Run("allows multiple devices on the same session", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-dup",
			AccountID:  account.ID,
			SessionID:  sessionID,
			Name:       "Alice phone",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-dup",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		if err != nil {
			t.Fatalf("save second device for same session: %v", err)
		}

		devices, err := store.DevicesByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load devices after second attachment: %v", err)
		}
		if len(devices) != 3 {
			t.Fatalf("expected three devices on the account, got %d", len(devices))
		}
	})

	t.Run("allows a second device on a different session", func(t *testing.T) {
		err := store.WithinTx(ctx, func(tx identity.Store) error {
			if _, err := tx.SaveDevice(ctx, identity.Device{
				ID:         peerDeviceID,
				AccountID:  account.ID,
				SessionID:  peerSessionID,
				Name:       "Alice phone",
				Platform:   identity.DevicePlatformWeb,
				Status:     identity.DeviceStatusActive,
				PublicKey:  "pk-2",
				CreatedAt:  now,
				LastSeenAt: now,
			}); err != nil {
				return err
			}
			if _, err := tx.SaveSession(ctx, identity.Session{
				ID:             peerSessionID,
				AccountID:      account.ID,
				DeviceID:       peerDeviceID,
				DeviceName:     "Alice phone",
				DevicePlatform: identity.DevicePlatformWeb,
				Status:         identity.SessionStatusActive,
				Current:        false,
				CreatedAt:      now,
				LastSeenAt:     now,
			}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("save peer device/session pair: %v", err)
		}
	})

	t.Run("rejects invalid session status", func(t *testing.T) {
		session, err := store.SessionByID(ctx, sessionID)
		if err != nil {
			t.Fatalf("load valid session: %v", err)
		}
		session.Status = identity.SessionStatus("ghost")
		_, err = store.UpdateSession(ctx, session)
		requireInvalidInput(t, err)
	})

	t.Run("rejects cross-account session device reference", func(t *testing.T) {
		_, err := store.SaveSession(ctx, identity.Session{
			ID:             "sess-cross",
			AccountID:      account.ID,
			DeviceID:       peerAccountDeviceID,
			DeviceName:     "Cross-account session",
			DevicePlatform: identity.DevicePlatformWeb,
			Status:         identity.SessionStatusActive,
			Current:        true,
			CreatedAt:      now,
			LastSeenAt:     now,
		})
		if !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("rejects raw cross-account device and session pair on tx commit", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			_ = tx.Rollback()
		})

		const rawDeviceID = "dev-raw-cross"
		const rawSessionID = "sess-raw-cross"

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	session_id,
	name,
	platform,
	status,
	public_key,
	push_token,
	created_at,
	last_seen_at,
	revoked_at,
	last_rotated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)`, qualifiedName("tenant", "identity_devices")),
			rawDeviceID,
			account.ID,
			rawSessionID,
			"Raw cross-account device",
			identity.DevicePlatformWeb,
			identity.DeviceStatusActive,
			"pk-raw-device",
			"",
			now,
			now,
			nil,
			nil,
		)
		if err != nil {
			t.Fatalf("insert raw cross-account device: %v", err)
		}

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	device_id,
	device_name,
	device_platform,
	ip_address,
	user_agent,
	status,
	current,
	created_at,
	last_seen_at,
	revoked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)`, qualifiedName("tenant", "identity_sessions")),
			rawSessionID,
			peerAccount.ID,
			rawDeviceID,
			"Raw cross-account session",
			identity.DevicePlatformWeb,
			"",
			"",
			identity.SessionStatusActive,
			true,
			now,
			now,
			nil,
		)
		if err != nil {
			t.Fatalf("insert raw cross-account session: %v", err)
		}

		err = tx.Commit()
		requirePgCode(t, err, "23503")

		if _, err := store.DeviceByID(ctx, rawDeviceID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected raw cross-account device to roll back, got %v", err)
		}
		if _, err := store.SessionByID(ctx, rawSessionID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected raw cross-account session to roll back, got %v", err)
		}
	})

	t.Run("rejects session update to an already attached device", func(t *testing.T) {
		session, err := store.SessionByID(ctx, sessionID)
		if err != nil {
			t.Fatalf("load valid session: %v", err)
		}
		session.DeviceID = peerDeviceID
		_, err = store.UpdateSession(ctx, session)
		if !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("rejects deferred device session fk on tx commit", func(t *testing.T) {
		err := store.WithinTx(ctx, func(tx identity.Store) error {
			_, saveErr := tx.SaveDevice(ctx, identity.Device{
				ID:         "dev-deferred",
				AccountID:  account.ID,
				SessionID:  "sess-missing-tx",
				Name:       "Deferred",
				Platform:   identity.DevicePlatformWeb,
				Status:     identity.DeviceStatusActive,
				PublicKey:  "pk-deferred",
				CreatedAt:  now,
				LastSeenAt: now,
			})
			return saveErr
		})
		if !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("rejects deferred session device fk on tx commit", func(t *testing.T) {
		err := store.WithinTx(ctx, func(tx identity.Store) error {
			_, saveErr := tx.SaveSession(ctx, identity.Session{
				ID:             "sess-deferred",
				AccountID:      account.ID,
				DeviceID:       "dev-missing-tx",
				DeviceName:     "Deferred",
				DevicePlatform: identity.DevicePlatformWeb,
				Status:         identity.SessionStatusActive,
				Current:        true,
				CreatedAt:      now,
				LastSeenAt:     now,
			})
			return saveErr
		})
		if !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("rejects session deletion when attached devices remain", func(t *testing.T) {
		if err := store.DeleteSession(ctx, sessionID); !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("rejects transactional session deletion when attached devices remain", func(t *testing.T) {
		err := store.WithinTx(ctx, func(tx identity.Store) error {
			return tx.DeleteSession(ctx, sessionID)
		})
		if !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("allows device deletion when no peer devices remain", func(t *testing.T) {
		const deviceID = "dev-cascade"
		const attachedSessionID = "sess-cascade"

		err := store.WithinTx(ctx, func(tx identity.Store) error {
			if _, err := tx.SaveDevice(ctx, identity.Device{
				ID:         deviceID,
				AccountID:  account.ID,
				SessionID:  attachedSessionID,
				Name:       "Cascade device",
				Platform:   identity.DevicePlatformWeb,
				Status:     identity.DeviceStatusActive,
				PublicKey:  "pk-cascade",
				CreatedAt:  now,
				LastSeenAt: now,
			}); err != nil {
				return err
			}
			_, err := tx.SaveSession(ctx, identity.Session{
				ID:             attachedSessionID,
				AccountID:      account.ID,
				DeviceID:       deviceID,
				DeviceName:     "Cascade device",
				DevicePlatform: identity.DevicePlatformWeb,
				Status:         identity.SessionStatusActive,
				Current:        true,
				CreatedAt:      now,
				LastSeenAt:     now,
			})
			return err
		})
		if err != nil {
			t.Fatalf("save cascade device/session pair: %v", err)
		}

		if err := store.DeleteDevice(ctx, deviceID); err != nil {
			t.Fatalf("delete device with attached session: %v", err)
		}

		if _, err := store.DeviceByID(ctx, deviceID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected deleted device to be gone, got %v", err)
		}
		if _, err := store.SessionByID(ctx, attachedSessionID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected cascaded session to be gone, got %v", err)
		}
	})

	t.Run("exposes pending expiry index", func(t *testing.T) {
		var indexName sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, qualifiedName("tenant", "identity_join_requests_pending_expires_at_idx")).Scan(&indexName); err != nil {
			t.Fatalf("load expiry index: %v", err)
		}
		if !indexName.Valid || indexName.String == "" {
			t.Fatal("expected pending expiry index to exist")
		}
	})

	t.Run("exposes device session index", func(t *testing.T) {
		var indexName sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, qualifiedName("tenant", "identity_devices_session_id_idx")).Scan(&indexName); err != nil {
			t.Fatalf("load device session index: %v", err)
		}
		if !indexName.Valid || indexName.String == "" {
			t.Fatal("expected device session index to exist")
		}
	})

	t.Run("does not expose unique device session index", func(t *testing.T) {
		var indexName sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, qualifiedName("tenant", "identity_devices_session_id_key")).Scan(&indexName); err != nil {
			t.Fatalf("load unique device session index: %v", err)
		}
		if indexName.Valid && indexName.String != "" {
			t.Fatalf("expected unique device session index to be absent, got %s", indexName.String)
		}
	})

	t.Run("deletes account with linked session and device", func(t *testing.T) {
		if err := store.DeleteAccount(ctx, account.ID); err != nil {
			t.Fatalf("delete account: %v", err)
		}

		if _, err := store.AccountByID(ctx, account.ID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected deleted account to be gone, got %v", err)
		}
		if _, err := store.SessionByID(ctx, sessionID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected deleted session to be gone, got %v", err)
		}
		if _, err := store.DeviceByID(ctx, primaryDeviceID); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected deleted device to be gone, got %v", err)
		}
	})
}

func TestIdentityAccountBoundaryConstraints(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 0, 15, 0, 0, time.UTC)

	account, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-1",
		Kind:        identity.AccountKindUser,
		Username:    "alice",
		DisplayName: "Alice",
		Status:      identity.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save account: %v", err)
	}
	peerAccount, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-2",
		Kind:        identity.AccountKindUser,
		Username:    "bob",
		DisplayName: "Bob",
		Status:      identity.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save peer account: %v", err)
	}

	t.Run("allows direct same-account device/session pair on commit", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	session_id,
	name,
	platform,
	status,
	public_key,
	push_token,
	created_at,
	last_seen_at,
	revoked_at,
	last_rotated_at
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	'',
	$8,
	$9,
	NULL,
	NULL
)
`, qualifiedName("tenant", "identity_devices")),
			"dev-raw-valid",
			account.ID,
			"sess-raw-valid",
			"Raw valid device",
			identity.DevicePlatformWeb,
			identity.DeviceStatusActive,
			"pk-raw-valid",
			now,
			now,
		)
		if err != nil {
			t.Fatalf("insert valid device: %v", err)
		}

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	device_id,
	device_name,
	device_platform,
	ip_address,
	user_agent,
	status,
	current,
	created_at,
	last_seen_at,
	revoked_at
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	'',
	'',
	$6,
	TRUE,
	$7,
	$8,
	NULL
)
`, qualifiedName("tenant", "identity_sessions")),
			"sess-raw-valid",
			account.ID,
			"dev-raw-valid",
			"Raw valid session",
			identity.DevicePlatformWeb,
			identity.SessionStatusActive,
			now,
			now,
		)
		if err != nil {
			t.Fatalf("insert valid session: %v", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit valid device/session pair: %v", err)
		}
	})

	t.Run("rejects direct cross-account device/session pair on commit", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	session_id,
	name,
	platform,
	status,
	public_key,
	push_token,
	created_at,
	last_seen_at,
	revoked_at,
	last_rotated_at
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	'',
	$8,
	$9,
	NULL,
	NULL
)
`, qualifiedName("tenant", "identity_devices")),
			"dev-raw-cross",
			peerAccount.ID,
			"sess-raw-cross",
			"Raw cross device",
			identity.DevicePlatformWeb,
			identity.DeviceStatusActive,
			"pk-raw-cross",
			now,
			now,
		)
		if err != nil {
			t.Fatalf("insert cross-account device: %v", err)
		}

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	device_id,
	device_name,
	device_platform,
	ip_address,
	user_agent,
	status,
	current,
	created_at,
	last_seen_at,
	revoked_at
) VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	'',
	'',
	$6,
	TRUE,
	$7,
	$8,
	NULL
)
`, qualifiedName("tenant", "identity_sessions")),
			"sess-raw-cross",
			account.ID,
			"dev-raw-cross",
			"Raw cross session",
			identity.DevicePlatformWeb,
			identity.SessionStatusActive,
			now,
			now,
		)
		if err != nil {
			t.Fatalf("insert cross-account session: %v", err)
		}

		requirePgCode(t, tx.Commit(), "23503")
	})
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

	unlock := dockermutex.Acquire(t, "postgres-integration")
	t.Cleanup(unlock)

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), postgresReadinessTimeout())
	defer cancel()

	cfg := postgresDockerConfigForGOOS(runtime.GOOS)
	cmd := exec.CommandContext(ctx, "docker", cfg.runArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("docker postgres unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		t.Skip("docker returned an empty container id")
	}

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
	})

	dsn := cfg.dsn
	if cfg.needsPortMapping {
		portOut, err := exec.CommandContext(ctx, "docker", "port", containerID, "5432/tcp").CombinedOutput()
		if err != nil {
			t.Skipf("lookup postgres port: %v: %s", err, strings.TrimSpace(string(portOut)))
		}
		hostPort := strings.TrimSpace(string(portOut))
		if hostPort == "" {
			t.Skip("docker did not report a mapped port")
		}
		hostPort = hostPort[strings.LastIndex(hostPort, ":")+1:]
		dsn = fmt.Sprintf(cfg.dsn, hostPort)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres database: %v", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), postgresReadinessTimeout())
	defer pingCancel()
	for pingCtx.Err() == nil {
		if err := db.PingContext(pingCtx); err == nil {
			return db
		}
		time.Sleep(1 * time.Second)
	}

	_ = db.Close()
	t.Skip("postgres container did not become ready")
	return nil
}

func postgresReadinessTimeout() time.Duration {
	const envName = "ZVONILKA_PGSTORE_READY_TIMEOUT"

	if raw := strings.TrimSpace(os.Getenv(envName)); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}

	return 3 * time.Minute
}

type postgresDockerConfig struct {
	runArgs          []string
	dsn              string
	needsPortMapping bool
}

func postgresDockerConfigForGOOS(goos string) postgresDockerConfig {
	if goos == "linux" {
		return postgresDockerConfig{
			runArgs: []string{
				"run",
				"-d",
				"--rm",
				"--network",
				"host",
				"-e",
				"POSTGRES_PASSWORD=pass",
				"-e",
				"POSTGRES_DB=test",
				"postgres:16-alpine",
			},
			dsn: "postgres://postgres:pass@127.0.0.1:5432/test?sslmode=disable",
		}
	}

	return postgresDockerConfig{
		runArgs: []string{
			"run",
			"-d",
			"--rm",
			"-e",
			"POSTGRES_PASSWORD=pass",
			"-e",
			"POSTGRES_DB=test",
			"-p",
			"127.0.0.1::5432",
			"postgres:16-alpine",
		},
		dsn:              "postgres://postgres:pass@127.0.0.1:%s/test?sslmode=disable",
		needsPortMapping: true,
	}
}

func TestPostgresReadinessTimeout(t *testing.T) {
	t.Run("uses default when unset", func(t *testing.T) {
		t.Setenv("ZVONILKA_PGSTORE_READY_TIMEOUT", "")
		if got := postgresReadinessTimeout(); got != 3*time.Minute {
			t.Fatalf("expected default timeout, got %v", got)
		}
	})

	t.Run("uses valid override", func(t *testing.T) {
		t.Setenv("ZVONILKA_PGSTORE_READY_TIMEOUT", "45s")
		if got := postgresReadinessTimeout(); got != 45*time.Second {
			t.Fatalf("expected override timeout, got %v", got)
		}
	})

	t.Run("ignores invalid override", func(t *testing.T) {
		t.Setenv("ZVONILKA_PGSTORE_READY_TIMEOUT", "bogus")
		if got := postgresReadinessTimeout(); got != 3*time.Minute {
			t.Fatalf("expected default timeout for invalid override, got %v", got)
		}
	})
}

func TestPostgresDockerConfigForGOOS(t *testing.T) {
	t.Run("linux uses host network", func(t *testing.T) {
		cfg := postgresDockerConfigForGOOS("linux")
		if cfg.needsPortMapping {
			t.Fatal("expected linux config to avoid port mapping")
		}
		if !containsAll(cfg.runArgs, "--network", "host", "POSTGRES_PASSWORD=pass") {
			t.Fatalf("expected linux config to use host network and password auth, got %v", cfg.runArgs)
		}
		if cfg.dsn != "postgres://postgres:pass@127.0.0.1:5432/test?sslmode=disable" {
			t.Fatalf("unexpected linux DSN: %s", cfg.dsn)
		}
	})

	t.Run("non-linux uses published port", func(t *testing.T) {
		cfg := postgresDockerConfigForGOOS("darwin")
		if !cfg.needsPortMapping {
			t.Fatal("expected non-linux config to use port mapping")
		}
		if !containsAll(cfg.runArgs, "-p", "127.0.0.1::5432", "POSTGRES_PASSWORD=pass") {
			t.Fatalf("expected non-linux config to publish a port, got %v", cfg.runArgs)
		}
		if cfg.dsn != "postgres://postgres:pass@127.0.0.1:%s/test?sslmode=disable" {
			t.Fatalf("unexpected non-linux DSN: %s", cfg.dsn)
		}
	})
}

func containsAll(values []string, want ...string) bool {
	for _, needle := range want {
		found := false
		for _, value := range values {
			if value == needle {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func repoMigrationsPath(t *testing.T, names ...string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}

	sourceDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "deploy", "migrations", "postgres")
	if len(names) == 0 {
		return sourceDir
	}

	dir := t.TempDir()
	for _, name := range names {
		source, err := os.ReadFile(filepath.Join(sourceDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), source, 0o600); err != nil {
			t.Fatalf("copy migration %s: %v", name, err)
		}
	}

	return dir
}

func requirePgCode(t *testing.T, err error, code string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected postgres error code %s, got nil", code)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error code %s, got %v", code, err)
	}
	if pgErr.Code != code {
		t.Fatalf("expected postgres error code %s, got %s (%v)", code, pgErr.Code, err)
	}
}

func requireInvalidInput(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
