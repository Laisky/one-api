package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
)

// validateOAuthState verifies that the callback query state matches the session state.
// It returns true when the state is present and equal, then consumes the session state
// to prevent replay; otherwise it emits a sanitized DEBUG log and writes an error response.
func validateOAuthState(c *gin.Context, provider string) bool {
	logger := gmw.GetLogger(c)
	session := sessions.Default(c)
	requestState := c.Query("state")
	sessionStateRaw := session.Get("oauth_state")
	sessionState, sessionStateOK := sessionStateRaw.(string)
	stateMatch := requestState != "" && sessionStateOK && sessionState != "" &&
		subtle.ConstantTimeCompare([]byte(requestState), []byte(sessionState)) == 1

	if !stateMatch {
		logger.Debug("oauth state validation failed",
			zap.String("provider", provider),
			zap.Bool("request_state_present", requestState != ""),
			zap.Int("request_state_len", len(requestState)),
			zap.Bool("session_state_present", sessionStateRaw != nil),
			zap.String("session_state_type", fmt.Sprintf("%T", sessionStateRaw)),
			zap.Bool("session_state_valid", sessionStateOK && sessionState != ""),
			zap.Int("session_state_len", len(sessionState)),
			zap.Bool("session_username_present", session.Get("username") != nil),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
		)
		helper.RespondErrorWithStatus(c, http.StatusForbidden, errors.New("state is empty or not same"))
		return false
	}

	session.Delete("oauth_state")
	if err := session.Save(); err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "consume oauth state"))
		return false
	}

	logger.Debug("oauth state validation succeeded",
		zap.String("provider", provider),
		zap.Bool("session_username_present", session.Get("username") != nil),
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
	)

	return true
}
