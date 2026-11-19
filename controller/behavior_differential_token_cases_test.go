package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// behaviorDiffTokenCases covers Appendix A sites T1-T9 and the T18 error cases
// for the flipped token handlers.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: token capture set.
func behaviorDiffTokenCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "token", name: "get_all", site: "T1 GetAllTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/?p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: GetAllTokens,
			},
		},
		{
			entity: "token", name: "search", site: "T2 SearchTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/search",
				target:  behaviorDiffStatic("/api/token/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: SearchTokens,
			},
		},
		{
			entity: "token", name: "get_by_uuid", site: "T3 GetToken",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/token/" + f.token.UUID },
				prepare: behaviorDiffAsUser,
				handler: GetToken,
			},
		},
		{
			entity: "token", name: "get_by_uuid_not_found", site: "T3 GetToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/:id",
				target:  behaviorDiffStatic("/api/token/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsUser,
				handler: GetToken,
			},
		},
		{
			entity: "token", name: "get_by_uuid_foreign_owner", site: "T3 GetToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/:id",
				// The fixture user asks for another user's token.
				target:  func(f *behaviorDiffFixture) string { return "/api/token/" + f.otherToken.UUID },
				prepare: behaviorDiffAsUser,
				handler: GetToken,
			},
		},
		{
			entity: "token", name: "add", site: "T4 AddToken",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":"behaviordiff-added-token","remain_quota":4242,"expired_time":-1,"unlimited_quota":false,"models":"gpt-4o","subnet":"172.16.0.0/12"}`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				behaviorDiffNormalizeToken(t, "behaviordiff-added-token", n)
			},
		},
		{
			entity: "token", name: "add_malformed_json", site: "T4 AddToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
		},
		{
			entity: "token", name: "add_wrong_field_type", site: "T4 AddToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":"behaviordiff-bad","remain_quota":"lots"}`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
		},
		{
			entity: "token", name: "add_empty_name", site: "T4 AddToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":"   ","remain_quota":1}`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
		},
		{
			entity: "token", name: "consume_single", site: "T5 ConsumeToken",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/consume",
				target: behaviorDiffStatic("/api/token/consume"),
				body:   behaviorDiffStatic(`{"phase":"single","add_used_quota":150,"add_reason":"behaviordiff-consume"}`),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.UserUUID, f.user.UUID)
					c.Set(ctxkey.TokenId, f.token.Id)
					c.Set(ctxkey.TokenName, f.token.Name)
				},
				handler: ConsumeToken,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var txn model.TokenTransaction
				require.NoError(t, model.DB.Where("reason = ?", "behaviordiff-consume").First(&txn).Error)
				n.addString(txn.UUID, "<consume-txn-uuid>")
				n.addString(txn.TransactionID, "<consume-txn-id>")
				n.addString(txn.TraceId, "<trace-id>")
				if txn.LogUUID != nil {
					n.addString(*txn.LogUUID, "<consume-log-uuid>")
				}
				if txn.ConfirmedAt != nil {
					n.addNumber(*txn.ConfirmedAt, "<ts>")
				}
				// The response echoes the in-memory pre-hold expiry, which
				// processPostConsume zeroes in the row, so it cannot be read back:
				// it is pre-phase GetTimestamp() + the default hold window.
				n.addTimestampWindow(f, int64(config.ExternalBillingDefaultTimeoutSec), "<ts>")

				var tok model.Token
				require.NoError(t, model.DB.First(&tok, f.token.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, tok.UpdatedAt, "<ts-milli>")
				n.addChangedNumber(behaviorDiffAccessedTime, tok.AccessedTime, "<ts>")
			},
		},
		{
			entity: "token", name: "consume_missing_reason", site: "T5 ConsumeToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/consume",
				target: behaviorDiffStatic("/api/token/consume"),
				body:   behaviorDiffStatic(`{"phase":"single","add_used_quota":150,"add_reason":"   "}`),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.token.Id)
				},
				handler: ConsumeToken,
			},
		},
		{
			entity: "token", name: "consume_malformed_json", site: "T5 ConsumeToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/consume",
				target: behaviorDiffStatic("/api/token/consume"),
				body:   behaviorDiffStatic(`{"phase":`),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.token.Id)
				},
				handler: ConsumeToken,
			},
		},
		{
			entity: "token", name: "update", site: "T6 UpdateToken",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target: behaviorDiffStatic("/api/token/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff-token-renamed","status":1,"expired_time":-1,"remain_quota":123456,"unlimited_quota":false,"models":"gpt-4o","subnet":"192.168.0.0/24"}`, f.token.UUID)
				},
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var tok model.Token
				require.NoError(t, model.DB.First(&tok, f.token.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, tok.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "token", name: "update_missing_uuid", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"id":1,"name":"legacy-int-writer","status":1}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "update_not_found", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"uuid":"` + behaviorDiffMissingUUID + `","name":"nope","status":1}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "update_foreign_owner", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target: behaviorDiffStatic("/api/token/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"stolen","status":1}`, f.otherToken.UUID)
				},
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "update_malformed_json", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"uuid":`),
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "admin_get_all", site: "T7 AdminGetAllTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/",
				target:  behaviorDiffStatic("/api/admin/tokens/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: AdminGetAllTokens,
			},
		},
		{
			entity: "token", name: "admin_search", site: "T8 AdminSearchTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/search",
				target:  behaviorDiffStatic("/api/admin/tokens/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: AdminSearchTokens,
			},
		},
		{
			entity: "token", name: "admin_get_by_uuid", site: "T9 AdminGetToken",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/admin/tokens/" + f.otherToken.UUID },
				prepare: behaviorDiffAsRoot,
				handler: AdminGetToken,
			},
		},
		{
			entity: "token", name: "admin_get_by_uuid_not_found", site: "T9 AdminGetToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/:id",
				target:  behaviorDiffStatic("/api/admin/tokens/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: AdminGetToken,
			},
		},
	}
}
