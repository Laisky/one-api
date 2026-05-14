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

func TestGetChannelIncludesToolingFieldWithoutKey(t *testing.T) {
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
			Key     string  `json:"key"`
			Tooling *string `json:"tooling"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Empty(t, resp.Data.Key)
	require.NotNil(t, resp.Data.Tooling)
	require.Contains(t, *resp.Data.Tooling, "web_search")
}

func TestDuplicateChannelClonesServerSideWithoutExposingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	model.InitDB()

	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })

	testingModel := "gpt-4"
	baseURL := "https://example.com/v1"
	hiddenModels := "[\"gpt-4o-mini\"]"
	priority := int64(9)
	weight := uint(7)
	source := &model.Channel{
		Name:               "duplicate-source",
		Type:               1,
		Key:                "sk-test",
		Group:              "default",
		Models:             "gpt-4,gpt-4o-mini",
		Status:             model.ChannelStatusEnabled,
		Config:             "{\"api_format\":\"chat_completion\"}",
		Balance:            123.45,
		BalanceUpdatedTime: 456,
		UsedQuota:          789,
		ResponseTime:       321,
		TestTime:           654,
		BaseURL:            &baseURL,
		TestingModel:       &testingModel,
		HiddenModels:       &hiddenModels,
		Priority:           &priority,
		Weight:             &weight,
	}
	require.NoError(t, source.SetToolingConfig(&model.ChannelToolingConfig{Whitelist: []string{"web_search"}}))
	require.NoError(t, source.Insert())
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM abilities WHERE channel_id IN (?, ?)", source.Id, source.Id+1)
		model.DB.Exec("DELETE FROM channels WHERE id IN (?, ?)", source.Id, source.Id+1)
	})

	router := gin.New()
	router.POST("/api/channel/:id/duplicate", DuplicateChannel)

	requestPath := fmt.Sprintf("/api/channel/%d/duplicate", source.Id)
	req, err := http.NewRequest(http.MethodPost, requestPath, nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotZero(t, resp.Data.ID)
	require.Equal(t, "duplicate-source Copy", resp.Data.Name)

	duplicated, err := model.GetChannelById(resp.Data.ID, true)
	require.NoError(t, err)
	require.NotEqual(t, source.Id, duplicated.Id)
	require.Equal(t, "duplicate-source Copy", duplicated.Name)
	require.Equal(t, source.Key, duplicated.Key)
	require.Equal(t, source.Group, duplicated.Group)
	require.Equal(t, source.Models, duplicated.Models)
	require.Equal(t, source.Status, duplicated.Status)
	require.Equal(t, source.Config, duplicated.Config)
	require.Equal(t, source.GetBaseURL(), duplicated.GetBaseURL())
	require.NotNil(t, duplicated.TestingModel)
	require.Equal(t, testingModel, *duplicated.TestingModel)
	require.NotNil(t, duplicated.HiddenModels)
	require.Equal(t, hiddenModels, *duplicated.HiddenModels)
	require.NotNil(t, duplicated.Priority)
	require.Equal(t, priority, *duplicated.Priority)
	require.NotNil(t, duplicated.Weight)
	require.Equal(t, weight, *duplicated.Weight)
	require.Zero(t, duplicated.Balance)
	require.Zero(t, duplicated.BalanceUpdatedTime)
	require.Zero(t, duplicated.UsedQuota)
	require.Zero(t, duplicated.ResponseTime)
	require.Zero(t, duplicated.TestTime)
	require.NotZero(t, duplicated.CreatedTime)

	toolingCfg := duplicated.GetToolingConfig()
	require.NotNil(t, toolingCfg)
	require.ElementsMatch(t, []string{"web_search"}, toolingCfg.Whitelist)
}
