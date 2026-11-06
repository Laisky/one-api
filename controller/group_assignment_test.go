package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/model"
)

// groupAssignmentCatalog is the GroupRatio catalog every group-assignment test
// runs against: "default" and "vip" are configured, "partners" is not.
const groupAssignmentCatalog = `{"default":1,"vip":0.8}`

// groupAssignmentResponse is the standard {success, message} API envelope.
type groupAssignmentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// setupGroupAssignmentTest prepares a temporary SQLite database and the shared
// GroupRatio catalog, and returns a router that mounts the channel and admin
// user write handlers as a root administrator.
// Parameters: t is the active test.
// Returns the configured gin engine.
func setupGroupAssignmentTest(t *testing.T) *gin.Engine {
	t.Helper()

	setupUserControllerTest(t)
	useGroupRatioCatalog(t, groupAssignmentCatalog)

	asRoot := func(handler gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set(ctxkey.Role, model.RoleRootUser)
			handler(c)
		}
	}
	router := gin.New()
	router.POST("/api/channel/", asRoot(AddChannel))
	router.PUT("/api/channel/", asRoot(UpdateChannel))
	router.POST("/api/channel/:id/duplicate", asRoot(DuplicateChannel))
	router.POST("/api/user/", asRoot(CreateUser))
	router.PUT("/api/user/", asRoot(UpdateUser))
	return router
}

// sendGroupAssignmentRequest sends payload as JSON to path and decodes the API envelope.
// Parameters: t is the active test, router serves the request, method and path
// select the route, and payload is marshaled as the request body (nil sends none).
// Returns the decoded response envelope.
func sendGroupAssignmentRequest(t *testing.T, router *gin.Engine, method, path string, payload any) groupAssignmentResponse {
	t.Helper()

	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		body = encoded
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp groupAssignmentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	return resp
}

// seedGroupAssignmentChannel stores a channel with group written directly to
// the database, bypassing every write-path check, to model legacy data.
// Parameters: t is the active test, name is the channel name and group is the raw stored value.
// Returns the stored channel.
func seedGroupAssignmentChannel(t *testing.T, name, group string) *model.Channel {
	t.Helper()

	channel := &model.Channel{
		Type:   1,
		Name:   name,
		Key:    "sk-" + name,
		Models: "gpt-4o",
		Group:  group,
		Config: "{}",
		Status: model.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NotEmpty(t, channel.UUID)
	return channel
}

// storedChannelGroups returns the raw stored group value of every channel named name.
// Parameters: t is the active test and name is the channel name to look up.
// Returns the stored group values ordered by id.
func storedChannelGroups(t *testing.T, name string) []string {
	t.Helper()

	var groups []string
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("name = ?", name).Order("id").Pluck("group", &groups).Error)
	return groups
}

// storedAbilityGroups returns the distinct ability groups of the channels named name.
// Parameters: t is the active test and name is the channel name to look up.
// Returns the sorted distinct ability groups.
func storedAbilityGroups(t *testing.T, name string) []string {
	t.Helper()

	var channelIDs []int
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("name = ?", name).Pluck("id", &channelIDs).Error)
	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id IN ?", channelIDs).Find(&abilities).Error)
	groups := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		if !slices.Contains(groups, ability.Group) {
			groups = append(groups, ability.Group)
		}
	}
	slices.Sort(groups)
	return groups
}

// channelGroupPayload builds an AddChannel/UpdateChannel body; an empty group omits the field.
// Parameters: name is the channel name and group is the submitted group list.
// Returns the JSON-ready payload.
func channelGroupPayload(name, group string) map[string]any {
	payload := map[string]any{
		"name":   name,
		"type":   1,
		"key":    "sk-" + name,
		"models": "gpt-4o",
		"config": "{}",
	}
	if group != "" {
		payload["group"] = group
	}
	return payload
}

// TestAddChannel_RejectsUnconfiguredGroup verifies a channel cannot be created
// with a group that is not configured in the GroupRatio catalog.
func TestAddChannel_RejectsUnconfiguredGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)

	resp := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/channel/",
		channelGroupPayload("unknown-group-channel", "default,partners"))
	require.False(t, resp.Success, "unconfigured group must be rejected")
	require.Contains(t, resp.Message, `group "partners" is not configured`)
	require.Empty(t, storedChannelGroups(t, "unknown-group-channel"), "rejected channel must not be stored")
}

// TestAddChannel_RejectsInvalidGroupName verifies a newly assigned group must
// follow the naming rule even when it exists as a grandfathered catalog key.
func TestAddChannel_RejectsInvalidGroupName(t *testing.T) {
	router := setupGroupAssignmentTest(t)
	useGroupRatioCatalog(t, `{"default":1,"Old Team":1}`)

	resp := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/channel/",
		channelGroupPayload("invalid-group-channel", "Old Team"))
	require.False(t, resp.Success, "invalid group name must be rejected")
	require.Contains(t, resp.Message, `"Old Team"`)
	require.Empty(t, storedChannelGroups(t, "invalid-group-channel"))
}

// TestAddChannel_NormalizesGroupList verifies submitted group lists are
// trimmed and de-duplicated, for single and multi-key batch creation, and that
// an omitted group still defaults to "default".
func TestAddChannel_NormalizesGroupList(t *testing.T) {
	router := setupGroupAssignmentTest(t)

	resp := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/channel/",
		channelGroupPayload("normalized-channel", " default , vip ,, VIP "))
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"default,vip"}, storedChannelGroups(t, "normalized-channel"))
	require.Equal(t, []string{"default", "vip"}, storedAbilityGroups(t, "normalized-channel"))

	batch := channelGroupPayload("batch-channel", "vip , default")
	batch["key"] = "sk-batch-1\nsk-batch-2"
	resp = sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/channel/", batch)
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"vip,default", "vip,default"}, storedChannelGroups(t, "batch-channel"))

	resp = sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/channel/",
		channelGroupPayload("omitted-group-channel", ""))
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"default"}, storedChannelGroups(t, "omitted-group-channel"))
}

// TestUpdateChannel_KeepsLegacyGroup verifies editing a channel that already
// carries an unconfigured, rule-breaking group keeps working and keeps it.
func TestUpdateChannel_KeepsLegacyGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)
	channel := seedGroupAssignmentChannel(t, "legacy-channel", "default,old team")

	payload := channelGroupPayload("legacy-channel-renamed", " old team , default ")
	payload["uuid"] = channel.UUID
	resp := sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/channel/", payload)
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"old team,default"}, storedChannelGroups(t, "legacy-channel-renamed"))

	// An omitted group keeps the stored value, as before.
	payload = channelGroupPayload("legacy-channel-renamed-again", "")
	payload["uuid"] = channel.UUID
	resp = sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/channel/", payload)
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"old team,default"}, storedChannelGroups(t, "legacy-channel-renamed-again"))

	// Removing the legacy group and adding a configured one is allowed.
	payload = channelGroupPayload("legacy-channel-renamed-again", "vip")
	payload["uuid"] = channel.UUID
	resp = sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/channel/", payload)
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"vip"}, storedChannelGroups(t, "legacy-channel-renamed-again"))
}

// TestUpdateChannel_RejectsNewUnconfiguredGroup verifies an update cannot add
// a group that is neither stored on the channel nor configured in the catalog.
func TestUpdateChannel_RejectsNewUnconfiguredGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)
	channel := seedGroupAssignmentChannel(t, "update-channel", "default,old team")

	payload := channelGroupPayload("update-channel", "default,old team,partners")
	payload["uuid"] = channel.UUID
	resp := sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/channel/", payload)
	require.False(t, resp.Success, "unconfigured group must be rejected")
	require.Contains(t, resp.Message, `group "partners" is not configured`)
	require.Equal(t, []string{"default,old team"}, storedChannelGroups(t, "update-channel"))

	// A case variant of a stored group is a new name, not the stored one.
	payload = channelGroupPayload("update-channel", "default,Old Team")
	payload["uuid"] = channel.UUID
	resp = sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/channel/", payload)
	require.False(t, resp.Success, "case variant of a legacy group must be validated as new")
	require.Equal(t, []string{"default,old team"}, storedChannelGroups(t, "update-channel"))
}

// TestDuplicateChannel_CopiesLegacyGroup verifies duplicating a channel that
// carries a legacy, unconfigured group succeeds and copies the group as-is.
func TestDuplicateChannel_CopiesLegacyGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)
	channel := seedGroupAssignmentChannel(t, "duplicate-source", "default,old team")

	resp := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/channel/"+channel.UUID+"/duplicate", nil)
	require.True(t, resp.Success, resp.Message)
	require.Equal(t, []string{"default,old team"}, storedChannelGroups(t, "duplicate-source Copy"))
}

// seedGroupAssignmentUser stores a user with group written directly to the
// database, bypassing every write-path check, to model legacy data.
// Parameters: t is the active test, username is the account name and group is the raw stored value.
// Returns the stored user.
func seedGroupAssignmentUser(t *testing.T, username, group string) *model.User {
	t.Helper()

	user := &model.User{
		Username:    username,
		Password:    "hashed-password",
		DisplayName: username,
		Group:       group,
		Status:      model.UserStatusEnabled,
		Role:        model.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.NotEmpty(t, user.UUID)
	return user
}

// storedUserGroup returns the stored group of the user named username.
// Parameters: t is the active test and username is the account name.
// Returns the stored group and whether the user exists.
func storedUserGroup(t *testing.T, username string) (string, bool) {
	t.Helper()

	var users []model.User
	require.NoError(t, model.DB.Where("username = ?", username).Find(&users).Error)
	if len(users) == 0 {
		return "", false
	}
	return users[0].Group, true
}

// TestUpdateUser_RejectsUnconfiguredGroup verifies an administrator cannot move
// a user into a group that is not configured in the catalog.
func TestUpdateUser_RejectsUnconfiguredGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)
	user := seedGroupAssignmentUser(t, "group-user", "default")

	resp := sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/user/",
		map[string]any{"uuid": user.UUID, "group": "partners"})
	require.False(t, resp.Success, "unconfigured group must be rejected")
	require.Contains(t, resp.Message, `group "partners" is not configured`)
	group, ok := storedUserGroup(t, "group-user")
	require.True(t, ok)
	require.Equal(t, "default", group)

	resp = sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/user/",
		map[string]any{"uuid": user.UUID, "group": "  vip "})
	require.True(t, resp.Success, resp.Message)
	group, _ = storedUserGroup(t, "group-user")
	require.Equal(t, "vip", group)
}

// TestUpdateUser_KeepsUnchangedLegacyGroup verifies an administrator can edit a
// user whose stored group predates the catalog and naming rule.
func TestUpdateUser_KeepsUnchangedLegacyGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)
	user := seedGroupAssignmentUser(t, "legacy-user", "old team")

	resp := sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/user/",
		map[string]any{"uuid": user.UUID, "group": "old team", "display_name": "Renamed"})
	require.True(t, resp.Success, resp.Message)
	group, ok := storedUserGroup(t, "legacy-user")
	require.True(t, ok)
	require.Equal(t, "old team", group)

	resp = sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/user/",
		map[string]any{"uuid": user.UUID, "group": "   "})
	require.False(t, resp.Success, "an empty group must still be rejected")
	group, _ = storedUserGroup(t, "legacy-user")
	require.Equal(t, "old team", group)
}

// TestCreateUser_RejectsUnconfiguredGroup verifies an administrator cannot
// create a user in a group that is not configured in the catalog.
func TestCreateUser_RejectsUnconfiguredGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)

	resp := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/user/",
		map[string]any{"username": "new-partner", "password": "password123", "group": "partners"})
	require.False(t, resp.Success, "unconfigured group must be rejected")
	require.Contains(t, resp.Message, `group "partners" is not configured`)
	_, exists := storedUserGroup(t, "new-partner")
	require.False(t, exists, "rejected user must not be created")
}

// TestCreateUser_NormalizesGroup verifies the submitted group is trimmed before
// it is stored, and that an omitted group keeps the "default" group.
func TestCreateUser_NormalizesGroup(t *testing.T) {
	router := setupGroupAssignmentTest(t)

	resp := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/user/",
		map[string]any{"username": "new-vip", "password": "password123", "group": "  vip "})
	require.True(t, resp.Success, resp.Message)
	group, ok := storedUserGroup(t, "new-vip")
	require.True(t, ok)
	require.Equal(t, "vip", group)

	resp = sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/user/",
		map[string]any{"username": "new-default", "password": "password123"})
	require.True(t, resp.Success, resp.Message)
	group, ok = storedUserGroup(t, "new-default")
	require.True(t, ok)
	require.Equal(t, "default", group)
}

// TestResolveGroupAssignment covers the shared assignment rule directly: the
// default group is always assignable, stored names are grandfathered exactly,
// and new names must be valid catalog keys.
func TestResolveGroupAssignment(t *testing.T) {
	useGroupRatioCatalog(t, `{"vip":1,"Old Team":1}`)

	cases := []struct {
		name      string
		requested string
		stored    string
		want      string
		wantErr   string
	}{
		{name: "default without catalog entry", requested: "default", want: "default"},
		{name: "configured group", requested: " vip ", want: "vip"},
		{name: "blank submission", requested: " , ", want: ""},
		{name: "unconfigured group", requested: "partners", wantErr: `group "partners" is not configured`},
		{name: "invalid catalog key", requested: "Old Team", wantErr: `"Old Team"`},
		{name: "stored legacy group", requested: "Old Team,vip", stored: "Old Team", want: "Old Team,vip"},
		{name: "stored unconfigured group", requested: "partners", stored: "default,partners", want: "partners"},
		{name: "case variant of stored group", requested: "PARTNERS", stored: "partners", wantErr: `group "PARTNERS" is not configured`},
		{name: "case variant of catalog group", requested: "VIP", wantErr: `group "VIP" is not configured`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveChannelGroups(tc.requested, tc.stored)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
				require.True(t, errkind.IsClient(err), "rejections must be client errors")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	got, err := resolveUserGroup("  partners ", "partners")
	require.NoError(t, err)
	require.Equal(t, "partners", got)
	_, err = resolveUserGroup("default,vip", "")
	require.Error(t, err)
	require.True(t, errkind.IsClient(err))
}
