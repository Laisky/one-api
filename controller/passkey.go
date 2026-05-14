package controller

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

var (
	webAuthnInstance *webauthn.WebAuthn
	webAuthnOnce     sync.Once
	webAuthnErr      error
)

// getWebAuthn lazily initialises the global WebAuthn instance.
func getWebAuthn() (*webauthn.WebAuthn, error) {
	webAuthnOnce.Do(func() {
		rpID := config.WebAuthnRPID
		rpOrigins := parseRPOrigins()

		// Derive RP ID from ServerAddress when not explicitly set.
		if rpID == "" {
			u, err := url.Parse(config.ServerAddress)
			if err == nil && u.Hostname() != "" {
				rpID = u.Hostname()
			} else {
				rpID = "localhost"
			}
		}

		// Derive origins from ServerAddress when not explicitly set.
		if len(rpOrigins) == 0 {
			rpOrigins = []string{strings.TrimRight(config.ServerAddress, "/")}
		}

		cfg := &webauthn.Config{
			RPDisplayName: config.SystemName,
			RPID:          rpID,
			RPOrigins:     rpOrigins,
		}

		webAuthnInstance, webAuthnErr = webauthn.New(cfg)
		if webAuthnErr != nil {
			logger.Logger.Error("failed to initialise WebAuthn", zap.Error(webAuthnErr))
		} else {
			logger.Logger.Info("WebAuthn initialised",
				zap.String("rpId", rpID),
				zap.Strings("rpOrigins", rpOrigins))
		}
	})
	return webAuthnInstance, webAuthnErr
}

func parseRPOrigins() []string {
	raw := config.WebAuthnRPOrigins
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ------------------------------------------------------------------
// Registration (authenticated user adds a new passkey)
// ------------------------------------------------------------------

// PasskeyRegisterBegin starts the WebAuthn registration ceremony.
// The user must be logged in. The resulting challenge is stored in the session.
func PasskeyRegisterBegin(c *gin.Context) {
	w, err := getWebAuthn()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "WebAuthn not available"))
		return
	}

	userId := c.GetInt(ctxkey.Id)
	var user *model.User
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			user = u
		}
	}
	if user == nil {
		var err error
		user, err = model.GetUserById(userId, false)
		if err != nil {
			helper.RespondError(c, errors.Wrap(errors.Wrapf(err, "get user %d", userId), "failed to get user"))
			return
		}
	}

	if user.Metadata.PasswordLocked {
		helper.RespondError(c, errors.New("MFA enrollment is locked by administrator"))
		return
	}

	wUser, err := model.NewWebAuthnUser(user)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to create webauthn user"))
		return
	}

	// Exclude credentials already registered so the authenticator doesn't re-use them.
	excludeList := make([]protocol.CredentialDescriptor, 0, len(wUser.Credentials))
	for _, cred := range wUser.Credentials {
		excludeList = append(excludeList, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.CredentialID,
			Transport:    model.PasskeyCredentialToWebAuthn(cred).Transport,
		})
	}

	creation, sessionData, err := w.BeginRegistration(wUser,
		webauthn.WithExclusions(excludeList),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to begin registration"))
		return
	}

	// Store session data for the finish step.
	sessBytes, _ := json.Marshal(sessionData)
	session := sessions.Default(c)
	session.Set("webauthn_register_session", string(sessBytes))
	if err = session.Save(); err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to save session"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    creation,
	})
}

// PasskeyRegisterFinish completes the WebAuthn registration ceremony.
func PasskeyRegisterFinish(c *gin.Context) {
	w, err := getWebAuthn()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "WebAuthn not available"))
		return
	}

	userId := c.GetInt(ctxkey.Id)
	var user *model.User
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			user = u
		}
	}
	if user == nil {
		var err error
		user, err = model.GetUserById(userId, false)
		if err != nil {
			helper.RespondError(c, errors.Wrap(errors.Wrapf(err, "get user %d", userId), "failed to get user"))
			return
		}
	}

	if user.Metadata.PasswordLocked {
		helper.RespondError(c, errors.New("MFA enrollment is locked by administrator"))
		return
	}

	wUser, err := model.NewWebAuthnUser(user)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to create webauthn user"))
		return
	}

	// Restore session data.
	session := sessions.Default(c)
	sessStr, ok := session.Get("webauthn_register_session").(string)
	if !ok || sessStr == "" {
		helper.RespondError(c, errors.New("no registration session found, please start again"))
		return
	}

	var sessionData webauthn.SessionData
	if err = json.Unmarshal([]byte(sessStr), &sessionData); err != nil {
		helper.RespondError(c, errors.Wrap(err, "invalid session data"))
		return
	}

	credential, err := w.FinishRegistration(wUser, sessionData, c.Request)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "registration failed"))
		return
	}

	// Read optional credential name from query.
	credName := strings.TrimSpace(c.Query("name"))
	if credName == "" {
		credName = "Passkey"
	}
	if len(credName) > 128 {
		credName = credName[:128]
	}

	// Persist.
	dbCred := &model.PasskeyCredential{
		UserId:          userId,
		CredentialName:  credName,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		Transport:       model.TransportsToString(credential.Transport),
	}
	if err = model.CreatePasskeyCredential(dbCred); err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to save credential"))
		return
	}

	// Clean up session.
	session.Delete("webauthn_register_session")
	_ = session.Save()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Passkey registered successfully",
		"data": gin.H{
			"id":   dbCred.Id,
			"name": dbCred.CredentialName,
		},
	})
}

// ------------------------------------------------------------------
// Discoverable Login (passwordless – no user ID needed up front)
// ------------------------------------------------------------------

// PasskeyLoginBegin starts a discoverable login ceremony (passkey login).
// This endpoint is public – no authentication required.
func PasskeyLoginBegin(c *gin.Context) {
	w, err := getWebAuthn()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "WebAuthn not available"))
		return
	}

	assertion, sessionData, err := w.BeginDiscoverableLogin()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to begin login"))
		return
	}

	sessBytes, _ := json.Marshal(sessionData)
	session := sessions.Default(c)
	session.Set("webauthn_login_session", string(sessBytes))
	if err = session.Save(); err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to save session"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    assertion,
	})
}

// PasskeyLoginFinish completes the discoverable login ceremony.
func PasskeyLoginFinish(c *gin.Context) {
	w, err := getWebAuthn()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "WebAuthn not available"))
		return
	}

	session := sessions.Default(c)
	sessStr, ok := session.Get("webauthn_login_session").(string)
	if !ok || sessStr == "" {
		helper.RespondError(c, errors.New("no login session found, please start again"))
		return
	}

	var sessionData webauthn.SessionData
	if err = json.Unmarshal([]byte(sessStr), &sessionData); err != nil {
		helper.RespondError(c, errors.Wrap(err, "invalid session data"))
		return
	}

	// The handler resolves the user from the credential's userHandle.
	// We capture the resolved user in the closure so we can use it after FinishDiscoverableLogin,
	// because sessionData.UserID is empty for discoverable login (no user is known at begin time).
	var resolvedUser *model.User
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) < 8 {
			return nil, errors.New("invalid user handle")
		}
		userId := int(binary.BigEndian.Uint64(userHandle))
		user, err := model.GetUserById(userId, true)
		if err != nil {
			return nil, errors.Wrapf(err, "user not found for handle")
		}
		if user.Status != model.UserStatusEnabled {
			return nil, errors.New("user account is disabled")
		}
		resolvedUser = user
		wUser, err := model.NewWebAuthnUser(user)
		if err != nil {
			return nil, errors.Wrap(err, "build webauthn user")
		}
		return wUser, nil
	}

	credential, err := w.FinishDiscoverableLogin(handler, sessionData, c.Request)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "login failed"))
		return
	}

	// Update sign count and backup state.
	dbCred, err := model.GetPasskeyCredentialByCredentialID(credential.ID)
	if err == nil {
		model.UpdatePasskeyAfterLogin(dbCred.Id, credential.Authenticator.SignCount, credential.Flags.BackupState)
	}

	if resolvedUser == nil {
		helper.RespondError(c, errors.New("failed to resolve user from credential"))
		return
	}

	// Clean up session.
	session.Delete("webauthn_login_session")
	_ = session.Save()

	// Use the same login setup as password login.
	SetupLogin(resolvedUser, c)
}

// ------------------------------------------------------------------
// Management (list / delete)
// ------------------------------------------------------------------

// PasskeyList returns all passkey credentials for the current user.
func PasskeyList(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	creds, err := model.GetPasskeyCredentialsByUserId(userId)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to list passkeys"))
		return
	}

	type passkeyInfo struct {
		Id             int    `json:"id"`
		CredentialName string `json:"credential_name"`
		SignCount      uint32 `json:"sign_count"`
		CreatedAt      int64  `json:"created_at"`
	}

	list := make([]passkeyInfo, 0, len(creds))
	for _, cr := range creds {
		list = append(list, passkeyInfo{
			Id:             cr.Id,
			CredentialName: cr.CredentialName,
			SignCount:      cr.SignCount,
			CreatedAt:      cr.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
	})
}

// PasskeyDelete removes a passkey credential for the current user.
func PasskeyDelete(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		helper.RespondError(c, errors.New("invalid credential id"))
		return
	}

	if err = model.DeletePasskeyCredential(id, userId); err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to delete passkey"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Passkey deleted successfully",
	})
}

// PasskeyRename renames a passkey credential.
func PasskeyRename(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		helper.RespondError(c, errors.New("invalid credential id"))
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err = json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		helper.RespondError(c, errors.New(invalidParameterMessage))
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 128 {
		helper.RespondError(c, errors.New("name must be 1-128 characters"))
		return
	}

	// Verify ownership.
	cred, err := model.GetPasskeyCredentialByID(id)
	if err != nil || cred.UserId != userId {
		helper.RespondError(c, errors.Wrap(err, "passkey not found"))
		return
	}

	cred.CredentialName = name
	if err = model.DB.Save(cred).Error; err != nil {
		helper.RespondError(c, errors.Wrap(err, "failed to rename passkey"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Passkey renamed successfully",
	})
}
