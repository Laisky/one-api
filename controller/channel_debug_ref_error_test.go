package controller

import (
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// channelDebugRefHandlers lists every channel debug endpoint that resolves a
// channel UUID from the :id path parameter.
var channelDebugRefHandlers = []struct {
	name    string
	handler gin.HandlerFunc
	method  string
	suffix  string
}{
	{name: "debug", handler: DebugChannelModelConfigs, method: http.MethodPost, suffix: "/debug"},
	{name: "fix", handler: FixChannelModelConfigs, method: http.MethodPost, suffix: "/fix"},
	{name: "migration status", handler: GetChannelMigrationStatus, method: http.MethodGet, suffix: "/migration-status"},
}

// channelDebugRefCase builds the request for one channel debug handler.
// Parameters: idx selects the handler in channelDebugRefHandlers and ref is
// the raw :id path value. It returns the case consumed by serveRefErrorCase.
func channelDebugRefCase(idx int, ref string) refErrorCase {
	h := channelDebugRefHandlers[idx]
	return refErrorCase{
		name:    h.name,
		handler: h.handler,
		method:  h.method,
		target:  "/api/debug/channel/" + ref + h.suffix,
		params:  gin.Params{{Key: "id", Value: ref}},
	}
}

// requireServerRefError asserts the server-error contract for a reference
// lookup that failed for a reason other than the request (for example a
// database outage): the /api/* HTTP 200 {success:false} envelope and exactly
// one ERROR "http handler error" entry, with nothing logged as a client error.
func requireServerRefError(t *testing.T, outcome refErrorOutcome) {
	t.Helper()
	require.Equal(t, http.StatusOK, outcome.status, "a lookup failure keeps the /api/* error envelope")
	require.Equal(t, false, outcome.body["success"])
	require.NotEmpty(t, outcome.body["message"])

	require.Empty(t, outcome.logs.FilterMessage("http handler client error").All(),
		"a database failure is not the caller's mistake")
	entries := outcome.logs.FilterMessage("http handler error").All()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.ErrorLevel, entries[0].Level)
	require.Equal(t, "unknown", entries[0].ContextMap()["error_kind"])
}

// requireNotFoundRefError asserts the not-found contract shared by the rest of
// the API: HTTP 200 {success:false} and one stackless WARN with error_kind
// not_found.
func requireNotFoundRefError(t *testing.T, outcome refErrorOutcome) {
	t.Helper()
	requireNotFoundRefErrorMessage(t, outcome, "resource reference not found")
}

// requireNotFoundRefErrorMessage asserts the not-found contract for an
// endpoint that answers with its own message. Parameters: t owns assertions,
// outcome is the handler result and message is the expected client message.
func requireNotFoundRefErrorMessage(t *testing.T, outcome refErrorOutcome, message string) {
	t.Helper()
	require.Equal(t, http.StatusOK, outcome.status)
	require.Equal(t, false, outcome.body["success"])
	require.Equal(t, message, outcome.body["message"])

	require.Empty(t, outcome.logs.FilterLevelExact(zapcore.ErrorLevel).All())
	entries := outcome.logs.FilterMessage("http handler client error").All()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
	require.Equal(t, "not_found", entries[0].ContextMap()["error_kind"])
	require.NotContains(t, entries[0].ContextMap(), "errorVerbose")
}

// TestChannelDebugMalformedRefIsClientError pins that a malformed channel
// reference is answered HTTP 400 at WARN, like every other resolver site.
func TestChannelDebugMalformedRefIsClientError(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	for idx := range channelDebugRefHandlers {
		tc := channelDebugRefCase(idx, "123")
		tc.message = "invalid resource reference"
		t.Run(tc.name, func(t *testing.T) {
			requireClientRefError(t, tc, serveRefErrorCase(t, tc))
		})
	}
}

// TestChannelDebugUnknownRefIsNotFound pins that a well-formed reference to a
// missing channel follows the API-wide not-found convention instead of being
// reported as a malformed id.
func TestChannelDebugUnknownRefIsNotFound(t *testing.T) {
	_, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	for idx := range channelDebugRefHandlers {
		tc := channelDebugRefCase(idx, "00000000-0000-4000-8000-000000000000")
		t.Run(tc.name, func(t *testing.T) {
			requireNotFoundRefError(t, serveRefErrorCase(t, tc))
		})
	}
}

// TestChannelDebugLookupFailureIsServerError reproduces a database outage
// during reference resolution: it used to answer HTTP 400 "Invalid channel ID"
// and log a WARN client error, hiding the outage from alerting.
func TestChannelDebugLookupFailureIsServerError(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	defer cleanup()
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	for idx := range channelDebugRefHandlers {
		tc := channelDebugRefCase(idx, fixture.channel.UUID)
		t.Run(tc.name, func(t *testing.T) {
			requireServerRefError(t, serveRefErrorCase(t, tc))
		})
	}
}

// failChannelRowLoads makes every ORM query against the channels table fail
// with injected while leaving raw UUID resolution untouched, so a test can
// fail the row load that follows a successful reference resolution.
// Parameters: t owns assertions and injected is the error to return.
func failChannelRowLoads(t *testing.T, injected error) {
	t.Helper()
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(
		"test:fail_channel_row_loads",
		func(db *gorm.DB) {
			if db.Statement.Table == "channels" {
				// AddError records the failure on the statement, which makes
				// gorm:query skip execution; its return value is that same error.
				_ = db.AddError(injected)
			}
		}))
}

// TestChannelMigrationStatusRowLoadFailure covers the row load that follows a
// successful resolution: a row deleted in between is a not-found reference,
// and any other failure is a server error. Both used to answer HTTP 404
// "Channel not found" at WARN.
func TestChannelMigrationStatusRowLoadFailure(t *testing.T) {
	statusIdx := len(channelDebugRefHandlers) - 1
	t.Run("row vanished", func(t *testing.T) {
		fixture, cleanup := setupUUIDContractTestEnvironment(t)
		defer cleanup()
		failChannelRowLoads(t, gorm.ErrRecordNotFound)
		requireNotFoundRefError(t, serveRefErrorCase(t, channelDebugRefCase(statusIdx, fixture.channel.UUID)))
	})
	t.Run("database failure", func(t *testing.T) {
		fixture, cleanup := setupUUIDContractTestEnvironment(t)
		defer cleanup()
		failChannelRowLoads(t, errors.New("injected channel load failure"))
		requireServerRefError(t, serveRefErrorCase(t, channelDebugRefCase(statusIdx, fixture.channel.UUID)))
	})
}
