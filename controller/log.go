package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
)

// GetAllLogs lists logs across all users with pagination, filtering, and sorting options.
func GetAllLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	sortBy := c.DefaultQuery("sort_by", "")
	if sortBy == "" { // frontend sends 'sort'
		sortBy = c.Query("sort")
	}
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if c.Query("order") != "" { // frontend sends 'order'
		sortOrder = c.Query("order")
	}

	// Validate date range for sorting requests (max 30 days)
	if sortBy != "" && startTimestamp > 0 && endTimestamp > 0 {
		maxRange := int64(30 * 24 * 60 * 60) // 30 days in seconds
		if endTimestamp-startTimestamp > maxRange {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Date range for sorting cannot exceed 30 days",
			})
			return
		}
	}

	// Support both legacy 'items_per_page' and preferred 'size' param
	pageSizeStr := c.Query("size")
	if pageSizeStr == "" {
		pageSizeStr = c.Query("items_per_page")
	}
	itemsPerPage, err := strconv.Atoi(pageSizeStr)
	if err != nil || itemsPerPage <= 0 {
		itemsPerPage = config.DefaultItemsPerPage
	}
	if itemsPerPage > config.MaxItemsPerPage {
		itemsPerPage = config.MaxItemsPerPage
	}

	logs, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, p*itemsPerPage, itemsPerPage, channel, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetAllLogsCount(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
		"total":   totalCount,
	})
}

// GetUserLogs lists logs scoped to the current user, honoring filter and sorting options.
func GetUserLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	userId := c.GetInt(ctxkey.Id)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	sortBy := c.DefaultQuery("sort_by", "")
	if sortBy == "" { // frontend fallback
		sortBy = c.Query("sort")
	}
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if c.Query("order") != "" {
		sortOrder = c.Query("order")
	}

	// Validate date range for sorting requests (max 30 days)
	if sortBy != "" && startTimestamp > 0 && endTimestamp > 0 {
		maxRange := int64(30 * 24 * 60 * 60) // 30 days in seconds
		if endTimestamp-startTimestamp > maxRange {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Date range for sorting cannot exceed 30 days",
			})
			return
		}
	}

	// Get page size from query parameter, default to config value
	size, err := strconv.Atoi(c.Query("size"))
	if err != nil || size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	logs, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, p*size, size, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetUserLogsCount(userId, logType, startTimestamp, endTimestamp, modelName, tokenName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
		"total":   totalCount,
	})
}

// GetTokenLogs lists logs associated with the specific token used for authentication,
// scoped to its owning user.
func GetTokenLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	userId := c.GetInt(ctxkey.Id)
	tokenName := c.GetString(ctxkey.TokenName)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")

	// Get page size from query parameter, default to config value
	size, err := strconv.Atoi(c.Query("size"))
	if err != nil || size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	logs, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, p*size, size, "", "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	totalCount, err := model.GetUserLogsCount(userId, logType, startTimestamp, endTimestamp, modelName, tokenName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
		"total":   totalCount,
	})
}

// SearchAllLogs performs full-text search across all logs and returns paginated results.
func SearchAllLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	logs, total, err := model.SearchAllLogs(keyword, p*size, size, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
		"total":   total,
	})
}

// SearchUserLogs searches logs belonging to the current user and returns paginated results.
func SearchUserLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	userId := c.GetInt(ctxkey.Id)
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	logs, total, err := model.SearchUserLogs(userId, keyword, p*size, size, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
		"total":   total,
	})
}

// GetLogsStat summarizes quota usage metrics across logs matching the provided filters.
func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	quotaNum := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel)
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum,
			//"token": tokenNum,
		},
	})
}

// GetLogsSelfStat reports quota usage metrics for the authenticated user over the requested range.
func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString(ctxkey.Username)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	quotaNum := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel)
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum,
			//"token": tokenNum,
		},
	})
}

// DeleteHistoryLogs purges log entries older than the provided timestamp.
func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(targetTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}
