package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// assertLyriaBoundaryRejected exercises production decoding, conversion, admission
// and settlement with a loopback TLS provider and physical SQLite balances.
// Parameters: t owns the fixture; protocol, body and contentType describe the
// client request. Returns: none; mapped and identity-model requests must reject
// before provider work and leave both accounts and the consume ledger unchanged.
// Authentication is fixture context, not public router middleware coverage.
func assertLyriaBoundaryRejected(t *testing.T, protocol, body, contentType string) {
	t.Helper()
	for _, mapped := range []bool{false, true} {
		t.Run(fmt.Sprintf("mapped=%t", mapped), func(t *testing.T) {
			const balance = int64(100000)
			xaiVideoSetup(t, balance, false)
			previousEnforcement := config.EnforceIncludeUsage
			config.EnforceIncludeUsage = false
			t.Cleanup(func() { config.EnforceIncludeUsage = previousEnforcement })
			var calls atomic.Int32
			var wire atomic.Value
			wire.Store("")
			reply := lyriaChatReply(t)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				payload, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				wire.Store(string(payload))
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, reply); err != nil {
					return
				}
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })

			path, _ := lyriaProtocolRequest(protocol, false)
			requestBody := body
			if !mapped {
				requestBody = strings.Replace(body, `"model":"alias"`, `"model":"`+lyriaClipModel+`"`, 1)
			}
			c, _, id := protocolContext(t, channeltype.OpenRouter, lyriaClipModel, path, requestBody, upstream.URL+"/v1", balance, 1, false, nil)
			c.Request.Header.Set("Content-Type", contentType)
			c.Set(ctxkey.ContentType, contentType)
			if !mapped {
				c.Set(ctxkey.RequestModel, lyriaClipModel)
				c.Set(ctxkey.ModelMapping, map[string]string{})
			}
			apiErr := relayLyriaProtocol(c, protocol)
			drainCriticalTasks(t)
			require.Zero(t, calls.Load(), "request crossed the paid-generation boundary: error=%+v wire=%s", apiErr, wire.Load())
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, balance, reloadUserQuota(t), "rejection must not strand a user reservation")
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance, token.RemainQuota, "rejection must not strand a token reservation")
			require.Zero(t, token.UsedQuota)
			require.Zero(t, consumeLogQuota(t, id))
		})
	}
}

// TestSecurityLyriaCanonicalGenerationControls verifies explicitly unsupported
// count and tool controls across all supported client protocols. Parameters: t
// owns each isolated fixture. Returns: none; all cases reject before dispatch.
func TestSecurityLyriaCanonicalGenerationControls(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, fields := range []string{
			`"n":2`, `"n":0`, `"n":-1`, `"n":null`, `"n":"2"`, `"n":1.5`,
			`"candidate_count":2`, `"candidateCount":2`, `"num_outputs":2`, `"num_generations":2`,
			`"tool_choice":"auto"`, `"function_call":"auto"`,
			`"tools":[{"type":"function","function":{"name":"lookup"}}]`,
			`"functions":[{"name":"lookup"}]`,
			`"extra_body":{"n":2}`, `"extra_body":{"n":null}`,
			`"extra_body":{"candidateCount":2}`, `"extra_body":{"num_outputs":2}`,
			`"extra_body":{"tool_choice":"auto"}`,
			`"extra_body":{"tools":[{"type":"function","function":{"name":"lookup"}}]}`,
		} {
			t.Run(protocol+"/"+fields, func(t *testing.T) {
				_, body := lyriaProtocolRequest(protocol, false)
				body = strings.TrimSuffix(body, "}") + "," + fields + "}"
				assertLyriaBoundaryRejected(t, protocol, body, "application/json")
			})
		}
	}
}

// TestSecurityLyriaAmbiguousGenerationControls distinguishes the historical
// uppercase-N report from protocol-independent policy gaps in raw and nested
// controls. Parameters: t owns each fixture. Returns: none; duplicate keys are
// tested in both orders, including escaped names, before any lossy map decode.
func TestSecurityLyriaAmbiguousGenerationControls(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, fields := range []string{
			`"N":2`, `"\u004e":2`, `"n":1,"N":2`, `"N":2,"n":1`,
			`"n":2,"n":1`, `"n":1,"n":2`, `"n":2,"\u006e":1`, `"n":1,"n":1`,
			`"CandidateCount":2`, `"NUM_OUTPUTS":2`, `"Num_Generations":2`,
			`"TOOLS":[{"type":"function","function":{"name":"lookup"}}]`,
			`"extra_body":{"N":2}`, `"extra_body":{"n":2,"n":1}`,
			`"extra_body":{"n":1,"N":2}`, `"extra_body":{"n":2,"\u006e":1}`,
			`"extra_body":{"n":1,"n":1}`,
			`"extra_body":{"n":2},"extra_body":{}`,
			`"extra_body":{},"extra_body":{"n":2}`,
			`"Extra_Body":{"n":2}`, `"extra_body":{"Num_Outputs":2}`,
			`"extra_body":{"TOOLS":[{"type":"function","function":{"name":"lookup"}}]}`,
		} {
			t.Run(protocol+"/"+fields, func(t *testing.T) {
				_, body := lyriaProtocolRequest(protocol, false)
				body = strings.TrimSuffix(body, "}") + "," + fields + "}"
				assertLyriaBoundaryRejected(t, protocol, body, "application/json")
			})
		}
	}
}

// TestSecurityLyriaAmbiguousBodyFormat verifies a JSON object cannot acquire a
// different interpretation through its form Content-Type. Parameters: t owns
// each fixture. Returns: none; canonical and noncanonical media type casing
// produce the same pre-dispatch rejection for every client protocol.
func TestSecurityLyriaAmbiguousBodyFormat(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, contentType := range []string{"application/x-www-form-urlencoded", "Application/X-Www-Form-Urlencoded; charset=UTF-8"} {
			t.Run(protocol+"/"+contentType, func(t *testing.T) {
				_, body := lyriaProtocolRequest(protocol, false)
				body = strings.TrimSuffix(body, "}") + `,"n":1}`
				assertLyriaBoundaryRejected(t, protocol, body, contentType)
			})
		}
	}
}
