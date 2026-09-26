package muapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestEstimateVideoPricingUsesMuAPICostEndpoint verifies exact per-request
// pricing conversion using an offline HTTP fixture.
func TestEstimateVideoPricingUsesMuAPICostEndpoint(t *testing.T) {
	var gotPath string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = io.WriteString(w, `{"cost":0.42,"currency":"USD"}`)
		require.NoError(t, err)
	}))
	defer server.Close()

	previousClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"prompt":"a lighthouse","duration":5}`)
	metaInfo := &meta.Meta{BaseURL: server.URL, APIKey: "test-key", ActualModelName: "veo3-fast"}
	request := &model.VideoRequest{Model: "veo3-fast", Duration: float64Ptr(5)}

	pricing, err := (&Adaptor{}).EstimateVideoPricing(c, metaInfo, request)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	require.Equal(t, 0.42, pricing.TotalUsd)
	require.Equal(t, "0.42", pricing.TotalUsdDecimal)
	require.Equal(t, "/api/v1/models/veo3-fast/estimate-cost", gotPath)
	require.JSONEq(t, `{"prompt":"a lighthouse","duration":5}`, gotBody)
}

// TestEstimateVideoPricingDoesNotFollowRedirect ensures the pricing client
// rejects a redirect before credentials can reach an untrusted endpoint.
func TestEstimateVideoPricingDoesNotFollowRedirect(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	previousClient := client.HTTPClient
	client.HTTPClient = source.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"duration":5}`)
	_, err := (&Adaptor{}).EstimateVideoPricing(c, &meta.Meta{
		BaseURL: source.URL, APIKey: "secret-key", ActualModelName: "veo3-fast",
	}, &model.VideoRequest{Duration: float64Ptr(5)})
	require.Error(t, err)
	require.Zero(t, targetRequests.Load(), "redirect target must not receive the credential or request body")
}

// TestEstimateVideoPricingRejectsNonUSD verifies that a provider response in
// an unknown currency cannot silently enter one-api's USD quota arithmetic.
func TestEstimateVideoPricingRejectsNonUSD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"cost":1.0,"currency":"EUR"}`)
		require.NoError(t, err)
	}))
	defer server.Close()

	previousClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"duration":5}`)
	_, err := (&Adaptor{}).EstimateVideoPricing(c, &meta.Meta{BaseURL: server.URL, ActualModelName: "veo3-fast"}, &model.VideoRequest{Duration: float64Ptr(5)})
	require.Error(t, err)
}
