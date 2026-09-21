package adaptor

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
)

// urlDiagnosticScenario defines independent expected transport and log values.
type urlDiagnosticScenario struct {
	name, query, wantQuery, body string
	nilBody, unknownSize         bool
	secrets                      []string
}

// urlDiagnosticResponse defines the bytes, status and headers returned upstream.
type urlDiagnosticResponse struct {
	name, contentType, body string
	status                  int
}

// TestDoRequestHelperRedactsQueryCredentialsWithoutChangingDispatch exercises
// real HTTP requests and all emitted URL fields. The test parameter owns the
// fixtures; the function returns no value. Expected URLs do not use the
// production sanitizer, and missing diagnostic messages are test failures.
func TestDoRequestHelperRedactsQueryCredentialsWithoutChangingDispatch(t *testing.T) {
	// Prove that privacy and useful diagnostics do not depend on enabling OTel
	// or SQL tracing. These process globals are restored and never used in a
	// parallel test, matching the existing adaptor test isolation policy.
	oldSinks, oldClient := config.TraceSinks, client.HTTPClient
	config.TraceSinks = []string{config.TraceSinkNone}
	client.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() {
		config.TraceSinks, client.HTTPClient = oldSinks, oldClient
	})

	const payload = `{"query":"fixture-private-prompt","documents":["fixture-private-document"]}`
	allCredentials := "access_token=fixture-access-secret&api_key=fixture-api-secret&key=fixture-key-secret&password=fixture-password-secret&secret=fixture-secret-value&token=fixture-token-secret&turnstile=fixture-turnstile-secret&api-version=2026-01-01"
	redactedCredentials := url.Values{
		"access_token": {"[redacted]"}, "api_key": {"[redacted]"},
		"key": {"[redacted]"}, "password": {"[redacted]"},
		"secret": {"[redacted]"}, "token": {"[redacted]"},
		"turnstile": {"[redacted]"}, "api-version": {"2026-01-01"},
	}.Encode()
	mixedCredentials := "API_KEY=fixture-case-secret&%6Bey=fixture-encoded-secret&token=fixture-first-secret&token=fixture-second-secret&api_key=&safe=a%2Fb"
	redactedMixed := url.Values{
		"API_KEY": {"[redacted]"}, "key": {"[redacted]"},
		"token": {"[redacted]"}, "api_key": {"[redacted]"}, "safe": {"a/b"},
	}.Encode()

	cases := []urlDiagnosticScenario{
		{name: "no_query_nil_body", nilBody: true},
		{name: "safe_query_empty_body", query: "z=a%2Fb&z=two&a=", wantQuery: "z=a%2Fb&z=two&a="},
		{
			name: "every_supported_credential", query: allCredentials,
			wantQuery: redactedCredentials, body: payload,
			secrets: []string{"fixture-access-secret", "fixture-api-secret", "fixture-key-secret", "fixture-password-secret", "fixture-secret-value", "fixture-token-secret", "fixture-turnstile-secret"},
		},
		{
			name: "encoded_case_repeated_empty_keys", query: mixedCredentials,
			wantQuery: redactedMixed, body: payload, unknownSize: true,
			secrets: []string{"fixture-case-secret", "fixture-encoded-secret", "fixture-first-secret", "fixture-second-secret"},
		},
		{
			name:      "similar_names_are_not_credentials",
			query:     "monkey=keep&token_count=5&tokenizer=keep&api-version=2026-01-01",
			wantQuery: "monkey=keep&token_count=5&tokenizer=keep&api-version=2026-01-01", body: payload,
		},
	}
	responses := []urlDiagnosticResponse{
		{"json", "application/json", `{"results":[{"index":0,"score":0.9}]}`, http.StatusOK},
		{"stream", "text/event-stream", "data: {\"part\":1}\n\ndata: [DONE]\n\n", http.StatusOK},
		{"rate_limited", "application/json", `{"error":"rate limited"}`, http.StatusTooManyRequests},
		{"unavailable", "application/json", `{"error":"unavailable"}`, http.StatusServiceUnavailable},
	}

	for _, override := range []bool{false, true} {
		for _, response := range responses {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("override_%t/%s/%s", override, response.name, tc.name), func(t *testing.T) {
					runURLDiagnosticScenario(t, tc, response, override)
				})
			}
		}
	}
}
