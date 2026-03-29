package e2ee

import "time"

type PublicKey struct {
	KeyID     string
	Algorithm string
	PublicKey []byte
	CreatedAt time.Time
	RotatedAt time.Time
	ExpiresAt time.Time
}

type SignedPreKey struct {
	Key       PublicKey
	Signature []byte
}

type OneTimePreKey struct {
	Key                PublicKey
	ClaimedAt          time.Time
	ClaimedByAccountID string
	ClaimedByDeviceID  string
}

type DeviceBundle struct {
	AccountID             string
	DeviceID              string
	IdentityKey           PublicKey
	SignedPreKey          SignedPreKey
	OneTimePreKey         OneTimePreKey
	OneTimePreKeysAvail   uint32
	DeviceLastSeenAt      time.Time
}
