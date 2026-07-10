package pgstore

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageMetadataRoundTripTextEntities(t *testing.T) {
	metadata := map[string]string{
		"bot.text_entities":    `[{"type":"bold","offset":3,"length":4}]`,
		"bot.caption_entities": `[{"type":"text_link","offset":0,"length":4,"url":"https://example.org"}]`,
	}

	encoded, err := encodeMetadata(metadata)
	require.NoError(t, err)
	decoded, err := decodeMetadata(encoded)
	require.NoError(t, err)
	require.Equal(t, metadata, decoded)
}
