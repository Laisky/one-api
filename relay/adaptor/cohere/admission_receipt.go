package cohere

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

const admissionRejectedKey = "cohere_admission_rejected"

// RejectedBeforeInference reports an explicit pre-inference rejection: an
// invalid body, authentication, billing-limit, unknown resource or rate-limit
// refusal. Server errors, cancellation and transport failures remain uncertain.
func RejectedBeforeInference(c *gin.Context) bool { return c != nil && c.GetBool(admissionRejectedKey) }

// ClearRejection removes stale receipt state before a new network attempt.
func ClearRejection(c *gin.Context) {
	if c != nil {
		c.Set(admissionRejectedKey, false)
	}
}

// recordAdmissionRejection interprets only explicit rejected admission statuses.
// Cohere error semantics (https://docs.cohere.com/reference/errors, checked
// 2026-10-05): 400 "the body of the request is not valid", 401 missing/invalid
// key, 402 "the account has reached its billing limit", 404 "the requested
// resource is not found" and 429 rate limit all refuse the request before any
// search is performed, so the hold is released. 403 is treated the same as 401.
// 499 (client cancellation) and any 5xx are not proof that inference was free
// and must never release a paid hold.
func recordAdmissionRejection(c *gin.Context, response *http.Response) {
	if c == nil || response == nil {
		return
	}
	switch response.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusPaymentRequired,
		http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests:
		c.Set(admissionRejectedKey, true)
	}
}
