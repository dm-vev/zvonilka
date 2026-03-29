package e2ee

type UploadDevicePreKeysParams struct {
	AccountID            string
	DeviceID             string
	SignedPreKey         SignedPreKey
	OneTimePreKeys       []OneTimePreKey
	ReplaceOneTimePreKey bool
}

type FetchAccountBundlesParams struct {
	RequesterAccountID   string
	RequesterDeviceID    string
	TargetAccountID      string
	ConsumeOneTimePreKey bool
}
