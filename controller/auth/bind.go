package auth

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

// resolveBindingUserID authenticates the dashboard session that started an
// OAuth bind with the same checks as the UserAuth middleware: the session must
// carry a well-formed identity of an existing, enabled and unbanned account,
// and a rejected session is cleared. It returns the account id and true, or
// false after writing a 401/403 response. Callers must resolve the account
// before contacting the identity provider.
func resolveBindingUserID(c *gin.Context) (int, bool) {
	if !middleware.AuthenticateDashboardUser(c, model.RoleCommonUser) {
		return 0, false
	}
	userID := c.GetInt(ctxkey.Id)
	if userID <= 0 {
		helper.RespondErrorWithStatus(c, http.StatusUnauthorized, errkind.UnauthorizedErr(errors.New("No permission to perform this operation, authentication is invalid")))
		return 0, false
	}
	return userID, true
}

// bindOAuthIdentity stores value in column for the enabled account userID and
// writes the bind response: successMessage on success, or the classified error.
// It returns no value.
func bindOAuthIdentity(c *gin.Context, userID int, column model.OAuthIdentityColumn, value, successMessage string) {
	if err := model.BindUserOAuthIdentity(gmw.Ctx(c), userID, column, value); err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": successMessage,
	})
}
