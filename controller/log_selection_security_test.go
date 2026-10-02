package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestLogSelectionSortingCannotWidenScope exercises HTTP decoding through the
// real database query. Hostile sort inputs cannot execute SQL, select another
// user's logs, include provisional rows, or discard an explicit exclusion.
func TestLogSelectionSortingCannotWidenScope(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	own := []*model.Log{
		{UserId: fixture.user.Id, Type: model.LogTypeConsume, Quota: 30, Content: "sort-security"},
		{UserId: fixture.user.Id, Type: model.LogTypeConsume, Quota: 10, Content: "sort-security"},
		{UserId: fixture.user.Id, Type: model.LogTypeConsume, Quota: 20, Content: "sort-security"},
	}
	foreign := &model.Log{UserId: fixture.user.Id + 1, Type: model.LogTypeConsume, Content: "sort-security"}
	provisional := &model.Log{UserId: fixture.user.Id, Type: model.LogTypeProvisional, Content: "sort-security"}
	for _, row := range append(append([]*model.Log{}, own...), foreign, provisional) {
		require.NoError(t, model.LOG_DB.Create(row).Error)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		c.Set(ctxkey.Id, fixture.user.Id)
		c.Set(ctxkey.Role, model.RoleCommonUser)
	})
	engine.POST("/api/log/selection", ResolveLogSelection)
	for _, tc := range []struct {
		name, field, order string
		want               []string
	}{
		{"ascending", "quota", "asc", []string{own[1].UUID, own[0].UUID}},
		{"descending", "quota", "desc", []string{own[0].UUID, own[1].UUID}},
		{"invalid direction", "quota", "asc; DROP TABLE logs; --", []string{own[0].UUID, own[1].UUID}},
		{"invalid field", "quota; DELETE FROM logs; --", "asc", []string{own[1].UUID, own[0].UUID}},
		{"SQL expression", "(SELECT 1)", "asc", []string{own[1].UUID, own[0].UUID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"keyword": "sort-security", "sort": tc.field, "order": tc.order,
				"selection": model.ListSelection{Mode: "all_matching", ExcludedIDs: []string{own[2].UUID}},
			})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/log/selection", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var response struct {
				Success bool `json:"success"`
				Data    []struct {
					UUID string `json:"uuid"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			got := make([]string, len(response.Data))
			for i, row := range response.Data {
				got[i] = row.UUID
			}
			require.Equal(t, tc.want, got)
			var count int64
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "sort-security").Count(&count).Error)
			require.EqualValues(t, 5, count, "selection must remain read-only")
		})
	}
}
