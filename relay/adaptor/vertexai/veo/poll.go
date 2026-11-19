package veo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

const (
	// pollInterval is the default delay between fetchPredictOperation calls.
	pollInterval = 5 * time.Second
	// defaultPollTimeout bounds how long one request waits for an accepted
	// operation. The create call's Vertex access token comes from a 50-minute
	// cache of one-hour tokens, so it stays valid for at least this long.
	defaultPollTimeout = 10 * time.Minute
	// maxConsecutivePollFailures is how many consecutive transient poll
	// failures (transport errors, 429, 5xx) end polling. The accepted job keeps
	// running upstream, so one blip must not abandon it.
	maxConsecutivePollFailures = 3
)

// operationIDPattern accepts only opaque operation IDs, so the client-visible
// reference never carries the operator's project path.
var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// operationPoller performs fetchPredictOperation calls for one accepted job
// with the create call's endpoint, credentials and the shared relay client.
type operationPoller struct {
	client *http.Client
	url    string
	header http.Header
	body   []byte
	ref    string
	lg     glog.Logger
}

// pollOutcome is the result of one fetchPredictOperation call. transient
// marks a failure that may succeed on a later poll.
type pollOutcome struct {
	operation *PollVideoTaskResponse
	err       *model.ErrorWithStatusCode
	transient bool
}

// pollSettings returns the poll interval and the overall poll timeout.
// Zero adaptor fields select pollInterval and defaultPollTimeout.
func (a *Adaptor) pollSettings() (time.Duration, time.Duration) {
	interval, timeout := pollInterval, defaultPollTimeout
	if a.pollInterval > 0 {
		interval = a.pollInterval
	}
	if a.pollTimeout > 0 {
		timeout = a.pollTimeout
	}
	return interval, timeout
}

// pollVideoTask polls the operation an accepted predictLongRunning call
// created until it completes, then writes the OpenAI-style response.
// It accepts the gin context, the create response (whose Request supplies the
// endpoint and credentials) and its body, and returns nil on success or the
// relay error to report. The caller keeps billing the accepted job either way.
func (a *Adaptor) pollVideoTask(c *gin.Context, resp *http.Response, respBody []byte) *model.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	task := new(CreateVideoTaskResponse)
	if err := json.Unmarshal(respBody, task); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "unmarshal_poll_response_failed"), "unmarshal_poll_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(task.Name) == "" {
		return openai.ErrorWrapper(errors.New("veo create response carries no operation name"), "veo_operation_name_missing", http.StatusBadGateway)
	}
	pollURL, err := fetchOperationURL(resp.Request)
	if err != nil {
		return openai.ErrorWrapper(err, "veo_poll_endpoint_unresolved", http.StatusInternalServerError)
	}
	pollBody, err := json.Marshal(PollVideoTaskRequest{OperationName: task.Name})
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "marshal_poll_request_failed"), "marshal_poll_request_failed", http.StatusInternalServerError)
	}

	poller := &operationPoller{
		client: relayHTTPClient(),
		url:    pollURL,
		header: pollHeader(resp.Request),
		body:   pollBody,
		ref:    operationRef(task.Name),
		lg:     lg,
	}
	interval, timeout := a.pollSettings()
	parent := gmw.Ctx(c)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	failures := 0
	for attempt := 1; ; attempt++ {
		outcome := poller.poll(ctx)
		switch {
		case outcome.err == nil && outcome.operation.Done:
			lg.Debug("veo operation finished", zap.String("operation", task.Name), zap.Int("poll_attempts", attempt))
			if outcome.operation.Error != nil {
				return operationFailure(poller.ref, outcome.operation.Error)
			}
			return convert2OpenaiResponse(c, outcome.operation)
		case outcome.err == nil:
			failures = 0
		case outcome.transient && ctx.Err() != nil:
			return pollContextError(parent, poller.ref, timeout)
		case outcome.transient && failures+1 < maxConsecutivePollFailures:
			failures++
			lg.Debug("veo operation poll failed transiently, retrying",
				zap.String("operation", task.Name),
				zap.Int("status", outcome.err.StatusCode),
				zap.String("code", fmt.Sprint(outcome.err.Code)),
				zap.Int("consecutive_failures", failures))
		default:
			return outcome.err
		}

		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return pollContextError(parent, poller.ref, timeout)
		}
	}
}

// poll performs one fetchPredictOperation call.
// It accepts the polling context and returns the decoded operation, or the
// relay error with its transient classification.
func (p *operationPoller) poll(ctx context.Context) pollOutcome {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(p.body))
	if err != nil {
		return pollOutcome{err: openai.ErrorWrapper(errors.Wrap(adaptor.SanitizeRequestURLError(err), "create_poll_request_failed"), "create_poll_request_failed", http.StatusInternalServerError)}
	}
	req.Header = p.header.Clone()
	resp, err := p.client.Do(req)
	if err != nil {
		return pollOutcome{transient: true, err: openai.ErrorWrapper(
			errors.Wrap(adaptor.SanitizeRequestURLError(err), p.acceptedPrefix()+"do_poll_request_failed"),
			"do_poll_request_failed", http.StatusServiceUnavailable)}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.lg.Debug("close veo poll response body failed", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, tooLarge, readErr := readBoundedBody(resp.Body, maxErrorBodyBytes)
		if readErr != nil {
			// The provider status is authoritative; an unreadable error body
			// only degrades the message to the generic status text.
			p.lg.Debug("read veo poll error body failed", zap.Int("status", resp.StatusCode), zap.Error(readErr))
			body = nil
		}
		if tooLarge {
			body = nil
		}
		message := p.acceptedPrefix() + upstreamErrorMessage(body, resp.StatusCode)
		return pollOutcome{
			transient: isTransientStatus(resp.StatusCode),
			err:       openai.UpstreamErrorWrapper(errors.New(message), "poll_request_failed_status", resp.StatusCode),
		}
	}

	body, tooLarge, err := readBoundedBody(resp.Body, maxPollBodyBytes)
	if err != nil {
		return pollOutcome{transient: true, err: openai.ErrorWrapper(
			errors.Wrap(err, p.acceptedPrefix()+"read_poll_response_failed"), "read_poll_response_failed", http.StatusBadGateway)}
	}
	if tooLarge {
		return pollOutcome{err: openai.ErrorWrapper(
			errors.Errorf("%sveo operation response exceeds %d bytes", p.acceptedPrefix(), maxPollBodyBytes),
			"poll_response_too_large", http.StatusBadGateway)}
	}
	operation := new(PollVideoTaskResponse)
	if err := json.Unmarshal(body, operation); err != nil {
		return pollOutcome{err: openai.ErrorWrapper(errors.Wrap(err, "unmarshal_poll_response_failed"), "unmarshal_poll_response_failed", http.StatusInternalServerError)}
	}
	return pollOutcome{operation: operation}
}

// acceptedPrefix returns the client-visible prefix naming the accepted
// operation, so a poll failure is never mistaken for a rejected request.
func (p *operationPoller) acceptedPrefix() string {
	if p.ref == "" {
		return "veo operation was accepted but polling failed: "
	}
	return "veo operation " + p.ref + " was accepted but polling failed: "
}

// fetchOperationURL derives the fetchPredictOperation endpoint of the same
// publisher model from the URL the predictLongRunning call was sent to, which
// already reflects the channel's base URL, region and endpoint override.
// It accepts the create request and returns the poll URL or an error when the
// create URL is not a predictLongRunning endpoint.
func fetchOperationURL(createReq *http.Request) (string, error) {
	if createReq == nil || createReq.URL == nil {
		return "", errors.New("veo create response carries no request URL")
	}
	target := *createReq.URL
	if !strings.HasSuffix(target.Path, actionPredictLongRunning) {
		return "", errors.New("veo create URL is not a predictLongRunning endpoint")
	}
	target.Path = strings.TrimSuffix(target.Path, actionPredictLongRunning) + actionFetchOperation
	if strings.HasSuffix(target.RawPath, actionPredictLongRunning) {
		target.RawPath = strings.TrimSuffix(target.RawPath, actionPredictLongRunning) + actionFetchOperation
	} else {
		target.RawPath = ""
	}
	target.Fragment, target.RawFragment = "", ""
	return target.String(), nil
}

// pollHeader returns the headers for fetchPredictOperation calls: a copy of
// the create call's headers (Vertex bearer token and channel custom headers)
// with a JSON content type. It accepts the create request and returns the
// header set; a nil request yields only the content headers.
func pollHeader(createReq *http.Request) http.Header {
	header := http.Header{}
	if createReq != nil && createReq.Header != nil {
		header = createReq.Header.Clone()
	}
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	return header
}

// relayHTTPClient returns the shared relay HTTP client, which carries the
// configured relay proxy and timeout, initializing it on first use.
func relayHTTPClient() *http.Client {
	if client.HTTPClient == nil {
		client.Init()
	}
	if client.HTTPClient == nil {
		return http.DefaultClient
	}
	return client.HTTPClient
}

// operationRef returns the opaque ID at the end of an operation name, or ""
// when it is not identifier-shaped. It accepts the full operation name.
func operationRef(name string) string {
	_, id, found := strings.Cut(name, "/operations/")
	if !found || !operationIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// isTransientStatus reports whether a poll HTTP status may succeed on retry.
func isTransientStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// pollContextError reports why polling stopped before the operation finished.
// It accepts the request context, the operation reference and the poll
// timeout, and returns a 408 when the client request ended or a 504 when the
// poll timeout elapsed.
func pollContextError(parent context.Context, ref string, timeout time.Duration) *model.ErrorWithStatusCode {
	if err := parent.Err(); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "request context done while waiting for veo operation"),
			"request_context_done_while_waiting_for_poll", http.StatusRequestTimeout)
	}
	label := "veo operation"
	if ref != "" {
		label += " " + ref
	}
	return openai.ErrorWrapper(errors.Errorf("%s was accepted but did not finish within %s", label, timeout),
		"veo_poll_timeout", http.StatusGatewayTimeout)
}

// operationFailure reports an operation the provider finished with an error.
// It accepts the operation reference and the google.rpc.Status, and returns a
// provider-attributed error with a sanitized message and an HTTP status mapped
// from the google.rpc code.
func operationFailure(ref string, opErr *OperationError) *model.ErrorWithStatusCode {
	label := "veo operation"
	if ref != "" {
		label += " " + ref
	}
	detail := sanitizeProviderMessage(opErr.Message)
	if detail == "" {
		detail = fmt.Sprintf("upstream reported operation error code %d", opErr.Code)
	}
	return openai.UpstreamErrorWrapper(errors.New(label+" failed: "+detail), "veo_operation_failed", rpcCodeHTTPStatus(opErr.Code))
}

// rpcCodeHTTPStatus maps a google.rpc.Code to its documented HTTP status.
// It accepts the code and returns 502 for codes without a specific mapping.
func rpcCodeHTTPStatus(code int) int {
	switch code {
	case 3, 9, 11: // INVALID_ARGUMENT, FAILED_PRECONDITION, OUT_OF_RANGE
		return http.StatusBadRequest
	case 4: // DEADLINE_EXCEEDED
		return http.StatusGatewayTimeout
	case 5: // NOT_FOUND
		return http.StatusNotFound
	case 6, 10: // ALREADY_EXISTS, ABORTED
		return http.StatusConflict
	case 7: // PERMISSION_DENIED
		return http.StatusForbidden
	case 8: // RESOURCE_EXHAUSTED
		return http.StatusTooManyRequests
	case 12: // UNIMPLEMENTED
		return http.StatusNotImplemented
	case 13, 15: // INTERNAL, DATA_LOSS
		return http.StatusInternalServerError
	case 14: // UNAVAILABLE
		return http.StatusServiceUnavailable
	case 16: // UNAUTHENTICATED
		return http.StatusUnauthorized
	default:
		return http.StatusBadGateway
	}
}
