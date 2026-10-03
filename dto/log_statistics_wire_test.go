package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogStatisticsWireFormat pins the JSON the dashboard API returns and its Redis cache stores
// for the per-day statistics. These structs had no json tags for years, so their field names on
// the wire are Go's defaults, and the modern and berry dashboards read them by exactly those
// names. The tags only spell the names out for musttag; any change to this output is a
// wire-format break for the frontends and for cache entries written by other replicas.
//
// Parameters:
//   - t: the test owns the fixtures and assertions.
//
// Return values: none; failures are reported through t.
func TestLogStatisticsWireFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "LogStatistic",
			value: LogStatistic{Day: "2026-09-10", ModelName: "gpt-x", RequestCount: 1, Quota: 2, PromptTokens: 3,
				CompletionTokens: 4, CachedPromptTokens: 5, CacheHitCount: 6, CacheHitQuota: 7},
			want: `{"Day":"2026-09-10","ModelName":"gpt-x","RequestCount":1,"Quota":2,"PromptTokens":3,"CompletionTokens":4,"CachedPromptTokens":5,"CacheHitCount":6,"CacheHitQuota":7}`,
		},
		{
			name: "LogStatisticByUser",
			value: LogStatisticByUser{Day: "2026-09-10", Username: "alice", UserId: 9, UserUUID: "u-1", RequestCount: 1,
				Quota: 2, PromptTokens: 3, CompletionTokens: 4, CachedPromptTokens: 5, CacheHitCount: 6, CacheHitQuota: 7},
			want: `{"Day":"2026-09-10","Username":"alice","user_uuid":"u-1","RequestCount":1,"Quota":2,"PromptTokens":3,"CompletionTokens":4,"CachedPromptTokens":5,"CacheHitCount":6,"CacheHitQuota":7}`,
		},
		{
			name: "LogStatisticByToken",
			value: LogStatisticByToken{Day: "2026-09-10", Username: "alice", UserId: 9, UserUUID: "u-1", TokenName: "t",
				RequestCount: 1, Quota: 2, PromptTokens: 3, CompletionTokens: 4, CachedPromptTokens: 5, CacheHitCount: 6,
				CacheHitQuota: 7},
			want: `{"Day":"2026-09-10","Username":"alice","user_uuid":"u-1","TokenName":"t","RequestCount":1,"Quota":2,"PromptTokens":3,"CompletionTokens":4,"CachedPromptTokens":5,"CacheHitCount":6,"CacheHitQuota":7}`,
		},
		{
			name:  "ToolLogStatistic",
			value: ToolLogStatistic{Day: "2026-09-10", ToolName: "search", RequestCount: 1, Quota: 2},
			want:  `{"Day":"2026-09-10","ToolName":"search","RequestCount":1,"Quota":2}`,
		},
		{
			name:  "ToolLogStatisticByUser",
			value: ToolLogStatisticByUser{Day: "2026-09-10", Username: "alice", UserId: 9, UserUUID: "u-1", RequestCount: 1, Quota: 2},
			want:  `{"Day":"2026-09-10","Username":"alice","user_uuid":"u-1","RequestCount":1,"Quota":2}`,
		},
		{
			name: "ToolLogStatisticByToken",
			value: ToolLogStatisticByToken{Day: "2026-09-10", Username: "alice", UserId: 9, UserUUID: "u-1", TokenName: "t",
				RequestCount: 1, Quota: 2},
			want: `{"Day":"2026-09-10","Username":"alice","user_uuid":"u-1","TokenName":"t","RequestCount":1,"Quota":2}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(encoded))
		})
	}
}
