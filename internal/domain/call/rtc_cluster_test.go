package call

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTCConfigEndpointForSessionUsesNodeEndpoint(t *testing.T) {
	t.Parallel()

	cfg := RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		Nodes: []RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls"},
		},
	}.normalize()

	require.Equal(t, "webrtc://node-b/calls", cfg.endpointForSession("node-b:rtc_call-1"))
	require.Equal(t, "webrtc://gateway/calls", cfg.endpointForSession("node-c:rtc_call-1"))
}
