package federation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// SignBundle applies federation integrity and authentication metadata to one bundle snapshot.
func (s *Service) SignBundle(ctx context.Context, peerID string, linkID string, bundle Bundle) (Bundle, error) {
	if err := s.validateContext(ctx, "sign federation bundle"); err != nil {
		return Bundle{}, err
	}

	peer, link, err := s.peerAndLink(ctx, peerID, linkID)
	if err != nil {
		return Bundle{}, err
	}

	return signBundle(peer, link, bundle)
}

func (s *Service) verifyBundle(ctx context.Context, peerID string, linkID string, bundle Bundle) error {
	peer, link, err := s.peerAndLink(ctx, peerID, linkID)
	if err != nil {
		return err
	}

	return verifyBundle(peer, link, bundle)
}

func (s *Service) peerAndLink(ctx context.Context, peerID string, linkID string) (Peer, Link, error) {
	peerID = strings.TrimSpace(peerID)
	linkID = strings.TrimSpace(linkID)
	if peerID == "" || linkID == "" {
		return Peer{}, Link{}, ErrInvalidInput
	}

	link, err := s.store.LinkByID(ctx, linkID)
	if err != nil {
		return Peer{}, Link{}, fmt.Errorf("load federation link %s: %w", linkID, err)
	}
	if link.PeerID != peerID {
		return Peer{}, Link{}, ErrConflict
	}

	peer, err := s.store.PeerByID(ctx, peerID)
	if err != nil {
		return Peer{}, Link{}, fmt.Errorf("load federation peer %s: %w", peerID, err)
	}

	return peer, link, nil
}

func signBundle(peer Peer, link Link, bundle Bundle) (Bundle, error) {
	bundle.IntegrityHash = payloadIntegrityHash(bundle.Payload)
	bundle.AuthTag = bundleAuthTag(peer, link, bundle, bundle.IntegrityHash)
	return bundle, nil
}

func verifyBundle(peer Peer, link Link, bundle Bundle) error {
	expectedIntegrity := payloadIntegrityHash(bundle.Payload)
	if subtle.ConstantTimeCompare([]byte(expectedIntegrity), []byte(strings.TrimSpace(strings.ToLower(bundle.IntegrityHash)))) != 1 {
		return ErrUnauthorized
	}

	expectedAuth := bundleAuthTag(peer, link, bundle, expectedIntegrity)
	if subtle.ConstantTimeCompare([]byte(expectedAuth), []byte(strings.TrimSpace(strings.ToLower(bundle.AuthTag)))) != 1 {
		return ErrUnauthorized
	}

	return nil
}

func payloadIntegrityHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func bundleAuthTag(peer Peer, link Link, bundle Bundle, integrityHash string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(peer.SharedSecret)))
	writeSignedField(mac, strings.TrimSpace(strings.ToLower(peer.ServerName)))
	writeSignedField(mac, strings.TrimSpace(strings.ToLower(link.Name)))
	writeSignedField(mac, strconv.FormatUint(bundle.CursorFrom, 10))
	writeSignedField(mac, strconv.FormatUint(bundle.CursorTo, 10))
	writeSignedField(mac, strconv.Itoa(bundle.EventCount))
	writeSignedField(mac, strings.TrimSpace(strings.ToLower(bundle.PayloadType)))
	writeSignedField(mac, strings.TrimSpace(strings.ToLower(string(bundle.Compression))))
	writeSignedField(mac, strings.TrimSpace(strings.ToLower(integrityHash)))
	return hex.EncodeToString(mac.Sum(nil))
}

func writeSignedField(mac interface{ Write([]byte) (int, error) }, value string) {
	_, _ = mac.Write([]byte(value))
	_, _ = mac.Write([]byte{'\n'})
}
