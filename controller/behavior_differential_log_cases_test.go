package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
)

// behaviorDiffLogCases covers Appendix A sites L1-L5 and the T18 error cases for
// the flipped log handlers.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: log capture set.
func behaviorDiffLogCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "log", name: "get_all", site: "L1 GetAllLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target:  behaviorDiffStatic("/api/log/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_all_filtered_by_channel", site: "L1 GetAllLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target: func(f *behaviorDiffFixture) string {
					return "/api/log/?p=0&size=10&channel=" + f.channel.UUID
				},
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_all_bad_channel_ref", site: "L1 GetAllLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target:  behaviorDiffStatic("/api/log/?p=0&size=10&channel=" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_all_sort_range_too_large", site: "L1 GetAllLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target:  behaviorDiffStatic("/api/log/?p=0&size=10&sort=quota&start_timestamp=1700000000&end_timestamp=1799999999"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_user", site: "L2 GetUserLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/self",
				target:  behaviorDiffStatic("/api/log/self?p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: GetUserLogs,
			},
		},
		{
			entity: "log", name: "get_user_sort_range_too_large", site: "L2 GetUserLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/self",
				target:  behaviorDiffStatic("/api/log/self?p=0&size=10&sort=quota&start_timestamp=1700000000&end_timestamp=1799999999"),
				prepare: behaviorDiffAsUser,
				handler: GetUserLogs,
			},
		},
		{
			entity: "log", name: "get_token", site: "L3 GetTokenLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/logs",
				target: behaviorDiffStatic("/api/token/logs?p=0&size=10"),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.token.Id)
					c.Set(ctxkey.TokenName, f.token.Name)
				},
				handler: GetTokenLogs,
			},
		},
		{
			entity: "log", name: "get_token_foreign_owner", site: "L3 GetTokenLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/logs",
				// Token name of another user's token combined with this user's id
				// must yield no rows (ownership is enforced by the user_id filter).
				target: behaviorDiffStatic("/api/token/logs?p=0&size=10"),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.otherToken.Id)
					c.Set(ctxkey.TokenName, f.otherToken.Name)
				},
				handler: GetTokenLogs,
			},
		},
		{
			entity: "log", name: "search_all", site: "L4 SearchAllLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/search",
				target:  behaviorDiffStatic("/api/log/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: SearchAllLogs,
			},
		},
		{
			entity: "log", name: "search_user", site: "L5 SearchUserLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/self/search",
				target:  behaviorDiffStatic("/api/log/self/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: SearchUserLogs,
			},
		},
	}
}
