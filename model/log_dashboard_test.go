package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/dto"
)

// seedToolLogs writes count rows of a tool invocation directly using the
// LogTypeTool write helper. Each row carries the per-invocation cost so the
// dashboard's COUNT/SUM aggregation produces predictable values.
func seedToolLogs(t *testing.T, base *Log, count int, totalCost int64) {
	t.Helper()
	require.NotNil(t, base)
	require.Greater(t, count, 0)

	summary := &ToolUsageSummary{
		Counts:     map[string]int{base.ModelName: count},
		CostByTool: map[string]int64{base.ModelName: totalCost},
		TotalCost:  totalCost,
	}
	// Snapshot timestamp/username so all rows share the same audit fields.
	captured := *base
	if captured.CreatedAt == 0 {
		captured.CreatedAt = time.Now().Unix()
	}
	// Bypass RecordToolLogs's now-stamping so the test can place rows on a
	// specific day; write rows directly.
	perCall := int64(0)
	remainder := int64(0)
	if count > 0 {
		perCall = totalCost / int64(count)
		remainder = totalCost % int64(count)
	}
	_ = summary // kept for readability of intent
	for i := 0; i < count; i++ {
		quota := perCall
		if i == 0 {
			quota += remainder
		}
		row := captured
		row.Type = LogTypeTool
		row.Quota = int(quota)
		row.Content = fmt.Sprintf("test-dashboard-agg-tool-%s-%d", captured.ModelName, i)
		row.Id = 0
		require.NoError(t, LOG_DB.Create(&row).Error)
	}
}

func TestDashboardAggregations(t *testing.T) {
	setupTestDatabase(t)

	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE 'test-dashboard-agg-%'").Error)
	t.Cleanup(func() {
		LOG_DB.Exec("DELETE FROM logs WHERE content LIKE 'test-dashboard-agg-%'")
	})

	base := time.Now().UTC().Truncate(24 * time.Hour)
	day1 := base.Add(-24 * time.Hour)
	day2 := base

	// Model-billing rows (LogTypeConsume) keep their existing semantics.
	logs := []Log{
		{
			UserId:           101,
			Username:         "alice",
			TokenName:        "alpha",
			ModelName:        "gpt-4",
			Type:             LogTypeConsume,
			CreatedAt:        day1.Unix() + 3600,
			Quota:            100,
			PromptTokens:     70,
			CompletionTokens: 30,
		},
		{
			UserId:           102,
			Username:         "bob",
			TokenName:        "beta",
			ModelName:        "gpt-3.5-turbo",
			Type:             LogTypeConsume,
			CreatedAt:        day1.Unix() + 7200,
			Quota:            50,
			PromptTokens:     40,
			CompletionTokens: 10,
		},
		{
			UserId:           101,
			Username:         "alice",
			TokenName:        "alpha",
			ModelName:        "gpt-4",
			Type:             LogTypeConsume,
			CreatedAt:        day2.Unix() + 3600,
			Quota:            80,
			PromptTokens:     50,
			CompletionTokens: 30,
		},
		{
			UserId:           102,
			Username:         "bob",
			TokenName:        "gamma",
			ModelName:        "gpt-3.5-turbo",
			Type:             LogTypeConsume,
			CreatedAt:        day2.Unix() + 7200,
			Quota:            120,
			PromptTokens:     90,
			CompletionTokens: 30,
		},
		{
			UserId:           101,
			Username:         "alice",
			TokenName:        "delta",
			ModelName:        "gpt-4",
			Type:             LogTypeConsume,
			CreatedAt:        day2.Unix() + 9000,
			Quota:            30,
			PromptTokens:     20,
			CompletionTokens: 10,
		},
	}

	for i := range logs {
		logs[i].Content = fmt.Sprintf("test-dashboard-agg-%d", i)
		require.NoError(t, LOG_DB.Create(&logs[i]).Error)
	}

	// Tool invocation rows (LogTypeTool). Counts/Costs match what the old
	// metadata-based test asserted so downstream assertions stay equivalent.
	type seed struct {
		userId    int
		username  string
		tokenName string
		modelName string
		ts        int64
		count     int
		totalCost int64
	}
	seeds := []seed{
		// day1: alice/alpha used web_search x2 ($30) + calculator x1 ($15)
		{101, "alice", "alpha", "web_search", day1.Unix() + 3600, 2, 30},
		{101, "alice", "alpha", "calculator", day1.Unix() + 3600, 1, 15},
		// day1: bob/beta used web_search x1 ($12)
		{102, "bob", "beta", "web_search", day1.Unix() + 7200, 1, 12},
		// day2: alice/alpha used web_search x1 ($10) + file_read x2 ($15)
		{101, "alice", "alpha", "web_search", day2.Unix() + 3600, 1, 10},
		{101, "alice", "alpha", "file_read", day2.Unix() + 3600, 2, 15},
		// day2: alice/delta used calculator x2 ($8)
		{101, "alice", "delta", "calculator", day2.Unix() + 9000, 2, 8},
	}
	for _, s := range seeds {
		seedToolLogs(t, &Log{
			UserId:    s.userId,
			Username:  s.username,
			TokenName: s.tokenName,
			ModelName: s.modelName,
			CreatedAt: s.ts,
		}, s.count, s.totalCost)
	}

	start := int(day1.Unix())
	end := int(base.Add(24 * time.Hour).Unix())

	modelStats, err := SearchLogsByDayAndModel(0, start, end)
	require.NoError(t, err)

	day1Str := day1.Format("2006-01-02")
	day2Str := day2.Format("2006-01-02")

	modelByKey := make(map[string]*dto.LogStatistic)
	for _, stat := range modelStats {
		modelByKey[fmt.Sprintf("%s|%s", stat.Day, stat.ModelName)] = stat
	}

	require.Equal(t, 4, len(modelStats))

	mDay2 := modelByKey[fmt.Sprintf("%s|%s", day2Str, "gpt-4")]
	require.NotNil(t, mDay2)
	require.Equal(t, 2, mDay2.RequestCount)
	require.Equal(t, 110, mDay2.Quota)
	require.Equal(t, 70, mDay2.PromptTokens)
	require.Equal(t, 40, mDay2.CompletionTokens)

	mDay1 := modelByKey[fmt.Sprintf("%s|%s", day1Str, "gpt-4")]
	require.NotNil(t, mDay1)
	require.Equal(t, 1, mDay1.RequestCount)
	require.Equal(t, 100, mDay1.Quota)

	userStats, err := SearchLogsByDayAndUser(0, start, end)
	require.NoError(t, err)

	userByKey := make(map[string]*dto.LogStatisticByUser)
	for _, stat := range userStats {
		userByKey[fmt.Sprintf("%s|%s", stat.Day, stat.Username)] = stat
	}

	aliceDay2 := userByKey[fmt.Sprintf("%s|%s", day2Str, "alice")]
	require.NotNil(t, aliceDay2)
	require.Equal(t, 2, aliceDay2.RequestCount)
	require.Equal(t, 110, aliceDay2.Quota)

	bobDay1 := userByKey[fmt.Sprintf("%s|%s", day1Str, "bob")]
	require.NotNil(t, bobDay1)
	require.Equal(t, 50, bobDay1.Quota)

	tokenStats, err := SearchLogsByDayAndToken(0, start, end)
	require.NoError(t, err)

	tokenByKey := make(map[string]*dto.LogStatisticByToken)
	for _, stat := range tokenStats {
		tokenByKey[fmt.Sprintf("%s|%s|%s", stat.Day, stat.TokenName, stat.Username)] = stat
	}

	alphaAliceDay2 := tokenByKey[fmt.Sprintf("%s|%s|%s", day2Str, "alpha", "alice")]
	require.NotNil(t, alphaAliceDay2)
	require.Equal(t, 80, alphaAliceDay2.Quota)

	deltaAliceDay2 := tokenByKey[fmt.Sprintf("%s|%s|%s", day2Str, "delta", "alice")]
	require.NotNil(t, deltaAliceDay2)
	require.Equal(t, 30, deltaAliceDay2.Quota)

	// Verify scoped queries filter by user correctly.
	aliceScoped, err := SearchLogsByDayAndUser(101, start, end)
	require.NoError(t, err)
	require.Len(t, aliceScoped, 2)
	for _, stat := range aliceScoped {
		require.Equal(t, 101, stat.UserId)
	}

	tokenScoped, err := SearchLogsByDayAndToken(101, start, end)
	require.NoError(t, err)
	for _, stat := range tokenScoped {
		require.Equal(t, 101, stat.UserId)
		require.Equal(t, "alice", stat.Username)
	}

	// Tool dashboards now read straight from LogTypeTool rows.
	toolStats, err := SearchToolLogsByDayAndTool(0, start, end)
	require.NoError(t, err)

	toolByKey := make(map[string]*dto.ToolLogStatistic)
	for _, stat := range toolStats {
		toolByKey[fmt.Sprintf("%s|%s", stat.Day, stat.ToolName)] = stat
	}

	webSearchDay1 := toolByKey[fmt.Sprintf("%s|%s", day1Str, "web_search")]
	require.NotNil(t, webSearchDay1)
	require.Equal(t, 3, webSearchDay1.RequestCount)
	require.EqualValues(t, 42, webSearchDay1.Quota)

	calculatorDay2 := toolByKey[fmt.Sprintf("%s|%s", day2Str, "calculator")]
	require.NotNil(t, calculatorDay2)
	require.Equal(t, 2, calculatorDay2.RequestCount)
	require.EqualValues(t, 8, calculatorDay2.Quota)

	toolUserStats, err := SearchToolLogsByDayAndUser(0, start, end)
	require.NoError(t, err)

	toolUserByKey := make(map[string]*dto.ToolLogStatisticByUser)
	for _, stat := range toolUserStats {
		toolUserByKey[fmt.Sprintf("%s|%s", stat.Day, stat.Username)] = stat
	}

	aliceToolDay2 := toolUserByKey[fmt.Sprintf("%s|%s", day2Str, "alice")]
	require.NotNil(t, aliceToolDay2)
	require.Equal(t, 5, aliceToolDay2.RequestCount)
	require.EqualValues(t, 33, aliceToolDay2.Quota)

	bobToolDay1 := toolUserByKey[fmt.Sprintf("%s|%s", day1Str, "bob")]
	require.NotNil(t, bobToolDay1)
	require.Equal(t, 1, bobToolDay1.RequestCount)
	require.EqualValues(t, 12, bobToolDay1.Quota)

	toolTokenStats, err := SearchToolLogsByDayAndToken(0, start, end)
	require.NoError(t, err)

	toolTokenByKey := make(map[string]*dto.ToolLogStatisticByToken)
	for _, stat := range toolTokenStats {
		toolTokenByKey[fmt.Sprintf("%s|%s|%s", stat.Day, stat.TokenName, stat.Username)] = stat
	}

	alphaToolsDay2 := toolTokenByKey[fmt.Sprintf("%s|%s|%s", day2Str, "alpha", "alice")]
	require.NotNil(t, alphaToolsDay2)
	require.Equal(t, 3, alphaToolsDay2.RequestCount)
	require.EqualValues(t, 25, alphaToolsDay2.Quota)

	deltaToolsDay2 := toolTokenByKey[fmt.Sprintf("%s|%s|%s", day2Str, "delta", "alice")]
	require.NotNil(t, deltaToolsDay2)
	require.Equal(t, 2, deltaToolsDay2.RequestCount)
	require.EqualValues(t, 8, deltaToolsDay2.Quota)

	toolUserScoped, err := SearchToolLogsByDayAndUser(101, start, end)
	require.NoError(t, err)
	require.Len(t, toolUserScoped, 2)
	for _, stat := range toolUserScoped {
		require.Equal(t, 101, stat.UserId)
	}

	toolTokenScoped, err := SearchToolLogsByDayAndToken(101, start, end)
	require.NoError(t, err)
	for _, stat := range toolTokenScoped {
		require.Equal(t, 101, stat.UserId)
		require.Equal(t, "alice", stat.Username)
	}

	// Sanity check that LogTypeConsume rows are not double-counted in tool
	// charts: model rows above carry no LogTypeTool sibling for "gpt-4", so
	// no tool aggregation should mention gpt-4.
	for _, stat := range toolStats {
		require.NotEqual(t, "gpt-4", stat.ToolName, "model name leaked into tool charts: %+v", stat)
	}
}

// TestRecordToolLogs covers the high-level write helper used by the relay
// billing path. It asserts:
//   - one row is written per invocation in summary.Counts
//   - per-row quota sums back to the per-tool total cost (rounding included)
//   - rows carry the originating model name as OriginModelName so dashboards
//     can correlate tools back to the model that triggered them
//   - nil/empty summaries are no-ops
func TestRecordToolLogs(t *testing.T) {
	setupTestDatabase(t)

	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE 'Tool invocation:%'").Error)
	t.Cleanup(func() {
		LOG_DB.Exec("DELETE FROM logs WHERE content LIKE 'Tool invocation:%'")
	})

	base := &Log{
		UserId:          501,
		Username:        "carol",
		TokenName:       "primary",
		ModelName:       "claude-haiku-4-5",
		OriginModelName: "claude-haiku-4-5",
		ChannelId:       77,
		RequestId:       "req-tool-test",
		TraceId:         "trace-tool-test",
	}

	// nil summary should not write anything.
	RecordToolLogs(context.Background(), base, nil)

	// empty summary should not write anything.
	RecordToolLogs(context.Background(), base, &ToolUsageSummary{})

	summary := &ToolUsageSummary{
		Counts: map[string]int{
			"web_search": 3,
			"file_read":  1,
		},
		CostByTool: map[string]int64{
			"web_search": 100, // 100 / 3 = 33 with remainder 1; first row carries 34
			"file_read":  25,
		},
		TotalCost: 125,
	}
	RecordToolLogs(context.Background(), base, summary)

	var rows []Log
	require.NoError(t, LOG_DB.Where("type = ? AND request_id = ?", LogTypeTool, base.RequestId).Order("model_name, id").Find(&rows).Error)
	require.Len(t, rows, 4)

	byTool := map[string][]Log{}
	for _, r := range rows {
		byTool[r.ModelName] = append(byTool[r.ModelName], r)
	}
	require.Len(t, byTool["web_search"], 3)
	require.Len(t, byTool["file_read"], 1)

	for _, r := range rows {
		require.Equal(t, base.UserId, r.UserId)
		require.Equal(t, base.Username, r.Username)
		require.Equal(t, base.TokenName, r.TokenName)
		require.Equal(t, base.ChannelId, r.ChannelId)
		require.Equal(t, base.OriginModelName, r.OriginModelName)
		require.Equal(t, LogTypeTool, r.Type)
	}

	var webTotal int
	for _, r := range byTool["web_search"] {
		webTotal += r.Quota
	}
	require.Equal(t, 100, webTotal)
	require.Equal(t, 25, byTool["file_read"][0].Quota)

	// The first row of web_search absorbs the rounding remainder.
	first := byTool["web_search"][0]
	require.Equal(t, 34, first.Quota)
	require.Equal(t, 33, byTool["web_search"][1].Quota)
	require.Equal(t, 33, byTool["web_search"][2].Quota)
}

// TestRecordToolLog covers the single-row write path used by the external
// /api/token/consume endpoint and the MCP proxy direct path.
func TestRecordToolLog(t *testing.T) {
	setupTestDatabase(t)

	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE request_id = 'req-single-tool-test'").Error)
	t.Cleanup(func() {
		LOG_DB.Exec("DELETE FROM logs WHERE request_id = 'req-single-tool-test'")
	})

	entry := &Log{
		UserId:    902,
		ModelName: "web search",
		TokenName: "external-bill-token",
		Quota:     250,
		Content:   "External (web search) consumed $0.025000 quota",
		RequestId: "req-single-tool-test",
		TraceId:   "trace-single",
	}
	RecordToolLog(context.Background(), entry)

	var stored Log
	require.NoError(t, LOG_DB.Where("request_id = ?", entry.RequestId).First(&stored).Error)
	require.Equal(t, LogTypeTool, stored.Type)
	require.Equal(t, "web search", stored.ModelName)
	require.Equal(t, 250, stored.Quota)
	require.NotZero(t, stored.CreatedAt)
}
