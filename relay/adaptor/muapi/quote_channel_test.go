package muapi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// bindMuAPIQuoteTestChannel supplies the trusted channel that distribution
// attaches in production. It returns an independent fixture for mismatch tests.
func bindMuAPIQuoteTestChannel(c *gin.Context, info *meta.Meta) *dbmodel.Channel {
	base := info.BaseURL
	channel := &dbmodel.Channel{Id: 11, UUID: "00000000-0000-4000-8000-000000000011",
		Type: channeltype.MuAPI, BaseURL: &base, Key: info.APIKey}
	info.ChannelId, info.ChannelUUID, info.ChannelType = channel.Id, channel.UUID, channel.Type
	c.Set(ctxkey.ChannelModel, channel)
	return channel
}

// TestMuAPIQuoteRequiresSelectedChannel proves that stale or inconsistent relay
// metadata cannot choose a pricing destination or leak the selected channel key.
func TestMuAPIQuoteRequiresSelectedChannel(t *testing.T) {
	for _, scenario := range []string{"missing", "wrong_value", "nil_channel", "zero_id", "wrong_id", "wrong_uuid", "wrong_type", "wrong_meta_type", "other_origin", "other_path", "other_key"} {
		t.Run(scenario, func(t *testing.T) {
			var selectedCalls, otherCalls atomic.Int32
			selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				selectedCalls.Add(1)
				_, _ = w.Write([]byte(`{"cost":0.42,"currency":"USD"}`))
			}))
			defer selected.Close()
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				otherCalls.Add(1)
				_, _ = w.Write([]byte(`{"cost":0.42,"currency":"USD"}`))
			}))
			defer other.Close()
			previous := client.HTTPClient
			client.HTTPClient = selected.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c := newMuAPITestContext(http.MethodPost, "/v1/async/videos", `{"duration":5}`)
			info := &meta.Meta{BaseURL: selected.URL, APIKey: "fixture-channel-key", ActualModelName: "veo3-fast"}
			channel := bindMuAPIQuoteTestChannel(c, info)
			switch scenario {
			case "missing":
				delete(c.Keys, ctxkey.ChannelModel)
			case "wrong_value":
				c.Set(ctxkey.ChannelModel, "untrusted")
			case "nil_channel":
				c.Set(ctxkey.ChannelModel, (*dbmodel.Channel)(nil))
			case "zero_id":
				channel.Id, info.ChannelId = 0, 0
			case "wrong_id":
				info.ChannelId++
			case "wrong_uuid":
				info.ChannelUUID = "replacement-channel"
			case "wrong_type":
				channel.Type = channeltype.OpenAI
			case "wrong_meta_type":
				info.ChannelType = channeltype.OpenAI
			case "other_origin":
				info.BaseURL = other.URL
			case "other_path":
				info.BaseURL += "/different-account"
			case "other_key":
				info.APIKey = "other-fixture-key"
			}
			pricing, err := (&Adaptor{}).EstimateVideoPricing(c, info, &model.VideoRequest{Duration: float64Ptr(5)})
			require.Error(t, err)
			require.Nil(t, pricing)
			require.Zero(t, selectedCalls.Load(), "inconsistent metadata must fail before quoting")
			require.Zero(t, otherCalls.Load(), "unselected host must never receive a prompt or key")
			require.NotContains(t, err.Error(), "fixture-channel-key")
			require.NotContains(t, err.Error(), "other-fixture-key")
		})
	}
}

// TestMuAPIQuoteSelectedProxyCompatibility keeps supported administrator base
// forms and exact credentials while deriving the quote from the routed channel.
func TestMuAPIQuoteSelectedProxyCompatibility(t *testing.T) {
	for _, prefix := range []string{"", "/v1", "/api/v1/", "/proxy/muapi/api/v1"} {
		t.Run(prefix, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				expected := "/api/v1/models/veo3-fast/estimate-cost"
				if prefix == "/proxy/muapi/api/v1" {
					expected = "/proxy/muapi" + expected
				}
				require.Equal(t, expected, r.URL.Path)
				require.Equal(t, "fixture-channel-key", r.Header.Get("x-api-key"))
				require.Empty(t, r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(`{"cost":0.42,"currency":"USD"}`))
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c := newMuAPITestContext(http.MethodPost, "/v1/async/videos", `{"duration":5}`)
			info := &meta.Meta{BaseURL: server.URL + prefix, APIKey: "fixture-channel-key", ActualModelName: "veo3-fast"}
			bindMuAPIQuoteTestChannel(c, info)
			pricing, err := (&Adaptor{}).EstimateVideoPricing(c, info, &model.VideoRequest{Duration: float64Ptr(5)})
			require.NoError(t, err)
			require.Equal(t, "0.42", pricing.TotalUsdDecimal)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
