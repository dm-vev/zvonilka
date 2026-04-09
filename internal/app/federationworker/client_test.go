package federationworker

import (
	"context"
	"testing"

	"github.com/dm-vev/zvonilka/internal/domain/federation"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestDialTargetSupportsBridgeSchemes(t *testing.T) {
	t.Parallel()

	target, _, err := dialTarget("meshtastic://127.0.0.1:7443")
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:7443", target)

	target, _, err = dialTarget("meshcore+grpc://bridge.internal:9000")
	require.NoError(t, err)
	require.Equal(t, "bridge.internal:9000", target)

	target, _, err = dialTarget("meshtastic+grpcs://mesh.example")
	require.NoError(t, err)
	require.Equal(t, "mesh.example:443", target)
}

func TestWithFederationCredentialsIncludesTransportMetadata(t *testing.T) {
	t.Parallel()

	ctx := withFederationCredentials(
		context.Background(),
		"Alpha.Example",
		"secret",
		federation.TransportKindMeshtastic,
	)
	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	require.Equal(t, []string{"Bearer secret"}, md.Get("authorization"))
	require.Equal(t, []string{"alpha.example"}, md.Get(federationServerNameMetadataKey))
	require.Equal(t, []string{"meshtastic"}, md.Get(federationTransportKindMetadataKey))
}
