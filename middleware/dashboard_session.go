package middleware

import (
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/model"
)

// resolveDashboardUser verifies a signed session's identity against the current
// primary account record, not a stale cache or cookie-carried authorization.
// The signed role is an upper bound: promotions require a new login, while
// demotions, disablement and deletion apply on the very next request.
func resolveDashboardUser(c *gin.Context) (*model.User, error) {
	session := sessions.Default(c)
	claimedUsername := session.Get("username")
	if claimedUsername == nil {
		token := c.Request.Header.Get("Authorization")
		if token == "" {
			return nil, nil
		}
		if model.DB == nil {
			return nil, errors.New("dashboard identity store unavailable")
		}
		user := model.ValidateAccessToken(token)
		if user == nil || user.Username == "" {
			return nil, nil
		}
		return user, nil
	}
	username, validName := claimedUsername.(string)
	id, validID := session.Get("id").(int)
	signedRole, validRole := session.Get("role").(int)
	if !validName || username == "" || !validID || id <= 0 || !validRole || signedRole < model.RoleCommonUser {
		return nil, nil
	}
	if model.DB == nil {
		return nil, errors.New("dashboard identity store unavailable")
	}
	var user model.User
	err := model.DB.WithContext(gmw.Ctx(c)).Omit("password", "access_token").Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "resolve current dashboard account")
	}
	user.Role = min(user.Role, signedRole)
	return &user, nil
}

// bindDashboardUser gives downstream handlers one consistent effective identity;
// the role on UserObj matches the context role and cannot bypass the session cap.
func bindDashboardUser(c *gin.Context, user *model.User) {
	c.Set(ctxkey.UserObj, user)
	c.Set(ctxkey.UserUUID, user.UUID)
	c.Set(ctxkey.Username, user.Username)
	c.Set(ctxkey.Role, user.Role)
	c.Set(ctxkey.Id, user.Id)
	identity.BindFromGin(c)
}

// clearInvalidDashboardSession drops a rejected browser session while preserving
// bearer-only and anonymous requests. Failure diagnostics contain no credentials.
func clearInvalidDashboardSession(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get("username") == nil {
		return
	}
	session.Clear()
	if err := session.Save(); err != nil {
		gmw.GetLogger(c).Warn("failed to clear invalid dashboard session", zap.Error(err))
	}
}

// dashboardSessionRoleInsufficient permits only an early rejection. An absent
// or malformed claim is handled by the validating resolver, never promoted.
func dashboardSessionRoleInsufficient(c *gin.Context, minRole int) bool {
	session := sessions.Default(c)
	if session.Get("username") == nil {
		return false
	}
	role, ok := session.Get("role").(int)
	return ok && role < minRole
}
