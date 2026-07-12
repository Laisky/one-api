package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// isDuplicateDBErrorForField reports whether err looks like a database unique
// constraint violation for one of the given field or index names. It accepts
// wrapped errors from model methods and normalizes common database driver text.
func isDuplicateDBErrorForField(err error, fieldNames ...string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	duplicateSignals := []string{
		"duplicate",
		"unique constraint",
		"unique violation",
		"sqlstate 23505",
		"error 1062",
	}
	hasDuplicateSignal := false
	for _, signal := range duplicateSignals {
		if strings.Contains(msg, signal) {
			hasDuplicateSignal = true
			break
		}
	}
	if !hasDuplicateSignal {
		return false
	}
	for _, fieldName := range fieldNames {
		if strings.Contains(msg, strings.ToLower(fieldName)) {
			return true
		}
	}
	return false
}

// IsUsernameAlreadyTakenError reports whether err came from the users.username
// unique constraint. It accepts wrapped database-driver errors from model
// insert and update paths.
func IsUsernameAlreadyTakenError(err error) bool {
	return isDuplicateDBErrorForField(err, "username", "users.username", "uni_users_username")
}

// RespondUsernameAlreadyExists returns the public duplicate-username response
// shared by registration, user management, and OAuth auto-provisioning flows.
func RespondUsernameAlreadyExists(c *gin.Context) {
	respondDuplicateOperation(c, "Username already exists")
}

// respondDuplicateOperation returns a public duplicate-operation failure
// without exposing database driver details to the caller.
func respondDuplicateOperation(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": message,
	})
}
