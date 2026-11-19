package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// adaptiveCooldownChannelID is the channel ambiguousKeysRouter creates.
const adaptiveCooldownChannelID = 929293

// adaptiveCooldownRow reads the persisted cooldown state of the fixture's only
// ability for modelName. It fails the test if the row does not exist.
func adaptiveCooldownRow(t *testing.T, modelName string) model.Ability {
	t.Helper()
	var row model.Ability
	require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", adaptiveCooldownChannelID, modelName).Take(&row).Error)
	return row
}

// adaptiveCooldownRung returns the adaptive deadline minus the latest failure,
// which is the ladder rung the last persisted failure selected.
func adaptiveCooldownRung(t *testing.T, row model.Ability) time.Duration {
	t.Helper()
	require.NotNil(t, row.AdaptiveSuspendUntil, "expected an adaptive deadline")
	return row.AdaptiveSuspendUntil.Sub(time.Unix(0, row.CooldownLastFailure).UTC())
}

// unsetEnvForTest removes legacy cooldown overrides for one test, because an
// override that is merely present (even empty) selects the fixed legacy duration.
// The original values are restored when the test ends.
func unsetEnvForTest(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		t.Setenv(name, "") // registers restoration of the original value
		require.NoError(t, os.Unsetenv(name))
	}
}

// postRouterJSON sends one authenticated JSON request through a live listener for
// the public router and drains detached processing before returning the status.
func postRouterJSON(t *testing.T, serverURL, key, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, serverURL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer sk-"+key)
	req.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	drainCriticalTasksForRouter(t)
	return resp.StatusCode, string(raw)
}

// TestGeminiSpeechProviderFailureUsesAdaptiveCooldown sends real public
// /v1/audio/speech requests to a Gemini channel whose provider answers with an
// HTTP error. A provider 429 or 5xx is channel evidence and must advance the
// adaptive ladder (first rung, 10s) instead of taking the legacy fixed fallback
// (429) or being skipped as a one-api internal error (5xx). A provider 400 is
// the control: it must not suspend anything.
func TestGeminiSpeechProviderFailureUsesAdaptiveCooldown(t *testing.T) {
	const speechModel = "gemini-3.8-flash-tts"
	for _, tc := range []struct {
		status   int
		cooldown bool
	}{
		{status: http.StatusTooManyRequests, cooldown: true},
		{status: http.StatusServiceUnavailable, cooldown: true},
		{status: http.StatusInternalServerError, cooldown: true},
		{status: http.StatusBadRequest, cooldown: false},
	} {
		t.Run(fmt.Sprintf("provider_%d", tc.status), func(t *testing.T) {
			var calls atomic.Int32
			var wrongPath atomic.Value
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if !strings.HasSuffix(r.URL.Path, "/models/"+speechModel+":generateContent") {
					wrongPath.Store(r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"code":`+fmt.Sprint(tc.status)+`,"message":"provider-private-detail","status":"UNAVAILABLE"}}`)
			}))
			t.Cleanup(provider.Close)
			engine, key := ambiguousKeysRouter(t, provider.URL+"/prefix", channeltype.Gemini, speechModel, false)
			previousClient, previousRetries := client.HTTPClient, config.RetryTimes
			client.HTTPClient = provider.Client()
			config.RetryTimes = 0
			t.Cleanup(func() { client.HTTPClient, config.RetryTimes = previousClient, previousRetries })
			unsetEnvForTest(t, "CHANNEL_SUSPEND_SECONDS_FOR_429", "CHANNEL_SUSPEND_SECONDS_FOR_5XX")
			server := httptest.NewServer(engine)
			t.Cleanup(server.Close)

			before := time.Now().UTC()
			status, body := postRouterJSON(t, server.URL, key, "/v1/audio/speech",
				`{"model":"`+speechModel+`","input":"Exact transcript.","voice":"Kore","response_format":"pcm"}`)
			after := time.Now().UTC()

			require.EqualValues(t, 1, calls.Load(), "the provider must be reached exactly once: %s", body)
			require.Nil(t, wrongPath.Load(), "unexpected provider path")
			require.Equal(t, tc.status, status, body)
			require.NotContains(t, body, "provider-private-detail")
			row := adaptiveCooldownRow(t, speechModel)
			require.Nil(t, row.SuspendUntil, "a provider failure must not take the legacy fixed suspension")
			if !tc.cooldown {
				require.Zero(t, row.CooldownFailures)
				require.Nil(t, row.AdaptiveSuspendUntil)
				return
			}
			require.Equal(t, 1, row.CooldownFailures, "the provider failure must advance the adaptive ladder")
			require.Equal(t, 10*time.Second, adaptiveCooldownRung(t, row))
			lastFailure := time.Unix(0, row.CooldownLastFailure).UTC()
			require.False(t, lastFailure.Before(before) || lastFailure.After(after), "failure time must be the attempt completion")
		})
	}
}

// updateAdaptiveCooldownChannel saves one edit of the fixture channel through
// the shipped model update path (the one the admin API uses).
func updateAdaptiveCooldownChannel(t *testing.T, edit func(ch *model.Channel)) {
	t.Helper()
	channel, err := model.GetChannelById(adaptiveCooldownChannelID, true)
	require.NoError(t, err)
	edit(channel)
	require.NoError(t, channel.Update())
}

// cycleAdaptiveCooldownChannelStatus disables and re-enables the fixture channel
// through the given status writer pair, as an administrator or the automatic
// disable/probe re-enable flow would.
func cycleAdaptiveCooldownChannelStatus(t *testing.T, automatic bool) {
	t.Helper()
	ctx := context.Background()
	if !automatic {
		require.NoError(t, model.SetChannelStatusWithContext(ctx, adaptiveCooldownChannelID, model.ChannelStatusManuallyDisabled))
		require.NoError(t, model.SetChannelStatusWithContext(ctx, adaptiveCooldownChannelID, model.ChannelStatusEnabled))
		return
	}
	change, err := model.TransitionChannelStatusWithContext(ctx, adaptiveCooldownChannelID, model.ChannelStatusAutoDisabled, model.ChannelStatusEnabled)
	require.NoError(t, err)
	require.True(t, change.Changed)
	change, err = model.TransitionChannelStatusWithContext(ctx, adaptiveCooldownChannelID, model.ChannelStatusEnabled, model.ChannelStatusAutoDisabled)
	require.NoError(t, err)
	require.True(t, change.Changed)
}

// TestAdaptiveCooldownRestartsAfterUpstreamReplacementPublicRouter drives an
// ability to the capped rung with real public requests, applies a channel change
// through the shipped model write paths, and sends the next request. Replacing
// the upstream credentials or endpoint, or re-enabling a disabled channel (an
// operator repair or a successful automatic probe), must restart the ladder at
// 10s; an edit that leaves the upstream and status unchanged (the control) must
// keep the capped streak.
func TestAdaptiveCooldownRestartsAfterUpstreamReplacementPublicRouter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		apply    func(t *testing.T, base string)
		wantRung time.Duration
	}{
		{name: "rotate_key", apply: func(t *testing.T, _ string) {
			updateAdaptiveCooldownChannel(t, func(ch *model.Channel) { ch.Key = "rotated-fixture-key" })
		}, wantRung: 10 * time.Second},
		{name: "repoint_base_url", apply: func(t *testing.T, base string) {
			updateAdaptiveCooldownChannel(t, func(ch *model.Channel) { repointed := base + "/v1"; ch.BaseURL = &repointed })
		}, wantRung: 10 * time.Second},
		{name: "admin_reenable", apply: func(t *testing.T, _ string) { cycleAdaptiveCooldownChannelStatus(t, false) }, wantRung: 10 * time.Second},
		{name: "automatic_reenable", apply: func(t *testing.T, _ string) { cycleAdaptiveCooldownChannelStatus(t, true) }, wantRung: 10 * time.Second},
		{name: "rename_only_control", apply: func(t *testing.T, _ string) {
			updateAdaptiveCooldownChannel(t, func(ch *model.Channel) { ch.Name = "renamed-fixture" })
		}, wantRung: 10 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, quotaFlatBody)
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
			send := func() {
				t.Helper()
				status, body := postRouterJSON(t, server.URL, key, "/v1/chat/completions",
					`{"model":"`+passthroughKeysAllowedModel+`","messages":[{"role":"user","content":"hi"}]}`)
				require.Equal(t, http.StatusTooManyRequests, status, body)
			}
			// Let the selector route to the ability again without waiting out its
			// deadline; only the deadline moves, the persisted streak is kept.
			expire := func() {
				t.Helper()
				require.NoError(t, model.DB.Model(&model.Ability{}).
					Where("channel_id = ? AND model = ?", adaptiveCooldownChannelID, passthroughKeysAllowedModel).
					Update("adaptive_suspend_until", time.Now().UTC().Add(-time.Second)).Error)
			}

			for i, rung := range []time.Duration{10 * time.Second, 30 * time.Second, time.Minute, 3 * time.Minute, 10 * time.Minute} {
				if i > 0 {
					expire()
				}
				send()
				row := adaptiveCooldownRow(t, passthroughKeysAllowedModel)
				require.Equal(t, i+1, row.CooldownFailures)
				require.Equal(t, rung, adaptiveCooldownRung(t, row))
			}
			require.EqualValues(t, 5, calls.Load())

			tc.apply(t, provider.URL)
			if tc.wantRung == 10*time.Minute {
				expire() // An unchanged upstream keeps its deadline; reopen it for the probe.
			}
			send()
			require.EqualValues(t, 6, calls.Load())
			require.Equal(t, tc.wantRung, adaptiveCooldownRung(t, adaptiveCooldownRow(t, passthroughKeysAllowedModel)))
		})
	}
}

// TestAdaptiveCooldownConcurrentBurstIsOneRungPublicRouter sends a burst of
// concurrent public requests that are all in flight when the provider answers
// 429. They describe one rate-limit episode, so the ability must cool for the
// first rung (10s), not climb the whole ladder to the 10-minute cap within
// milliseconds (the legacy fixed 429 suspension was 60s). A later request sent
// after that cooldown expired is a genuine failed probe and must escalate to the
// second rung, proving the burst rule does not stop real escalation.
func TestAdaptiveCooldownConcurrentBurstIsOneRungPublicRouter(t *testing.T) {
	const burst = 6
	var calls atomic.Int32
	arrived := make(chan struct{}, burst)
	release := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= burst {
			arrived <- struct{}{}
			<-release // Answer only once every burst request is in flight.
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, quotaFlatBody)
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

	statuses := make(chan int, burst)
	for range burst {
		go func() {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(body))
			if err != nil {
				statuses <- -1
				return
			}
			req.Header.Set("Authorization", "Bearer sk-"+key)
			req.Header.Set("Content-Type", "application/json")
			resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
			if err != nil {
				statuses <- -1
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	for range burst {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			t.Fatal("burst requests did not all reach the provider")
		}
	}
	close(release)
	for range burst {
		require.Equal(t, http.StatusTooManyRequests, <-statuses)
	}
	drainCriticalTasksForRouter(t)
	row := adaptiveCooldownRow(t, passthroughKeysAllowedModel)
	require.Equal(t, 1, row.CooldownFailures, "one concurrent burst is one rate-limit episode")
	require.Equal(t, 10*time.Second, adaptiveCooldownRung(t, row))

	// The cooldown ends; the next request is a probe dispatched after it.
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ? AND model = ?", adaptiveCooldownChannelID, passthroughKeysAllowedModel).
		Update("adaptive_suspend_until", time.Now().UTC().Add(-time.Millisecond)).Error)
	status, raw := postRouterJSON(t, server.URL, key, "/v1/chat/completions", body)
	require.Equal(t, http.StatusTooManyRequests, status, raw)
	require.EqualValues(t, burst+1, calls.Load())
	row = adaptiveCooldownRow(t, passthroughKeysAllowedModel)
	require.Equal(t, 2, row.CooldownFailures, "a failed probe after expiry escalates")
	require.Equal(t, 30*time.Second, adaptiveCooldownRung(t, row))
}
