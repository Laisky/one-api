package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestSelectedChannelMutationRequiresExplicitSnapshot rejects implicit-all requests before modifying any channel.
func TestSelectedChannelMutationRequiresExplicitSnapshot(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	for _, body := range []string{"", `{}`, `{"selection":{"mode":"all_matching"}}`, `{"selection":{"mode":"ids","ids":[]}}`, `{"selection":{"mode":"ids","ids":["1"]}}`} {
		recorder := httptest.NewRecorder()
		channelResetTestRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/reset_models", strings.NewReader(body)))
		var result map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		require.Equal(t, false, result["success"], recorder.Body.String())
		var stored model.Channel
		require.NoError(t, model.DB.First(&stored, "id = ?", fixture.channel.Id).Error)
		require.Equal(t, fixture.channel.Models, stored.Models)
	}
}

// TestSelectedResetDoesNotTouchUnselectedOrLaterCreatedChannels verifies only the confirmed UUIDs are mutated.
func TestSelectedResetDoesNotTouchUnselectedOrLaterCreatedChannels(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	other := model.Channel{Name: "unselected", Type: 1, Models: "keep-this", Group: "default"}
	require.NoError(t, other.Insert())
	router := channelResetTestRouter()
	recorder := httptest.NewRecorder()
	body := `{"selection":{"mode":"ids","ids":["` + fixture.channel.UUID + `"]}}`
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/channel/reset_models", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, recorder.Code)
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", other.Id).Error)
	require.Equal(t, "keep-this", stored.Models)
}

// TestLogSelectionDerivesOwnershipAndDeletesOnlyExplicitIDs covers caller isolation, exclusions and fail-closed mutation.
func TestLogSelectionDerivesOwnershipAndDeletesOnlyExplicitIDs(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	own := &model.Log{UserId: fixture.user.Id, Type: model.LogTypeConsume, Content: "selection-fixture", ModelName: "model-A", CreatedAt: 200}
	foreign := &model.Log{UserId: fixture.user.Id + 1, Type: model.LogTypeConsume, Content: "selection-fixture", ModelName: "model-A", CreatedAt: 200}
	provisional := &model.Log{UserId: fixture.user.Id, Type: model.LogTypeProvisional, CreatedAt: 200}
	for _, row := range []*model.Log{own, foreign, provisional} {
		require.NoError(t, model.LOG_DB.Create(row).Error)
	}
	role := model.RoleCommonUser
	router := gin.New()
	router.Use(func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		c.Set(ctxkey.Id, fixture.user.Id)
		c.Set(ctxkey.Role, role)
	})
	router.POST("/select", ResolveLogSelection)
	router.POST("/delete", DeleteSelectedLogs)
	request := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return r
	}
	body := `{"selection":{"mode":"ids","ids":["` + own.UUID + `","` + foreign.UUID + `","` + provisional.UUID + `"]}}`
	r := request("/select", body)
	require.Equal(t, http.StatusOK, r.Code)
	require.Contains(t, r.Body.String(), own.UUID)
	require.NotContains(t, r.Body.String(), foreign.UUID)
	require.NotContains(t, r.Body.String(), provisional.UUID)
	require.Equal(t, http.StatusForbidden, request("/delete", body).Code)
	role = model.RoleAdminUser
	r = request("/select", `{"selection":{"mode":"all_matching","excluded_ids":["`+foreign.UUID+`"]},"model_name":"model-A","start_timestamp":200,"end_timestamp":200}`)
	require.Equal(t, http.StatusOK, r.Code)
	require.Contains(t, r.Body.String(), own.UUID)
	require.NotContains(t, r.Body.String(), foreign.UUID)
	r = request("/delete", `{"selection":{"mode":"all_matching"}}`)
	require.NotEqual(t, http.StatusOK, r.Code, r.Body.String())
	r = request("/delete", `{"selection":{"mode":"ids","ids":["`+own.UUID+`"]}}`)
	require.Equal(t, http.StatusOK, r.Code)
	require.Contains(t, r.Body.String(), `"deleted":1`)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("uuid IN ?", []string{foreign.UUID, provisional.UUID}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

// TestLogSelectionRejectsOversizedResults refuses a large export rather than returning a silently truncated selection.
func TestLogSelectionRejectsOversizedResults(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	row := &model.Log{UserId: fixture.user.Id, Type: model.LogTypeConsume, Content: strings.Repeat("x", 17<<20)}
	require.NoError(t, model.LOG_DB.Create(row).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		c.Set(ctxkey.Id, fixture.user.Id)
		c.Set(ctxkey.Role, model.RoleCommonUser)
	})
	engine.POST("/select", ResolveLogSelection)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/select", strings.NewReader(`{"selection":{"mode":"all_matching"}}`)))
	require.NotEqual(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "narrow the filters")
	require.NotContains(t, recorder.Body.String(), row.UUID)
}
