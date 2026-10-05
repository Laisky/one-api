package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// TestAsyncVideoExistingBindingDoesNotDependOnRetryStore checks the legacy
// status/content/delete routes remain available when only the new retry store
// fails. A healthy primary binding is authoritative and needs no recovery read.
func TestAsyncVideoExistingBindingDoesNotDependOnRetryStore(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			db := setupVideoBindingTestDB(t)
			old := model.DB
			model.DB = db
			conn, err := db.DB()
			require.NoError(t, err)
			conn.SetMaxOpenConns(1)
			t.Cleanup(func() { model.DB = old; require.NoError(t, conn.Close()) })
			require.NoError(t, db.Create(&model.AsyncTaskBinding{
				TaskID: "healthy-job", TaskType: "video", UserID: 10,
				ChannelID: 5, ChannelType: 1, OriginModel: "sora-2",
			}).Error)
			var retryReads int
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("audit:retry-store-unavailable", func(tx *gorm.DB) {
				if tx.Statement.Table == "async_task_binding_retries" {
					retryReads++
					tx.AddError(errors.New("injected retry store read failure"))
				}
			}))
			engine := gin.New()
			engine.Use(func(c *gin.Context) { gmw.SetLogger(c, logger.Logger); c.Set(ctxkey.Id, 10); c.Next() })
			engine.Use(BindAsyncTaskChannel())
			engine.Handle(method, "/v1/videos/:video_id", func(c *gin.Context) {
				require.Equal(t, 5, c.GetInt(ctxkey.SpecificChannelId))
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(method, "/v1/videos/healthy-job", nil))
			require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
			require.Zero(t, retryReads)
		})
	}
}
