package controller

// Tests for the additive keyset log routes (W2.4 steps 6 and 7).
//
// The properties that matter most: the legacy routes are untouched, a cursor is
// never an authorization grant, a rejected cursor never reaches SQL, and a
// count never claims more than it established.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// cursorTestEnv provisions an isolated log database seeded with usage rows.
//
// Parameters:
//   - t: the test handle.
//   - rows: how many consume rows to seed for user 1.
//
// Return values: none.
func cursorTestEnv(t *testing.T, rows int) {
	t.Helper()

	dir := t.TempDir()
	logDB, err := gorm.Open(sqlite.Open(dir+"/logs.db"), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))

	primary, err := gorm.Open(sqlite.Open(dir+"/primary.db"), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, primary.AutoMigrate(&model.Channel{}))

	prevDB, prevLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = primary, logDB
	t.Cleanup(func() { model.DB, model.LOG_DB = prevDB, prevLogDB })

	base := time.Now().UTC().Unix() - int64(rows) - 10
	for i := range rows {
		require.NoError(t, logDB.Exec(
			"INSERT INTO logs (user_id, type, created_at, model_name, token_name, content) VALUES (?,?,?,?,?,?)",
			1, model.LogTypeConsume, base+int64(i), "gpt-4.1", "prod", fmt.Sprintf("row-%03d", i)).Error)
	}
	// The capability ships OFF by default because the keyset order needs an
	// access path the schema does not yet have; these tests are about the
	// capability itself, so they turn it on explicitly.
	prevEnabled := config.LogCursorEnabled
	config.LogCursorEnabled = true
	t.Cleanup(func() { config.LogCursorEnabled = prevEnabled })

	ResetLogCountCache()
	t.Cleanup(ResetLogCountCache)
}

// cursorGet performs one request against a cursor handler.
//
// Parameters:
//   - t: the test handle.
//   - handler: the handler under test.
//   - target: the request target including query string.
//   - userID: the authenticated user id.
//   - role: the authenticated role.
//
// Return values:
//   - map[string]any: the decoded response body.
func cursorGet(t *testing.T, handler gin.HandlerFunc, target string, userID, role int) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("cursor-test")),
	))
	engine.GET("/probe", func(c *gin.Context) {
		c.Set(ctxkey.Id, userID)
		c.Set(ctxkey.Role, role)
		handler(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe"+target, http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// TestCursorTraversalCoversEveryRowExactlyOnce walks the full listing through
// the HTTP surface and verifies the traversal is complete and duplicate-free.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorTraversalCoversEveryRowExactlyOnce(t *testing.T) {
	const rows = 37
	cursorTestEnv(t, rows)

	seen := map[string]int{}
	target := "?size=5"
	pages := 0

	for {
		body := cursorGet(t, GetUserLogsCursor, target, 1, model.RoleCommonUser)
		require.True(t, body["success"].(bool), "body: %v", body)
		pages++

		items := body["data"].([]any)
		for _, raw := range items {
			row := raw.(map[string]any)
			seen[row["content"].(string)]++
		}

		if !body["has_more"].(bool) {
			require.Empty(t, body["next_cursor"], "the terminal page carries no cursor")
			break
		}
		next := body["next_cursor"].(string)
		require.NotEmpty(t, next)
		target = "?size=5&cursor=" + next
		require.Less(t, pages, 100, "traversal must terminate")
	}

	require.Len(t, seen, rows, "every row must appear exactly once")
	for content, count := range seen {
		require.Equal(t, 1, count, "row %s appeared %d times", content, count)
	}
}

// TestCursorRejectsTamperingAndScopeChange verifies a cursor is unusable when
// altered, replayed by another user, or moved to the other route, and that the
// response is a restart instruction rather than rows.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRejectsTamperingAndScopeChange(t *testing.T) {
	cursorTestEnv(t, 20)

	first := cursorGet(t, GetUserLogsCursor, "?size=5", 1, model.RoleCommonUser)
	cursor := first["next_cursor"].(string)
	require.NotEmpty(t, cursor)

	cases := map[string]struct {
		handler gin.HandlerFunc
		target  string
		userID  int
		role    int
	}{
		"flipped character": {GetUserLogsCursor, "?size=5&cursor=" + flipLastChar(cursor), 1, model.RoleCommonUser},
		"truncated":         {GetUserLogsCursor, "?size=5&cursor=" + cursor[:len(cursor)-6], 1, model.RoleCommonUser},
		"replayed by other": {GetUserLogsCursor, "?size=5&cursor=" + cursor, 2, model.RoleCommonUser},
		"moved to admin":    {GetAllLogsCursor, "?size=5&cursor=" + cursor, 1, model.RoleRootUser},
		"changed filter":    {GetUserLogsCursor, "?size=5&model_name=other&cursor=" + cursor, 1, model.RoleCommonUser},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := cursorGet(t, tc.handler, tc.target, tc.userID, tc.role)
			require.False(t, body["success"].(bool), "a rejected cursor must not return rows")
			require.True(t, body["restart_required"].(bool))
			require.NotEmpty(t, body["code"])
			require.Nil(t, body["data"], "a rejected cursor must never reach SQL")
		})
	}
}

// flipLastChar returns the token with its final character changed.
//
// Parameters:
//   - token: the token to alter.
//
// Return values:
//   - string: the altered token.
func flipLastChar(token string) string {
	if len(token) == 0 {
		return token
	}
	last := token[len(token)-1]
	replacement := byte('A')
	if last == 'A' {
		replacement = 'B'
	}
	return token[:len(token)-1] + string(replacement)
}

// TestCursorRouteRejectsUnsupportedInput verifies invalid requests are refused
// before any SQL runs.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRouteRejectsUnsupportedInput(t *testing.T) {
	cursorTestEnv(t, 5)

	for name, target := range map[string]string{
		"page number": "?p=2",
		"bad version": "?v=9",
		"bad sort":    "?sort=quota",
		"bad order":   "?order=asc",
	} {
		t.Run(name, func(t *testing.T) {
			body := cursorGet(t, GetUserLogsCursor, target, 1, model.RoleCommonUser)
			require.False(t, body["success"].(bool))
		})
	}
}

// TestCursorCountQuality verifies the count reports what it established and
// never renders an unavailable count as a number.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorCountQuality(t *testing.T) {
	const rows = 12
	cursorTestEnv(t, rows)

	body := cursorGet(t, GetUserLogsCursor, "?size=5", 1, model.RoleCommonUser)
	count := body["count"].(map[string]any)

	require.Equal(t, string(model.LogCountExact), count["quality"])
	require.EqualValues(t, rows, count["value"])
	require.Positive(t, count["as_of"])
	require.False(t, count["cached"].(bool), "the first count of a page is computed, not cached")

	// The second identical request is served from the cache and says so.
	second := cursorGet(t, GetUserLogsCursor, "?size=5", 1, model.RoleCommonUser)
	secondCount := second["count"].(map[string]any)
	require.True(t, secondCount["cached"].(bool),
		"a reused count must be labelled cached so it is not read as live")
	require.Equal(t, count["as_of"], secondCount["as_of"],
		"a cached count keeps the instant it was actually taken")
}

// TestCursorProvisionalRowsAreNeverListed verifies the unconditional
// provisional exclusion holds on the additive route, including for an explicit
// type=6 request.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorProvisionalRowsAreNeverListed(t *testing.T) {
	cursorTestEnv(t, 3)

	require.NoError(t, model.LOG_DB.Exec(
		"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
		1, model.LogTypeProvisional, time.Now().UTC().Unix(), "prod", "provisional-row").Error)

	body := cursorGet(t, GetUserLogsCursor, "?size=50", 1, model.RoleCommonUser)
	for _, raw := range body["data"].([]any) {
		require.NotEqual(t, "provisional-row", raw.(map[string]any)["content"])
	}

	explicit := cursorGet(t, GetUserLogsCursor, "?size=50&type=6", 1, model.RoleCommonUser)
	require.Empty(t, explicit["data"], "an explicit type=6 request must select nothing")
	require.False(t, explicit["has_more"].(bool))
}

// TestCursorRouteRequiresAdminForSiteWide verifies the site-wide route enforces
// its own privilege check rather than relying on the router alone.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRouteRequiresAdminForSiteWide(t *testing.T) {
	cursorTestEnv(t, 3)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("cursor-test")),
	))
	engine.GET("/probe", func(c *gin.Context) {
		c.Set(ctxkey.Id, 1)
		c.Set(ctxkey.Role, model.RoleCommonUser)
		GetAllLogsCursor(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code,
		"the handler must refuse a non-admin even without the router guard")
}

// TestCursorByteCapKeepsTraversalComplete verifies the response byte cap cuts
// pages short without dropping, duplicating or truncating any row.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorByteCapKeepsTraversalComplete(t *testing.T) {
	const rows = 23
	cursorTestEnv(t, rows)

	prev := config.LogCursorMaxResponseBytes
	// Small enough that a requested page of 50 never fits whole.
	config.LogCursorMaxResponseBytes = 900
	t.Cleanup(func() { config.LogCursorMaxResponseBytes = prev })

	seen := map[string]int{}
	target := "?size=50"
	pages, capped := 0, 0

	for {
		body := cursorGet(t, GetUserLogsCursor, target, 1, model.RoleCommonUser)
		require.True(t, body["success"].(bool), "body: %v", body)
		pages++

		items := body["data"].([]any)
		require.NotEmpty(t, items, "a page must always make forward progress")
		for _, raw := range items {
			row := raw.(map[string]any)
			content := row["content"].(string)
			seen[content]++
			require.NotContains(t, content, "…", "no field may be truncated")
		}

		if wasCapped, ok := body["bytes_capped"].(bool); ok && wasCapped {
			capped++
			require.True(t, body["has_more"].(bool),
				"a page cut short by the byte cap always has more to read")
		}

		if !body["has_more"].(bool) {
			break
		}
		target = "?size=50&cursor=" + body["next_cursor"].(string)
		require.Less(t, pages, 100, "traversal must terminate")
	}

	require.Positive(t, capped, "the budget must actually have bound at least one page")
	require.Greater(t, pages, 1, "the cap must have split the listing")
	require.Len(t, seen, rows, "every row must still appear exactly once")
	for content, count := range seen {
		require.Equal(t, 1, count, "row %s appeared %d times", content, count)
	}
}

// TestCursorOversizedRecordIsFlaggedNotTruncated verifies a single record
// larger than the whole budget is returned intact and labelled.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorOversizedRecordIsFlaggedNotTruncated(t *testing.T) {
	cursorTestEnv(t, 0)

	huge := strings.Repeat("x", 4096)
	require.NoError(t, model.LOG_DB.Exec(
		"INSERT INTO logs (user_id, type, created_at, token_name, content) VALUES (?,?,?,?,?)",
		1, model.LogTypeConsume, time.Now().UTC().Unix(), "prod", huge).Error)

	prev := config.LogCursorMaxResponseBytes
	config.LogCursorMaxResponseBytes = 64
	t.Cleanup(func() { config.LogCursorMaxResponseBytes = prev })

	body := cursorGet(t, GetUserLogsCursor, "?size=50", 1, model.RoleCommonUser)
	require.True(t, body["success"].(bool))
	require.True(t, body["oversized_record"].(bool), "the caller must be told why the page is one row")

	items := body["data"].([]any)
	require.Len(t, items, 1)
	require.Equal(t, huge, items[0].(map[string]any)["content"],
		"an oversized record is returned whole, never shortened")
}

// TestCursorRouteReportsDisabledCapability verifies the default-off capability
// answers with a capability marker rather than with an empty page, so a client
// can tell "no logs" from "not offered here".
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCursorRouteReportsDisabledCapability(t *testing.T) {
	cursorTestEnv(t, 5)
	config.LogCursorEnabled = false

	body := cursorGet(t, GetUserLogsCursor, "?size=5", 1, model.RoleCommonUser)
	require.False(t, body["success"].(bool))
	require.Equal(t, "capability_disabled", body["code"])
	require.Nil(t, body["data"], "a disabled capability must never look like an empty log list")
	require.NotContains(t, body, "restart_required",
		"a disabled capability is not a cursor problem the client can restart out of")
}
