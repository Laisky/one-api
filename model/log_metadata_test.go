package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogMetadataValue verifies that Value serializes metadata to JSON when populated.
func TestLogMetadataValue(t *testing.T) {
	t.Parallel()
	var empty LogMetadata
	v, err := empty.Value()
	require.NoError(t, err)
	require.Nil(t, v)

	populated := LogMetadata{"foo": 42}
	v, err = populated.Value()
	require.NoError(t, err)
	str, ok := v.(string)
	require.True(t, ok)
	require.JSONEq(t, `{"foo":42}`, str)
}

// TestLogMetadataScan ensures Scan correctly deserializes JSON payloads.
func TestLogMetadataScan(t *testing.T) {
	t.Parallel()
	var metadata LogMetadata
	err := metadata.Scan([]byte(`{"bar": "baz"}`))
	require.NoError(t, err)
	require.Equal(t, LogMetadata{"bar": "baz"}, metadata)

	err = metadata.Scan(nil)
	require.NoError(t, err)
	require.Nil(t, metadata)

	metadata = nil
	err = metadata.Scan("{\"bar\":123}")
	require.NoError(t, err)
	require.Equal(t, LogMetadata{"bar": float64(123)}, metadata)
}

// TestAppendCacheWriteTokensMetadata confirms cache write tokens are appended as expected.
func TestAppendCacheWriteTokensMetadata(t *testing.T) {
	t.Parallel()
	metadata := AppendCacheWriteTokensMetadata(nil, 0, 0)
	require.Nil(t, metadata)

	metadata = AppendCacheWriteTokensMetadata(nil, 10, 0)
	require.NotNil(t, metadata)
	tokens, ok := metadata[LogMetadataKeyCacheWriteTokens].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 10, tokens[LogMetadataKeyCacheWrite5m])
	_, hasOneHour := tokens[LogMetadataKeyCacheWrite1h]
	require.False(t, hasOneHour)

	metadata = AppendCacheWriteTokensMetadata(metadata, 0, 5)
	tokens, ok = metadata[LogMetadataKeyCacheWriteTokens].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 10, tokens[LogMetadataKeyCacheWrite5m])
	require.Equal(t, 5, tokens[LogMetadataKeyCacheWrite1h])
}
