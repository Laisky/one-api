package utils

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/ctxkey"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/gin-gonic/gin"
	"net/http"
)

// NoInferenceRetry prevents SDK retry policy from replaying possibly accepted
// paid generation. The gateway's existing admission/retry lifecycle stays owner.
func NoInferenceRetry(options *bedrockruntime.Options) { options.Retryer = aws.NopRetryer{} }

// MarkInvocation runs immediately before the actual SDK/network invocation, not
// before request conversion or preparation, and never relies on client input.
func MarkInvocation(c *gin.Context) { c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true) }

// InvocationError preserves uncertain-work holds. A matching typed SDK admission
// rejection is evidence for refund only because this invocation cannot replay.
func InvocationError(c *gin.Context, err error) *relaymodel.ErrorWithStatusCode {
	status := http.StatusInternalServerError
	var response *smithyhttp.ResponseError
	if errors.As(err, &response) {
		status = response.HTTPStatusCode()
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDeniedException", "UnrecognizedClientException", "InvalidSignatureException", "ExpiredTokenException", "ValidationException", "ResourceNotFoundException", "ThrottlingException":
			if status == 400 || status == 401 || status == 403 || status == 404 || status == 429 {
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
				err = &admissionRejection{err}
			}
		}
	}
	failure := WrapErr(err)
	failure.StatusCode = status
	return failure
}

// admissionRejection is private proof of a typed, non-retried SDK admission error.
type admissionRejection struct{ error }

// IsAdmissionRejection never accepts caller JSON, message matching or arbitrary status codes.
func IsAdmissionRejection(failure *relaymodel.ErrorWithStatusCode) bool {
	if failure == nil {
		return false
	}
	var proof *admissionRejection
	return errors.As(failure.RawError, &proof)
}
