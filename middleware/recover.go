package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
)

func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				body, _ := common.GetRequestBody(c)
				gmw.GetLogger(c).Error("panic detected",
					zap.Any("panic", err),
					zap.String("stacktrace", string(debug.Stack())),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.ByteString("request_body", body))
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"message": fmt.Sprintf("Panic detected, error: %v. Please submit an issue with the related log here: https://github.com/Laisky/one-api", err),
						"type":    "one_api_panic",
					},
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
