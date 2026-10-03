package controller

// Additive keyset log-list routes (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4).
//
// These are SIBLING routes, not a mode of the existing ones. The compatibility
// contract freezes the legacy list's pagination, filters, sorts and exact
// `total`; making one handler emit two different envelopes chosen by a query
// parameter would put a conditional `total` on the wire and break that by
// construction. Two routes make it true by construction instead.
//
// The cursor is never an authorization grant: scope is re-derived from the
// authenticated principal on every request and supplied to the seal as
// associated data, so a cursor presented under a different scope simply fails
// to open.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/admission"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logcursor"
	"github.com/Laisky/one-api/model"
)

// logCursorVersion is the capability version clients negotiate.
const logCursorVersion = 1

// Route identifiers bound into a cursor so one cannot be replayed on the other.
const (
	logCursorEndpointAll  = "log.all"
	logCursorEndpointSelf = "log.self"
)

// countProbeGate and countExactGate bound concurrent counting work per node.
//
// The bounded probe and the explicit exact count have separate budgets because
// they are different costs: the probe stops at a bound, the exact count does
// not.
var (
	countProbeGate = admission.NewGate("log_count_probe",
		config.LogCountProbeMaxConcurrent, 50*time.Millisecond)
	countExactGate = admission.NewGate("log_count_exact",
		config.LogCountExactMaxConcurrent, 500*time.Millisecond)
)

// logCursorRequest is one parsed and validated cursor-list request.
type logCursorRequest struct {
	scope      model.LogListScope
	filter     model.LogListFilter
	endpoint   string
	pageSize   int
	cursor     string
	wantsExact bool
}

// parseLogCursorRequest validates a cursor-list request before any SQL is built.
//
// Parameters:
//   - c: the gin context.
//   - endpoint: the route identifier.
//   - scope: the authorization scope already derived from the request.
//
// Return values:
//   - logCursorRequest: the validated request.
//   - error: an invalid-request error describing the first rejected input.
func parseLogCursorRequest(c *gin.Context, endpoint string, scope model.LogListScope) (logCursorRequest, error) {
	out := logCursorRequest{scope: scope, endpoint: endpoint}

	if v := c.Query("v"); v != "" && v != strconv.Itoa(logCursorVersion) {
		return out, errkind.InvalidRequestErr(errors.Errorf("unsupported cursor capability version %q", v))
	}
	if c.Query("p") != "" {
		return out, errkind.InvalidRequestErr(errors.New("the cursor route does not accept a page number; use cursor"))
	}
	if sort := c.Query("sort"); sort != "" && sort != "created_at" {
		return out, errkind.InvalidRequestErr(errors.Errorf("unsupported cursor sort %q", sort))
	}
	if order := c.Query("order"); order != "" && order != "desc" {
		return out, errkind.InvalidRequestErr(errors.Errorf("unsupported cursor sort order %q", order))
	}

	size, err := strconv.Atoi(c.Query("size"))
	if err != nil || size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}
	out.pageSize = size

	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	filter := model.LogListFilter{
		LogType:        logType,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
		TokenName:      c.Query("token_name"),
	}
	if scope.Kind == model.LogListScopeAll {
		filter.Username = c.Query("username")
		// The channel reference is resolved BEFORE digesting, so a UUID and the
		// integer id it resolves to produce the same digest.
		channel, err := resolveOptionalChannelRef(c.Query("channel"))
		if err != nil {
			return out, err
		}
		filter.ChannelID = channel
	}
	out.filter = filter.Normalize(scope)

	out.cursor = c.Query("cursor")
	if len(out.cursor) > 0 && len(c.QueryArray("cursor")) != 1 {
		return out, errkind.InvalidRequestErr(errors.New("cursor must be supplied at most once"))
	}
	out.wantsExact = c.Query("count") == "exact"

	return out, nil
}

// serveLogCursorPage answers one cursor-list request.
//
// Parameters:
//   - c: the gin context.
//   - endpoint: the route identifier.
//   - scope: the authorization scope derived from this request.
//
// Return values: none; the response is written to c.
func serveLogCursorPage(c *gin.Context, endpoint string, scope model.LogListScope) {
	lg := gmw.GetLogger(c)

	if !config.LogCursorEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "log cursor pagination is disabled",
			"code":    "capability_disabled",
		})
		return
	}

	req, err := parseLogCursorRequest(c, endpoint, scope)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	digest := req.filter.DigestBytes(req.scope, req.endpoint)

	var anchor *model.LogCursorAnchor
	if req.cursor != "" {
		opened, err := logcursor.Open(req.cursor, string(req.scope.Kind), req.scope.SubjectUserID,
			req.endpoint, digest, time.Now().UTC())
		if err != nil {
			var reject *logcursor.RejectError
			code := string(logcursor.ReasonMalformed)
			if errors.As(err, &reject) {
				code = string(reject.Reason)
			}
			// A rejected cursor never reaches SQL.
			c.JSON(http.StatusOK, gin.H{
				"success":          false,
				"message":          "the cursor is no longer usable; restart the listing",
				"code":             code,
				"restart_required": true,
			})
			return
		}
		anchor = &model.LogCursorAnchor{CreatedAt: opened.CreatedAt, ID: opened.RowID}
	}

	ctx := gmw.Ctx(c)
	page, err := model.FetchLogCursorPage(ctx, req.scope, req.filter, anchor, req.pageSize)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	items, consumed, capped, oversized := encodeCursorItems(lg, model.LogCursorPageResponses(page))
	hasMore := page.HasMore
	if consumed < len(page.Logs) {
		// The page stopped short of the rows the query returned, so there is
		// more to read regardless of what the query itself reported.
		hasMore = true
	}

	var nextCursor string
	if hasMore && consumed > 0 {
		last := page.Logs[consumed-1]
		now := time.Now().UTC()
		token, sealErr := logcursor.Seal(logcursor.Cursor{
			CreatedAt:    last.CreatedAt,
			RowID:        int64(last.Id),
			FilterDigest: digest,
			IssuedAt:     now.Unix(),
			ExpiresAt:    now.Add(config.LogCursorTTL()).Unix(),
		}, string(req.scope.Kind), req.scope.SubjectUserID, req.endpoint)
		if sealErr != nil {
			helper.RespondError(c, sealErr)
			return
		}
		nextCursor = token
	}

	response := gin.H{
		"success":     true,
		"message":     "",
		"version":     logCursorVersion,
		"data":        items,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
		"count":       countPayload(c, lg, req),
	}
	if capped {
		response["bytes_capped"] = true
	}
	if oversized {
		response["oversized_record"] = true
	}

	c.JSON(http.StatusOK, response)
}

// countPayload computes the count object for a cursor page.
//
// Counting is bounded and gated: an ordinary page uses the probe, and an exact
// count is a separate, explicitly requested and separately budgeted operation.
//
// Parameters:
//   - c: the gin context, for cancellation.
//   - lg: the request logger.
//   - req: the validated request.
//
// Return values:
//   - gin.H: the count object.
func countPayload(c *gin.Context, lg glog.Logger, req logCursorRequest) gin.H {
	bound := config.LogCountProbeMaxRows
	budget := config.LogCountProbeTimeout()
	gate := countProbeGate
	if req.wantsExact {
		bound = config.LogCountExactMaxRows
		budget = config.LogCountExactTimeout()
		gate = countExactGate
	}

	ctx := gmw.Ctx(c)
	// The gate is taken BEFORE the budget starts, so queueing never consumes
	// the query's time.
	release, err := gate.Acquire(ctx)
	if err != nil {
		return gin.H{"value": nil, "quality": string(model.LogCountUnavailable), "as_of": time.Now().UTC().Unix(), "cached": false}
	}
	defer release()

	if cached, ok := lookupCachedLogCount(req); ok {
		return countToPayload(cached)
	}

	count, err := model.ProbeLogCount(ctx, req.scope, req.filter, bound, budget)
	if err != nil {
		lg.Debug("log count probe did not complete", zap.Error(err))
		return countToPayload(model.LogCount{Quality: model.LogCountUnavailable, AsOf: time.Now().UTC().Unix()})
	}

	storeCachedLogCount(req, count)
	return countToPayload(count)
}

// countToPayload renders a count for the wire.
//
// The value stays null when nothing was established, so a missing count can
// never be displayed as zero, and `as_of` always accompanies it so a cached
// exact count cannot be read as a live one.
//
// Parameters:
//   - count: the count to render.
//
// Return values:
//   - gin.H: the wire object.
func countToPayload(count model.LogCount) gin.H {
	return gin.H{
		"value":   count.Value,
		"quality": string(count.Quality),
		"as_of":   count.AsOf,
		"cached":  count.Cached,
	}
}

// GetAllLogsCursor lists logs site-wide using keyset pagination.
//
// Parameters:
//   - c: the gin context.
//
// Return values: none; the response is written to c.
func GetAllLogsCursor(c *gin.Context) {
	role := c.GetInt(ctxkey.Role)
	// Re-checked in the handler so the route does not silently depend on the
	// router still wiring an administrative guard in front of it.
	if role < model.RoleAdminUser {
		helper.RespondErrorWithStatus(c, http.StatusForbidden,
			errors.New("administrative privileges are required"))
		return
	}

	serveLogCursorPage(c, logCursorEndpointAll, model.LogListScope{
		Kind:            model.LogListScopeAll,
		PrincipalUserID: c.GetInt(ctxkey.Id),
		Role:            role,
	})
}

// GetUserLogsCursor lists the caller's own logs using keyset pagination.
//
// Parameters:
//   - c: the gin context.
//
// Return values: none; the response is written to c.
func GetUserLogsCursor(c *gin.Context) {
	userID := c.GetInt(ctxkey.Id)
	if userID <= 0 {
		helper.RespondErrorWithStatus(c, http.StatusUnauthorized, errors.New("authentication is required"))
		return
	}

	serveLogCursorPage(c, logCursorEndpointSelf, model.LogListScope{
		Kind:            model.LogListScopeSelf,
		SubjectUserID:   userID,
		PrincipalUserID: userID,
		Role:            c.GetInt(ctxkey.Role),
	})
}
