package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// behaviorDiffChannelCases covers Appendix A sites C1-C4 and the T18 error cases
// for the flipped channel handlers (including the channelListItem wrapper and
// the buildChannelResponsePayload tooling splice).
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: channel capture set.
func behaviorDiffChannelCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "channel", name: "get_all", site: "C1 GetAllChannels",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllChannels,
			},
		},
		{
			entity: "channel", name: "search", site: "C2 SearchChannels",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/search",
				target:  behaviorDiffStatic("/api/channel/search?keyword=behaviordiff"),
				prepare: behaviorDiffAsRoot,
				handler: SearchChannels,
			},
		},
		{
			entity: "channel", name: "get_by_uuid", site: "C3 GetChannel",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/channel/" + f.channel.UUID },
				prepare: behaviorDiffAsRoot,
				handler: GetChannel,
			},
		},
		{
			entity: "channel", name: "get_by_uuid_not_found", site: "C3 GetChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/:id",
				target:  behaviorDiffStatic("/api/channel/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetChannel,
			},
		},
		{
			entity: "channel", name: "update", site: "C4 UpdateChannel",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target: behaviorDiffStatic("/api/channel/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff-channel-renamed","type":1,"key":"behaviordiff-channel-key","models":"gpt-4o,gpt-3.5-turbo","group":"default,vip","status":1,"priority":6,"weight":9}`, f.channel.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var ch model.Channel
				require.NoError(t, model.DB.First(&ch, f.channel.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, ch.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "channel", name: "update_missing_uuid", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/"),
				body:    behaviorDiffStatic(`{"id":1,"name":"legacy-int-writer","type":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_not_found", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/"),
				body:    behaviorDiffStatic(`{"uuid":"` + behaviorDiffMissingUUID + `","name":"nope","type":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_empty_name", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target: behaviorDiffStatic("/api/channel/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"   ","type":1}`, f.channel.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_malformed_json", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/"),
				body:    behaviorDiffStatic(`{"uuid":`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_wrong_field_type", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target: behaviorDiffStatic("/api/channel/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff","type":"openai"}`, f.channel.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
	}
}
