package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// GetGroups returns the configured billing group names (the GroupRatio keys),
// sorted. It reads the catalog through the lock-guarded accessor.
func GetGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    billingratio.GroupNames(),
	})
}
