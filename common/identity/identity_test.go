package identity

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// fieldMap flattens zap fields into key -> rendered value for assertions.
func fieldMap(fields []zap.Field) map[string]any {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	return enc.Fields
}

// keyCount counts how many times a field key occurs in a field slice.
func keyCount(fields []zap.Field, key string) int {
	n := 0
	for _, f := range fields {
		if f.Key == key {
			n++
		}
	}
	return n
}

func TestClean_TrimsPostgresBpcharPadding(t *testing.T) {
	// PostgreSQL returns char(36) columns space-padded, so an unset uuid arrives
	// as 36 spaces. It must not become a 36-space user_uuid field.
	ref := NewUserRef(1, strings.Repeat(" ", 36), " alice ")
	require.Empty(t, ref.UUID)
	require.Equal(t, "alice", ref.Name)

	got := fieldMap(ref.Zap())
	require.NotContains(t, got, "user_uuid")
	require.Equal(t, "alice", got["username"])
	require.Equal(t, int64(1), got["user_id"])
}

func TestRef_ZapOmitsEmpty(t *testing.T) {
	require.Empty(t, UserRef{}.Zap())
	require.Empty(t, TokenRef{}.Zap())
	require.Empty(t, ChannelRef{}.Zap())
	require.Empty(t, LogRef{}.Zap())

	got := fieldMap(NewChannelRef(42, "", "openai-main").Zap())
	require.Equal(t, int64(42), got["channel_id"])
	require.Equal(t, "openai-main", got["channel_name"])
	require.NotContains(t, got, "channel_uuid")
}

func TestSet_MergeOverrides(t *testing.T) {
	base := Set{
		User:    NewUserRef(175, "u-uuid", "alice"),
		Channel: NewChannelRef(1, "c1-uuid", "channel-one"),
	}

	// A retry rebinds a different channel: it must REPLACE, not accumulate.
	merged := base.Merge(Set{Channel: NewChannelRef(2, "c2-uuid", "channel-two")})
	require.Equal(t, 2, merged.Channel.ID)
	require.Equal(t, "channel-two", merged.Channel.Name)
	// A zero sub-ref must not clobber known identity.
	require.Equal(t, 175, merged.User.ID)

	untouched := base.Merge(Set{})
	require.Equal(t, base, untouched)
}

func TestRefString_RendersNameUUIDAndID(t *testing.T) {
	require.Equal(t, "channel openai-main(9f2c)#42",
		NewChannelRef(42, "9f2c", "openai-main").String())
	require.Equal(t, "channel#42", NewChannelRef(42, "", "").String())
	require.Equal(t, "user alice#175", NewUserRef(175, "", "alice").String())
}

// newTestGinContext builds a gin context with a recorded logger attached, the way
// gmw.NewLoggerMiddleware would.
func newTestGinContext(t *testing.T) (*gin.Context, *observer.ObservedLogs) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	gmw.SetLogger(c, lg)

	return c, logs
}

func TestFromGin_ReadsCtxKeys(t *testing.T) {
	c, _ := newTestGinContext(t)
	c.Set(ctxkey.Id, 175)
	c.Set(ctxkey.UserUUID, "user-uuid")
	c.Set(ctxkey.Username, "alice")
	c.Set(ctxkey.TokenId, 257)
	c.Set(ctxkey.TokenUUID, "token-uuid")
	c.Set(ctxkey.TokenName, "laptop-cli")
	c.Set(ctxkey.ChannelId, 42)
	c.Set(ctxkey.ChannelUUID, "channel-uuid")
	c.Set(ctxkey.ChannelName, "openai-main")

	got := FromGin(c)
	require.Equal(t, NewUserRef(175, "user-uuid", "alice"), got.User)
	require.Equal(t, NewTokenRef(257, "token-uuid", "laptop-cli"), got.Token)
	require.Equal(t, NewChannelRef(42, "channel-uuid", "openai-main"), got.Channel)
}

// TestBind_RetryDoesNotDuplicateChannelField is the regression test for the relay
// retry hazard: rebinding a new channel must replace the previous channel fields
// rather than appending a second channel_id (zap does not de-duplicate).
func TestBind_RetryDoesNotDuplicateChannelField(t *testing.T) {
	c, logs := newTestGinContext(t)
	BindBase(c, zap.String("request_id", "req-1"))

	c.Set(ctxkey.Id, 175)
	c.Set(ctxkey.UserUUID, "user-uuid")
	c.Set(ctxkey.Username, "alice")
	BindFromGin(c)

	c.Set(ctxkey.ChannelId, 1)
	c.Set(ctxkey.ChannelUUID, "c1-uuid")
	c.Set(ctxkey.ChannelName, "channel-one")
	BindFromGin(c)

	// retry: a different channel is selected
	c.Set(ctxkey.ChannelId, 2)
	c.Set(ctxkey.ChannelUUID, "c2-uuid")
	c.Set(ctxkey.ChannelName, "channel-two")
	BindFromGin(c)

	gmw.GetLogger(c).Warn("upstream failed")

	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].Context
	require.Equal(t, 1, keyCount(fields, "channel_id"))
	require.Equal(t, 1, keyCount(fields, "user_id"))
	require.Equal(t, 1, keyCount(fields, "request_id"))

	got := fieldMap(fields)
	require.Equal(t, int64(2), got["channel_id"])
	require.Equal(t, "c2-uuid", got["channel_uuid"])
	require.Equal(t, "channel-two", got["channel_name"])
	require.Equal(t, "alice", got["username"])
	require.Equal(t, "user-uuid", got["user_uuid"])
}

func TestBind_PopulatesRequestContext(t *testing.T) {
	c, _ := newTestGinContext(t)
	BindBase(c)
	c.Set(ctxkey.Id, 175)
	c.Set(ctxkey.UserUUID, "user-uuid")
	c.Set(ctxkey.Username, "alice")
	BindFromGin(c)

	// Code that only receives a context.Context must recover the identity.
	got := FromContext(c.Request.Context())
	require.Equal(t, 175, got.User.ID)
	require.Equal(t, "alice", got.User.Name)

	// And the derived context's logger must already carry the fields.
	require.Equal(t, got, FromContext(gmw.Ctx(c)))
}

func TestFromContext_NilAndBareContext(t *testing.T) {
	require.True(t, FromContext(nil).IsZero())
	require.True(t, FromContext(context.Background()).IsZero())
	require.Empty(t, Of(context.Background()))

	ctx := NewContext(context.Background(), Set{User: NewUserRef(7, "u", "bob")})
	require.Equal(t, 7, FromContext(ctx).User.ID)
}

func TestFromContext_NilGinContextDoesNotPanic(t *testing.T) {
	var c *gin.Context
	require.NotPanics(t, func() {
		require.True(t, FromContext(c).IsZero())
		require.True(t, Current(nil).IsZero())
		require.True(t, FromGin(nil).IsZero())
		Bind(nil, Set{})
		BindBase(nil)
	})
}

// TestExtraFields_SuppressesAlreadyBoundDuplicates guards the error funnels: the
// bound logger already carries the request identity, so a funnel must not print
// user_id twice.
func TestExtraFields_SuppressesAlreadyBoundDuplicates(t *testing.T) {
	c, _ := newTestGinContext(t)
	BindBase(c)
	c.Set(ctxkey.Id, 175)
	c.Set(ctxkey.UserUUID, "user-uuid")
	c.Set(ctxkey.Username, "alice")
	BindFromGin(c)

	// Same user tagged on the error: nothing new to add.
	tagged := Tag(errTest("insufficient user quota"), NewUserRef(175, "user-uuid", "alice"))
	require.Empty(t, ExtraFields(c, tagged))

	// A DIFFERENT user (admin acting on another account) must still be reported.
	other := Tag(errTest("target user is banned"), NewUserRef(999, "other-uuid", "bob"))
	got := fieldMap(ExtraFields(c, other))
	require.Equal(t, int64(999), got["user_id"])
	require.Equal(t, "bob", got["username"])
}

// TestExtraFields_ReportsUnboundGinIdentity covers a handler that resolved an
// entity onto the gin context without re-binding the logger.
func TestExtraFields_ReportsUnboundGinIdentity(t *testing.T) {
	c, _ := newTestGinContext(t)
	BindBase(c)
	c.Set(ctxkey.ChannelId, 42)
	c.Set(ctxkey.ChannelUUID, "c-uuid")
	c.Set(ctxkey.ChannelName, "openai-main")

	got := fieldMap(ExtraFields(c, nil))
	require.Equal(t, int64(42), got["channel_id"])
	require.Equal(t, "openai-main", got["channel_name"])
}

type errTest string

func (e errTest) Error() string { return string(e) }
