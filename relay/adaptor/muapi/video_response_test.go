package muapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMuAPIVideoResponseBindsAcceptedTask verifies that a native request_id
// is persisted through the provider-independent async task contract.
func TestMuAPIVideoResponseBindsAcceptedTask(t *testing.T) {
	c, recorder := newMuAPITestContextWithRecorder(http.MethodPost, "/v1/videos", "")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&dbmodel.AsyncTaskBinding{}))
	previousDB := dbmodel.DB
	dbmodel.DB = db
	t.Cleanup(func() { dbmodel.DB = previousDB })
	meta.Set2Context(c, &meta.Meta{
		UserId: 77, TokenId: 88, ChannelId: 9, ChannelType: 61,
		OriginModelName: "veo3-fast", ActualModelName: "veo3-fast",
	})
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"job-123","status":"processing"}`)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.True(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, `{"request_id":"job-123","status":"processing"}`, recorder.Body.String())
	binding, err := dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "job-123")
	require.NoError(t, err)
	require.Equal(t, 77, binding.UserID)
	require.Equal(t, 9, binding.ChannelID)
	require.Equal(t, 61, binding.ChannelType)
	require.Equal(t, "veo3-fast", binding.ActualModel)
}

// TestMuAPIVideoResponseRetriesOnlyBindingPersistence verifies a temporary
// local database failure does not resend the paid upstream creation and that
// the saved task routing record is recoverable when storage returns.
func TestMuAPIVideoResponseRetriesOnlyBindingPersistence(t *testing.T) {
	c, recorder := newMuAPITestContextWithRecorder(http.MethodPost, "/v1/videos", "")
	meta.Set2Context(c, &meta.Meta{
		UserId: 77, TokenId: 88, ChannelId: 9, ChannelType: 61,
		OriginModelName: "veo3-fast", ActualModelName: "veo3-fast",
	})

	recoveryDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	recoverySQL, err := recoveryDB.DB()
	require.NoError(t, err)
	recoverySQL.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = recoverySQL.Close() })
	require.NoError(t, recoveryDB.AutoMigrate(&dbmodel.AsyncTaskBinding{}))
	var failedWrites atomic.Int32
	require.NoError(t, recoveryDB.Callback().Create().Before("gorm:create").Register("test:transient_async_task_binding_write", func(tx *gorm.DB) {
		if tx.Statement.Table == "async_task_bindings" && failedWrites.Add(1) <= 4 {
			tx.AddError(errors.New("simulated temporary database write failure"))
		}
	}))

	previousDB := dbmodel.DB
	dbmodel.DB = recoveryDB
	t.Cleanup(func() { dbmodel.DB = previousDB })
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"job-retry","status":"processing"}`)),
	}
	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.True(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
	require.JSONEq(t, `{"request_id":"job-retry","status":"processing"}`, recorder.Body.String())

	require.Eventually(t, func() bool { return failedWrites.Load() >= 4 }, 3*time.Second, 25*time.Millisecond,
		"initial persistence and all automatic retries should have failed")
	_, err = dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "job-retry")
	require.Error(t, err)
	recovered, err := asyncvideo.RetryPendingTaskBinding(context.Background(), "job-retry", 77)
	require.True(t, recovered)
	require.NoError(t, err)
	binding, err := dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "job-retry")
	require.NoError(t, err)
	require.Equal(t, 9, binding.ChannelID)
	require.Equal(t, 77, binding.UserID)
}

// TestMuAPIVideoResponseForwardsPollingResult verifies completed results stay
// in MuAPI's native response shape and do not create another task binding.
func TestMuAPIVideoResponseForwardsPollingResult(t *testing.T) {
	t.Parallel()
	c, recorder := newMuAPITestContextWithRecorder(http.MethodGet, "/v1/videos/job-123", "")
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"status":"completed","outputs":[{"video_url":"https://cdn.example/video.mp4"}]}`)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.False(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
	require.JSONEq(t, `{"status":"completed","outputs":[{"video_url":"https://cdn.example/video.mp4"}]}`, recorder.Body.String())
}

// TestMuAPIVideoResponseForwardsSafeHeadersOnly keeps provider billing data
// internal while preserving headers useful to the client and transport.
func TestMuAPIVideoResponseForwardsSafeHeadersOnly(t *testing.T) {
	t.Parallel()
	c, recorder := newMuAPITestContextWithRecorder(http.MethodGet, "/v1/videos/job-123", "")
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":         []string{"application/json"},
			"Retry-After":          []string{"2"},
			"X-Request-Id":         []string{"request-123"},
			"X-MuAPI-Cost-USD":     []string{"0.12"},
			"X-MuAPI-Cost-Credits": []string{"12"},
		},
		Body: io.NopCloser(strings.NewReader(`{"status":"completed"}`)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	require.Nil(t, apiErr)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, "2", recorder.Header().Get("Retry-After"))
	require.Equal(t, "request-123", recorder.Header().Get("X-Request-Id"))
	require.Empty(t, recorder.Header().Get("X-MuAPI-Cost-USD"))
	require.Empty(t, recorder.Header().Get("X-MuAPI-Cost-Credits"))
}

func TestHasMuAPIError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "missing", raw: nil, want: false},
		{name: "whitespace", raw: json.RawMessage("  \t\n"), want: false},
		{name: "null", raw: json.RawMessage("null"), want: false},
		{name: "empty string", raw: json.RawMessage(`""`), want: false},
		{name: "whitespace string", raw: json.RawMessage(`"  \t"`), want: false},
		{name: "message", raw: json.RawMessage(`"provider failed"`), want: true},
		{name: "false", raw: json.RawMessage("false"), want: true},
		{name: "object", raw: json.RawMessage("{}"), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, hasMuAPIError(test.raw))
		})
	}
}

// TestMuAPIVideoResponseRejectsMissingRequestID prevents billing an accepted
// creation whose task cannot be polled safely.
func TestMuAPIVideoResponseRejectsMissingRequestID(t *testing.T) {
	t.Parallel()
	c, _ := newMuAPITestContextWithRecorder(http.MethodPost, "/v1/videos", "")
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader(`{"status":"processing"}`)),
	}

	_, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	require.False(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
}
