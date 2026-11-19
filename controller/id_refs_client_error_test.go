package controller

import (
	"encoding/json"
	"net/http"
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

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// refErrorCase is one admin/user API request whose resource reference is
// missing or malformed.
type refErrorCase struct {
	name    string
	handler gin.HandlerFunc
	method  string
	target  string
	params  gin.Params
	body    string
	message string
}

// refErrorOutcome is the observable result of one handler invocation.
type refErrorOutcome struct {
	status int
	body   map[string]any
	logs   *observer.ObservedLogs
}

// serveRefErrorCase invokes tc.handler as a root administrator with an
// observed DEBUG logger. Parameters: t owns assertions and tc is the request.
// It returns the response status, decoded body and captured log entries.
func serveRefErrorCase(t *testing.T, tc refErrorCase) refErrorOutcome {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("id-refs-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	var body *strings.Reader
	if tc.body != "" {
		body = strings.NewReader(tc.body)
	} else {
		body = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(tc.method, tc.target, body)
	if tc.body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	c.Params = tc.params
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.Role, model.RoleRootUser)
	gmw.SetLogger(c, lg)
	tc.handler(c)

	outcome := refErrorOutcome{status: recorder.Code, logs: logs}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &outcome.body), "body: %s", recorder.Body.String())
	return outcome
}

// requireClientRefError asserts the client-error contract for a bad
// reference: HTTP 400, success=false with the reference message, and exactly
// one WARN "http handler client error" entry without a stack trace.
func requireClientRefError(t *testing.T, tc refErrorCase, outcome refErrorOutcome) {
	t.Helper()
	require.Equal(t, http.StatusBadRequest, outcome.status, "a bad resource reference is a client error")
	require.Equal(t, false, outcome.body["success"])
	require.Contains(t, outcome.body["message"], tc.message)

	require.Empty(t, outcome.logs.FilterLevelExact(zapcore.ErrorLevel).All(), "a client mistake must not page on-call")
	entries := outcome.logs.FilterMessage("http handler client error").All()
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, zapcore.WarnLevel, entry.Level)
	fields := entry.ContextMap()
	require.Equal(t, "invalid_request", fields["error_kind"])
	require.EqualValues(t, http.StatusBadRequest, fields["status"])
	require.NotContains(t, fields, "errorVerbose", "client errors are logged without a stack trace")
}

// TestResourceRefMissingUUIDIsClientError reproduces the admin update payloads
// that omit uuid: they used to answer HTTP 200 and log "http handler error"
// at ERROR with a stack trace.
func TestResourceRefMissingUUIDIsClientError(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	const missing = "resource uuid is required"
	for _, tc := range []refErrorCase{
		{name: "token", handler: UpdateToken, method: http.MethodPut, target: "/api/token/", body: `{"id":1,"name":"renamed"}`, message: missing},
		{name: "user", handler: UpdateUser, method: http.MethodPut, target: "/api/user/", body: `{"id":1,"display_name":"renamed"}`, message: missing},
		{name: "topup", handler: AdminTopUp, method: http.MethodPost, target: "/api/topup", body: `{"user_id":1,"quota":10}`, message: missing},
		{name: "channel", handler: UpdateChannel, method: http.MethodPut, target: "/api/channel/", body: `{"id":1,"name":"renamed"}`, message: missing},
		{name: "redemption", handler: UpdateRedemption, method: http.MethodPut, target: "/api/redemption/", body: `{"id":1,"name":"renamed"}`, message: missing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireClientRefError(t, tc, serveRefErrorCase(t, tc))
		})
	}
}

// TestResourceRefMalformedIsClientError covers every id_refs.go resolver
// family with a malformed (legacy integer) reference in a path, query or body.
func TestResourceRefMalformedIsClientError(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	previousCursor := config.LogCursorEnabled
	config.LogCursorEnabled = true
	t.Cleanup(func() { config.LogCursorEnabled = previousCursor })
	const invalid = "invalid resource reference"
	id := gin.Params{{Key: "id", Value: "123"}}
	for _, tc := range []refErrorCase{
		{name: "update token body", handler: UpdateToken, method: http.MethodPut, target: "/api/token/", body: `{"uuid":"123"}`, message: invalid},
		{name: "update user body", handler: UpdateUser, method: http.MethodPut, target: "/api/user/", body: `{"uuid":"123"}`, message: invalid},
		{name: "topup body", handler: AdminTopUp, method: http.MethodPost, target: "/api/topup", body: `{"user_uuid":"123","quota":10}`, message: invalid},
		{name: "update channel body", handler: UpdateChannel, method: http.MethodPut, target: "/api/channel/", body: `{"uuid":"123"}`, message: invalid},
		{name: "update redemption body", handler: UpdateRedemption, method: http.MethodPut, target: "/api/redemption/", body: `{"uuid":"123"}`, message: invalid},
		{name: "get token", handler: GetToken, method: http.MethodGet, target: "/api/token/123", params: id, message: invalid},
		{name: "delete token", handler: DeleteToken, method: http.MethodDelete, target: "/api/token/123", params: id, message: invalid},
		{name: "admin get token", handler: AdminGetToken, method: http.MethodGet, target: "/api/admin/token/123", params: id, message: invalid},
		{name: "get user", handler: GetUser, method: http.MethodGet, target: "/api/user/123", params: id, message: invalid},
		{name: "delete user", handler: DeleteUser, method: http.MethodDelete, target: "/api/user/123", params: id, message: invalid},
		{name: "user totp status", handler: AdminGetUserTotpStatus, method: http.MethodGet, target: "/api/user/totp/status/123", params: id, message: "Invalid user ID"},
		{name: "get channel", handler: GetChannel, method: http.MethodGet, target: "/api/channel/123", params: id, message: invalid},
		{name: "delete channel", handler: DeleteChannel, method: http.MethodDelete, target: "/api/channel/123", params: id, message: invalid},
		{name: "duplicate channel", handler: DuplicateChannel, method: http.MethodPost, target: "/api/channel/123/duplicate", params: id, message: invalid},
		{name: "channel pricing", handler: GetChannelPricing, method: http.MethodGet, target: "/api/channel/pricing/123", params: id, message: invalid},
		{name: "reset channel models", handler: ResetChannelModels, method: http.MethodPost, target: "/api/channel/123/reset_models", params: id, message: invalid},
		{name: "update channel balance", handler: UpdateChannelBalance, method: http.MethodPost, target: "/api/channel/update_balance/123", params: id, message: invalid},
		{name: "get redemption", handler: GetRedemption, method: http.MethodGet, target: "/api/redemption/123", params: id, message: invalid},
		{name: "delete redemption", handler: DeleteRedemption, method: http.MethodDelete, target: "/api/redemption/123", params: id, message: invalid},
		{name: "get mcp server", handler: GetMCPServer, method: http.MethodGet, target: "/api/mcp_servers/123", params: id, message: invalid},
		{name: "mcp tools filter", handler: GetMCPTools, method: http.MethodGet, target: "/api/mcp_tools/?server_id=123", message: invalid},
		{name: "logs channel filter", handler: GetAllLogs, method: http.MethodGet, target: "/api/log/?channel=123", message: invalid},
		{name: "log cursor channel filter", handler: GetAllLogsCursor, method: http.MethodGet, target: "/api/log/cursor?channel=123", message: invalid},
		{name: "log stat channel filter", handler: GetLogsStat, method: http.MethodGet, target: "/api/log/stat?channel=123", message: invalid},
		{name: "passkey delete", handler: PasskeyDelete, method: http.MethodDelete, target: "/api/user/self/passkey/123", params: id, message: "invalid credential id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireClientRefError(t, tc, serveRefErrorCase(t, tc))
		})
	}
}

// TestResourceRefNotFoundKeepsConvention pins that a well-formed reference to
// a missing row keeps the /api/* HTTP 200 {success:false} convention while
// still logging at WARN without a stack (it was already client-attributed).
func TestResourceRefNotFoundKeepsConvention(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	tc := refErrorCase{name: "get token", handler: GetToken, method: http.MethodGet, target: "/api/token/x", params: gin.Params{{Key: "id", Value: "00000000-0000-4000-8000-000000000000"}}}
	outcome := serveRefErrorCase(t, tc)
	require.Equal(t, http.StatusOK, outcome.status)
	require.Equal(t, false, outcome.body["success"])
	require.Empty(t, outcome.logs.FilterLevelExact(zapcore.ErrorLevel).All())
	entries := outcome.logs.FilterMessage("http handler client error").All()
	require.Len(t, entries, 1)
	require.Equal(t, "not_found", entries[0].ContextMap()["error_kind"])
}
