package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// setupTokenAuthListModelsEnv builds a database with the full set of tables
// TokenAuth touches, so the middleware can be exercised for real rather than
// simulated by hand-setting context keys.
func setupTokenAuthListModelsEnv(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))

	originalDB := model.DB
	originalSQLite := common.UsingSQLite.Load()
	originalRedis := common.IsRedisEnabled()
	model.DB = db
	common.UsingSQLite.Store(true)
	common.SetRedisEnabled(false)

	t.Cleanup(func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		common.SetRedisEnabled(originalRedis)
	})
}

// TestTokenAuthToListModels_EndToEnd exercises the real middleware rather than
// hand-setting ctxkey.AvailableModels, which is what every other test in this area
// does. Those pin the filter's contract; this pins the wiring that feeds it -- that
// TokenAuth actually publishes a restricted token's allow-list from the Token row,
// and that ListModels honors it.
//
// Verified by mutation: deleting the c.Set in TokenAuth makes the restricted
// subtest fail (the key sees the whole group). Conversely, hoisting that c.Set out
// of its `if` is harmless, because filterAbilitiesByTokenAllowList treats an empty
// allow-list as unrestricted -- the two guards are deliberately redundant, since
// getting this wrong would blank the catalog for every key on the deployment.
func TestTokenAuthToListModels_EndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupModels := []string{"glm-4.7", "glm-5.3", "glm-4.6v"}

	emptyAllowList := ""
	restricted := "glm-4.7,glm-4.6v"

	for name, tc := range map[string]struct {
		tokenModels *string
		want        []string
	}{
		"nil allow-list lists the whole group":   {tokenModels: nil, want: groupModels},
		"empty allow-list lists the whole group": {tokenModels: &emptyAllowList, want: groupModels},
		"restricted allow-list is honored":       {tokenModels: &restricted, want: []string{"glm-4.7", "glm-4.6v"}},
	} {
		t.Run(name, func(t *testing.T) {
			setupTokenAuthListModelsEnv(t)

			groupName := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
			userUUID := "018f0000-0000-7000-8000-0000000003e2"
			require.NoError(t, model.DB.Create(&model.User{
				Id: 1, UUID: userUUID, Username: "e2e-user", Password: "hash",
				Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Group: groupName,
			}).Error)
			require.NoError(t, model.DB.Create(&model.Token{
				Id: 1, UUID: "018f0000-0000-7000-8000-0000000003e3",
				UserId: 1, UserUUID: &userUUID, Key: "e2etokenkey",
				Status: model.TokenStatusEnabled, Name: "e2e-token",
				ExpiredTime: -1, UnlimitedQuota: true,
				Models: tc.tokenModels,
			}).Error)

			require.NoError(t, model.DB.Create(&model.Channel{
				Id: 1, Name: "e2e-channel", Status: model.ChannelStatusEnabled,
				Type: channeltype.Zhipu, Models: "glm-4.7,glm-5.3,glm-4.6v", Group: groupName,
			}).Error)
			for _, m := range groupModels {
				require.NoError(t, model.DB.Create(&model.Ability{
					Group: groupName, Model: m, ChannelId: 1,
					Enabled: true, Priority: ptrInt64(0),
				}).Error)
			}

			router := gin.New()
			router.GET("/v1/models", middleware.TokenAuth(), ListModels)

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set("Authorization", "Bearer e2etokenkey")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

			var payload struct {
				Data []struct {
					Id string `json:"id"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			ids := make([]string, 0, len(payload.Data))
			for _, m := range payload.Data {
				ids = append(ids, m.Id)
			}
			require.ElementsMatch(t, tc.want, ids)
		})
	}
}
