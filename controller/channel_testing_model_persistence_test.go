package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// testingModelPersistenceModels is the model list every persistence scenario starts from.
const testingModelPersistenceModels = "gpt-4o-mini,gpt-4o"

// newTestingModelPersistenceRouter registers the admin channel endpoints that
// read or write testing_model, mirroring the production routes in router/api.go.
// Parameters: t is the test handle used to initialise the database and restore config.
// Returns: a gin engine serving the channel list, single read, create, update,
// duplicate, and pricing endpoints.
func newTestingModelPersistenceRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	model.InitDB()

	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })

	router := gin.New()
	router.GET("/api/channel/", GetAllChannels)
	router.GET("/api/channel/:id", GetChannel)
	router.POST("/api/channel/", AddChannel)
	router.PUT("/api/channel/", UpdateChannel)
	router.POST("/api/channel/:id/duplicate", DuplicateChannel)
	router.PUT("/api/channel/pricing/:id", UpdateChannelPricing)
	return router
}

// testingModelEnvelope is the subset of the admin API envelope these tests read.
type testingModelEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// serveTestingModelJSON sends one JSON request through router and decodes the envelope.
// Parameters: t is the test handle, router serves the request, method and path select
// the endpoint, and payload is marshalled as the body when non-nil.
// Returns: the decoded response envelope; the HTTP status is asserted to be 200.
func serveTestingModelJSON(t *testing.T, router *gin.Engine, method, path string, payload any) testingModelEnvelope {
	t.Helper()
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		body = encoded
	}
	req, err := http.NewRequest(method, path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var envelope testingModelEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, "message: %s", envelope.Message)
	return envelope
}

// insertTestingModelChannel creates a channel and then writes testing_model
// directly to the column, bypassing every API normalisation. That is how the
// scenarios seed already-persisted state, including legacy spellings.
// Parameters: t is the test handle, models is the channel model list, and stored is
// the raw testing_model column value (nil leaves it NULL).
// Returns: the inserted channel with its generated UUID.
func insertTestingModelChannel(t *testing.T, models string, stored *string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Name:   "testing-model-" + uuid.NewString(),
		Type:   channeltype.OpenAI,
		Key:    "sk-test",
		Group:  "default",
		Models: models,
		Status: model.ChannelStatusEnabled,
	}
	require.NoError(t, channel.Insert())
	t.Cleanup(func() { deleteTestingModelChannel(channel.Id) })
	if stored != nil {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
			Update("testing_model", *stored).Error)
	}
	return channel
}

// deleteTestingModelChannel removes a channel row and its abilities created by a scenario.
// Parameters: id is the internal channel id. Returns: no values.
func deleteTestingModelChannel(id int) {
	model.DB.Exec("DELETE FROM abilities WHERE channel_id = ?", id)
	model.DB.Exec("DELETE FROM channels WHERE id = ?", id)
}

// storedTestingModel reads the testing_model column straight from the database.
// Parameters: t is the test handle and id is the internal channel id.
// Returns: the persisted value, or nil when the column is NULL.
func storedTestingModel(t *testing.T, id int) *string {
	t.Helper()
	var stored model.Channel
	require.NoError(t, model.DB.Select("id", "testing_model").First(&stored, "id = ?", id).Error)
	return stored.TestingModel
}

// listedTestingModel fetches the admin channel list exactly as the Modern channel
// page does after a refresh and returns the testing_model of the channel with channelUUID.
// Parameters: t is the test handle, router serves the list, and channelUUID identifies the row.
// Returns: the listed testing_model value, or nil when the API returned null.
func listedTestingModel(t *testing.T, router *gin.Engine, channelUUID string) *string {
	t.Helper()
	envelope := serveTestingModelJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/channel/?p=0&size=%d&sort=id&order=desc", config.MaxItemsPerPage), nil)
	var rows []struct {
		UUID         string  `json:"uuid"`
		TestingModel *string `json:"testing_model"`
	}
	require.NoError(t, json.Unmarshal(envelope.Data, &rows))
	for _, row := range rows {
		if row.UUID == channelUUID {
			return row.TestingModel
		}
	}
	require.FailNow(t, "channel missing from list response", "uuid=%s", channelUUID)
	return nil
}

// responseTestingModel decodes testing_model from a single-channel response payload.
// Parameters: t is the test handle and data is the envelope data field.
// Returns: the testing_model value, or nil when the payload carried null.
func responseTestingModel(t *testing.T, data json.RawMessage) *string {
	t.Helper()
	var row struct {
		TestingModel *string `json:"testing_model"`
	}
	require.NoError(t, json.Unmarshal(data, &row))
	return row.TestingModel
}

// requireTestingModel asserts a testing_model pointer against the expected value.
// Parameters: t is the test handle, want is the expected value (nil means NULL/auto),
// got is the observed pointer, and where names the observation point.
// Returns: no values.
func requireTestingModel(t *testing.T, want, got *string, where string) {
	t.Helper()
	if want == nil {
		require.Nil(t, got, "%s: testing_model must be NULL (auto-select cheapest), got %v", where, derefTestingModel(got))
		return
	}
	require.NotNil(t, got, "%s: testing_model must be %q, got NULL", where, *want)
	require.Equal(t, *want, *got, "%s", where)
}

// derefTestingModel renders an optional testing_model for assertion messages.
// Parameters: value is the optional testing_model. Returns: the value or "<nil>".
func derefTestingModel(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

// testingModelPtr returns a pointer to value for table-driven expectations.
// Parameters: value is the string to point at. Returns: a new pointer to a copy of value.
func testingModelPtr(value string) *string {
	return &value
}

// TestUpdateChannelTestingModelPersistence is the issue #609 behavior suite. It drives
// PUT /api/channel/ with the exact payloads the Modern admin UI sends, then verifies the
// database row, the PUT response, and the channel list a page refresh reads.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelTestingModelPersistence(t *testing.T) {
	router := newTestingModelPersistenceRouter(t)
	skip := model.ChannelTestingModelSkip

	cases := []struct {
		name    string
		models  string
		stored  *string
		payload map[string]any
		want    *string
	}{
		{
			name:    "list selector SKIP is persisted",
			payload: map[string]any{"testing_model": skip},
			want:    &skip,
		},
		{
			name:    "SKIP spelling variants are stored canonically",
			payload: map[string]any{"testing_model": " __SKIP__ "},
			want:    &skip,
		},
		{
			name:    "SKIP replaces a named testing model",
			stored:  testingModelPtr("gpt-4o"),
			payload: map[string]any{"testing_model": skip},
			want:    &skip,
		},
		{
			name:    "stored SKIP survives an update without testing_model",
			stored:  &skip,
			payload: map[string]any{"priority": 5},
			want:    &skip,
		},
		{
			name:    "stored SKIP survives a model list change",
			stored:  &skip,
			payload: map[string]any{"models": "claude-sonnet-4-5"},
			want:    &skip,
		},
		{
			name:    "a round-tripped SKIP value is kept",
			stored:  &skip,
			payload: map[string]any{"testing_model": skip, "models": testingModelPersistenceModels},
			want:    &skip,
		},
		{
			name:    "legacy SKIP spelling is canonicalised by the next update",
			stored:  testingModelPtr(" __Skip__ "),
			payload: map[string]any{"priority": 1},
			want:    &skip,
		},
		{
			name:    "null switches SKIP back to auto-select",
			stored:  &skip,
			payload: map[string]any{"testing_model": nil},
			want:    nil,
		},
		{
			name:    "null switches a named model back to auto-select",
			stored:  testingModelPtr("gpt-4o"),
			payload: map[string]any{"testing_model": nil},
			want:    nil,
		},
		{
			name:    "empty string switches a named model back to auto-select",
			stored:  testingModelPtr("gpt-4o"),
			payload: map[string]any{"testing_model": ""},
			want:    nil,
		},
		{
			name:    "a listed model is persisted",
			payload: map[string]any{"testing_model": "gpt-4o"},
			want:    testingModelPtr("gpt-4o"),
		},
		{
			name:    "a listed model is trimmed",
			payload: map[string]any{"testing_model": "  gpt-4o  "},
			want:    testingModelPtr("gpt-4o"),
		},
		{
			name:    "a model outside the channel list is dropped",
			payload: map[string]any{"testing_model": "not-listed"},
			want:    nil,
		},
		{
			name:    "an omitted testing_model keeps a still-listed model",
			stored:  testingModelPtr("gpt-4o"),
			payload: map[string]any{"priority": 2},
			want:    testingModelPtr("gpt-4o"),
		},
		{
			name:    "an omitted testing_model drops a model removed from the list",
			stored:  testingModelPtr("gpt-4o"),
			payload: map[string]any{"models": "gpt-4o-mini"},
			want:    nil,
		},
		{
			name:    "a named model is validated against the new model list",
			payload: map[string]any{"testing_model": "claude-sonnet-4-5", "models": "claude-sonnet-4-5"},
			want:    testingModelPtr("claude-sonnet-4-5"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			models := tc.models
			if models == "" {
				models = testingModelPersistenceModels
			}
			channel := insertTestingModelChannel(t, models, tc.stored)

			payload := map[string]any{"uuid": channel.UUID, "name": channel.Name}
			for key, value := range tc.payload {
				payload[key] = value
			}
			envelope := serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/", payload)

			requireTestingModel(t, tc.want, responseTestingModel(t, envelope.Data), "PUT response")
			requireTestingModel(t, tc.want, storedTestingModel(t, channel.Id), "database row")
			requireTestingModel(t, tc.want, listedTestingModel(t, router, channel.UUID), "channel list after refresh")
		})
	}
}

// TestUpdateChannelPricingKeepsTestingModelSkip verifies that saving channel pricing,
// which reloads and rewrites the whole row, does not discard the SKIP opt-out.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingKeepsTestingModelSkip(t *testing.T) {
	router := newTestingModelPersistenceRouter(t)
	skip := model.ChannelTestingModelSkip
	channel := insertTestingModelChannel(t, testingModelPersistenceModels, &skip)

	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 1.25}},
	})

	requireTestingModel(t, &skip, storedTestingModel(t, channel.Id), "database row")
	requireTestingModel(t, &skip, listedTestingModel(t, router, channel.UUID), "channel list after refresh")
}

// TestCreateChannelTestingModelPersistence verifies POST /api/channel/ applies the same
// testing_model policy as updates: SKIP and listed models persist, unknown names do not.
// Parameters: t is the test handle. Returns: no values.
func TestCreateChannelTestingModelPersistence(t *testing.T) {
	router := newTestingModelPersistenceRouter(t)
	skip := model.ChannelTestingModelSkip

	cases := []struct {
		name         string
		testingModel any
		want         *string
	}{
		{name: "SKIP", testingModel: skip, want: &skip},
		{name: "SKIP spelling variant", testingModel: "__SKIP__", want: &skip},
		{name: "listed model", testingModel: "gpt-4o", want: testingModelPtr("gpt-4o")},
		{name: "unlisted model", testingModel: "not-listed", want: nil},
		{name: "null", testingModel: nil, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "testing-model-create-" + uuid.NewString()
			serveTestingModelJSON(t, router, http.MethodPost, "/api/channel/", map[string]any{
				"name":          name,
				"type":          channeltype.OpenAI,
				"key":           "sk-test",
				"group":         "default",
				"models":        testingModelPersistenceModels,
				"testing_model": tc.testingModel,
			})

			var created model.Channel
			require.NoError(t, model.DB.Select("id", "uuid").First(&created, "name = ?", name).Error)
			t.Cleanup(func() { deleteTestingModelChannel(created.Id) })

			requireTestingModel(t, tc.want, storedTestingModel(t, created.Id), "database row")
			requireTestingModel(t, tc.want, listedTestingModel(t, router, created.UUID), "channel list after refresh")
		})
	}
}

// TestDuplicateChannelKeepsTestingModelSkip verifies a duplicated channel inherits the
// source's SKIP opt-out instead of silently reverting to auto-select.
// Parameters: t is the test handle. Returns: no values.
func TestDuplicateChannelKeepsTestingModelSkip(t *testing.T) {
	router := newTestingModelPersistenceRouter(t)
	skip := model.ChannelTestingModelSkip
	source := insertTestingModelChannel(t, testingModelPersistenceModels, &skip)

	envelope := serveTestingModelJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/channel/%s/duplicate", source.UUID), nil)
	var duplicated struct {
		UUID string `json:"uuid"`
	}
	require.NoError(t, json.Unmarshal(envelope.Data, &duplicated))
	require.NotEmpty(t, duplicated.UUID)
	stored, err := model.GetChannelByUUID(duplicated.UUID)
	require.NoError(t, err)
	t.Cleanup(func() { deleteTestingModelChannel(stored.Id) })

	requireTestingModel(t, &skip, storedTestingModel(t, stored.Id), "database row")
	requireTestingModel(t, &skip, listedTestingModel(t, router, duplicated.UUID), "channel list after refresh")
}

// TestChannelReadsCanonicaliseLegacySkipSpelling verifies that a legacy SKIP spelling
// already in the database, which the health checker honours case-insensitively, is
// reported to the admin UI as the canonical sentinel without rewriting the row.
// Parameters: t is the test handle. Returns: no values.
func TestChannelReadsCanonicaliseLegacySkipSpelling(t *testing.T) {
	router := newTestingModelPersistenceRouter(t)
	legacy := " __SKIP__ "
	channel := insertTestingModelChannel(t, testingModelPersistenceModels, &legacy)
	skip := model.ChannelTestingModelSkip

	requireTestingModel(t, &skip, listedTestingModel(t, router, channel.UUID), "channel list")
	envelope := serveTestingModelJSON(t, router, http.MethodGet, "/api/channel/"+channel.UUID, nil)
	requireTestingModel(t, &skip, responseTestingModel(t, envelope.Data), "single channel read")
	requireTestingModel(t, &legacy, storedTestingModel(t, channel.Id), "database row is not rewritten by a read")
}
