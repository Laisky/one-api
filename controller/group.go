package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// GetGroups returns the list of configured billing groups.
func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range billingratio.GroupRatio {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}
