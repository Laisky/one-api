package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestXAIVideoTaskOwner exercises authenticated routing without mocking the
// SQLite lookup, ensuring another user cannot reach a bound provider job.
func TestXAIVideoTaskOwner(t *testing.T) {
	for _, tc := range []struct {
		name, id, allowed string
		user, want       int
	}{
		{"owner", "xai-job", "", 10, http.StatusNoContent},
		{"other_user", "xai-job", "", 11, http.StatusNotFound},
		{"unknown", "missing", "", 10, http.StatusNotFound},
		{"restricted_token", "xai-job", "grok-4.7", 10, http.StatusForbidden},
		{"allowed_token", "xai-job", "grok-imagine-video-1.5", 10, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupVideoBindingTestDB(t)
			previous := model.DB
			model.DB = db
			t.Cleanup(func() { model.DB = previous })
			binding := &model.AsyncTaskBinding{TaskID: "xai-job", TaskType: "video", UserID: 10, TokenID: 20, ChannelID: 5, ChannelType: channeltype.XAI, OriginModel: "grok-imagine-video-1.5", ActualModel: "grok-imagine-video-1.5", LastAccessedAt: 1}
			require.NoError(t, db.Create(binding).Error)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				gmw.SetLogger(c, logger.Logger)
				c.Set(ctxkey.Id, tc.user)
				c.Set(ctxkey.AvailableModels, tc.allowed)
				c.Next()
			}, BindAsyncTaskChannel())
			called := false
			r.GET("/v1/videos/:video_id", func(c *gin.Context) {
				called = true
				require.Equal(t, 5, c.GetInt(ctxkey.SpecificChannelId))
				require.Equal(t, "grok-imagine-video-1.5", c.GetString(ctxkey.RequestModel))
				c.Status(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/videos/"+tc.id, nil))
			require.Equal(t, tc.want, w.Code)
			require.Equal(t, tc.want == http.StatusNoContent, called)
			if tc.want != http.StatusNoContent {
				var stored model.AsyncTaskBinding
				require.NoError(t, db.First(&stored, binding.Id).Error)
				require.EqualValues(t, 1, stored.LastAccessedAt, "unauthorized lookups must not touch another user's task")
			}
		})
	}
}
