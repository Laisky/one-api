package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

func TestGetAllLogs_SortingValidation(t *testing.T) {
	// initialize databases to avoid nil LOG_DB panics
	model.InitDB()
	model.InitLogDB()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/log/", GetAllLogs)

	tests := []struct {
		name           string
		sortBy         string
		sortOrder      string
		startTime      int64
		endTime        int64
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid sorting parameters",
			sortBy:         "quota",
			sortOrder:      "desc",
			startTime:      time.Now().Unix() - 7*24*60*60, // 7 days ago
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid sort column",
			sortBy:         "invalid_column",
			sortOrder:      "desc",
			startTime:      time.Now().Unix() - 7*24*60*60,
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK, // Should fallback to default sorting
		},
		{
			name:           "Date range exceeds 30 days",
			sortBy:         "quota",
			sortOrder:      "desc",
			startTime:      time.Now().Unix() - 35*24*60*60, // 35 days ago
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK,
			expectedError:  "Date range for sorting cannot exceed 30 days",
		},
		{
			name:           "Valid sort orders",
			sortBy:         "elapsed_time",
			sortOrder:      "asc",
			startTime:      time.Now().Unix() - 7*24*60*60,
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid sort order defaults to desc",
			sortBy:         "prompt_tokens",
			sortOrder:      "invalid",
			startTime:      time.Now().Unix() - 7*24*60*60,
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request URL with parameters
			params := url.Values{}
			params.Add("sort_by", tt.sortBy)
			params.Add("sort_order", tt.sortOrder)
			params.Add("start_timestamp", strconv.FormatInt(tt.startTime, 10))
			params.Add("end_timestamp", strconv.FormatInt(tt.endTime, 10))
			params.Add("p", "0")
			params.Add("page_size", "10")

			req := httptest.NewRequest("GET", "/api/log/?"+params.Encode(), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				success, exists := response["success"]
				assert.True(t, exists)
				assert.False(t, success.(bool))

				message, exists := response["message"]
				assert.True(t, exists)
				assert.Contains(t, message.(string), tt.expectedError)
			}
		})
	}
}

func TestGetAllLogs_SortFallbackParams(t *testing.T) {
	model.InitDB()
	model.InitLogDB()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/log/", GetAllLogs)

	params := url.Values{}
	// Use new frontend style params 'sort' & 'order' only
	params.Add("sort", "quota")
	params.Add("order", "asc")
	params.Add("start_timestamp", strconv.FormatInt(time.Now().Add(-24*time.Hour).Unix(), 10))
	params.Add("end_timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	params.Add("p", "0")
	params.Add("size", "5")

	req := httptest.NewRequest("GET", "/api/log/?"+params.Encode(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Response should be JSON with success field
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	_, ok := response["success"]
	assert.True(t, ok)
}

func TestGetUserLogs_SortingValidation(t *testing.T) {
	model.InitDB()
	model.InitLogDB()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/log/self", func(c *gin.Context) {
		c.Set(ctxkey.Id, 1) // inject test user id
		GetUserLogs(c)
	})

	tests := []struct {
		name           string
		sortBy         string
		sortOrder      string
		startTime      int64
		endTime        int64
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid user log sorting",
			sortBy:         "completion_tokens",
			sortOrder:      "asc",
			startTime:      time.Now().Unix() - 7*24*60*60,
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "User log date range validation",
			sortBy:         "quota",
			sortOrder:      "desc",
			startTime:      time.Now().Unix() - 40*24*60*60, // 40 days ago
			endTime:        time.Now().Unix(),
			expectedStatus: http.StatusOK,
			expectedError:  "Date range for sorting cannot exceed 30 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := url.Values{}
			params.Add("sort_by", tt.sortBy)
			params.Add("sort_order", tt.sortOrder)
			params.Add("start_timestamp", strconv.FormatInt(tt.startTime, 10))
			params.Add("end_timestamp", strconv.FormatInt(tt.endTime, 10))
			params.Add("p", "0")
			params.Add("page_size", "10")

			req := httptest.NewRequest("GET", "/api/log/self?"+params.Encode(), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				success, exists := response["success"]
				assert.True(t, exists)
				assert.False(t, success.(bool))
			}
		})
	}
}

func TestLogOrderClause(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		expected  string
	}{
		{"Valid quota desc", "quota", "desc", "quota desc"},
		{"Valid prompt_tokens asc", "prompt_tokens", "asc", "prompt_tokens asc"},
		{"Valid completion_tokens desc", "completion_tokens", "desc", "completion_tokens desc"},
		{"Valid elapsed_time asc", "elapsed_time", "asc", "elapsed_time asc"},
		{"Valid created_time desc", "created_time", "desc", "created_at desc"},
		{"Invalid sort column", "invalid", "desc", "id desc"},
		{"Invalid sort order", "quota", "invalid", "quota desc"},
		{"Empty sort by", "", "asc", "id desc"},
		{"Empty sort order", "quota", "", "quota desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.GetLogOrderClause(tt.sortBy, tt.sortOrder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDateRangeValidation(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name      string
		startTime int64
		endTime   int64
		sortBy    string
		isValid   bool
	}{
		{
			name:      "Valid 7 day range with sorting",
			startTime: now - 7*24*60*60,
			endTime:   now,
			sortBy:    "quota",
			isValid:   true,
		},
		{
			name:      "Valid 30 day range with sorting",
			startTime: now - 30*24*60*60,
			endTime:   now,
			sortBy:    "elapsed_time",
			isValid:   true,
		},
		{
			name:      "Invalid 31 day range with sorting",
			startTime: now - 31*24*60*60,
			endTime:   now,
			sortBy:    "quota",
			isValid:   false,
		},
		{
			name:      "Invalid 60 day range with sorting",
			startTime: now - 60*24*60*60,
			endTime:   now,
			sortBy:    "prompt_tokens",
			isValid:   false,
		},
		{
			name:      "No sorting - any range allowed",
			startTime: now - 60*24*60*60,
			endTime:   now,
			sortBy:    "",
			isValid:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic
			maxRange := int64(30 * 24 * 60 * 60) // 30 days in seconds
			isValid := true

			if tt.sortBy != "" && tt.startTime > 0 && tt.endTime > 0 {
				if tt.endTime-tt.startTime > maxRange {
					isValid = false
				}
			}

			assert.Equal(t, tt.isValid, isValid,
				fmt.Sprintf("Date range validation failed for %s", tt.name))
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkGetLogOrderClause(b *testing.B) {
	testCases := []struct {
		sortBy    string
		sortOrder string
	}{
		{"quota", "desc"},
		{"prompt_tokens", "asc"},
		{"completion_tokens", "desc"},
		{"elapsed_time", "asc"},
		{"invalid", "desc"},
	}

	for _, tc := range testCases {
		b.Run(fmt.Sprintf("%s_%s", tc.sortBy, tc.sortOrder), func(b *testing.B) {
			for b.Loop() {
				model.GetLogOrderClause(tc.sortBy, tc.sortOrder)
			}
		})
	}
}
