package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

func TestUpdateChannelToolingLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	model.InitDB()

	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })

	channel := &model.Channel{
		Name:   "tooling-update-lifecycle",
		Type:   1,
		Key:    "sk-test",
		Group:  "default",
		Models: "gpt-4",
		Status: model.ChannelStatusEnabled,
		Config: "{\"api_format\":\"chat_completion\"}",
	}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM abilities WHERE channel_id = ?", channel.Id)
		model.DB.Exec("DELETE FROM channels WHERE id = ?", channel.Id)
	})

	router := gin.New()
	router.PUT("/api/channel/", UpdateChannel)

	updatePayload := map[string]any{
		"id":        channel.Id,
		"name":      channel.Name,
		"type":      channel.Type,
		"models":    channel.Models,
		"group":     channel.Group,
		"config":    channel.Config,
		"status":    channel.Status,
		"tooling":   "{\"whitelist\":[\"code_interpreter\"]}",
		"priority":  0,
		"weight":    0,
		"ratelimit": 0,
	}

	body, err := json.Marshal(updatePayload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Tooling *string `json:"tooling"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotNil(t, resp.Data.Tooling)
	require.Contains(t, *resp.Data.Tooling, "code_interpreter")

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	toolingCfg := updated.GetToolingConfig()
	require.NotNil(t, toolingCfg)
	require.ElementsMatch(t, []string{"code_interpreter"}, toolingCfg.Whitelist)

	// Clear tooling configuration by sending an empty string in the payload
	clearPayload := make(map[string]any, len(updatePayload))
	maps.Copy(clearPayload, updatePayload)
	clearPayload["tooling"] = ""

	clearBody, err := json.Marshal(clearPayload)
	require.NoError(t, err)

	clearReq, err := http.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(clearBody))
	require.NoError(t, err)
	clearReq.Header.Set("Content-Type", "application/json")

	clearW := httptest.NewRecorder()
	router.ServeHTTP(clearW, clearReq)

	require.Equal(t, http.StatusOK, clearW.Code)
	var clearResp struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(clearW.Body.Bytes(), &clearResp))
	require.True(t, clearResp.Success)

	refreshed, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Nil(t, refreshed.GetToolingConfig())
}

func TestGetChannelIncludesToolingField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	model.InitDB()

	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })

	channel := &model.Channel{
		Name:   "tooling-get",
		Type:   1,
		Key:    "sk-test",
		Group:  "default",
		Models: "gpt-4",
		Status: model.ChannelStatusEnabled,
		Config: "{\"api_format\":\"chat_completion\"}",
	}
	require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Whitelist: []string{"web_search"}}))
	require.NoError(t, channel.Insert())
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM abilities WHERE channel_id = ?", channel.Id)
		model.DB.Exec("DELETE FROM channels WHERE id = ?", channel.Id)
	})

	router := gin.New()
	route := fmt.Sprintf("/api/channel/%d", channel.Id)
	router.GET("/api/channel/:id", GetChannel)

	req, err := http.NewRequest(http.MethodGet, route, nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Tooling *string `json:"tooling"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotNil(t, resp.Data.Tooling)
	require.Contains(t, *resp.Data.Tooling, "web_search")
}
