package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

// RelayPanicRecover recovers relay panics and records content-free diagnostics.
func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Never read or log request content while recovering. Panic values may
				// also contain upstream payloads or credentials, so record their type.
				gmw.GetLogger(c).Error("panic detected",
					zap.String("panic_type", fmt.Sprintf("%T", err)),
					zap.String("stacktrace", string(debug.Stack())),
					zap.String("method", c.Request.Method),
					zap.String("route", c.FullPath()),
					zap.Int64("body_bytes", max(c.Request.ContentLength, 0)),
					zap.Bool("body_logging_suppressed", true))
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
