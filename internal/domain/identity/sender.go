package identity

import "context"

// CodeSender delivers verification codes to login targets.
//
// The interface stays small so tests can replace delivery with a deterministic fake.
type CodeSender interface {
	SendLoginCode(ctx context.Context, target LoginTarget, code string) error
}

// NoopCodeSender ignores all verification code delivery attempts.
type NoopCodeSender struct{}

// SendLoginCode ignores the login code.
func (NoopCodeSender) SendLoginCode(context.Context, LoginTarget, string) error {
	return nil
}
