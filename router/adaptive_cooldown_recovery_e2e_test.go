package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// countAbilityWrites registers a GORM callback on the fixture database that
// counts executed UPDATE statements against the abilities table, which is how a
// relay success persists its cooldown recovery. It returns the live counter.
func countAbilityWrites(t *testing.T) *atomic.Int64 {
	t.Helper()
	var writes atomic.Int64
	require.NoError(t, model.DB.Callback().Update().After("gorm:update").Register("count_ability_writes", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			writes.Add(1)
		}
	}))
	return &writes
}

// TestAdaptiveCooldownSuccessWritesAreCoalescedPublicRouter sends a burst of
// successful public requests to one healthy ability. A success must not cost a
// database write on the shared ability row each time (one hot row serializes
// every success of a popular model and adds lock wait to responses); writes are
// bounded per ability per process. A failure recorded by this process re-arms
// recovery, so the next success still resets the streak at once.
func TestAdaptiveCooldownSuccessWritesAreCoalescedPublicRouter(t *testing.T) {
	var failing atomic.Bool
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if failing.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, quotaFlatBody)
			return
		}
		_, _ = io.WriteString(w, `{"id":"chatcmpl-ok","object":"chat.completion","created":1767225600,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`)
	}))
	t.Cleanup(provider.Close)
	engine, key := ambiguousKeysRouter(t, provider.URL, channeltype.OpenAICompatible, passthroughKeysAllowedModel, true)
	previousClient, previousRetries := client.HTTPClient, config.RetryTimes
	client.HTTPClient = provider.Client()
	config.RetryTimes = 0
	t.Cleanup(func() { client.HTTPClient, config.RetryTimes = previousClient, previousRetries })
	unsetEnvForTest(t, "CHANNEL_SUSPEND_SECONDS_FOR_429", "CHANNEL_SUSPEND_SECONDS_FOR_5XX")
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	body := `{"model":"` + passthroughKeysAllowedModel + `","messages":[{"role":"user","content":"hi"}]}`
	writes := countAbilityWrites(t)

	const successes = 8
	for range successes {
		status, raw := postRouterJSON(t, server.URL, key, "/v1/chat/completions", body)
		require.Equal(t, http.StatusOK, status, raw)
	}
	require.EqualValues(t, successes, calls.Load())
	require.LessOrEqual(t, writes.Load(), int64(1), "successes within one flush interval must share one durable write")

	// A failure recorded here re-arms recovery for the next success.
	failing.Store(true)
	status, raw := postRouterJSON(t, server.URL, key, "/v1/chat/completions", body)
	require.Equal(t, http.StatusTooManyRequests, status, raw)
	require.Equal(t, 1, adaptiveCooldownRow(t, passthroughKeysAllowedModel).CooldownFailures)
	failing.Store(false)
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ? AND model = ?", adaptiveCooldownChannelID, passthroughKeysAllowedModel).
		Update("adaptive_suspend_until", time.Now().UTC().Add(-time.Millisecond)).Error)
	status, raw = postRouterJSON(t, server.URL, key, "/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, status, raw)
	row := adaptiveCooldownRow(t, passthroughKeysAllowedModel)
	require.Zero(t, row.CooldownFailures, "the first success after a local failure must reset the streak immediately")
	require.Nil(t, row.AdaptiveSuspendUntil)
}
