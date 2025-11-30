package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common/ctxkey"
	dbmodel "github.com/songquanpeng/one-api/model"
	metalib "github.com/songquanpeng/one-api/relay/meta"
)

// TestVideoHandlerPassThroughJSON ensures JSON metadata is forwarded unchanged.
func TestVideoHandlerPassThroughJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Request = req

	original := map[string]any{
		"id":      "video_123",
		"object":  "video",
		"model":   "sora-2",
		"status":  "queued",
		"seconds": "4",
	}

	body, err := json.Marshal(original)
	require.NoError(t, err, "failed to marshal upstream response")

	upstream := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Video":      []string{"pass-through"},
		},
	}

	errResp, usage := VideoHandler(c, upstream)
	require.Nil(t, errResp, "video handler returned unexpected error")
	require.Nil(t, usage, "expected nil usage")
	require.Equal(t, http.StatusAccepted, w.Code, "unexpected status code")
	require.Equal(t, "pass-through", w.Header().Get("X-Video"), "header not forwarded")
	require.Equal(t, body, w.Body.Bytes(), "response body mutated")
}

// TestVideoHandlerSurfaceError ensures upstream error payloads surface appropriately.
func TestVideoHandlerSurfaceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Request = req

	errorBody := map[string]any{
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": "missing prompt",
		},
	}

	body, err := json.Marshal(errorBody)
	require.NoError(t, err, "failed to marshal error response")

	upstream := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	errResp, usage := VideoHandler(c, upstream)
	require.NotNil(t, errResp, "expected error from video handler")
	require.Equal(t, http.StatusBadRequest, errResp.StatusCode, "unexpected status")
	require.Nil(t, usage, "expected nil usage on error")
}

// TestVideoHandlerBinary ensures binary payloads stream without JSON parsing requirements.
func TestVideoHandlerBinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/videos/video_123/content", nil)
	c.Request = req

	binaryBody := []byte{0x01, 0x02, 0x03, 0x04}

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(binaryBody)),
		Header: http.Header{
			"Content-Type":   []string{"application/octet-stream"},
			"Content-Length": []string{"4"},
		},
	}

	errResp, usage := VideoHandler(c, upstream)
	require.Nil(t, errResp, "video handler returned unexpected error")
	require.Nil(t, usage, "expected nil usage")
	require.Equal(t, http.StatusOK, w.Code, "unexpected status code")
	require.Equal(t, binaryBody, w.Body.Bytes(), "binary body mutated")
}

func TestVideoHandlerPersistsAsyncTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", nil).WithContext(context.Background())
	c.Request = req

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	originalDB := dbmodel.DB
	dbmodel.DB = db
	defer func() { dbmodel.DB = originalDB }()
	require.NoError(t, db.AutoMigrate(&dbmodel.AsyncTaskBinding{}))

	meta := &metalib.Meta{
		ChannelId:       9,
		ChannelType:     1,
		UserId:          77,
		TokenId:         88,
		OriginModelName: "sora-2",
		ActualModelName: "sora-2",
	}
	metalib.Set2Context(c, meta)
	c.Set(ctxkey.AsyncTaskRequestMetadata, map[string]any{"model": "sora-2", "prompt": "hi"})

	body, err := json.Marshal(map[string]any{
		"id":     "video_binding",
		"object": "video",
	})
	require.NoError(t, err)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	errResp, usage := VideoHandler(c, upstream)
	require.Nil(t, errResp)
	require.Nil(t, usage)
	require.Equal(t, http.StatusOK, w.Code)

	fetched, err := dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "video_binding")
	require.NoError(t, err)
	require.Equal(t, 9, fetched.ChannelID)
	require.Equal(t, "sora-2", fetched.ActualModel)
}
