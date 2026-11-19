package controller

import (
	"net/http"
	"testing"

	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// Client messages of the endpoints that answer a failed reference resolution
// with their own documented text instead of the shared resolver message.
const (
	passkeyRefMessage   = "invalid credential id"
	totpUserRefMessage  = "Invalid user ID"
	dashboardRefMessage = "Invalid user_id parameter"
)

// messageOverrideRefCases builds one request per message-replacing endpoint.
// Parameters: userRef is the :id/user_id value for the user-scoped endpoints
// and passkeyRef the :id value for the passkey endpoints. It returns the
// cases, each carrying the client message the endpoint must answer with.
func messageOverrideRefCases(userRef, passkeyRef string) []refErrorCase {
	userID := gin.Params{{Key: "id", Value: userRef}}
	passkeyID := gin.Params{{Key: "id", Value: passkeyRef}}
	return []refErrorCase{
		{name: "passkey delete", handler: PasskeyDelete, method: http.MethodDelete, target: "/api/user/self/passkey/" + passkeyRef, params: passkeyID, message: passkeyRefMessage},
		{name: "passkey rename", handler: PasskeyRename, method: http.MethodPut, target: "/api/user/self/passkey/" + passkeyRef, params: passkeyID, body: `{"name":"renamed"}`, message: passkeyRefMessage},
		{name: "admin totp status", handler: AdminGetUserTotpStatus, method: http.MethodGet, target: "/api/user/totp/status/" + userRef, params: userID, message: totpUserRefMessage},
		{name: "admin disable totp", handler: AdminDisableUserTotp, method: http.MethodPost, target: "/api/user/totp/disable/" + userRef, params: userID, message: totpUserRefMessage},
		{name: "dashboard user filter", handler: GetUserDashboard, method: http.MethodGet, target: "/api/user/dashboard?user_id=" + userRef, message: dashboardRefMessage},
	}
}

// TestResourceRefMessageOverrideMalformedIsClientError pins that every
// message-replacing endpoint answers a malformed reference with HTTP 400, its
// own message, and one stackless WARN. GetUserDashboard used to answer HTTP 200
// and log nothing at all.
func TestResourceRefMessageOverrideMalformedIsClientError(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	for _, tc := range messageOverrideRefCases("123", "123") {
		t.Run(tc.name, func(t *testing.T) {
			outcome := serveRefErrorCase(t, tc)
			requireClientRefError(t, tc, outcome)
			require.Equal(t, tc.message, outcome.body["message"])
		})
	}
}

// TestResourceRefMessageOverrideNotFound pins that a well-formed reference to
// a missing row keeps the HTTP 200 {success:false} convention and the
// endpoint's message, logged once at WARN as not_found. GetUserDashboard used
// to log nothing, so an unknown user_id left no trace.
func TestResourceRefMessageOverrideNotFound(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	const missing = "00000000-0000-4000-8000-000000000000"
	for _, tc := range messageOverrideRefCases(missing, missing) {
		t.Run(tc.name, func(t *testing.T) {
			requireNotFoundRefErrorMessage(t, serveRefErrorCase(t, tc), tc.message)
		})
	}
}

// TestResourceRefMessageOverrideLookupFailureKeepsCause reproduces a database
// outage during reference resolution on the message-replacing endpoints. The
// client keeps the documented message, but the ERROR entry must carry the
// resolver's error: the passkey and TOTP endpoints used to replace it with a
// fresh errors.New, so on-call saw "Invalid user ID" with no cause, and
// GetUserDashboard logged nothing.
func TestResourceRefMessageOverrideLookupFailureKeepsCause(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	for _, tc := range messageOverrideRefCases(fixture.user.UUID, fixture.passkey.UUID) {
		t.Run(tc.name, func(t *testing.T) {
			outcome := serveRefErrorCase(t, tc)
			requireServerRefError(t, outcome)
			require.Equal(t, tc.message, outcome.body["message"], "the client keeps the documented message")
			requireLoggedCause(t, outcome, "resolve uuid reference", "database is closed")
		})
	}
}

// TestAdminGetTokenRefMessages pins the AdminGetToken mapping: only a
// malformed id is reported as "invalid token id". A well-formed UUID that
// matches no token, and a lookup failure, used to gain the same prefix, which
// told the caller their id was malformed when it was not.
func TestAdminGetTokenRefMessages(t *testing.T) {
	adminGetTokenCase := func(ref string) refErrorCase {
		return refErrorCase{name: "admin get token", handler: AdminGetToken, method: http.MethodGet,
			target: "/api/admin/tokens/" + ref, params: gin.Params{{Key: "id", Value: ref}}}
	}
	t.Run("malformed", func(t *testing.T) {
		_, cleanup := setupUUIDContractTestEnvironment(t)
		defer cleanup()
		tc := adminGetTokenCase("123")
		tc.message = "invalid token id: invalid resource reference"
		outcome := serveRefErrorCase(t, tc)
		requireClientRefError(t, tc, outcome)
		require.Equal(t, tc.message, outcome.body["message"])
	})
	t.Run("not found", func(t *testing.T) {
		_, cleanup := setupUUIDContractTestEnvironment(t)
		defer cleanup()
		requireNotFoundRefError(t, serveRefErrorCase(t, adminGetTokenCase("00000000-0000-4000-8000-000000000000")))
	})
	t.Run("lookup failure", func(t *testing.T) {
		fixture, cleanup := setupUUIDContractTestEnvironment(t)
		defer cleanup()
		sqlDB, err := model.DB.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		outcome := serveRefErrorCase(t, adminGetTokenCase(fixture.token.UUID))
		requireServerRefError(t, outcome)
		require.NotContains(t, outcome.body["message"], "invalid token id",
			"a database outage is not a malformed token id")
		require.Contains(t, outcome.body["message"], "resolve uuid reference")
	})
}

// requireLoggedCause asserts that the single ERROR entry recorded for a
// lookup failure carries every fragment of the underlying cause.
// Parameters: t owns assertions, outcome is the handler result and fragments
// are substrings the logged error chain must contain.
func requireLoggedCause(t *testing.T, outcome refErrorOutcome, fragments ...string) {
	t.Helper()
	entries := outcome.logs.FilterLevelExact(zapcore.ErrorLevel).All()
	require.Len(t, entries, 1)
	verbose, ok := entries[0].ContextMap()["errorVerbose"].(string)
	require.True(t, ok, "the ERROR entry must record the error chain with its stack")
	for _, fragment := range fragments {
		require.Contains(t, verbose, fragment, "the ERROR entry lost the resolver's cause")
	}
}
