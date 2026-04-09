package meshcore

import (
	"context"
	"strings"
	"testing"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	meshtastictransport "github.com/dm-vev/zvonilka/internal/platform/federation/transport/meshtastic"
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
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
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

func TestAdapterSendAndReceive(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	adapter, err := newWithRunner(Config{
		InterfaceKind:    "serial",
		Device:           "/dev/ttyUSB1",
		HelperPython:     "/usr/bin/python3",
		HelperScriptPath: "/tmp/meshcore_bridge.py",
		ReceiveTimeout:   3 * time.Second,
		TextPrefix:       "zv1:",
		Destination:      "peer-pubkey",
	}, runner)
	require.NoError(t, err)

	sent, err := adapter.Send(context.Background(), "mesh.example", "meshcore", nil, []*federationv1.BundleFragment{{
		FragmentId:    "frag-1",
		BundleId:      "bundle-1",
		DedupKey:      "bundle-1:frag:000000",
		CursorFrom:    1,
		CursorTo:      1,
		EventCount:    1,
		PayloadType:   "bundle",
		Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
		IntegrityHash: "hash-1",
		AuthTag:       "auth-1",
		FragmentIndex: 0,
		FragmentCount: 1,
		Payload:       []byte("hello"),
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"frag-1"}, sent)
	require.Len(t, runner.commands, 1)
	require.Contains(t, strings.Join(runner.commands[0].args, " "), "--destination peer-pubkey")

	text, err := meshtastictransport.EncodeTextEnvelope("mesh.example", "meshcore", &federationv1.BundleFragment{
		BundleId:      "bundle-2",
		DedupKey:      "bundle-2:frag:000000",
		CursorFrom:    2,
		CursorTo:      2,
		EventCount:    1,
		PayloadType:   "bundle",
		Compression:   federationv1.CompressionKind_COMPRESSION_KIND_NONE,
		IntegrityHash: "hash-2",
		AuthTag:       "auth-2",
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
	require.Equal(t, "hash-2", received[0].Fragment.GetIntegrityHash())
	require.Equal(t, "auth-2", received[0].Fragment.GetAuthTag())
}
