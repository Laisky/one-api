package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/dto"
)

func TestDashboardAggregations(t *testing.T) {
	setupTestDatabase(t)

	require.NoError(t, LOG_DB.Exec("DELETE FROM logs WHERE content LIKE 'test-dashboard-agg-%'").Error)
	t.Cleanup(func() {
		LOG_DB.Exec("DELETE FROM logs WHERE content LIKE 'test-dashboard-agg-%'")
	})

	base := time.Now().UTC().Truncate(24 * time.Hour)
	day1 := base.Add(-24 * time.Hour)
	day2 := base

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
}
