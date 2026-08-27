package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// tokenScopeFixture provisions one enabled channel serving every model in models,
// all inside a fresh group, and returns the group name.
func tokenScopeFixture(t *testing.T, models []string) string {
	t.Helper()

	// Unique per test: getGroupModelsV2Cache has a 10s in-process TTL.
	groupName := fmt.Sprintf("token-scope-%d", time.Now().UnixNano())
	channelID := 6100 + len(models)

	csv := ""
	for i, m := range models {
		if i > 0 {
			csv += ","
		}
		csv += m
	}
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Name: "scope-channel", Status: model.ChannelStatusEnabled,
		Type: channeltype.Zhipu, Models: csv, Group: groupName,
	}).Error)
	for _, m := range models {
		require.NoError(t, model.DB.Create(&model.Ability{
			Group: groupName, Model: m, ChannelId: channelID,
			Enabled: true, Priority: ptrInt64(0),
		}).Error)
	}
	return groupName
}

// listModelsAsToken drives GET /v1/models. allowList == nil means an unrestricted
// token, which is how TokenAuth represents Token.Models being nil or empty: it
// simply never sets ctxkey.AvailableModels.
func listModelsAsToken(t *testing.T, groupName string, allowList *string) []string {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(ctxkey.UserObj, &model.User{
		Username: "scope-user", Group: groupName,
		Role: model.RoleCommonUser, Status: model.UserStatusEnabled,
	})
	if allowList != nil {
		c.Set(ctxkey.AvailableModels, *allowList)
	}

	ListModels(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload struct {
		Data []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	ids := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		ids = append(ids, m.Id)
	}
	sort.Strings(ids)
	return ids
}

// TestListModels_UnrestrictedTokenSeesWholeGroup is the regression guard for the
// dangerous failure mode of this change: an unrestricted token must keep seeing
// everything. TokenAuth omits ctxkey.AvailableModels entirely for such a token,
// so treating "absent" as "allow nothing" would blank the catalog for most keys.
func TestListModels_UnrestrictedTokenSeesWholeGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupModels := []string{"glm-4.7", "glm-5.3", "glm-4.6v"}
	groupName := tokenScopeFixture(t, groupModels)

	// Absent key: the real unrestricted case.
	require.ElementsMatch(t, groupModels, listModelsAsToken(t, groupName, nil))

	// Defensive: an empty string must also mean unrestricted, never "nothing".
	empty := ""
	require.ElementsMatch(t, groupModels, listModelsAsToken(t, groupName, &empty))
}

// TestListModels_RestrictedTokenSeesOnlyItsAllowList is the headline contract.
func TestListModels_RestrictedTokenSeesOnlyItsAllowList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := tokenScopeFixture(t, []string{"glm-4.7", "glm-5.3", "glm-4.6v"})

	allow := "glm-4.7,glm-4.6v"
	require.Equal(t, []string{"glm-4.6v", "glm-4.7"}, listModelsAsToken(t, groupName, &allow))

	// An allow-list naming a model the group cannot serve grants nothing extra:
	// the token allow-list narrows, it never widens.
	broader := "glm-4.7,gpt-4o,claude-opus-4.5"
	require.Equal(t, []string{"glm-4.7"}, listModelsAsToken(t, groupName, &broader))

	// A token allowing nothing the group serves lists nothing.
	disjoint := "gpt-4o"
	require.Empty(t, listModelsAsToken(t, groupName, &disjoint))

	// Whitespace-only is a LIVE restriction, not an absent one: TokenAuth's guard
	// is `*token.Models != ""` without trimming, so such a key is denied every
	// model and must therefore be shown none. Listing anything here would advertise
	// models the caller is guaranteed to be 403'd on.
	whitespace := " "
	require.Empty(t, listModelsAsToken(t, groupName, &whitespace))
	for _, m := range []string{"glm-4.7", "glm-5.3", "glm-4.6v"} {
		require.False(t, middleware.IsModelInList(m, whitespace),
			"precondition: a whitespace-only allow-list must deny %q at the relay too", m)
	}
}

// TestListModels_DiscoveryEqualsInvocability is the acceptance property, and the
// reason the filter reuses middleware.IsModelInList rather than reimplementing it:
// for every model the group can serve, being listed must be exactly equivalent to
// being callable. Any divergence in casing, trimming, or duplicate handling fails
// here rather than in production as a 403 on an advertised model.
func TestListModels_DiscoveryEqualsInvocability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupModels := []string{"glm-4.7", "glm-5.3", "glm-4.6v", "GLM-Case", "glm-case"}
	groupName := tokenScopeFixture(t, groupModels)

	for _, allowList := range []string{
		"glm-4.7",
		"glm-4.7,glm-5.3",
		"glm-4.7,glm-4.7,glm-5.3", // duplicates
		"GLM-Case",                // casing must be respected exactly
		"glm-case,GLM-Case",       // both casings are distinct routing keys
		"glm-4.7, glm-5.3",        // untrimmed second entry: NOT callable, so NOT listed
		"nonexistent",
		"glm-4.7,nonexistent",
	} {
		t.Run(allowList, func(t *testing.T) {
			listed := listModelsAsToken(t, groupName, &allowList)

			listedSet := make(map[string]struct{}, len(listed))
			for _, id := range listed {
				listedSet[id] = struct{}{}
			}

			wantCount := 0
			for _, m := range groupModels {
				callable := middleware.IsModelInList(m, allowList)
				if callable {
					wantCount++
				}
				_, shown := listedSet[m]
				require.Equalf(t, callable, shown,
					"model %q: callable=%v but listed=%v -- discovery must equal invocability",
					m, callable, shown)
			}
			// Without this the loop above cannot see an id that leaked in from
			// outside the fixture, since it only ever inspects groupModels.
			require.Lenf(t, listed, wantCount,
				"listing must contain exactly the callable models and nothing else, got %v", listed)
		})
	}
}

// TestRetrieveModel_RespectsTokenAllowList pins that the single-model endpoint is
// scoped identically. A model the key cannot call must read as not found rather
// than returning metadata, which would both mislead the caller and disclose part
// of the group catalog the key has no access to.
func TestRetrieveModel_RespectsTokenAllowList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := tokenScopeFixture(t, []string{"glm-4.7", "glm-5.3"})
	allow := "glm-4.7"

	retrieve := func(t *testing.T, id string) (int, string, bool) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models/"+id, nil)
		c.Params = gin.Params{{Key: "model", Value: id}}
		c.Set(ctxkey.UserObj, &model.User{
			Username: "scope-user", Group: groupName,
			Role: model.RoleCommonUser, Status: model.UserStatusEnabled,
		})
		c.Set(ctxkey.AvailableModels, allow)

		RetrieveModel(c)

		var entry struct {
			Id    string `json:"id"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
		notFound := entry.Error != nil && entry.Error.Code == "model_not_found"
		return w.Code, entry.Id, notFound
	}

	code, id, notFound := retrieve(t, "glm-4.7")
	require.Equal(t, http.StatusOK, code)
	require.False(t, notFound, "an allow-listed model must be retrievable")
	require.Equal(t, "glm-4.7", id)

	// Served by the group, but outside this key's allow-list.
	code, _, notFound = retrieve(t, "glm-5.3")
	require.True(t, notFound, "a model the key cannot call must read as not found")
	// Pin the status too, not just the body: a 500 carrying a model_not_found-shaped
	// payload would otherwise pass silently.
	require.Equal(t, http.StatusNotFound, code)
}

// TestRetrieveModel_NotFoundIsIndistinguishable pins both halves of the
// not-found contract: the status is 404 (OpenAI SDKs raise NotFoundError from the
// status, not from the body's code), and an unknown model, a model outside the
// key's allow-list, and a model the group does not serve all produce a byte-identical
// response. Three distinguishable answers would turn this endpoint into an oracle
// for enumerating the deployment's catalog with any valid key.
func TestRetrieveModel_NotFoundIsIndistinguishable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := tokenScopeFixture(t, []string{"glm-4.7", "glm-5.3"})

	bodyFor := func(t *testing.T, id string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models/"+id, nil)
		c.Params = gin.Params{{Key: "model", Value: id}}
		c.Set(ctxkey.UserObj, &model.User{
			Username: "scope-user", Group: groupName,
			Role: model.RoleCommonUser, Status: model.UserStatusEnabled,
		})
		c.Set(ctxkey.AvailableModels, "glm-4.7")
		RetrieveModel(c)
		return w.Code, w.Body.String()
	}

	// Served by the group but outside the allow-list.
	forbiddenCode, forbiddenBody := bodyFor(t, "glm-5.3")
	// Not served by anything.
	unknownCode, unknownBody := bodyFor(t, "does-not-exist-anywhere")

	require.Equal(t, http.StatusNotFound, forbiddenCode)
	require.Equal(t, http.StatusNotFound, unknownCode)

	// Same status, and the payload differs only by the echoed model id.
	require.Equal(t,
		strings.Replace(forbiddenBody, "glm-5.3", "MODEL", 1),
		strings.Replace(unknownBody, "does-not-exist-anywhere", "MODEL", 1),
		"a forbidden model must be indistinguishable from a nonexistent one")

	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(forbiddenBody), &payload))
	require.Equal(t, "model_not_found", payload.Error.Code)
	require.Equal(t, "invalid_request_error", payload.Error.Type)
	require.Contains(t, payload.Error.Message, "do not have access to it",
		"the message must not assert the model does not exist when it does")
}

// TestRetrieveModel_CaseFoldedRequestResolvesToCallableID pins the one place the
// list and retrieve endpoints intentionally differ. Retrieve is case-insensitive
// by design (issue #352: a client holding a non-routable casing must be pointed at
// the routable one), while the listing is case-sensitive because each casing is a
// distinct routing key. So an allowed "foo" is not listed as "Foo", yet
// GET /v1/models/Foo still resolves -- to "foo", the id the key can actually call.
func TestRetrieveModel_CaseFoldedRequestResolvesToCallableID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := tokenScopeFixture(t, []string{"Foo", "foo"})
	allow := "foo"

	require.Equal(t, []string{"foo"}, listModelsAsToken(t, groupName, &allow),
		"only the exact allow-listed casing may be listed")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models/Foo", nil)
	c.Params = gin.Params{{Key: "model", Value: "Foo"}}
	c.Set(ctxkey.UserObj, &model.User{
		Username: "scope-user", Group: groupName,
		Role: model.RoleCommonUser, Status: model.UserStatusEnabled,
	})
	c.Set(ctxkey.AvailableModels, allow)

	RetrieveModel(c)
	require.Equal(t, http.StatusOK, w.Code)

	var entry struct {
		Id      string `json:"id"`
		OwnedBy string `json:"owned_by"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
	require.Equal(t, "foo", entry.Id,
		"the case-folded request must resolve to the callable routing key, never the uncallable Foo")
	require.True(t, middleware.IsModelInList(entry.Id, allow),
		"whatever id retrieve returns must itself be callable with this key")
}

// TestListModels_RestrictedTokenCodexCatalogIsEmpty covers the Codex discovery
// signal: a key whose allow-list matches nothing this group serves must get empty
// arrays, not null, in both the standard and the Codex-specific field.
func TestListModels_RestrictedTokenCodexCatalogIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupUserAvailableModelsTestEnvironment(t)
	t.Cleanup(cleanup)

	groupName := tokenScopeFixture(t, []string{"glm-4.7"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?client_version=0.1.0", nil)
	c.Set(ctxkey.UserObj, &model.User{
		Username: "codex-user", Group: groupName,
		Role: model.RoleCommonUser, Status: model.UserStatusEnabled,
	})
	c.Set(ctxkey.AvailableModels, "gpt-4o")

	ListModels(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`)
	require.Contains(t, w.Body.String(), `"models":[]`)

	// The catalog is key-scoped, so it must never be cached by a shared proxy.
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "Authorization", w.Header().Get("Vary"))
}
