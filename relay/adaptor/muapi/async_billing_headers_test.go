package muapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestAsyncBillingResponseHeadersPreserveCharge verifies actual HTTP submit/poll
// responses retain the largest documented charge independently of body decoding.
func TestAsyncBillingResponseHeadersPreserveCharge(t *testing.T) {
	cases := []struct {
		name    string
		headers []string
		cost    string
		want    string
		badBody bool
		wantErr bool
	}{
		{name: "header_only", headers: []string{"0.800000"}, want: "0.800000"},
		{name: "header_exceeds_body", headers: []string{"0.800000"}, cost: `,"cost":{"amount_usd":0.4}`, want: "0.800000"},
		{name: "body_exceeds_header", headers: []string{"0.400000"}, cost: `,"cost":{"amount_usd":0.8}`, want: "0.8"},
		{name: "multiple_headers", headers: []string{"0.8", "1.2"}, want: "1.2"},
		{name: "malformed_json", headers: []string{"0.8"}, want: "0.8", badBody: true, wantErr: true},
		{name: "bad_body_cost", headers: []string{"0.8"}, cost: `,"cost":{"amount_usd":"invalid"}`, want: "0.8", wantErr: true},
		{name: "bad_header_cost", headers: []string{"NaN"}, cost: `,"cost":{"amount_usd":0.8}`, want: "0.8", wantErr: true},
		{name: "oversize_header", headers: []string{strings.Repeat("1", 129)}, wantErr: true},
		{name: "negative_header", headers: []string{"-1"}, wantErr: true},
		{name: "exponent_header", headers: []string{"1e999999999"}, wantErr: true},
	}
	for _, operation := range []string{"submit", "poll"} {
		for _, tc := range cases {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					for _, value := range tc.headers {
						w.Header().Add("X-MuAPI-Cost-USD", value)
					}
					body := `{"request_id":"header-job"` + tc.cost + `}`
					if operation == "poll" {
						body = `{"id":"header-job","status":"completed","outputs":["https://media.example/header.mp4"]` + tc.cost + `}`
					}
					if tc.badBody {
						body = `{"truncated":`
					}
					_, _ = io.WriteString(w, body)
				}))
				defer server.Close()
				info := &meta.Meta{BaseURL: server.URL, ActualModelName: "veo3-fast"}
				var charge string
				var err error
				if operation == "submit" {
					receipt, callErr := (&Adaptor{}).SubmitVideo(context.Background(), info, []byte(`{"duration":5}`))
					charge, err = receipt.CostUSD, callErr
					require.False(t, receipt.Rejected)
					if !tc.badBody {
						require.Equal(t, "header-job", receipt.ID, "financial metadata must not erase the accepted ID")
					}
				} else {
					observation, callErr := (&Adaptor{}).PollVideo(context.Background(), info, "header-job")
					charge, err = observation.CostUSD, callErr
					require.False(t, observation.Refunded)
				}
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, tc.want, charge)
			})
		}
	}
}

// TestAsyncBillingInterruptedResponseKeepsHeaderCharge verifies incomplete HTTP
// bodies and the size guard cannot discard a cost header already received.
func TestAsyncBillingInterruptedResponseKeepsHeaderCharge(t *testing.T) {
	for _, operation := range []string{"submit", "poll"} {
		for _, failure := range []string{"truncated", "complete_json_short_http", "oversized"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("X-MuAPI-Cost-USD", "1.200000")
					body := `{"request_id":"interrupted-job"}`
					if operation == "poll" {
						body = `{"id":"interrupted-job","status":"completed","outputs":["https://media.example/v.mp4"]}`
					}
					switch failure {
					case "truncated":
						body = `{"request_id":`
						w.Header().Set("Content-Length", strconv.Itoa(len(body)+100))
					case "complete_json_short_http":
						w.Header().Set("Content-Length", strconv.Itoa(len(body)+100))
					case "oversized":
						body = strings.Repeat("x", (1<<20)+1)
					}
					_, _ = io.WriteString(w, body)
				}))
				defer server.Close()
				info := &meta.Meta{BaseURL: server.URL, ActualModelName: "veo3-fast"}
				if operation == "submit" {
					receipt, err := (&Adaptor{}).SubmitVideo(context.Background(), info, []byte(`{"duration":5}`))
					require.Error(t, err)
					require.False(t, receipt.Rejected)
					require.Equal(t, "1.200000", receipt.CostUSD)
					if failure == "complete_json_short_http" {
						require.Equal(t, "interrupted-job", receipt.ID)
					}
				} else {
					observation, err := (&Adaptor{}).PollVideo(context.Background(), info, "interrupted-job")
					require.Error(t, err)
					require.False(t, observation.Refunded)
					require.Nil(t, observation.Result)
					require.Equal(t, "1.200000", observation.CostUSD)
				}
			})
		}
	}
}

// TestAsyncBillingHeaderEvidenceIdentityAndRefund keeps conservative charge
// capture from becoming authority to refund an unrelated or unconfirmed job.
func TestAsyncBillingHeaderEvidenceIdentityAndRefund(t *testing.T) {
	cases := []struct {
		name, body, cost, want string
		wantErr, refunded      bool
	}{
		{name: "different_task", body: `{"id":"another-job","status":"failed","cost":{"amount_usd":0.8,"refunded":true}}`, cost: "0.8", wantErr: true},
		{name: "header_refund_only", body: `{"id":"paid-job","status":"failed"}`, cost: "0.8", want: "0.8"},
		{name: "anonymous_refund", body: `{"status":"failed","cost":{"refunded":true}}`, cost: "0.8", want: "0.8", wantErr: true},
		{name: "matching_body_refund", body: `{"id":"paid-job","status":"failed","cost":{"refunded":true}}`, cost: "0.8", want: "0.8", refunded: true},
		{name: "invalid_charge_blocks_refund", body: `{"id":"paid-job","status":"failed","cost":{"refunded":true,"amount_usd":0.8}}`, cost: "NaN", want: "0.8", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-MuAPI-Cost-USD", tc.cost)
				w.Header().Set("X-MuAPI-Cost-Refunded", "true")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			observation, err := (&Adaptor{}).PollVideo(context.Background(), &meta.Meta{BaseURL: server.URL}, "paid-job")
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, observation.CostUSD)
			require.Equal(t, tc.refunded, observation.Refunded)
			require.Nil(t, observation.Result)
		})
	}
}

// TestAsyncBillingRedirectKeepsReceiptWithoutReplay stops credential forwarding
// and paid POST replay while preserving evidence returned by the original host.
func TestAsyncBillingRedirectKeepsReceiptWithoutReplay(t *testing.T) {
	for _, operation := range []string{"submit", "poll"} {
		t.Run(operation, func(t *testing.T) {
			var redirects atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { redirects.Add(1); w.WriteHeader(http.StatusOK) }))
			defer target.Close()
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-MuAPI-Cost-USD", "0.8")
				w.Header().Set("Location", target.URL+"/must-not-replay")
				w.WriteHeader(http.StatusTemporaryRedirect)
				_, _ = io.WriteString(w, `{"request_id":"redirect-job","status":"completed","outputs":["https://media.example/v.mp4"]}`)
			}))
			defer provider.Close()
			info := &meta.Meta{BaseURL: provider.URL, ActualModelName: "veo3-fast", APIKey: "fixture-secret"}
			if operation == "submit" {
				receipt, err := (&Adaptor{}).SubmitVideo(context.Background(), info, []byte(`{"duration":5}`))
				require.Error(t, err)
				require.Equal(t, "0.8", receipt.CostUSD)
				require.Equal(t, "redirect-job", receipt.ID)
				require.False(t, receipt.Rejected)
			} else {
				result, err := (&Adaptor{}).PollVideo(context.Background(), info, "redirect-job")
				require.Error(t, err)
				require.Equal(t, "0.8", result.CostUSD)
				require.Nil(t, result.Result)
				require.False(t, result.Refunded)
			}
			require.Zero(t, redirects.Load(), "no request or credential may reach the redirect target")
		})
	}
}
