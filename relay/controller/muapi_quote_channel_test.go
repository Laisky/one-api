package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
)

// TestMuAPIQuoteChannelMismatchCannotReserveWork sends inconsistent route
// metadata through the actual sync/async admission controller. Neither upstream
// receives a quote or generation, and physical task/wallet rows remain untouched.
func TestMuAPIQuoteChannelMismatchCannotReserveWork(t *testing.T) {
	oldWait := config.AsyncVideoWaitSeconds
	config.AsyncVideoWaitSeconds = 1
	t.Cleanup(func() { config.AsyncVideoWaitSeconds = oldWait })
	for _, syncWait := range []bool{false, true} {
		name := "async"
		path := "/v1/async/videos"
		if syncWait {
			name, path = "sync", "/v1/videos/generations"
		}
		t.Run(name, func(t *testing.T) {
			muapiTaskSetup(t, 10000000)
			selected := newMuapiTaskServer(t)
			other := newMuapiTaskServer(t)
			c, _ := muapiVideoContext(t, http.MethodPost, path,
				`{"model":"alias","prompt":"private test prompt","duration":5}`,
				selected.URL, 10000000, fallbackUserID)
			info := meta.GetByContext(c)
			info.BaseURL = other.URL
			meta.Set2Context(c, info)
			relayErr := RelayAsyncVideoHelper(c, syncWait)
			require.NotNil(t, relayErr)
			require.Equal(t, "video_pricing_unavailable", relayErr.Code)
			require.Zero(t, selected.estimates.Load())
			require.Zero(t, selected.creates.Load())
			require.Zero(t, other.estimates.Load())
			require.Zero(t, other.creates.Load())
			var count int64
			require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Count(&count).Error)
			require.Zero(t, count)
			var user dbmodel.User
			var token dbmodel.Token
			require.NoError(t, dbmodel.DB.WithContext(context.Background()).First(&user, fallbackUserID).Error)
			require.NoError(t, dbmodel.DB.First(&token, fallbackTokenID).Error)
			require.EqualValues(t, 10000000, user.Quota)
			require.EqualValues(t, 10000000, token.RemainQuota)
			require.Zero(t, user.UsedQuota)
			require.Zero(t, token.UsedQuota)
		})
	}
}
