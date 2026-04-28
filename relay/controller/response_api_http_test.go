package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	gmw "github.com/Laisky/gin-middlewares/v7"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const responseAPISuccessPayload = `{
  "id": "resp_test",
  "object": "response",
  "created_at": 1741386163,
  "status": "completed",
  "error": null,
  "incomplete_details": null,
  "instructions": null,
  "max_output_tokens": null,
  "model": "gpt-4o-2024-08-06",
  "output": [
    {
      "type": "message",
      "id": "msg_1",
      "status": "completed",
      "role": "assistant",
      "content": [
        {
          "type": "output_text",
          "text": "Hello from upstream",
          "annotations": []
        }
      ]
    }
  ],
  "parallel_tool_calls": true,
  "previous_response_id": null,
  "reasoning": {
    "effort": null,
    "summary": null
  },
  "store": true,
  "temperature": 1.0,
  "text": {
    "format": {
      "type": "text"
    }
  },
  "tool_choice": "auto",
  "tools": [],
  "top_p": 1.0,
  "truncation": "disabled",
  "usage": {
    "input_tokens": 32,
    "input_tokens_details": {
      "cached_tokens": 0
    },
    "output_tokens": 18,
    "output_tokens_details": {
      "reasoning_tokens": 0
    },
    "total_tokens": 50
  },
  "user": null,
  "metadata": {}
}`

const responseAPIDeletePayload = `{
  "id": "resp_delete",
  "object": "response",
  "deleted": true
}`

var traceDBSetup sync.Once

func init() {
	gin.SetMode(gin.TestMode)

	traceDBSetup.Do(func() {
		if model.DB != nil {
			return
		}

		db, err := gorm.Open(sqlite.Open("file:response_api_tests?mode=memory&cache=shared"), &gorm.Config{})
		if err != nil {
			panic(err)
		}

		if err := db.AutoMigrate(&model.Trace{}); err != nil {
			panic(err)
		}

		model.DB = db
	})

	if client.HTTPClient == nil {
		client.HTTPClient = &http.Client{}
	}
}

func setupResponseAPIContext(t *testing.T, method, target, baseURL string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer upstream-key")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.OpenAI)
	c.Set(ctxkey.ChannelId, 42)
	c.Set(ctxkey.TokenId, 7)
	c.Set(ctxkey.TokenName, "test-token")
	c.Set(ctxkey.Id, 99)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.BaseURL, baseURL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestModel, "gpt-4o-2024-08-06")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestId, "req-test")

	return c, recorder
}

func TestRelayResponseAPIGetHelper_PassThrough(t *testing.T) {
	capturedQuery := url.Values{}
	var capturedAuthorization string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/responses/resp_test", r.URL.Path)
		capturedQuery = r.URL.Query()
		capturedAuthorization = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, responseAPISuccessPayload)
	}))
	defer upstream.Close()

	c, recorder := setupResponseAPIContext(t, http.MethodGet, "/v1/responses/resp_test?include=output&include=usage", upstream.URL)
	require.NotNil(t, client.HTTPClient)

	err := RelayResponseAPIGetHelper(c)
	require.Nil(t, err)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "ok", recorder.Header().Get("X-Upstream"))
	require.JSONEq(t, responseAPISuccessPayload, recorder.Body.String())

	require.ElementsMatch(t, []string{"output", "usage"}, capturedQuery["include"])
	require.Equal(t, "Bearer upstream-key", capturedAuthorization)
}

func TestRelayResponseAPIDeleteHelper_PassThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.Equal(t, "/v1/responses/resp_delete", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, responseAPIDeletePayload)
	}))
	defer upstream.Close()

	c, recorder := setupResponseAPIContext(t, http.MethodDelete, "/v1/responses/resp_delete", upstream.URL)
	require.NotNil(t, client.HTTPClient)

	err := RelayResponseAPIDeleteHelper(c)
	require.Nil(t, err)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, responseAPIDeletePayload, recorder.Body.String())
}

func TestRelayResponseAPICancelHelper_PassThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/responses/resp_cancel/cancel", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, responseAPISuccessPayload)
	}))
	defer upstream.Close()

	c, recorder := setupResponseAPIContext(t, http.MethodPost, "/v1/responses/resp_cancel/cancel", upstream.URL)
	require.NotNil(t, client.HTTPClient)

	err := RelayResponseAPICancelHelper(c)
	require.Nil(t, err)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, responseAPISuccessPayload, recorder.Body.String())
}
