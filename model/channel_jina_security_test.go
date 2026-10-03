package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestJinaChannelConfigLoadSecurity validates persisted configurations, including
// a base URL with no serialized config and inactive endpoint overrides.
func TestJinaChannelConfigLoadSecurity(t *testing.T) {
	t.Setenv(channeltype.JinaAllowLoopbackHTTPEnv, "false")
	bad, good := "http://proxy.example", "https://api.jina.ai"
	for _, channel := range []*Channel{
		{Type: channeltype.Jina, BaseURL: &bad},
		{Type: channeltype.Jina, BaseURL: &good, Config: `{"endpoint_urls":{"rerank":"http://proxy.example"}}`},
		{Type: channeltype.Jina, Config: `{`},
	} {
		_, err := channel.LoadConfig()
		require.Error(t, err)
	}
	_, err := (&Channel{Type: channeltype.Jina, BaseURL: &good}).LoadConfig()
	require.NoError(t, err)
	_, err = (&Channel{Type: channeltype.OpenAI, BaseURL: &bad}).LoadConfig()
	require.NoError(t, err, "other providers keep their existing configuration policy")
	loopback := "http://127.0.0.1:8080"
	local := &Channel{Type: channeltype.Jina, BaseURL: &loopback}
	_, err = local.LoadConfig()
	require.Error(t, err)
	t.Setenv(channeltype.JinaAllowLoopbackHTTPEnv, "true")
	_, err = local.LoadConfig()
	require.NoError(t, err)
}

// TestJinaChannelSecurityTransactions exercises the same Insert, BatchInsert and
// partial Update methods used by channel administration. Invalid inherited URLs
// roll back all changes, including a type-only conversion into a Jina channel.
func TestJinaChannelSecurityTransactions(t *testing.T) {
	setupTestDatabase(t)
	t.Setenv(channeltype.JinaAllowLoopbackHTTPEnv, "false")
	bad, good := "http://proxy.example", "https://api.jina.ai"
	invalid := &Channel{Type: channeltype.Jina, Name: "test-jina-security-invalid", BaseURL: &bad, Models: "jina-embeddings-v3", Group: "default"}
	require.Error(t, invalid.Insert())
	var count int64
	require.NoError(t, DB.Model(&Channel{}).Where("name = ?", invalid.Name).Count(&count).Error)
	require.Zero(t, count)
	batch := []Channel{
		{Type: channeltype.Jina, Name: "test-jina-security-batch-ok", BaseURL: &good, Models: "jina-embeddings-v3", Group: "default"},
		{Type: channeltype.Jina, Name: "test-jina-security-batch-bad", BaseURL: &bad, Models: "jina-embeddings-v3", Group: "default"},
	}
	require.Error(t, BatchInsertChannels(batch))
	require.NoError(t, DB.Model(&Channel{}).Where("name IN ?", []string{batch[0].Name, batch[1].Name}).Count(&count).Error)
	require.Zero(t, count, "one invalid entry must prevent the entire batch")
	channel := &Channel{Type: channeltype.Jina, Name: "test-jina-security-update", BaseURL: &good, Models: "jina-embeddings-v3", Group: "default"}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() { require.NoError(t, channel.Delete()) })
	for _, update := range []*Channel{
		{Id: channel.Id, BaseURL: &bad, Name: "test-jina-security-must-rollback"},
		{Id: channel.Id, Config: `{"endpoint_urls":{"embeddings":"http://proxy.example"}}`},
	} {
		require.Error(t, update.UpdateWithContext(context.Background()))
		var stored Channel
		require.NoError(t, DB.First(&stored, channel.Id).Error)
		require.Equal(t, good, stored.GetBaseURL())
		require.Equal(t, channel.Name, stored.Name)
		require.Empty(t, stored.Config)
		require.Equal(t, channeltype.Jina, stored.Type)
	}
	legacy := &Channel{Type: channeltype.OpenAI, Name: "test-jina-security-legacy", BaseURL: &bad, Models: "jina-embeddings-v3", Group: "default"}
	require.NoError(t, legacy.Insert())
	t.Cleanup(func() { require.NoError(t, legacy.Delete()) })
	require.Error(t, (&Channel{Id: legacy.Id, Type: channeltype.Jina}).Update())
	var stored Channel
	require.NoError(t, DB.First(&stored, legacy.Id).Error)
	require.Equal(t, channeltype.OpenAI, stored.Type)
	require.Equal(t, bad, stored.GetBaseURL())
	fixed := &Channel{Id: legacy.Id, Type: channeltype.Jina, BaseURL: &good}
	require.NoError(t, fixed.Update())
	require.Equal(t, good, fixed.GetBaseURL())
	require.Equal(t, channeltype.Jina, fixed.Type)
}
