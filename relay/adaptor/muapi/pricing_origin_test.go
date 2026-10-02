package muapi

import (
	"io"
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

// bindMuAPIPricingChannel supplies the same typed, selected configuration that
// the production distributor binds before invoking a pricing adaptor.
func bindMuAPIPricingChannel(c *gin.Context, info *meta.Meta) *dbmodel.Channel {
	info.ChannelId, info.ChannelUUID, info.ChannelType = 42, "pricing-channel", channeltype.MuAPI
	base := info.BaseURL
	channel := &dbmodel.Channel{Id: info.ChannelId, UUID: info.ChannelUUID, Type: channeltype.MuAPI, BaseURL: &base, Key: info.APIKey}
	c.Set(ctxkey.ChannelModel, channel)
	return channel
}

// TestMuAPIPricingUsesSelectedChannelOrigin checks the credential-bearing
// boundary directly. A stale/spoofed metadata URL or key cannot supersede the
// server-selected channel, and an absent/mismatched channel fails closed.
func TestMuAPIPricingUsesSelectedChannelOrigin(t *testing.T) {
	for _, scenario := range []string{"valid", "metadata_origin", "metadata_key", "missing", "nil", "wrong_type", "wrong_id", "wrong_uuid"} {
		t.Run(scenario, func(t *testing.T) {
			var trustedCalls, otherCalls atomic.Int32
			var badKey atomic.Bool
			trusted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trustedCalls.Add(1)
				if r.Header.Get("x-api-key") != "selected-provider-key" {
					badKey.Store(true)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"cost":0.4,"currency":"USD"}`)
			}))
			defer trusted.Close()
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				otherCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"cost":0.4,"currency":"USD"}`)
			}))
			defer other.Close()
			previous := client.HTTPClient
			client.HTTPClient = trusted.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c := newMuAPITestContext(http.MethodPost, "/v1/async/videos", `{"duration":5}`)
			info := &meta.Meta{BaseURL: trusted.URL, APIKey: "selected-provider-key", ActualModelName: "veo3-fast"}
			selected := bindMuAPIPricingChannel(c, info)
			switch scenario {
			case "metadata_origin":
				info.BaseURL = other.URL
			case "metadata_key":
				info.APIKey = "untrusted-shadow-key"
			case "missing":
				delete(c.Keys, ctxkey.ChannelModel)
			case "nil":
				c.Set(ctxkey.ChannelModel, (*dbmodel.Channel)(nil))
			case "wrong_type":
				selected.Type = channeltype.OpenAI
			case "wrong_id":
				selected.Id++
			case "wrong_uuid":
				selected.UUID = "replacement-channel"
			}
			pricing, err := (&Adaptor{}).EstimateVideoPricing(c, info, &model.VideoRequest{Duration: float64Ptr(5)})
			if scenario == "valid" || scenario == "metadata_origin" || scenario == "metadata_key" {
				require.NoError(t, err)
				require.Equal(t, "0.4", pricing.TotalUsdDecimal)
				require.EqualValues(t, 1, trustedCalls.Load())
			} else {
				require.Error(t, err)
				require.Zero(t, trustedCalls.Load())
			}
			require.Zero(t, otherCalls.Load(), "metadata must not select a credential-bearing destination")
			require.False(t, badKey.Load(), "credentials must come from the selected channel snapshot")
		})
	}
}
