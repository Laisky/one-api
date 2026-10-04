package cohere

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

const admissionRejectedKey = "cohere_admission_rejected"

// RejectedBeforeInference reports an explicit authentication or rate-limit
// rejection. Server errors, cancellation and transport failures remain uncertain.
func RejectedBeforeInference(c *gin.Context) bool { return c != nil && c.GetBool(admissionRejectedKey) }

// ClearRejection removes stale receipt state before a new network attempt.
func ClearRejection(c *gin.Context) {
	if c != nil {
		c.Set(admissionRejectedKey, false)
	}
}

// recordAdmissionRejection interprets only explicit rejected admission statuses.
// Cohere error semantics: https://docs.cohere.com/reference/errors . A generic
// 5xx is not proof that inference was free and must never release a paid hold.
func recordAdmissionRejection(c *gin.Context, response *http.Response) {
	if c == nil || response == nil {
		return
	}
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		c.Set(admissionRejectedKey, true)
	}
}
