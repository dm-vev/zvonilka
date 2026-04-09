package meshtastic

import (
	"context"
	"strings"
	"testing"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/stretchr/testify/require"
)

type recordedCommand struct {
	name string
	args []string
}

type stubRunner struct {
	commands []recordedCommand
	outputs  [][]byte
	err      error
}

func (r *stubRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	_ = ctx
	r.commands = append(r.commands, recordedCommand{
		name: name,
		args: append([]string(nil), args...),
	})
	if r.err != nil {
		return nil, r.err
	}
	if len(r.outputs) == 0 {
		return nil, nil
	}
	output := r.outputs[0]
	r.outputs = r.outputs[1:]
	return output, nil
}

func TestEncodeAndDecodeTextEnvelope(t *testing.T) {
	t.Parallel()

	fragment := &federationv1.BundleFragment{
		BundleId:      "bundle-1",
		DedupKey:      "bundle-1:frag:000000",
		CursorFrom:    10,
		CursorTo:      11,
		EventCount:    2,
		PayloadType:   "conversation.events.v1",
		Compression:   federationv1.CompressionKind_COMPRESSION_KIND_GZIP,
		IntegrityHash: "hash-1",
		AuthTag:       "auth-1",
		FragmentIndex: 0,
		FragmentCount: 2,
		Payload:       []byte("payload"),
	}

	text, err := EncodeTextEnvelope("mesh.example", "mesh", fragment, "zv1:")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(text, "zv1:"))

	received, err := DecodeTextEnvelope(text, "zv1:")
	require.NoError(t, err)
	require.Equal(t, "mesh.example", received.PeerServerName)
	require.Equal(t, "mesh", received.LinkName)
	require.Equal(t, "bundle-1", received.Fragment.GetBundleId())
	require.Equal(t, []byte("payload"), received.Fragment.GetPayload())

	legacyText, err := encodeLegacyTextEnvelope("mesh.example", "mesh", fragment, "zv1:")
	require.NoError(t, err)
	require.Less(t, len(text), len(legacyText))
}

func TestDecodeTextEnvelopeSupportsLegacyFormat(t *testing.T) {
	t.Parallel()

	text, err := encodeLegacyTextEnvelope("mesh.example", "mesh", &federationv1.BundleFragment{
		BundleId:      "bundle-legacy",
		DedupKey:      "bundle-legacy:frag:000000",
		CursorFrom:    20,
		CursorTo:      22,
		EventCount:    1,
		PayloadType:   "bundle",
		Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
		IntegrityHash: "hash-legacy",
		AuthTag:       "auth-legacy",
		FragmentIndex: 0,
		FragmentCount: 1,
		Payload:       []byte("world"),
	}, "zv1:")
	require.NoError(t, err)

	received, err := DecodeTextEnvelope(text, "zv1:")
	require.NoError(t, err)
	require.Equal(t, "mesh.example", received.PeerServerName)
	require.Equal(t, "mesh", received.LinkName)
	require.Equal(t, "bundle-legacy", received.Fragment.GetBundleId())
	require.Equal(t, []byte("world"), received.Fragment.GetPayload())
}

func TestAdapterSendAndReceive(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	adapter, err := newWithRunner(Config{
		InterfaceKind:    "serial",
		Device:           "/dev/ttyUSB0",
		HelperPython:     "/usr/bin/python3",
		HelperScriptPath: "/tmp/meshtastic_bridge.py",
		ReceiveTimeout:   3 * time.Second,
		TextPrefix:       "zv1:",
	}, runner)
	require.NoError(t, err)

	sent, err := adapter.Send(context.Background(), "mesh.example", "mesh", nil, []*federationv1.BundleFragment{{
		FragmentId:    "frag-1",
		BundleId:      "bundle-1",
		DedupKey:      "bundle-1:frag:000000",
		CursorFrom:    1,
		CursorTo:      1,
		EventCount:    1,
		PayloadType:   "bundle",
		Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
		IntegrityHash: "hash-2",
		AuthTag:       "auth-2",
		FragmentIndex: 0,
		FragmentCount: 1,
		Payload:       []byte("hello"),
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"frag-1"}, sent)
	require.Len(t, runner.commands, 1)
	require.Equal(t, "/usr/bin/python3", runner.commands[0].name)
	require.Contains(t, strings.Join(runner.commands[0].args, " "), "send")

	text, err := EncodeTextEnvelope("mesh.example", "mesh", &federationv1.BundleFragment{
		BundleId:      "bundle-2",
		DedupKey:      "bundle-2:frag:000000",
		CursorFrom:    2,
		CursorTo:      2,
		EventCount:    1,
		PayloadType:   "bundle",
		Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
		IntegrityHash: "hash-3",
		AuthTag:       "auth-3",
		FragmentIndex: 0,
		FragmentCount: 1,
		Payload:       []byte("world"),
	}, "zv1:")
	require.NoError(t, err)

	runner.outputs = [][]byte{[]byte(text + "\n")}
	received, err := adapter.Receive(context.Background(), 5)
	require.NoError(t, err)
	require.Len(t, received, 1)
	require.Equal(t, "mesh.example", received[0].PeerServerName)
	require.Equal(t, "bundle-2", received[0].Fragment.GetBundleId())
	require.Equal(t, []byte("world"), received[0].Fragment.GetPayload())
	require.Equal(t, "hash-3", received[0].Fragment.GetIntegrityHash())
	require.Equal(t, "auth-3", received[0].Fragment.GetAuthTag())
	require.Len(t, runner.commands, 2)
	require.Contains(t, strings.Join(runner.commands[1].args, " "), "listen")
}
