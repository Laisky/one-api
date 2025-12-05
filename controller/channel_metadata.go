package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/channeltype"
)

// GetChannelMetadata returns server-side metadata about a channel type
// Currently includes:
// - default_base_url: string (may be empty if unknown)
// - base_url_editable: bool (whether the user can modify the base URL)
// - default_endpoints: []string (list of default supported endpoint names)
// - all_endpoints: []EndpointInfo (list of all available endpoints with metadata)
// This endpoint is designed to be extended with more metadata later.
func GetChannelMetadata(c *gin.Context) {
	typeStr := c.Query("type")
	if typeStr == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "type is required",
		})
		return
	}

	channelType, err := strconv.Atoi(typeStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid type",
		})
		return
	}

	config := channeltype.GetChannelBaseURLConfig(channelType)
	defaultEndpoints := channeltype.DefaultEndpointNamesForChannelType(channelType)
	allEndpoints := channeltype.AllEndpoints()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"default_base_url":  config.URL,
			"base_url_editable": config.Editable,
			"default_endpoints": defaultEndpoints,
			"all_endpoints":     allEndpoints,
		},
	})
}
