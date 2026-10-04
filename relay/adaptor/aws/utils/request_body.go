package utils

import (
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// PreserveRequestBodyEOF keeps request cleanup from aborting the response.
// Smithy's safeWriteToReadCloser returns io.EOF from WriteTo after Close.
// net/http treats that optional fast-path result as a request-write failure,
// unlike Read's normal EOF, and closes an otherwise valid response connection.
// Use only Read/Close for these request bodies; do not change response errors,
// retry policy, request bytes, or the already configured SDK transport.
func PreserveRequestBodyEOF(options *bedrockruntime.Options) {
	if options.HTTPClient == nil {
		return // The SDK resolves its default client before applying this option.
	}
	if _, wrapped := options.HTTPClient.(requestBodyReadClient); !wrapped {
		options.HTTPClient = requestBodyReadClient{next: options.HTTPClient}
	}
}

// requestBodyReadClient preserves the SDK client's resolved transport settings.
type requestBodyReadClient struct{ next bedrockruntime.HTTPClient }

// Do hides only the optional request-body WriterTo optimization in a private
// request copy. The original SDK body remains the owner of Read and Close.
func (c requestBodyReadClient) Do(request *http.Request) (*http.Response, error) {
	if request.Body == nil || request.Body == http.NoBody {
		return c.next.Do(request)
	}
	if _, fastPath := request.Body.(io.WriterTo); !fastPath {
		return c.next.Do(request)
	}
	private := *request
	private.Body = requestBodyReadCloser{ReadCloser: request.Body}
	return c.next.Do(&private)
}

// requestBodyReadCloser intentionally exposes no io.WriterTo method.
type requestBodyReadCloser struct{ io.ReadCloser }
