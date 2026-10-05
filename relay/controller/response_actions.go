package controller

import (
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

// RelayResponseAPIGetHelper retrieves an owner-bound response for c and returns any authorization or storage error.
func RelayResponseAPIGetHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	_, err := serveGatewayResponseGet(c, metalib.GetByContext(c), c.Param("response_id"))
	return err
}

// RelayResponseAPIDeleteHelper tombstones an owner-bound response for c and returns any authorization or storage error.
func RelayResponseAPIDeleteHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	_, err := serveGatewayResponseDelete(c, metalib.GetByContext(c), c.Param("response_id"))
	return err
}

// RelayResponseAPICancelHelper validates ownership for c and returns the unsupported cancellation or lookup error.
func RelayResponseAPICancelHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	_, err := serveGatewayResponseCancel(c, metalib.GetByContext(c), c.Param("response_id"))
	return err
}
