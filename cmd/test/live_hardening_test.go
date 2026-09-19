package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
)

// TestLiveRESTGuardUsesExplicitChannel verifies deterministic negative probing
// without changing the credentials used by Live itself. Parameters: t owns the
// test. Returns: none.
func TestLiveRESTGuardUsesExplicitChannel(t *testing.T) {
	t.Parallel()
	cfg := config{APIBase: "http://localhost:3000", Token: "sk-example"}
	opts, err := parseLiveArgs(nil, cfg)
	require.NoError(t, err)
	require.NotContains(t, opts.scenarios, "rest-guard")
	_, err = parseLiveArgs([]string{"--scenarios", "rest-guard"}, cfg)
	require.ErrorContains(t, err, "--rest-channel")
	opts, err = parseLiveArgs([]string{"--scenarios", "rest-guard", "--rest-channel", "42"}, cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"rest-guard"}, opts.scenarios)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-example-42" {
			t.Errorf("REST probe did not pin the requested channel")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"code":"unsupported_model_transport"}}`)
	}))
	t.Cleanup(server.Close)
	opts.apiBase = server.URL
	logger, err := glog.NewConsoleWithName("live-pin", glog.LevelInfo)
	require.NoError(t, err)
	require.NoError(t, runLiveRESTGuardScenario(context.Background(), logger, "", opts))
	require.Equal(t, "sk-example", opts.apiToken)
}

// TestLiveSettlementSearchFindsOlderConcurrentSession verifies that unrelated
// newer traffic cannot hide a matching request on a later page. Parameters:
// t owns the test. Returns: none.
func TestLiveSettlementSearchFindsOlderConcurrentSession(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("model_name") != "" {
			t.Errorf("alias-sensitive model filter was sent")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("p") == "0" {
			_, _ = fmt.Fprint(w, `{"success":true,"total":21,"data":[{"request_id":"unrelated"}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"success":true,"total":21,"data":[{"request_id":"target","prompt_tokens":1,"completion_tokens":2,"metadata":{"realtime_billing_complete":true,"realtime_usage":{"receipt_count":1}}}]}`)
	}))
	t.Cleanup(server.Close)
	logger, err := glog.NewConsoleWithName("live-pages", glog.LevelInfo)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, verifyLiveSettlement(ctx, logger, liveOptions{apiBase: server.URL, apiToken: "sk-example"}, "target",
		[]map[string]any{{"promptTokenCount": 1, "responseTokenCount": 2, "totalTokenCount": 3}}))
	require.EqualValues(t, 2, calls.Load())
}

// TestExpectedLiveTokensReturnsValidationErrors verifies fail-closed oracle
// behavior instead of accepting unusable counters as zero. Parameters: t owns
// the test. Returns: none.
func TestExpectedLiveTokensReturnsValidationErrors(t *testing.T) {
	t.Parallel()
	_, _, err := expectedLiveTokens([]map[string]any{{"promptTokenCount": 100, "responseTokenCount": -1, "totalTokenCount": 99}})
	require.Error(t, err)
}
