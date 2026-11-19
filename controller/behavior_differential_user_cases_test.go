package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
)

// behaviorDiffUserCases covers Appendix A sites U1-U6 and the T18 error cases
// for the flipped user handlers and the three inbound user request DTOs
// (register / create / self-update). The request DTOs are referenced only
// through HTTP bodies, never by Go type, so this file compiles pre-refactor too.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: user capture set.
func behaviorDiffUserCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "user", name: "login_success", site: "U1 SetupLogin",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/login",
				target:   behaviorDiffStatic("/api/user/login"),
				body:     behaviorDiffStatic(`{"username":"behaviordiff-user","password":"userpass123"}`),
				handler:  Login,
				sessions: true,
			},
		},
		{
			entity: "user", name: "login_bad_password", site: "U1 SetupLogin (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/login",
				target:   behaviorDiffStatic("/api/user/login"),
				body:     behaviorDiffStatic(`{"username":"behaviordiff-user","password":"wrongpass123"}`),
				handler:  Login,
				sessions: true,
			},
		},
		{
			entity: "user", name: "login_malformed_json", site: "U1 SetupLogin (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/login",
				target:   behaviorDiffStatic("/api/user/login"),
				body:     behaviorDiffStatic(`{"username":`),
				handler:  Login,
				sessions: true,
			},
		},
		{
			entity: "user", name: "get_all", site: "U2 GetAllUsers",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllUsers,
			},
		},
		{
			entity: "user", name: "search", site: "U3 SearchUsers",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/search",
				target:  behaviorDiffStatic("/api/user/search?keyword=behaviordiff"),
				prepare: behaviorDiffAsRoot,
				handler: SearchUsers,
			},
		},
		{
			entity: "user", name: "get_by_uuid", site: "U4 GetUser",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/user/" + f.user.UUID },
				prepare: behaviorDiffAsRoot,
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_by_uuid_not_found", site: "U4 GetUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				target:  behaviorDiffStatic("/api/user/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_by_uuid_insufficient_role", site: "U4 GetUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				// A non-root admin may not read a peer admin's record.
				target:  func(f *behaviorDiffFixture) string { return "/api/user/" + f.other.UUID },
				prepare: behaviorDiffAsAdmin,
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_by_uuid_missing_role", site: "U4 GetUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				target: func(f *behaviorDiffFixture) string { return "/api/user/" + f.user.UUID },
				// No role key at all: the handler must still deny.
				prepare: func(c *gin.Context, f *behaviorDiffFixture) { c.Set(ctxkey.Id, f.user.Id) },
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_self", site: "U5 GetSelf",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/self",
				target:  behaviorDiffStatic("/api/user/self"),
				prepare: behaviorDiffAsUser,
				handler: GetSelf,
			},
		},
		{
			entity: "user", name: "manage_disable", site: "U6 ManageUser",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-user","action":"disable"}`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_malformed_json", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_wrong_field_type", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":123,"action":"disable"}`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_unknown_user", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-nobody","action":"disable"}`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_insufficient_role", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-root","action":"disable"}`),
				prepare: behaviorDiffAsAdmin,
				handler: ManageUser,
			},
		},
		// --- T18: inbound request-DTO error paths (Register) ---
		{
			entity: "user", name: "register_malformed_json", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":`),
				handler: Register,
			},
		},
		{
			entity: "user", name: "register_wrong_field_type", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":123,"password":"newpass123"}`),
				handler: Register,
			},
		},
		{
			entity: "user", name: "register_wrong_type_unbound_field", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target: behaviorDiffStatic("/api/user/register"),
				// "quota" is a real, JSON-tagged field of the struct this handler
				// binds into, but the handler never reads it. A type mismatch on it
				// is still a decode error for the whole body, so the request is
				// rejected — that rejection is part of the frozen inbound contract
				// (proposal I3) and must survive any change to what the handler
				// binds into.
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","quota":"not-a-number"}`),
				handler: Register,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				behaviorDiffNormalizeUserIfCreated(t, "behaviordiff-new", n)
			},
		},
		{
			entity: "user", name: "register_short_password", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"short"}`),
				handler: Register,
			},
		},
		{
			entity: "user", name: "register_duplicate_username", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-user","password":"newpass123"}`),
				handler: Register,
			},
		},
		// --- T18: inbound request-DTO error paths (CreateUser) ---
		{
			entity: "user", name: "create_malformed_json", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
		},
		{
			entity: "user", name: "create_wrong_field_type", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","quota":"not-a-number"}`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
		},
		{
			entity: "user", name: "create_wrong_type_unbound_field", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target: behaviorDiffStatic("/api/user/"),
				// "status" is a real JSON-tagged field of the struct this handler
				// binds into but is never read by it; a type mismatch on it must
				// still reject the whole body (proposal I3).
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","status":"enabled"}`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				behaviorDiffNormalizeUserIfCreated(t, "behaviordiff-new", n)
			},
		},
		{
			entity: "user", name: "create_role_too_high", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","role":100}`),
				prepare: behaviorDiffAsAdmin,
				handler: CreateUser,
			},
		},
		{
			entity: "user", name: "create_invalid_input", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"waaaaaaaaaaaaaaaaaaaaay-too-long-password"}`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
		},
		// --- T18: inbound request-DTO error paths (UpdateSelf) ---
		{
			entity: "user", name: "update_self_malformed_json", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target:  behaviorDiffStatic("/api/user/self"),
				body:    behaviorDiffStatic(`{"username":`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
		{
			entity: "user", name: "update_self_wrong_field_type", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target:  behaviorDiffStatic("/api/user/self"),
				body:    behaviorDiffStatic(`{"display_name":123}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
		{
			entity: "user", name: "update_self_wrong_type_unbound_field", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target: behaviorDiffStatic("/api/user/self"),
				// "quota" is a real JSON-tagged field of the struct this handler
				// binds into but is never read by it; a type mismatch on it must
				// still reject the whole body (proposal I3).
				body:    behaviorDiffStatic(`{"display_name":"Renamed","quota":"lots"}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
		{
			entity: "user", name: "update_self_validation_error", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target: behaviorDiffStatic("/api/user/self"),
				// display_name exceeds the validate:"max=20" tag; the emitted
				// validator message embeds the validated struct's name, which the
				// request-DTO rebinding must not change.
				body:    behaviorDiffStatic(`{"display_name":"this display name is definitely far too long"}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
	}
}
