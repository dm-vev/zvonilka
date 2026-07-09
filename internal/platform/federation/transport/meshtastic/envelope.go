package meshtastic

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/platform/federation/transport"
)

type legacyEnvelope struct {
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

var errMalformedEnvelope = errors.New("malformed meshtastic envelope")

const (
	legacyEnvelopeVersion = 2
	binaryEnvelopeVersion = 1
)

var binaryEnvelopeMagic = []byte{'Z', 'V', 'F'}

// EncodeTextEnvelope renders one bridge fragment as a compact text-safe frame.
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

	frame, err := encodeBinaryEnvelope(peerServerName, linkName, fragment)
	if err != nil {
		return "", err
	}

	return prefix + base64.RawURLEncoding.EncodeToString(frame), nil
}

// DecodeTextEnvelope parses one received text-safe frame back into a bridge fragment.
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

	frame, frameErr := decodeBinaryEnvelope(raw)
	if frameErr == nil {
		return frame, nil
	}

	legacy, legacyErr := decodeLegacyEnvelope(raw)
	if legacyErr == nil {
		return legacy, nil
	}

	if errors.Is(frameErr, errUnsupportedEnvelope) {
		return transport.ReceivedFragment{}, legacyErr
	}

	return transport.ReceivedFragment{}, frameErr
}

func encodeBinaryEnvelope(
	peerServerName string,
	linkName string,
	fragment *federationv1.BundleFragment,
) ([]byte, error) {
	bundleID := strings.TrimSpace(fragment.GetBundleId())
	dedupKey := strings.TrimSpace(fragment.GetDedupKey())
	payloadType := strings.TrimSpace(fragment.GetPayloadType())
	integrityHash := strings.TrimSpace(fragment.GetIntegrityHash())
	authTag := strings.TrimSpace(fragment.GetAuthTag())
	fragmentCount := fragment.GetFragmentCount()
	if bundleID == "" || dedupKey == "" || fragmentCount == 0 || integrityHash == "" || authTag == "" {
		return nil, errInvalidConfig
	}

	buf := bytes.NewBuffer(make([]byte, 0, len(fragment.GetPayload())+128))
	buf.Write(binaryEnvelopeMagic)
	buf.WriteByte(binaryEnvelopeVersion)
	writeFrameString(buf, peerServerName)
	writeFrameString(buf, linkName)
	writeFrameString(buf, bundleID)
	writeFrameString(buf, dedupKey)
	writeUvarint(buf, fragment.GetCursorFrom())
	writeUvarint(buf, fragment.GetCursorTo())
	writeUvarint(buf, uint64(fragment.GetEventCount()))
	writeFrameString(buf, payloadType)
	writeUvarint(buf, uint64(fragment.GetCompression()))
	writeFrameString(buf, integrityHash)
	writeFrameString(buf, authTag)
	writeUvarint(buf, uint64(fragment.GetFragmentIndex()))
	writeUvarint(buf, uint64(fragmentCount))
	writeFrameBytes(buf, fragment.GetPayload())

	return buf.Bytes(), nil
}

func decodeBinaryEnvelope(raw []byte) (transport.ReceivedFragment, error) {
	if len(raw) < len(binaryEnvelopeMagic)+1 || !bytes.Equal(raw[:len(binaryEnvelopeMagic)], binaryEnvelopeMagic) {
		return transport.ReceivedFragment{}, errUnsupportedEnvelope
	}
	if raw[len(binaryEnvelopeMagic)] != binaryEnvelopeVersion {
		return transport.ReceivedFragment{}, errUnsupportedEnvelope
	}

	reader := frameReader{raw: raw[len(binaryEnvelopeMagic)+1:]}
	peerServerName, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	linkName, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	bundleID, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	dedupKey, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	cursorFrom, err := reader.readUvarint()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	cursorTo, err := reader.readUvarint()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	eventCount, err := reader.readUvarint()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	payloadType, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	compression, err := reader.readUvarint()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	integrityHash, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	authTag, err := reader.readString()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	fragmentIndex, err := reader.readUvarint()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	fragmentCount, err := reader.readUvarint()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	payload, err := reader.readBytes()
	if err != nil {
		return transport.ReceivedFragment{}, err
	}
	if !reader.empty() {
		return transport.ReceivedFragment{}, errMalformedEnvelope
	}

	return buildReceivedFragment(
		peerServerName,
		linkName,
		bundleID,
		dedupKey,
		cursorFrom,
		cursorTo,
		eventCount,
		payloadType,
		compression,
		integrityHash,
		authTag,
		fragmentIndex,
		fragmentCount,
		payload,
	)
}

func decodeLegacyEnvelope(raw []byte) (transport.ReceivedFragment, error) {
	var envelope legacyEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return transport.ReceivedFragment{}, fmt.Errorf("unmarshal meshtastic frame envelope: %w", err)
	}
	if envelope.Version != legacyEnvelopeVersion {
		return transport.ReceivedFragment{}, errUnsupportedEnvelope
	}

	payload, err := base64.RawURLEncoding.DecodeString(envelope.PayloadBase64)
	if err != nil {
		return transport.ReceivedFragment{}, fmt.Errorf("decode meshtastic fragment payload: %w", err)
	}

	return buildReceivedFragment(
		envelope.PeerServerName,
		envelope.LinkName,
		envelope.BundleID,
		envelope.DedupKey,
		envelope.CursorFrom,
		envelope.CursorTo,
		uint64(envelope.EventCount),
		envelope.PayloadType,
		uint64(uint32(envelope.Compression)),
		envelope.IntegrityHash,
		envelope.AuthTag,
		uint64(envelope.FragmentIndex),
		uint64(envelope.FragmentCount),
		payload,
	)
}

func buildReceivedFragment(
	peerServerName string,
	linkName string,
	bundleID string,
	dedupKey string,
	cursorFrom uint64,
	cursorTo uint64,
	eventCount uint64,
	payloadType string,
	compression uint64,
	integrityHash string,
	authTag string,
	fragmentIndex uint64,
	fragmentCount uint64,
	payload []byte,
) (transport.ReceivedFragment, error) {
	peerServerName = strings.TrimSpace(strings.ToLower(peerServerName))
	linkName = strings.TrimSpace(strings.ToLower(linkName))
	bundleID = strings.TrimSpace(bundleID)
	dedupKey = strings.TrimSpace(dedupKey)
	payloadType = strings.TrimSpace(payloadType)
	integrityHash = strings.TrimSpace(integrityHash)
	authTag = strings.TrimSpace(authTag)
	if peerServerName == "" || linkName == "" || bundleID == "" || dedupKey == "" || fragmentCount == 0 ||
		integrityHash == "" || authTag == "" {
		return transport.ReceivedFragment{}, errMalformedEnvelope
	}
	if eventCount > uint64(^uint32(0)) || compression > uint64(^uint32(0)>>1) ||
		fragmentIndex > uint64(^uint32(0)) || fragmentCount > uint64(^uint32(0)) {
		return transport.ReceivedFragment{}, errMalformedEnvelope
	}

	return transport.ReceivedFragment{
		PeerServerName: peerServerName,
		LinkName:       linkName,
		Fragment: &federationv1.BundleFragment{
			BundleId:      bundleID,
			DedupKey:      dedupKey,
			CursorFrom:    cursorFrom,
			CursorTo:      cursorTo,
			EventCount:    uint32(eventCount),
			PayloadType:   payloadType,
			Compression:   federationv1.CompressionKind(int32(compression)),
			IntegrityHash: integrityHash,
			AuthTag:       authTag,
			FragmentIndex: uint32(fragmentIndex),
			FragmentCount: uint32(fragmentCount),
			Payload:       append([]byte(nil), payload...),
		},
	}, nil
}

func encodeLegacyTextEnvelope(
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

	envelope := legacyEnvelope{
		Version:        legacyEnvelopeVersion,
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
	if envelope.BundleID == "" || envelope.DedupKey == "" || envelope.FragmentCount == 0 ||
		envelope.IntegrityHash == "" || envelope.AuthTag == "" {
		return "", errInvalidConfig
	}

	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal meshtastic fragment envelope: %w", err)
	}

	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func writeFrameString(buf *bytes.Buffer, value string) {
	writeFrameBytes(buf, []byte(value))
}

func writeFrameBytes(buf *bytes.Buffer, value []byte) {
	writeUvarint(buf, uint64(len(value)))
	buf.Write(value)
}

func writeUvarint(buf *bytes.Buffer, value uint64) {
	var encoded [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(encoded[:], value)
	buf.Write(encoded[:n])
}

type frameReader struct {
	raw []byte
}

func (r *frameReader) readUvarint() (uint64, error) {
	value, n := binary.Uvarint(r.raw)
	if n <= 0 {
		return 0, errMalformedEnvelope
	}
	r.raw = r.raw[n:]
	return value, nil
}

func (r *frameReader) readString() (string, error) {
	value, err := r.readBytes()
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (r *frameReader) readBytes() ([]byte, error) {
	length, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	if length > uint64(len(r.raw)) {
		return nil, errMalformedEnvelope
	}
	size := int(length)
	value := append([]byte(nil), r.raw[:size]...)
	r.raw = r.raw[size:]
	return value, nil
}

func (r *frameReader) empty() bool {
	return len(r.raw) == 0
}
