package openai_compatible

import (
	"io"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/errkind"
)

// readAndCloseResponseBody reads and closes a buffered upstream body exactly once.
//
// Parameters:
//   - c: the request context used for reporting a secondary cleanup failure.
//   - body: the upstream response body whose ownership transfers to this function.
//
// Return values:
//   - []byte: bytes received, including a prefix when reading fails.
//   - error: the original read failure, marked as upstream without changing its message.
//   - error: a close failure when reading succeeded; secondary close failures are logged.
//
// A failed read takes precedence over a failed Close, preserving cancellation and
// transport causes for the caller's existing error and health policies.
func readAndCloseResponseBody(c *gin.Context, body io.ReadCloser) ([]byte, error, error) {
	responseBody, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil {
		if closeErr != nil {
			logger := gmw.GetLogger(c)
			logger.Debug("failed to close upstream response body after read failure",
				zap.Error(errors.WithStack(closeErr)))
		}
		return responseBody, errkind.Mark(errors.WithStack(readErr), errkind.Upstream), nil
	}
	if closeErr != nil {
		return responseBody, nil, errors.WithStack(closeErr)
	}
	return responseBody, nil, nil
}
