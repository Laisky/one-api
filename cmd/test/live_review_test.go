package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
)

// TestLiveReviewThinkingModelSelection checks scenario-aware required values.
// Parameters: t owns the test. Returns: none.
func TestLiveReviewThinkingModelSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{"default suite empty", []string{"--thinking-model", ""}, true},
		{"selected thinking blank", []string{"--scenarios", "thinking", "--thinking-model", " \t "}, true},
		{"mixed selection blank", []string{"--scenarios", "conversation, THINKING ,thinking", "--thinking-model", ""}, true},
		{"conversation only", []string{"--scenarios", "conversation", "--thinking-model", ""}, false},
		{"other scenarios", []string{"--scenarios", "setup-guard,subprotocol", "--thinking-model", " \t "}, false},
		{"trimmed model", []string{"--scenarios", "thinking", "--thinking-model", " " + defaultLiveThinkingModel + " "}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, err := parseLiveArgs(tc.args, config{APIBase: "http://127.0.0.1:3000", Token: "test-token"})
			if tc.wantError {
				require.ErrorContains(t, err, "thinking-model is required")
				return
			}
			require.NoError(t, err)
			if tc.name == "trimmed model" {
				require.Equal(t, defaultLiveThinkingModel, opts.thinkingModel)
			}
		})
	}
}

// TestLiveReviewPlaintextPolicy verifies that credential-bearing Live runs
// require TLS except for literal loopback addresses and localhost. Parameters:
// t owns the test. Returns: none; no external network requests are made.
func TestLiveReviewPlaintextPolicy(t *testing.T) {
	t.Parallel()
	for _, host := range []string{
		"example.test", "192.0.2.1", "10.0.0.1", "[2001:db8::1]", "0.0.0.0", "[::]",
		"localhost.example.test", "127.0.0.1.example.test",
	} {
		for _, scheme := range []string{"http", "ws"} {
			t.Run(scheme+"/"+host, func(t *testing.T) {
				t.Parallel()
				_, err := parseLiveArgs([]string{"--scenarios", "conversation"}, config{
					APIBase: scheme + "://" + host + ":3000", Token: "test-token",
				})
				require.ErrorContains(t, err, "HTTPS or WSS")
				require.NotContains(t, err.Error(), "test-token")
			})
		}
	}
	for _, base := range []string{
		"http://127.0.0.1:3000", "ws://127.0.0.2:3000", "http://[::1]:3000",
		"ws://[::ffff:127.0.0.1]:3000", "http://localhost:3000", "ws://LOCALHOST.:3000",
		"https://example.test", "wss://example.test", "https://10.0.0.1",
	} {
		t.Run("allowed/"+base, func(t *testing.T) {
			t.Parallel()
			_, err := parseLiveArgs([]string{"--api-base", base, "--scenarios", "conversation"}, config{
				APIBase: "https://unused.example.test", Token: "test-token",
			})
			require.NoError(t, err)
		})
	}
}

// TestLiveReviewWebSocketBasesServeHTTP exercises real REST and settlement
// requests for HTTP(S) and WS(S) CLI bases. Parameters: t owns the local server
// and client cleanup. Returns: none. This test is sequential because each TLS
// fixture installs its trusted client while parallel package tests are paused.
func TestLiveReviewWebSocketBasesServeHTTP(t *testing.T) {
	for _, scheme := range []string{"http", "https", "ws", "wss"} {
		t.Run(scheme, func(t *testing.T) {
			var restCalls, logCalls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/gateway/v1/chat/completions":
					restCalls.Add(1)
					if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token-42" {
						t.Error("REST request lost its method or pinned credentials")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"error":{"code":"unsupported_model_transport"}}`)
				case "/gateway/api/token/logs":
					logCalls.Add(1)
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Query().Get("model_name") != "" {
						t.Error("settlement request lost its method, credentials, or alias-independent lookup")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"success":true,"total":1,"data":[{"request_id":"review-session","prompt_tokens":1,"completion_tokens":2,"metadata":{"realtime_billing_complete":true,"realtime_usage":{"receipt_count":1}}}]}`)
				default:
					t.Errorf("unexpected request path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			if scheme == "https" || scheme == "wss" {
				server.StartTLS()
			} else {
				server.Start()
			}
			t.Cleanup(server.Close)
			previousClient := http.DefaultClient
			http.DefaultClient = server.Client()
			t.Cleanup(func() { http.DefaultClient = previousClient })
			base := scheme + "://" + strings.SplitN(server.URL, "://", 2)[1] + "/gateway/"
			opts, err := parseLiveArgs([]string{"--api-base", base, "--scenarios", "conversation,rest-guard", "--rest-channel", "42"}, config{Token: "test-token"})
			require.NoError(t, err)
			wsBase, err := resolveLiveWSEndpoint(opts.apiBase)
			require.NoError(t, err)
			require.Equal(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/gateway/v1/realtime", wsBase)
			logger, err := glog.NewConsoleWithName("live-review-http", glog.LevelInfo)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			t.Run("REST guard", func(t *testing.T) {
				require.NoError(t, runLiveRESTGuardScenario(ctx, logger, wsBase, opts))
				require.EqualValues(t, 1, restCalls.Load())
			})
			t.Run("settlement", func(t *testing.T) {
				require.NoError(t, verifyLiveSettlement(ctx, logger, opts, "review-session", []map[string]any{
					{"promptTokenCount": 1, "responseTokenCount": 2, "totalTokenCount": 3},
				}))
				require.EqualValues(t, 1, logCalls.Load())
			})
			require.Equal(t, "test-token", opts.apiToken)
		})
	}
}
