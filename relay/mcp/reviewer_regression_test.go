package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestCallToolLatestNormalizesNilArguments verifies zero-argument calls transmit an empty object instead of null.
//
// Parameters:
//   - t: The test owns the mock MCP server and assertions.
//
// Return values: none; failures are reported through t.
func TestCallToolLatestNormalizesNilArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			ID     any `json:"id"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
		require.NotNil(t, envelope.Params.Arguments)
		require.Empty(t, envelope.Params.Arguments)
		writer.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result":  map[string]any{"resultType": ResultTypeComplete, "content": []any{}},
		}))
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	result, err := client.CallToolLatestWithDescriptor(context.Background(), ToolDescriptor{
		Name:        "health",
		InputSchema: map[string]any{"type": "object", "additionalProperties": false},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, ResultTypeComplete, result.ResultType)
}

// TestModernMCPClientRejectsUncorrelatedSSE verifies an unrelated SSE response cannot satisfy the active request.
//
// Parameters:
//   - t: The test owns the mock MCP server and correlation assertion.
//
// Return values: none; failures are reported through t.
func TestModernMCPClientRejectsUncorrelatedSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, err := fmt.Fprint(writer, "data: {\"jsonrpc\":\"2.0\",\"id\":\"other-request\",\"result\":{\"resultType\":\"complete\"}}\n\n")
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	_, err := client.DiscoverLatest(context.Background())
	require.ErrorContains(t, err, "no event for request id")
}

// TestModernMCPClientHeaderSnapshotIsRaceSafe verifies internal header updates cannot race request construction.
//
// Parameters:
//   - t: The test owns concurrent requests, header mutations, and assertions.
//
// Return values: none; failures are reported through t.
func TestModernMCPClientHeaderSnapshotIsRaceSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var envelope struct {
			ID any `json:"id"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
		writer.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"resultType":        ResultTypeComplete,
				"supportedVersions": []string{ProtocolVersion},
				"capabilities":      map[string]any{},
			},
		}))
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	const operations = 64
	start := make(chan struct{})
	errorsChannel := make(chan error, operations)
	var group sync.WaitGroup
	group.Add(operations + 1)
	go func() {
		defer group.Done()
		<-start
		for index := 0; index < operations*4; index++ {
			client.setClientHeader("X-Regression-Header", fmt.Sprintf("value-%d", index))
		}
	}()
	for index := 0; index < operations; index++ {
		go func() {
			defer group.Done()
			<-start
			_, err := client.DiscoverLatest(context.Background())
			errorsChannel <- err
		}()
	}
	close(start)
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		require.NoError(t, err)
	}
}

// TestMCPClientRedirectPolicyPreventsCredentialLeakage verifies credentials never cross an origin or HTTPS downgrade.
//
// Parameters:
//   - t: The test owns redirect targets and transport policy assertions.
//
// Return values: none; failures are reported through t.
func TestMCPClientRedirectPolicyPreventsCredentialLeakage(t *testing.T) {
	t.Run("cross-origin API key redirect is blocked before target delivery", func(t *testing.T) {
		var targetHits atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			targetHits.Add(1)
			writer.WriteHeader(http.StatusNoContent)
		}))
		defer target.Close()

		source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
		}))
		defer source.Close()

		client := NewStreamableHTTPClient(
			&model.MCPServer{BaseURL: source.URL},
			map[string]string{"X-API-Key": "secret"},
			5*time.Second,
		)
		_, err := client.DiscoverLatest(context.Background())
		require.ErrorContains(t, err, "preserve the endpoint origin")
		require.Zero(t, targetHits.Load())
	})

	t.Run("HTTPS downgrade is blocked", func(t *testing.T) {
		client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: "https://example.com/mcp"}, nil, time.Second)
		initial := &http.Request{URL: mustMCPReviewerURL(t, "https://example.com/mcp")}
		next := &http.Request{URL: mustMCPReviewerURL(t, "http://example.com/mcp")}
		require.ErrorContains(t, client.httpClient().CheckRedirect(next, []*http.Request{initial}), "downgrade")
	})
}

// TestMCPClientKeepsExplicitCredentialedHTTPCompatibility verifies direct operator-configured HTTP endpoints remain supported.
//
// Parameters:
//   - t: The test owns the explicit HTTP endpoint and credential assertion.
//
// Return values: none; failures are reported through t.
func TestMCPClientKeepsExplicitCredentialedHTTPCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "secret", request.Header.Get("X-API-Key"))
		var envelope struct {
			ID any `json:"id"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
		writer.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"resultType":        ResultTypeComplete,
				"supportedVersions": []string{ProtocolVersion},
				"capabilities":      map[string]any{},
			},
		}))
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, map[string]string{"X-API-Key": "secret"}, 5*time.Second)
	_, err := client.DiscoverLatest(context.Background())
	require.NoError(t, err)
}

// mustMCPReviewerURL parses a fixed URL fixture and reports impossible errors through the test.
//
// Parameters:
//   - t: The test receives URL parsing failures.
//   - raw: The fixed absolute URL is parsed for redirect construction.
//
// Return values:
//   - *url.URL: The parsed URL is returned.
func mustMCPReviewerURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}
