package controller

import (
	"fmt"
	"net/http"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/utils"
	"github.com/Laisky/one-api/model"
)

// GetUserDashboard returns per-day per-model usage statistics and quota info.
// Date Range Semantics:
//
//	The API accepts `from_date` and `to_date` in YYYY-MM-DD format (UTC) and
//	interprets them as an inclusive range of whole days. Internally this is
//	converted into a half-open Unix second interval: [from_date 00:00:00 UTC, to_date+1 00:00:00 UTC).
//	This guarantees that the entire final day is included without relying on
//	second-based inclusivity or adding 24h-1s hacks, eliminating off-by-one
//	errors and DST complications.
//	Maximum range: regular users 7 days, administrators/root users 365 days (subject to the site-wide cap).
//
// Parameters: c carries the authenticated identity and requested date/user scope.
// Return values: none; writes the dashboard envelope without changing user state.
func GetUserDashboard(c *gin.Context) {
	lg := gmw.GetLogger(c)
	id := c.GetInt(ctxkey.Id)
	role := c.GetInt(ctxkey.Role)
	now := time.Now()

	// Parse date range parameters
	fromDateStr := c.Query("from_date")
	toDateStr := c.Query("to_date")

	// We will use half-open interval: [startTs, endTsExclusive)
	// to avoid off-by-one second issues and ensure full-day coverage.
	var startTs, endTsExclusive int64

	if fromDateStr != "" && toDateStr != "" {
		maxDays := 7
		if role >= model.RoleAdminUser {
			maxDays = 365
		}
		s, e, err := utils.NormalizeDateRange(fromDateStr, toDateStr, maxDays)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
			return
		}
		startTs = s
		endTsExclusive = e
	} else {
		// Default last 7 days including today: [today-6, today]
		today := now.UTC().Truncate(24 * time.Hour)
		startTs = today.AddDate(0, 0, -6).Unix()
		endTsExclusive = today.Add(24 * time.Hour).Unix()
	}

	// Check if user wants to view specific user's data (administrators and root users only)
	targetUserId := id // Default to current user
	userIdParam := c.Query("user_id")

	if userIdParam != "" {
		// Administrators and root users can view other users' data or site-wide data
		if role < model.RoleAdminUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "No permission to view other users' dashboard data",
				"data":    nil,
			})
			return
		}

		if userIdParam == "all" {
			targetUserId = 0 // 0 means site-wide statistics
		} else {
			var err error
			targetUserId, err = resolveUserRef(userIdParam)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Invalid user_id parameter",
					"data":    nil,
				})
				return
			}
		}
	} else if role >= model.RoleAdminUser {
		// For administrators and root users, default to site-wide statistics
		targetUserId = 0
	}

	// A site-wide aggregate scans every consume log in the window, so an
	// unbounded range is a guaranteed timeout once the table is large. The cap
	// only applies to site-wide queries; per-user ranges are unaffected.
	if targetUserId == 0 {
		if days := sitewideRangeDays(startTs, endTsExclusive); days > config.DashboardMaxSitewideRangeDays {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf(
					"Site-wide dashboard range is limited to %d days (requested %d). Narrow the range or raise DASHBOARD_MAX_SITEWIDE_RANGE_DAYS.",
					config.DashboardMaxSitewideRangeDays, days),
				"data": nil,
			})
			return
		}
	}

	// Get log statistics, using half-open interval [startTs, endTsExclusive).
	aggregates, err := resolveDashboardAggregates(gmw.Ctx(c), targetUserId, startTs, endTsExclusive)
	if err != nil {
		lg.Error("failed to get dashboard data", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to get dashboard data",
			"data":    nil,
		})
		return
	}

	// Get quota and status information
	var totalQuota, usedQuota int64
	var status string

	if targetUserId == 0 {
		// Site-wide statistics for admin/root users. Cached in-process: it is a
		// full aggregate over the users table, which no index can make cheap
		// once there are a million rows.
		totalQuota, usedQuota, status, err = model.GetSiteWideQuotaStatsCached()
		if err != nil {
			lg.Error("failed to get site-wide quota stats", zap.Error(err))
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to get site-wide quota stats",
				"data":    nil,
			})
			return
		}
	} else {
		// Individual user statistics
		user, err := model.GetUserById(targetUserId, false)
		if err != nil {
			lg.Error("failed to get user data", zap.Error(err))
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to get user data",
				"data":    nil,
			})
			return
		}
		totalQuota = user.Quota
		usedQuota = user.UsedQuota
		switch user.Status {
		case model.UserStatusEnabled:
			status = "Active"
		case model.UserStatusDisabled:
			status = "Disabled"
		case model.UserStatusDeleted:
			status = "Deleted"
		default:
			status = "Unknown"
		}
	}

	// Create response with both log data and quota/status info
	response := gin.H{
		"logs":            aggregates.Logs,
		"user_logs":       aggregates.UserLogs,
		"token_logs":      aggregates.TokenLogs,
		"tool_logs":       aggregates.ToolLogs,
		"tool_user_logs":  aggregates.ToolUserLogs,
		"tool_token_logs": aggregates.ToolTokenLogs,
		"total_quota":     totalQuota,
		"used_quota":      usedQuota,
		"status":          status,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetDashboardUsers lists public user references for administrator reporting.
// Parameters: c carries the authenticated viewer. Returns: no value; only UUID,
// username and display name are emitted, never credentials or internal IDs.
func GetDashboardUsers(c *gin.Context) {
	lg := gmw.GetLogger(c)
	role := c.GetInt(ctxkey.Role)

	// Administrators and root users can access this endpoint
	if role < model.RoleAdminUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to access user list",
			"data":    nil,
		})
		return
	}

	// Get all users with basic info (id, username, display_name)
	users, err := model.GetAllUsers(0, 1000, "", "", "") // Get up to 1000 users
	if err != nil {
		lg.Error("failed to get user list", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to get user list",
			"data":    nil,
		})
		return
	}

	// Create simplified user list for dropdown
	type UserOption struct {
		UUID        string `json:"uuid"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	}

	var userOptions []UserOption
	// Add "All Users" option first
	userOptions = append(userOptions, UserOption{
		UUID:        "all",
		Username:    "all",
		DisplayName: "All Users (Site-wide)",
	})

	// Add individual users
	for _, user := range users {
		userOptions = append(userOptions, UserOption{
			UUID:        user.UUID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    userOptions,
	})
}

// sitewideRangeDays converts a half-open second range into whole days, rounding
// up so a partial day still counts against the cap.
//
// Parameters:
//   - startTs: inclusive start, in Unix seconds.
//   - endTsExclusive: exclusive end, in Unix seconds.
//
// Return values:
//   - int: the number of days the range spans; 0 for an empty or inverted range.
func sitewideRangeDays(startTs, endTsExclusive int64) int {
	const secondsPerDay = 24 * 60 * 60
	if endTsExclusive <= startTs {
		return 0
	}
	return int((endTsExclusive - startTs + secondsPerDay - 1) / secondsPerDay)
}
