package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsureLogContentTopupFallback verifies that top-up logs receive a descriptive default when content is missing.
func TestEnsureLogContentTopupFallback(t *testing.T) {
	t.Parallel()
	logEntry := &Log{Type: LogTypeTopup, Quota: 500000}
	ensureLogContent(logEntry)
	require.NotEmpty(t, logEntry.Content)
	require.Contains(t, logEntry.Content, "Top-up event")
}

// TestBuildManageLogContentRedaction confirms that sensitive fields are redacted in manage logs.
func TestBuildManageLogContentRedaction(t *testing.T) {
	t.Parallel()
	content := buildManageLogContent("password", "secret123", "newSecret456", "actor=42")
	require.Contains(t, content, manageLogRedactedPlaceholder)
	require.NotContains(t, content, "secret123")
	require.Contains(t, content, "actor=42")
}

// TestBuildManageLogContentValues ensures non-sensitive field changes are captured verbatim.
func TestBuildManageLogContentValues(t *testing.T) {
	t.Parallel()
	content := buildManageLogContent("quota", 100, 200, "")
	require.Contains(t, content, "quota")
	require.Contains(t, content, "100")
	require.Contains(t, content, "200")
}

// TestLogContentUsesExternalIdentifiersOnly pins that generated log content
// identifies users and channels by name + external UUID and never by the
// internal integer id.
func TestLogContentUsesExternalIdentifiersOnly(t *testing.T) {
	userUUID := "018f0000-0000-7000-8000-0000000000aa"
	channelUUID := "018f0000-0000-7000-8000-0000000000bb"

	cases := []*Log{
		{Type: LogTypeConsume, UserId: 7, UserUUID: &userUUID, Username: "alice", ChannelId: 3, ChannelUUID: &channelUUID, ChannelName: "primary", ModelName: "gpt-x"},
		{Type: LogTypeTopup, UserId: 7, ChannelId: 3, ChannelUUID: &channelUUID, ChannelName: "primary", Quota: 10},
		{Type: LogTypeTest, ChannelId: 3, ChannelUUID: &channelUUID, ChannelName: "primary", ModelName: "gpt-x"},
		{Type: LogTypeSystem, UserId: 7, UserUUID: &userUUID, Username: "alice", Quota: 5},
		{Type: LogTypeManage, UserId: 7, UserUUID: &userUUID, Username: "alice", RequestId: "req-1"},
		{Type: 99, UserId: 7, UserUUID: &userUUID, Username: "alice"},
	}
	for _, logEntry := range cases {
		ensureLogContent(logEntry)
		require.NotContains(t, logEntry.Content, "user_id=", "type %d: %s", logEntry.Type, logEntry.Content)
		require.NotContains(t, logEntry.Content, "channel_id=", "type %d: %s", logEntry.Type, logEntry.Content)
		// Consume/top-up/test rows carry the user in their own columns and
		// never embedded it in content; only the user-centric types do.
		userCentric := logEntry.Type == LogTypeSystem || logEntry.Type == LogTypeManage || logEntry.Type == 99
		if logEntry.UserUUID != nil && userCentric {
			require.Contains(t, logEntry.Content, "user=alice")
			require.Contains(t, logEntry.Content, "user_uuid="+userUUID)
		}
		if logEntry.ChannelUUID != nil {
			require.Contains(t, logEntry.Content, "channel=primary")
			require.Contains(t, logEntry.Content, "channel_uuid="+channelUUID)
		}
	}

	// Without a resolvable identity nothing internal leaks either.
	bare := &Log{Type: LogTypeConsume, UserId: 7, ChannelId: 3}
	ensureLogContent(bare)
	require.NotContains(t, bare.Content, "7")
	require.NotContains(t, bare.Content, "3")
}
