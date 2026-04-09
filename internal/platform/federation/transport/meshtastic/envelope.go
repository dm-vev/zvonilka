package meshtastic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/platform/federation/transport"
)

type textEnvelope struct {
	Version        int    `json:"v"`
	PeerServerName string `json:"peer"`
	LinkName       string `json:"link"`
	BundleID       string `json:"bundle"`
	DedupKey       string `json:"dedup"`
	CursorFrom     uint64 `json:"from"`
	CursorTo       uint64 `json:"to"`
	EventCount     uint32 `json:"events"`
	PayloadType    string `json:"type"`
	Compression    int32  `json:"comp"`
	IntegrityHash  string `json:"ih"`
	AuthTag        string `json:"auth"`
	FragmentIndex  uint32 `json:"idx"`
	FragmentCount  uint32 `json:"count"`
	PayloadBase64  string `json:"payload"`
}

const currentEnvelopeVersion = 2

// EncodeTextEnvelope renders one bridge fragment as a Meshtastic-safe text frame.
func EncodeTextEnvelope(
	peerServerName string,
	linkName string,
	fragment *federationv1.BundleFragment,
	prefix string,
) (string, error) {
	if fragment == nil {
		return "", errInvalidConfig
	}

	peerServerName = strings.TrimSpace(strings.ToLower(peerServerName))
	linkName = strings.TrimSpace(strings.ToLower(linkName))
	prefix = strings.TrimSpace(prefix)
	if peerServerName == "" || linkName == "" || prefix == "" {
		return "", errInvalidConfig
	}

	envelope := textEnvelope{
		Version:        currentEnvelopeVersion,
		PeerServerName: peerServerName,
		LinkName:       linkName,
		BundleID:       strings.TrimSpace(fragment.GetBundleId()),
		DedupKey:       strings.TrimSpace(fragment.GetDedupKey()),
		CursorFrom:     fragment.GetCursorFrom(),
		CursorTo:       fragment.GetCursorTo(),
		EventCount:     fragment.GetEventCount(),
		PayloadType:    strings.TrimSpace(fragment.GetPayloadType()),
		Compression:    int32(fragment.GetCompression()),
		IntegrityHash:  strings.TrimSpace(fragment.GetIntegrityHash()),
		AuthTag:        strings.TrimSpace(fragment.GetAuthTag()),
		FragmentIndex:  fragment.GetFragmentIndex(),
		FragmentCount:  fragment.GetFragmentCount(),
		PayloadBase64:  base64.RawURLEncoding.EncodeToString(fragment.GetPayload()),
	}
	if envelope.BundleID == "" || envelope.DedupKey == "" || envelope.FragmentCount == 0 || envelope.IntegrityHash == "" || envelope.AuthTag == "" {
		return "", errInvalidConfig
	}

	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal meshtastic fragment envelope: %w", err)
	}

	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeTextEnvelope parses one received Meshtastic text frame back into a bridge fragment.
func DecodeTextEnvelope(text string, prefix string) (transport.ReceivedFragment, error) {
	text = strings.TrimSpace(text)
	prefix = strings.TrimSpace(prefix)
	if text == "" || prefix == "" || !strings.HasPrefix(text, prefix) {
		return transport.ReceivedFragment{}, errEnvelopePrefix
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(text, prefix))
	if err != nil {
		return transport.ReceivedFragment{}, fmt.Errorf("decode meshtastic frame envelope: %w", err)
	}

	var envelope textEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return transport.ReceivedFragment{}, fmt.Errorf("unmarshal meshtastic frame envelope: %w", err)
	}
	if envelope.Version != currentEnvelopeVersion {
		return transport.ReceivedFragment{}, errUnsupportedEnvelope
	}

	payload, err := base64.RawURLEncoding.DecodeString(envelope.PayloadBase64)
	if err != nil {
		return transport.ReceivedFragment{}, fmt.Errorf("decode meshtastic fragment payload: %w", err)
	}

	return transport.ReceivedFragment{
		PeerServerName: strings.TrimSpace(strings.ToLower(envelope.PeerServerName)),
		LinkName:       strings.TrimSpace(strings.ToLower(envelope.LinkName)),
		Fragment: &federationv1.BundleFragment{
			BundleId:      envelope.BundleID,
			DedupKey:      envelope.DedupKey,
			CursorFrom:    envelope.CursorFrom,
			CursorTo:      envelope.CursorTo,
			EventCount:    envelope.EventCount,
			PayloadType:   envelope.PayloadType,
			Compression:   federationv1.CompressionKind(envelope.Compression),
			IntegrityHash: envelope.IntegrityHash,
			AuthTag:       envelope.AuthTag,
			FragmentIndex: envelope.FragmentIndex,
			FragmentCount: envelope.FragmentCount,
			Payload:       payload,
		},
	}, nil
}
