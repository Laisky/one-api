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
	sharedconfig "github.com/Laisky/one-api/common/config"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestRunLiveRESTGuardScenarioRequiresExactTransportError verifies that the
// probe cannot treat authentication, routing, or rate-limit failures as proof
// of the Live-only REST guard.
func TestRunLiveRESTGuardScenarioRequiresExactTransportError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   string
		wantOK bool
	}{
		{"exact error", http.StatusBadRequest, `{"error":{"code":"unsupported_model_transport"}}`, true},
		{"wrong code", http.StatusBadRequest, `{"error":{"code":"invalid_request"}}`, false},
		{"unauthorized", http.StatusUnauthorized, `{"error":{"code":"invalid_api_key"}}`, false},
		{"not found", http.StatusNotFound, `{"error":{"code":"not_found"}}`, false},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"code":"rate_limit"}}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			logger, err := glog.NewConsoleWithName("live-rest-guard", glog.LevelInfo)
			require.NoError(t, err)
			err = runLiveRESTGuardScenario(context.Background(), logger, "", liveOptions{
				apiBase: server.URL, apiToken: "test-token", model: defaultLiveModel,
				prompt: "test", timeout: time.Second,
			})
			if tt.wantOK {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

// TestReadLiveTurnCompletesRegardlessOfReceiptOrder verifies that a receipt
// emitted before or after turnComplete completes the same turn.
func TestReadLiveTurnCompletesRegardlessOfReceiptOrder(t *testing.T) {
	t.Parallel()

	for _, messages := range [][]string{
		{`{"serverContent":{"turnComplete":true}}`, `{"usageMetadata":{"totalTokenCount":2}}`},
		{`{"usageMetadata":{"totalTokenCount":2}}`, `{"serverContent":{"turnComplete":true}}`},
	} {
		messages := messages
		t.Run(strings.Join(messages, ","), func(t *testing.T) {
			t.Parallel()
			conn := liveTestConnection(t, func(conn *websocket.Conn) {
				for _, message := range messages {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
						t.Errorf("write frame: %v", err)
						return
					}
				}
			})
			turn, err := readLiveTurn(conn, time.Second)
			require.NoError(t, err)
			require.True(t, turn.turnComplete)
			require.NotNil(t, turn.usage)
		})
	}
}

// TestReadLiveTurnWaitsForExtendedThinkingIdle verifies that the probe waits
// through IN_PROGRESS until the terminal IDLE status arrives.
func TestReadLiveTurnWaitsForExtendedThinkingIdle(t *testing.T) {
	t.Parallel()
	conn := liveTestConnection(t, func(conn *websocket.Conn) {
		for _, message := range []string{
			`{"serverContent":{"interactionStatus":"IN_PROGRESS"}}`,
			`{"serverContent":{"turnComplete":true}}`,
			`{"usageMetadata":{"totalTokenCount":2}}`,
			`{"serverContent":{"interactionStatus":"IDLE"}}`,
		} {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
				t.Errorf("write frame: %v", err)
				return
			}
		}
	})
	turn, err := readLiveTurn(conn, time.Second)
	require.NoError(t, err)
	require.True(t, turn.turnComplete)
	require.True(t, turn.interactionIdle)
	require.Equal(t, 4, turn.frames)
}

// TestReadLiveTurnWaitsForTopLevelExtendedThinkingIdle verifies that Gemini's
// equivalent top-level interaction lifecycle also gates turn completion.
func TestReadLiveTurnWaitsForTopLevelExtendedThinkingIdle(t *testing.T) {
	t.Parallel()
	conn := liveTestConnection(t, func(conn *websocket.Conn) {
		for _, message := range []string{
			`{"interactionStatus":"IN_PROGRESS"}`,
			`{"serverContent":{"turnComplete":true}}`,
			`{"usageMetadata":{"totalTokenCount":2}}`,
			`{"interactionStatus":"IDLE"}`,
		} {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
				t.Errorf("write frame: %v", err)
				return
			}
		}
	})
	turn, err := readLiveTurn(conn, time.Second)
	require.NoError(t, err)
	require.True(t, turn.interactionIdle)
	require.Equal(t, 4, turn.frames)
}

// TestReadLiveTurnBoundsTranscript verifies that a provider cannot cause the
// diagnostic transcript preview to retain unbounded frame content.
func TestReadLiveTurnBoundsTranscript(t *testing.T) {
	t.Parallel()
	conn := liveTestConnection(t, func(conn *websocket.Conn) {
		message := fmt.Sprintf(`{"serverContent":{"outputTranscription":{"text":%q},"turnComplete":true},"usageMetadata":{"totalTokenCount":2}}`, strings.Repeat("x", maxLiveTranscriptBytes*2))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			t.Errorf("write frame: %v", err)
		}
	})
	turn, err := readLiveTurn(conn, time.Second)
	require.NoError(t, err)
	require.LessOrEqual(t, len(turn.transcript), maxLiveTranscriptBytes)
}

// TestParseLiveArgsDeduplicatesScenarios verifies a repeated selection cannot
// execute quota-consuming scenarios more than once.
func TestParseLiveArgsDeduplicatesScenarios(t *testing.T) {
	t.Parallel()
	opts, err := parseLiveArgs([]string{"--scenarios", "conversation,thinking,conversation"}, config{
		APIBase: "http://example.test", Token: "test-token",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"conversation", "thinking"}, opts.scenarios)
}

// TestLiveUsesFlagTokenWithoutGeneralHarnessConfiguration verifies that the
// live command reaches flag validation when API_TOKEN and unrelated run-suite
// environment configuration are absent or invalid.
func TestLiveUsesFlagTokenWithoutGeneralHarnessConfiguration(t *testing.T) {
	originalToken := sharedconfig.APIToken
	sharedconfig.APIToken = ""
	t.Cleanup(func() { sharedconfig.APIToken = originalToken })
	logger, err := glog.NewConsoleWithName("live-token-override", glog.LevelInfo)
	require.NoError(t, err)
	err = live(context.Background(), logger, []string{
		"--token", "override-token", "--scenarios", "not-a-scenario",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown scenario")
}

// TestVerifyLiveSettlementMatchesRequestID verifies that a concurrent consume
// log with the same model and receipt count cannot satisfy this session.
func TestVerifyLiveSettlementMatchesRequestID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("model_name"); got != "" {
			t.Errorf("Live settlement lookup unexpectedly filtered model %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[` +
			`{"request_id":"other-request","prompt_tokens":999,"completion_tokens":999,"content":"other","metadata":{"realtime_billing_complete":true,"realtime_usage":{"receipt_count":1}}},` +
			`{"request_id":"live-request","prompt_tokens":1,"completion_tokens":2,"content":"expected","metadata":{"realtime_billing_complete":true,"realtime_usage":{"receipt_count":1}}}` +
			`]}`))
	}))
	t.Cleanup(server.Close)
	logger, err := glog.NewConsoleWithName("live-settlement", glog.LevelInfo)
	require.NoError(t, err)
	err = verifyLiveSettlement(context.Background(), logger, liveOptions{
		apiBase: server.URL, apiToken: "test-token", model: defaultLiveModel,
	}, "live-request", []map[string]any{{
		"promptTokenCount": float64(1), "responseTokenCount": float64(2), "totalTokenCount": float64(3),
	}})
	require.NoError(t, err)
}

// TestVerifyLiveSettlementWaitsForReconciliation verifies that the probe does
// not mistake the matching provisional consume log for a failed settlement.
func TestVerifyLiveSettlementWaitsForReconciliation(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"success":true,"data":[{"request_id":"live-request","prompt_tokens":0,"completion_tokens":0,"content":"provisional","metadata":{"realtime_billing_complete":false,"realtime_usage":{"receipt_count":0}}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"request_id":"live-request","prompt_tokens":1,"completion_tokens":2,"content":"settled","metadata":{"realtime_billing_complete":true,"realtime_usage":{"receipt_count":1}}}]}`))
	}))
	t.Cleanup(server.Close)
	logger, err := glog.NewConsoleWithName("live-provisional-settlement", glog.LevelInfo)
	require.NoError(t, err)
	err = verifyLiveSettlement(context.Background(), logger, liveOptions{
		apiBase: server.URL, apiToken: "test-token", model: "mapped-alias",
	}, "live-request", []map[string]any{{
		"promptTokenCount": float64(1), "responseTokenCount": float64(2), "totalTokenCount": float64(3),
	}})
	require.NoError(t, err)
	require.GreaterOrEqual(t, calls.Load(), int32(2))
}

// TestVerifyLiveSettlementRejectsPartiallyReconciledLog verifies that a
// receipt-bearing but incomplete matching row is reported instead of polled.
func TestVerifyLiveSettlementRejectsPartiallyReconciledLog(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"request_id":"live-request","content":"partial","metadata":{"realtime_billing_complete":false,"realtime_usage":{"receipt_count":1}}}]}`))
	}))
	t.Cleanup(server.Close)
	logger, err := glog.NewConsoleWithName("live-partial-settlement", glog.LevelInfo)
	require.NoError(t, err)
	err = verifyLiveSettlement(context.Background(), logger, liveOptions{
		apiBase: server.URL, apiToken: "test-token",
	}, "live-request", []map[string]any{{
		"promptTokenCount": float64(1), "responseTokenCount": float64(2), "totalTokenCount": float64(3),
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "incomplete")
}

// TestDialLiveCapturesRequestID verifies the successful WebSocket handshake's
// request ID is preserved for the settlement lookup.
func TestDialLiveCapturesRequestID(t *testing.T) {
	t.Parallel()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, http.Header{"X-Oneapi-Request-Id": {"live-request"}})
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
	}))
	t.Cleanup(server.Close)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, requestID, err := dialLiveWithRequestID(context.Background(), wsURL, "test-token", nil, time.Second)
	require.NoError(t, err)
	require.Equal(t, "live-request", requestID)
	require.NoError(t, conn.Close())
}

// liveTestConnection creates a local WebSocket peer that writes a deterministic
// frame sequence. Parameters: t owns test cleanup and write emits server frames.
// Returns: the client connection under test.
func liveTestConnection(t *testing.T, write func(*websocket.Conn)) *websocket.Conn {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		write(conn)
	}))
	t.Cleanup(server.Close)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := dialLiveWithRequestID(context.Background(), wsURL, "", nil, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
