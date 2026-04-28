package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
)

func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := helper.GenRequestID()
		c.Set(helper.RequestIdKey, id)
		c.Header(helper.RequestIdKey, id)
		c.Next()
	}
}
