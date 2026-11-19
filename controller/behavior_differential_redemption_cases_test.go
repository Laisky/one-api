package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// behaviorDiffRedemptionCases covers Appendix A sites R1-R4 and the T18 error
// cases for the flipped redemption handlers.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: redemption capture set.
func behaviorDiffRedemptionCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "redemption", name: "get_all", site: "R1 GetAllRedemptions",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllRedemptions,
			},
		},
		{
			entity: "redemption", name: "search", site: "R2 SearchRedemptions",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/search",
				target:  behaviorDiffStatic("/api/redemption/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: SearchRedemptions,
			},
		},
		{
			entity: "redemption", name: "get_by_uuid", site: "R3 GetRedemption",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/redemption/" + f.redemption.UUID },
				prepare: behaviorDiffAsRoot,
				handler: GetRedemption,
			},
		},
		{
			entity: "redemption", name: "get_by_uuid_not_found", site: "R3 GetRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/:id",
				target:  behaviorDiffStatic("/api/redemption/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetRedemption,
			},
		},
		{
			entity: "redemption", name: "update", site: "R4 UpdateRedemption",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff-renamed","quota":5150}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var row model.Redemption
				require.NoError(t, model.DB.First(&row, f.redemption.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, row.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "redemption", name: "update_status_only", site: "R4 UpdateRedemption",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/?status_only=true"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"status":2}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var row model.Redemption
				require.NoError(t, model.DB.First(&row, f.redemption.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, row.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "redemption", name: "update_missing_uuid", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/?status_only=true"),
				body:    behaviorDiffStatic(`{"id":1,"status":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_not_found", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/?status_only=true"),
				body:    behaviorDiffStatic(`{"uuid":"` + behaviorDiffMissingUUID + `","status":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_empty_name", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"  ","quota":1}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_malformed_json", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/"),
				body:    behaviorDiffStatic(`{"uuid":`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_wrong_field_type", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff","quota":"lots"}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
	}
}
