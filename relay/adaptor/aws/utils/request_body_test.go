package utils

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/require"
)

// requestBodyTestClient preserves an exact configured client instance for tests.
type requestBodyTestClient func(*http.Request) (*http.Response, error)

// Do dispatches through the installed test callback.
func (client requestBodyTestClient) Do(request *http.Request) (*http.Response, error) {
	return client(request)
}

// requestBodyReadOnly deliberately implements no optional WriterTo method.
type requestBodyReadOnly struct{ io.ReadCloser }

// requestBodyTracked verifies that the SDK's original body still owns cleanup.
type requestBodyTracked struct {
	*bytes.Reader
	closes int
}

// Close records cleanup without manufacturing an I/O error.
func (body *requestBodyTracked) Close() error { body.closes++; return nil }

// TestPreserveRequestBodyEOFContracts verifies that only the unsafe optional
// request-body fast path is hidden; response errors and transport ownership stay intact.
func TestPreserveRequestBodyEOFContracts(t *testing.T) {
	for _, kind := range []string{"writer_to", "reader_only", "nil", "no_body"} {
		t.Run(kind, func(t *testing.T) {
			sentinel := errors.New("synthetic transport failure")
			response := &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("synthetic"))}
			request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.invalid/local-only", nil)
			require.NoError(t, err)
			tracked := &requestBodyTracked{Reader: bytes.NewReader([]byte("unchanged synthetic payload"))}
			switch kind {
			case "writer_to":
				request.Body = tracked
			case "reader_only":
				request.Body = requestBodyReadOnly{ReadCloser: tracked}
			case "no_body":
				request.Body = http.NoBody
			}
			original := request.Body
			calls := 0
			options := bedrockruntime.Options{HTTPClient: requestBodyTestClient(func(got *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, request.Context(), got.Context())
				require.Equal(t, request.URL, got.URL)
				require.Equal(t, request.Header, got.Header)
				require.Equal(t, request.ContentLength, got.ContentLength)
				if kind == "writer_to" {
					require.NotSame(t, request, got, "the caller's request must not be mutated")
					_, fast := got.Body.(io.WriterTo)
					require.False(t, fast)
				} else {
					require.Same(t, request, got, "unaffected bodies keep the existing request path")
				}
				if kind == "writer_to" || kind == "reader_only" {
					data, readErr := io.ReadAll(got.Body)
					require.NoError(t, readErr)
					require.Equal(t, "unchanged synthetic payload", string(data))
					require.NoError(t, got.Body.Close())
				}
				return response, sentinel
			})}
			PreserveRequestBodyEOF(&options)
			first := options.HTTPClient
			PreserveRequestBodyEOF(&options)
			guard, ok := options.HTTPClient.(requestBodyReadClient)
			require.True(t, ok)
			_, nested := guard.next.(requestBodyReadClient)
			require.False(t, nested, "repeated application must not nest wrappers")
			require.IsType(t, first, options.HTTPClient)
			gotResponse, gotErr := options.HTTPClient.Do(request)
			require.Same(t, response, gotResponse)
			require.ErrorIs(t, gotErr, sentinel)
			require.Equal(t, 1, calls)
			require.Equal(t, original, request.Body, "SDK cleanup keeps the original body owner")
			if kind == "writer_to" || kind == "reader_only" {
				require.Equal(t, 1, tracked.closes)
			}
		})
	}
	var options bedrockruntime.Options
	require.NotPanics(t, func() { PreserveRequestBodyEOF(&options) })
	require.Nil(t, options.HTTPClient, "default resolution remains the SDK's responsibility")
}
