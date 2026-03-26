package identity

import (
	"sort"
	"strconv"
	"strings"
)

// beginLoginFingerprint captures the fields that make a login-start request unique.
func beginLoginFingerprint(params BeginLoginParams) string {
	username, email, phone := normalizeUsername(params.Username), normalizeEmail(params.Email), normalizePhone(params.Phone)
	return idempotencyFingerprint(
		"begin-login",
		username,
		email,
		phone,
		string(params.Delivery),
		params.DeviceName,
		string(params.Platform),
		params.ClientVersion,
		params.Locale,
	)
}

// verifyLoginFingerprint captures the fields that define a login verification attempt.
func verifyLoginFingerprint(params VerifyLoginCodeParams) string {
	return idempotencyFingerprint(
		"verify-login",
		params.ChallengeID,
		hashSecret(params.Code),
		hashSecret(params.TwoFactorCode),
		hashSecret(params.RecoveryPassword),
		strconv.FormatBool(params.EnablePasswordRecovery),
		params.DeviceName,
		string(params.Platform),
		params.PublicKey,
		params.PushToken,
	)
}

// registerDeviceFingerprint captures the fields that define a device registration attempt.
func registerDeviceFingerprint(params RegisterDeviceParams) string {
	return idempotencyFingerprint(
		"register-device",
		params.SessionID,
		params.DeviceName,
		string(params.Platform),
		params.PublicKey,
		params.PushToken,
	)
}

// revokeSessionFingerprint captures the fields that define a single-session revocation.
func revokeSessionFingerprint(params RevokeSessionParams) string {
	return idempotencyFingerprint(
		"revoke-session",
		params.SessionID,
		params.Reason,
	)
}

// revokeAllSessionsFingerprint captures the fields that define an account-wide revocation.
func revokeAllSessionsFingerprint(accountID string, params RevokeAllSessionsParams) string {
	return idempotencyFingerprint(
		"revoke-all-sessions",
		accountID,
		params.Reason,
	)
}

// submitJoinRequestFingerprint captures the fields that define a join request submission.
func submitJoinRequestFingerprint(params SubmitJoinRequestParams) string {
	username, email, phone := normalizeUsername(params.Username), normalizeEmail(params.Email), normalizePhone(params.Phone)
	return idempotencyFingerprint(
		"submit-join-request",
		username,
		email,
		phone,
		trimmed(params.DisplayName),
		trimmed(params.Note),
		trimmed(params.InviteCode),
	)
}

// createAccountFingerprint captures the fields that define an account creation request.
func createAccountFingerprint(params CreateAccountParams) string {
	username, email, phone := normalizeUsername(params.Username), normalizeEmail(params.Email), normalizePhone(params.Phone)
	return idempotencyFingerprint(
		"create-account",
		username,
		email,
		phone,
		trimmed(params.DisplayName),
		rolesFingerprint(params.Roles),
		trimmed(params.Note),
		trimmed(params.InviteCode),
		string(params.AccountKind),
		trimmed(params.CreatedBy),
	)
}

// approveJoinRequestFingerprint captures the fields that define a join-request approval.
func approveJoinRequestFingerprint(params ApproveJoinRequestParams) string {
	return idempotencyFingerprint(
		"approve-join-request",
		params.JoinRequestID,
		rolesFingerprint(params.Roles),
		trimmed(params.Note),
		trimmed(params.ReviewedBy),
		trimmed(params.DecisionReason),
	)
}

// rejectJoinRequestFingerprint captures the fields that define a join-request rejection.
func rejectJoinRequestFingerprint(params RejectJoinRequestParams) string {
	return idempotencyFingerprint(
		"reject-join-request",
		params.JoinRequestID,
		trimmed(params.Reason),
		trimmed(params.ReviewedBy),
	)
}

// authenticateBotFingerprint captures the fields that define a bot authentication attempt.
func authenticateBotFingerprint(params AuthenticateBotParams) string {
	return idempotencyFingerprint(
		"authenticate-bot",
		hashSecret(params.BotToken),
		params.DeviceName,
		string(params.Platform),
		params.PublicKey,
		params.ClientVersion,
		params.Locale,
	)
}

// refreshSessionFingerprint captures the fields that define a refresh-token rotation attempt.
func refreshSessionFingerprint(params RefreshSessionParams) string {
	return idempotencyFingerprint(
		"refresh-session",
		hashSecret(params.RefreshToken),
		params.DeviceID,
	)
}

// idempotencyFingerprint builds a length-prefixed fingerprint from ordered parts.
//
// Length-prefixing keeps the serialization unambiguous even when a field contains the
// separator character.
func idempotencyFingerprint(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, part := range parts {
		if i > 0 {
			builder.WriteByte('|')
		}
		builder.WriteString(strconv.Itoa(len(part)))
		builder.WriteByte(':')
		builder.WriteString(part)
	}

	return builder.String()
}

// rolesFingerprint makes role order irrelevant before fingerprinting the request.
func rolesFingerprint(roles []Role) string {
	if len(roles) == 0 {
		return ""
	}

	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	sort.Strings(values)
	return idempotencyFingerprint(values...)
}
