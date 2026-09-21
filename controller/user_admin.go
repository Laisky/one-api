package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/blacklist"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
)

// GetAllUsers lists paginated user DTOs for an administrator.
// Parameter c supplies paging and sorting values and receives the JSON response.
// It returns no value; route middleware authorizes access.
func GetAllUsers(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}

	// Get page size from query parameter, default to config value

	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	order := c.DefaultQuery("order", "")
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	users, err := model.GetAllUsers(p*size, size, order, sortBy, sortOrder)

	if err != nil {
		helper.RespondError(c, err)
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetUserCount()
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    model.UsersToResponses(users),
		"total":   totalCount,
	})
}

// SearchUsers searches user DTOs by keyword for an administrator.
// Parameter c supplies the search and sorting values and receives the response.
// It returns no value; route middleware authorizes access.
func SearchUsers(c *gin.Context) {
	keyword := c.Query("keyword")
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	users, err := model.SearchUsers(keyword, sortBy, sortOrder)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    model.UsersToResponses(users),
	})
}

// GetUser reads a referenced user subject to the caller's role.
// Parameter c supplies the user reference and authenticated role. It returns no
// value and writes either a user DTO or an error response.
func GetUser(c *gin.Context) {
	id, err := resolveUserRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	myRole := c.GetInt(ctxkey.Role)
	if myRole <= user.Role && myRole != model.RoleRootUser {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to get information of users at the same level or higher")))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.ToResponse(),
	})
}

// UpdateUser applies validated, authorized partial account updates.
// Parameter c supplies the administrator identity and JSON payload. It returns
// no value and writes the outcome, recording quota changes in the audit log.
func UpdateUser(c *gin.Context) {
	ctx := gmw.Ctx(c)
	adminUserID := c.GetInt(ctxkey.Id)
	body, err := common.GetRequestBody(c)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	var payload dto.UserAdminUpdatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	ref, err := preferUUIDRef(payload.UUID, payload.Id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	payload.Id, err = resolveUserRef(ref)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	payload.UUID = ""

	originUser, err := model.GetUserById(payload.Id, false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	myRole := c.GetInt(ctxkey.Role)
	if myRole <= originUser.Role && myRole != model.RoleRootUser {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to update user information with the same permission level or higher permission level")))
		return
	}

	updates := make(map[string]any)
	var (
		quotaUpdated  bool
		newQuota      int64
		statusChanged bool
		newStatus     int
	)

	if rawFieldPresent(raw, "username") {
		if jsonRawIsNull(raw["username"]) {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Username cannot be null")))
			return
		}
		var username string
		if err := json.Unmarshal(raw["username"], &username); err != nil {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
			return
		}
		username = strings.TrimSpace(username)
		if username == "" {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Username cannot be empty")))
			return
		}
		if utf8.RuneCountInString(username) < 3 || utf8.RuneCountInString(username) > 30 {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Username must be between 3 and 30 characters")))
			return
		}
		if myRole <= originUser.Role && myRole != model.RoleRootUser && username != originUser.Username {
			helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to rename this user")))
			return
		}
		updates["username"] = username
	}

	if rawFieldPresent(raw, "display_name") {
		if jsonRawIsNull(raw["display_name"]) {
			// nil => no change
		} else {
			var displayName string
			if err := json.Unmarshal(raw["display_name"], &displayName); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			displayName = strings.TrimSpace(displayName)
			if utf8.RuneCountInString(displayName) > 20 {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Display name cannot exceed 20 characters")))
				return
			}
			updates["display_name"] = displayName
		}
	}

	if rawFieldPresent(raw, "email") {
		if jsonRawIsNull(raw["email"]) {
			// nil => no change
		} else {
			var email string
			if err := json.Unmarshal(raw["email"], &email); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			email = strings.TrimSpace(email)
			if email != "" {
				if utf8.RuneCountInString(email) > 50 {
					helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Email cannot exceed 50 characters")))
					return
				}
				if err := common.Validate.Var(email, "email"); err != nil {
					helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Valid email is required")))
					return
				}
			}
			updates["email"] = email
		}
	}

	if rawFieldPresent(raw, "group") {
		if jsonRawIsNull(raw["group"]) {
			// nil => no change
		} else {
			var group string
			if err := json.Unmarshal(raw["group"], &group); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			group = strings.TrimSpace(group)
			if group == "" {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Group cannot be empty")))
				return
			}
			if utf8.RuneCountInString(group) > 32 {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Group cannot exceed 32 characters")))
				return
			}
			updates["group"] = group
		}
	}

	if rawFieldPresent(raw, "mcp_tool_blacklist") {
		if jsonRawIsNull(raw["mcp_tool_blacklist"]) {
			updates["mcp_tool_blacklist"] = nil
		} else {
			var blacklist model.JSONStringSlice
			if err := json.Unmarshal(raw["mcp_tool_blacklist"], &blacklist); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			updates["mcp_tool_blacklist"] = blacklist
		}
	}

	if rawFieldPresent(raw, "quota") {
		if jsonRawIsNull(raw["quota"]) {
			// nil => no change
		} else {
			if payload.Quota == nil {
				var quotaValue int64
				if err := json.Unmarshal(raw["quota"], &quotaValue); err != nil {
					helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
					return
				}
				payload.Quota = &quotaValue
			}
			if *payload.Quota < 0 {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Quota must be non-negative")))
				return
			}
			newQuota = *payload.Quota
			updates["quota"] = newQuota
			quotaUpdated = true
		}
	}

	var (
		metadataUpdated bool
		mergedMetadata  model.UserMetadata
	)

	if rawFieldPresent(raw, "metadata") {
		if jsonRawIsNull(raw["metadata"]) {
			// nil => no change
		} else {
			var metaPayload dto.UserMetadataPayload
			if err := json.Unmarshal(raw["metadata"], &metaPayload); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			merged := originUser.Metadata
			if metaPayload.PasswordLocked != nil {
				if myRole != model.RoleRootUser {
					helper.RespondError(c, errkind.ForbiddenErr(errors.New("Only root admin can change password lock")))
					return
				}
				merged.PasswordLocked = *metaPayload.PasswordLocked
			}
			encoded, encErr := json.Marshal(merged)
			if encErr != nil {
				helper.RespondError(c, errors.Wrap(encErr, "encode user metadata"))
				return
			}
			updates["metadata"] = string(encoded)
			mergedMetadata = merged
			metadataUpdated = true
		}
	}

	effectivePasswordLocked := originUser.Metadata.PasswordLocked
	if metadataUpdated {
		effectivePasswordLocked = mergedMetadata.PasswordLocked
	}

	if rawFieldPresent(raw, "password") {
		if jsonRawIsNull(raw["password"]) {
			// nil => no change
		} else {
			var password string
			if err := json.Unmarshal(raw["password"], &password); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			password = strings.TrimSpace(password)
			if password == "" {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Password cannot be empty")))
				return
			}
			if effectivePasswordLocked {
				helper.RespondError(c, errkind.ForbiddenErr(errors.New("Password is locked for this user")))
				return
			}
			if utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 20 {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Password length must be between 8 and 20 characters")))
				return
			}
			hashed, hashErr := common.Password2Hash(password)
			if hashErr != nil {
				helper.RespondError(c, hashErr)
				return
			}
			updates["password"] = hashed
		}
	}

	if rawFieldPresent(raw, "role") {
		if jsonRawIsNull(raw["role"]) {
			// nil => no change
		} else {
			var roleValue int
			if payload.Role != nil {
				roleValue = *payload.Role
			} else if err := json.Unmarshal(raw["role"], &roleValue); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			if myRole <= roleValue && myRole != model.RoleRootUser {
				helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to promote other users to a permission level greater than or equal to your own")))
				return
			}
			updates["role"] = roleValue
		}
	}

	if rawFieldPresent(raw, "status") {
		if jsonRawIsNull(raw["status"]) {
			// nil => no change
		} else {
			var statusValue int
			if payload.Status != nil {
				statusValue = *payload.Status
			} else if err := json.Unmarshal(raw["status"], &statusValue); err != nil {
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
				return
			}
			switch statusValue {
			case model.UserStatusEnabled, model.UserStatusDisabled, model.UserStatusDeleted:
			default:
				helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Invalid status provided")))
				return
			}
			updates["status"] = statusValue
			statusChanged = originUser.Status != statusValue
			newStatus = statusValue
		}
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
		})
		return
	}

	if username, ok := updates["username"].(string); ok && username != originUser.Username && model.IsUsernameAlreadyTaken(username) {
		respondRegisterUsernameTaken(c)
		return
	}

	if err := model.DB.Model(&model.User{}).Where("id = ?", payload.Id).Updates(updates).Error; err != nil {
		if isRegisterUsernameTakenError(err) {
			respondRegisterUsernameTaken(c)
			return
		}
		// Admin endpoint: the updated account is not the caller, so tag the error
		// with its identity (transparent to Error()/errors.Is/strings.Contains).
		helper.RespondError(c, identity.Tag(
			errors.Wrapf(err, "failed to update user: id=%d", payload.Id), originUser.Ref()))
		return
	}

	if statusChanged {
		switch newStatus {
		case model.UserStatusDisabled:
			blacklist.BanUser(payload.Id)
		case model.UserStatusEnabled:
			blacklist.UnbanUser(payload.Id)
		}
	}

	if quotaUpdated && originUser.Quota != newQuota {
		note := adminActorNote(adminUserID)
		model.RecordManageLog(ctx, originUser.Id, "quota", common.LogQuota(originUser.Quota), common.LogQuota(newQuota), note)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteUser deletes a referenced account below the caller's role.
// Parameter c supplies the user reference and authenticated role. It returns no
// value and writes a success or error response.
func DeleteUser(c *gin.Context) {
	id, err := resolveUserRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	originUser, err := model.GetUserById(id, false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to delete users with the same permission level or higher permission level")))
		return
	}
	err = model.DeleteUserById(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// CreateUser creates an account from an administrator's validated request.
// Parameter c carries the authenticated role and account payload. It returns no
// value and writes the result after applying requested quota and group overrides.
func CreateUser(c *gin.Context) {
	ctx := gmw.Ctx(c)
	lg := gmw.GetLogger(c)
	var req dto.UserCreateRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil || req.Username == "" || req.Password == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}
	// UUID/inviter_uuid are not part of the request DTO, so a client can no
	// longer smuggle them in; the explicit resets are no longer needed.
	if err := common.Validate.Struct(&req); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidInputMessage)))
		return
	}
	// Disallow empty username/display name
	if strings.TrimSpace(req.Username) == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Username cannot be empty")))
		return
	}
	if req.DisplayName != "" && strings.TrimSpace(req.DisplayName) == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Display name cannot be empty if provided")))
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	myRole := c.GetInt("role")
	if req.Role >= myRole {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("Unable to create users with permissions greater than or equal to your own")))
		return
	}
	// Even for admin users, we cannot fully trust them!
	cleanUser := model.User{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Email:       req.Email,
	}
	if model.IsUsernameAlreadyTaken(cleanUser.Username) {
		respondRegisterUsernameTaken(c)
		return
	}
	if err := cleanUser.Insert(ctx, 0); err != nil {
		if isRegisterUsernameTakenError(err) {
			respondRegisterUsernameTaken(c)
			return
		}
		helper.RespondError(c, err)
		return
	}

	// Apply admin-specified quota and group after Insert, which resets them to defaults.
	postUpdates := map[string]any{}
	if req.Quota > 0 {
		postUpdates["quota"] = req.Quota
	}
	if req.Group != "" {
		postUpdates["group"] = req.Group
	}
	if len(postUpdates) > 0 {
		if err := model.DB.Model(&model.User{}).Where("id = ?", cleanUser.Id).Updates(postUpdates).Error; err != nil {
			// The created account is not the caller, so it is named explicitly.
			lg.Error("failed to apply admin overrides on created user",
				append(cleanUser.Ref().Zap(), zap.Error(err))...)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type ManageRequest struct {
	Username string `json:"username"`
	Action   string `json:"action"`
}

// ManageUser Only admin user can do this
func ManageUser(c *gin.Context) {
	var req ManageRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)

	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}
	user := model.User{
		Username: req.Username,
	}
	// Fill attributes
	model.DB.Where(&user).First(&user)
	if user.Id == 0 {
		helper.RespondError(c, errkind.NotFoundErr(errors.New("User does not exist")))
		return
	}
	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != model.RoleRootUser {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to update user information with the same permission level or higher permission level")))
		return
	}
	switch req.Action {
	case "disable":
		user.Status = model.UserStatusDisabled
		if user.Role == model.RoleRootUser {
			helper.RespondError(c, errkind.ForbiddenErr(errors.New("Unable to disable super administrator user")))
			return
		}
	case "enable":
		user.Status = model.UserStatusEnabled
	case "delete":
		if user.Role == model.RoleRootUser {
			helper.RespondError(c, errkind.ForbiddenErr(errors.New("Unable to delete super administrator user")))
			return
		}
		if err := user.Delete(); err != nil {
			helper.RespondError(c, err)
			return
		}
	case "promote":
		if myRole != model.RoleRootUser {
			helper.RespondError(c, errkind.ForbiddenErr(errors.New("Ordinary administrator users cannot promote other users to administrators")))
			return
		}
		if user.Role >= model.RoleAdminUser {
			helper.RespondError(c, errkind.ConflictErr(errors.New("The user is already an administrator")))
			return
		}
		user.Role = model.RoleAdminUser
	case "demote":
		if user.Role == model.RoleRootUser {
			helper.RespondError(c, errkind.ForbiddenErr(errors.New("Unable to downgrade super administrator user")))
			return
		}
		if user.Role == model.RoleCommonUser {
			helper.RespondError(c, errkind.ConflictErr(errors.New("The user is already an ordinary user")))
			return
		}
		user.Role = model.RoleCommonUser
	}

	if err := user.Update(false); err != nil {
		helper.RespondError(c, err)
		return
	}
	clearUser := model.User{
		Role:   user.Role,
		Status: user.Status,
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    clearUser.ToResponse(),
	})
}

type adminTopUpRequest struct {
	UserId   int    `json:"user_id"`
	UserUUID string `json:"user_uuid"`
	Quota    int    `json:"quota"`
	Remark   string `json:"remark"`
}

// AdminTopUp adjusts the referenced account's quota and records a top-up log.
// Parameter c supplies the administrator-authorized JSON request and context.
// It returns no value and writes a success or error response.
func AdminTopUp(c *gin.Context) {
	ctx := gmw.Ctx(c)
	req := adminTopUpRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		// Malformed request body: the caller sent JSON this endpoint cannot bind.
		helper.RespondError(c, errkind.InvalidRequestErr(err))
		return
	}
	ref, err := preferUUIDRef(req.UserUUID, req.UserId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	req.UserId, err = resolveUserRef(ref)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	req.UserUUID = ""
	err = model.IncreaseUserQuota(ctx, req.UserId, int64(req.Quota))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if req.Remark == "" {
		req.Remark = fmt.Sprintf("Recharged via API %s", common.LogQuota(int64(req.Quota)))
	}
	model.RecordTopupLog(ctx, req.UserId, req.Remark, req.Quota)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// adminActorNote renders the acting admin for a management-log note using the
// external UUID and username only, never the internal integer id.
// Parameters:
//   - adminUserId: internal id of the acting admin.
//
// Return values:
//   - string: "admin=<username> admin_uuid=<uuid>" (whichever parts resolve),
//     or "admin=unknown" when neither can be resolved.
func adminActorNote(adminUserId int) string {
	parts := []string{}
	if username := strings.TrimSpace(model.GetUsernameById(adminUserId)); username != "" {
		parts = append(parts, "admin="+username)
	}
	if adminUUID, err := model.GetUserUUIDByID(adminUserId); err == nil && strings.TrimSpace(adminUUID) != "" {
		parts = append(parts, "admin_uuid="+strings.TrimSpace(adminUUID))
	}
	if len(parts) == 0 {
		return "admin=unknown"
	}
	return strings.Join(parts, " ")
}
