package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestProxyPolicyRejectionDoesNotPenalizeChannel sends real local proxy rejections
// through the error processor with absent and populated models and verifies no health penalty.
func TestProxyPolicyRejectionDoesNotPenalizeChannel(t *testing.T) {
	for _, modelName := range []string{"", suspension5XXModel} {
		t.Run("model="+modelName, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/oneapi/proxy/fixture/health", nil)
			meta.Set2Context(c, &meta.Meta{Mode: relaymode.Proxy, ChannelType: channeltype.OpenAICompatible, APIType: apitype.OpenAI})
			failure := relaycontroller.RelayProxyHelper(c, relaymode.Proxy)
			require.NotNil(t, failure)
			require.Equal(t, http.StatusForbidden, failure.StatusCode)
			require.Equal(t, "proxy_channel_required", failure.Code)
			fixture.params.Err = *failure
			fixture.params.OriginalModel = modelName
			processChannelRelayError(fixture.ctx, fixture.params)
			require.Empty(t, fixture.logs.FilterLevelExact(zapcore.ErrorLevel).All(), "local policy rejection must not attempt suspension")
			require.True(t, isUserOriginatedRelayError(failure))
			require.False(t, countsAgainstChannelHealth(failure))
			require.Error(t, shouldRetry(c, failure))
			var abilities []dbmodel.Ability
			require.NoError(t, fixture.db.Find(&abilities).Error)
			for _, ability := range abilities {
				require.True(t, ability.Enabled)
				require.Zero(t, ability.SuspendUntil)
			}
			var channel dbmodel.Channel
			require.NoError(t, fixture.db.First(&channel, suspension5XXChannel).Error)
			require.Equal(t, dbmodel.ChannelStatusEnabled, channel.Status)
			upstream := *failure
			upstream.Type = "permission_error"
			require.False(t, isUserOriginatedRelayError(&upstream), "same code from an upstream is not gateway policy")
			require.True(t, countsAgainstChannelHealth(&upstream))
			require.NoError(t, shouldRetry(c, &upstream))
		})
	}
}
