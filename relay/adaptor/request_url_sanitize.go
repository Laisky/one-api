package adaptor

import (
	"net/url"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/model"
)

// SanitizeRequestURLError redacts the URL in request-construction and transport
// errors. Parameters: err is the upstream failure. Returns: a sanitized copy for
// URL errors, preserving the underlying cause for errors.Is/As and timeout checks.
func SanitizeRequestURLError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	clean := *urlErr
	clean.URL = model.SanitizeLogUpstreamEndpoint(urlErr.URL)
	return &clean
}
