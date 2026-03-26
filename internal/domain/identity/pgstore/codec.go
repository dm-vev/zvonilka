package pgstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func decodeRoles(raw string) ([]identity.Role, error) {
	if raw == "" {
		return nil, nil
	}

	var roles []identity.Role
	if err := json.Unmarshal([]byte(raw), &roles); err != nil {
		return nil, fmt.Errorf("decode account roles: %w", err)
	}

	if len(roles) == 0 {
		return nil, nil
	}

	return roles, nil
}

func encodeRoles(roles []identity.Role) (string, error) {
	if len(roles) == 0 {
		return "[]", nil
	}

	raw, err := json.Marshal(roles)
	if err != nil {
		return "", fmt.Errorf("encode account roles: %w", err)
	}

	return string(raw), nil
}

func decodeTargets(raw string) ([]identity.LoginTarget, error) {
	if raw == "" {
		return nil, nil
	}

	var targets []identity.LoginTarget
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		return nil, fmt.Errorf("decode login targets: %w", err)
	}

	if len(targets) == 0 {
		return nil, nil
	}

	return targets, nil
}

func encodeTargets(targets []identity.LoginTarget) (string, error) {
	if len(targets) == 0 {
		return "[]", nil
	}

	raw, err := json.Marshal(targets)
	if err != nil {
		return "", fmt.Errorf("encode login targets: %w", err)
	}

	return string(raw), nil
}

func scanNullableTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func nullTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func scanAccount(row rowScanner) (identity.Account, error) {
	var (
		account    identity.Account
		rolesRaw   string
		disabledAt sql.NullTime
		lastAuthAt sql.NullTime
	)

	if err := row.Scan(
		&account.ID,
		&account.Kind,
		&account.Username,
		&account.DisplayName,
		&account.Bio,
		&account.Email,
		&account.Phone,
		&rolesRaw,
		&account.Status,
		&account.BotTokenHash,
		&account.CreatedBy,
		&account.CreatedAt,
		&account.UpdatedAt,
		&disabledAt,
		&lastAuthAt,
		&account.CustomBadgeEmoji,
	); err != nil {
		return identity.Account{}, err
	}

	roles, err := decodeRoles(rolesRaw)
	if err != nil {
		return identity.Account{}, err
	}

	account.CreatedAt = account.CreatedAt.UTC()
	account.UpdatedAt = account.UpdatedAt.UTC()
	account.DisabledAt = scanNullableTime(disabledAt)
	account.LastAuthAt = scanNullableTime(lastAuthAt)
	account.Roles = roles

	return account, nil
}

func scanJoinRequest(row rowScanner) (identity.JoinRequest, error) {
	var (
		joinRequest identity.JoinRequest
		reviewedAt  sql.NullTime
	)

	if err := row.Scan(
		&joinRequest.ID,
		&joinRequest.Username,
		&joinRequest.DisplayName,
		&joinRequest.Email,
		&joinRequest.Phone,
		&joinRequest.Note,
		&joinRequest.Status,
		&joinRequest.RequestedAt,
		&reviewedAt,
		&joinRequest.ReviewedBy,
		&joinRequest.DecisionReason,
		&joinRequest.ExpiresAt,
	); err != nil {
		return identity.JoinRequest{}, err
	}

	joinRequest.RequestedAt = joinRequest.RequestedAt.UTC()
	joinRequest.ReviewedAt = scanNullableTime(reviewedAt)
	joinRequest.ExpiresAt = joinRequest.ExpiresAt.UTC()

	return joinRequest, nil
}

func scanLoginChallenge(row rowScanner) (identity.LoginChallenge, error) {
	var (
		challenge  identity.LoginChallenge
		targetsRaw string
		usedAt     sql.NullTime
	)

	if err := row.Scan(
		&challenge.ID,
		&challenge.AccountID,
		&challenge.AccountKind,
		&challenge.CodeHash,
		&challenge.DeliveryChannel,
		&targetsRaw,
		&challenge.ExpiresAt,
		&challenge.CreatedAt,
		&usedAt,
		&challenge.Used,
	); err != nil {
		return identity.LoginChallenge{}, err
	}

	targets, err := decodeTargets(targetsRaw)
	if err != nil {
		return identity.LoginChallenge{}, err
	}

	challenge.Targets = targets
	challenge.CreatedAt = challenge.CreatedAt.UTC()
	challenge.ExpiresAt = challenge.ExpiresAt.UTC()
	challenge.UsedAt = scanNullableTime(usedAt)

	return challenge, nil
}

func scanDevice(row rowScanner) (identity.Device, error) {
	var (
		device    identity.Device
		revokedAt sql.NullTime
		rotatedAt sql.NullTime
	)

	if err := row.Scan(
		&device.ID,
		&device.AccountID,
		&device.SessionID,
		&device.Name,
		&device.Platform,
		&device.Status,
		&device.PublicKey,
		&device.PushToken,
		&device.CreatedAt,
		&device.LastSeenAt,
		&revokedAt,
		&rotatedAt,
	); err != nil {
		return identity.Device{}, err
	}

	device.CreatedAt = device.CreatedAt.UTC()
	device.LastSeenAt = device.LastSeenAt.UTC()
	device.RevokedAt = scanNullableTime(revokedAt)
	device.LastRotatedAt = scanNullableTime(rotatedAt)

	return device, nil
}

func scanSession(row rowScanner) (identity.Session, error) {
	var (
		session   identity.Session
		revokedAt sql.NullTime
	)

	if err := row.Scan(
		&session.ID,
		&session.AccountID,
		&session.DeviceID,
		&session.DeviceName,
		&session.DevicePlatform,
		&session.IPAddress,
		&session.UserAgent,
		&session.Status,
		&session.Current,
		&session.CreatedAt,
		&session.LastSeenAt,
		&revokedAt,
	); err != nil {
		return identity.Session{}, err
	}

	session.CreatedAt = session.CreatedAt.UTC()
	session.LastSeenAt = session.LastSeenAt.UTC()
	session.RevokedAt = scanNullableTime(revokedAt)

	return session, nil
}

func scanCredential(row rowScanner) (identity.SessionCredential, error) {
	var credential identity.SessionCredential

	if err := row.Scan(
		&credential.SessionID,
		&credential.AccountID,
		&credential.DeviceID,
		&credential.Kind,
		&credential.TokenHash,
		&credential.ExpiresAt,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	); err != nil {
		return identity.SessionCredential{}, err
	}

	credential.ExpiresAt = credential.ExpiresAt.UTC()
	credential.CreatedAt = credential.CreatedAt.UTC()
	credential.UpdatedAt = credential.UpdatedAt.UTC()

	return credential, nil
}
