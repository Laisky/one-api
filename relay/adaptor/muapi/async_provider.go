package muapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/client"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
)

var _ asyncvideo.Provider = (*Adaptor)(nil)

var errMuAPIResultTaskIDMismatch = errors.New("MuAPI result task ID mismatch")

// SubmitVideo submits one normalized request without retries or response writes.
// A response must positively identify an accepted job or an explicit rejection;
// network errors and malformed successful responses are unknown, never retryable.
func (a *Adaptor) SubmitVideo(ctx context.Context, info *meta.Meta, body []byte) (asyncvideo.Submission, error) {
	if info == nil || !validMuAPIModelName(info.ActualModelName) || !json.Valid(body) || len(body) > dbmodel.MaxAsyncTaskBody {
		return asyncvideo.Submission{Rejected: true}, errors.New("invalid MuAPI submission")
	}
	endpoint, err := muAPIEndpointURL(info.BaseURL, info.ActualModelName)
	if err != nil {
		// Local URL validation is provably pre-dispatch, unlike an HTTP error.
		return asyncvideo.Submission{Rejected: true}, errors.Wrap(err, "validate MuAPI submission endpoint")
	}
	response, transportErr := a.asyncVideoHTTP(ctx, info, http.MethodPost, endpoint, body)
	var envelope struct {
		RequestID json.RawMessage `json:"request_id"`
		Cost      json.RawMessage `json:"cost"`
	}
	decodeErr := json.Unmarshal(response.body, &envelope)
	receipt := asyncvideo.Submission{}
	var id string
	idErr := json.Unmarshal(envelope.RequestID, &id)
	if idErr == nil && validMuAPITaskID(id) {
		receipt.ID = id
	}
	cost, bodyCostErr := muAPIChargedCost(envelope.Cost)
	var headerCostErr error
	receipt.CostUSD, headerCostErr = muAPIMaxChargedCost(cost, response.costHeaders)
	if receipt.ID == "" {
		idErr = errors.New("MuAPI submission receipt has no valid task ID")
	}
	var statusErr error
	if response.status < 200 || response.status >= 300 {
		statusErr = errors.Errorf("MuAPI submission returned HTTP %d", response.status)
	}
	// A received header remains charge evidence even if body reading/decoding
	// fails. Likewise a complete accepted ID must survive a short HTTP envelope.
	// No error after dispatch proves that the provider did not create paid work.
	if err := errors.Join(transportErr, decodeErr, idErr, bodyCostErr, headerCostErr, statusErr); err != nil {
		return receipt, errors.Wrap(err, "read MuAPI submission receipt")
	}
	return receipt, nil
}

// PollVideo maps the documented result schema to one-api's public result. A
// malformed/unknown response is an observation error, not a failed paid job.
func (a *Adaptor) PollVideo(ctx context.Context, info *meta.Meta, id string) (asyncvideo.Observation, error) {
	if info == nil || !validMuAPITaskID(id) {
		return asyncvideo.Observation{}, errors.New("invalid MuAPI polling request")
	}
	endpoint, err := muAPIEndpointURL(info.BaseURL, "predictions", id, "result")
	if err != nil {
		return asyncvideo.Observation{}, errors.Wrap(err, "validate MuAPI polling endpoint")
	}
	response, transportErr := a.asyncVideoHTTP(ctx, info, http.MethodGet, endpoint, nil)
	observation, observationErr := normalizeAsyncObservation(response.body, id)
	if errors.Is(observationErr, errMuAPIResultTaskIDMismatch) {
		// Explicitly conflicting identity invalidates the whole response's
		// evidence, including headers: never charge/refund a different task.
		return asyncvideo.Observation{}, observationErr
	}
	charged, headerErr := muAPIMaxChargedCost(observation.CostUSD, response.costHeaders)
	var statusErr error
	if response.status < 200 || response.status >= 300 {
		statusErr = errors.Errorf("MuAPI poll returned HTTP %d", response.status)
	}
	if err := errors.Join(transportErr, observationErr, headerErr, statusErr); err != nil {
		// A failed envelope cannot authorize a result or refund. Preserve only
		// valid upward cost evidence; the worker keeps the task retryable/held.
		return asyncvideo.Observation{CostUSD: charged}, err
	}
	observation.CostUSD = charged
	return observation, nil
}

// normalizeAsyncObservation validates the provider's documented ID, status and
// outputs. Only an explicit refund flag authorizes releasing held quota.
func normalizeAsyncObservation(body []byte, expectedID string) (asyncvideo.Observation, error) {
	var response struct {
		ID        string          `json:"id"`
		RequestID string          `json:"request_id"`
		Status    json.RawMessage `json:"status"`
		Outputs   json.RawMessage `json:"outputs"`
		Cost      json.RawMessage `json:"cost"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return asyncvideo.Observation{}, errors.Wrap(err, "decode MuAPI result")
	}
	if (response.ID != "" && response.ID != expectedID) || (response.RequestID != "" && response.RequestID != expectedID) {
		return asyncvideo.Observation{}, errMuAPIResultTaskIDMismatch
	}
	charged, err := muAPIChargedCost(response.Cost)
	if err != nil {
		return asyncvideo.Observation{}, err
	}
	observed := asyncvideo.Observation{CostUSD: charged}
	var cost struct {
		Refunded bool `json:"refunded"`
	}
	if len(response.Cost) != 0 {
		if err := json.Unmarshal(response.Cost, &cost); err != nil {
			return observed, errors.Wrap(err, "decode MuAPI refund receipt")
		}
	}
	if cost.Refunded && response.ID != expectedID && response.RequestID != expectedID {
		return asyncvideo.Observation{}, errors.New("MuAPI refund requires a matching task ID")
	}
	observed.Refunded = cost.Refunded
	var status string
	if err := json.Unmarshal(response.Status, &status); err != nil {
		return observed, errors.Wrap(err, "decode MuAPI task status")
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending":
		observed.State = dbmodel.AsyncTaskQueued
	case "processing", "running":
		observed.State = dbmodel.AsyncTaskRunning
	case "completed", "succeeded", "success":
		observed.State = dbmodel.AsyncTaskCompleted
		var outputs []string
		if err := json.Unmarshal(response.Outputs, &outputs); err != nil {
			return observed, errors.Wrap(err, "decode MuAPI output URLs")
		}
		if len(outputs) == 0 || len(outputs) > 32 {
			return observed, errors.New("MuAPI completed result has no valid outputs")
		}
		observed.Result = &asyncvideo.Result{Videos: make([]asyncvideo.Video, 0, len(outputs))}
		for _, output := range outputs {
			parsed, err := url.Parse(output)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || len(output) > 8192 {
				return observed, errors.New("MuAPI output URL is invalid")
			}
			observed.Result.Videos = append(observed.Result.Videos, asyncvideo.Video{URL: output})
		}
	case "failed", "error":
		observed.State = dbmodel.AsyncTaskFailed
	case "cancelled", "canceled":
		observed.State = dbmodel.AsyncTaskCancelled
	default:
		return observed, errors.New("MuAPI returned an unknown task status")
	}
	if observed.Refunded && observed.State != dbmodel.AsyncTaskFailed && observed.State != dbmodel.AsyncTaskCancelled {
		return observed, errors.New("MuAPI refund flag conflicts with task status")
	}
	return observed, nil
}

// muAPIHTTPResponse retains bounded evidence even when response-body I/O fails.
// Headers are private provider metadata and are never copied to the client.
type muAPIHTTPResponse struct {
	status      int
	body        []byte
	costHeaders []string
}

// asyncVideoHTTP performs one bounded provider operation with redirects disabled.
// It reuses the shared transport without mutating global clients or forwarding
// caller headers; already-received cost headers survive short or oversized bodies.
func (a *Adaptor) asyncVideoHTTP(ctx context.Context, info *meta.Meta, method, endpoint string, body []byte) (muAPIHTTPResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return muAPIHTTPResponse{}, errors.Wrap(err, "build MuAPI task request")
	}
	request.Header.Set("x-api-key", info.APIKey)
	request.Header.Set("Content-Type", "application/json")
	source := client.HTTPClient
	if source == nil {
		source = http.DefaultClient
	}
	bounded := *source
	bounded.Timeout = 30 * time.Second
	// Stop redirects without discarding the original response. Returning an
	// ordinary error here makes net/http close its body and loses paid receipts.
	bounded.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := bounded.Do(request)
	if err != nil {
		return muAPIHTTPResponse{}, errors.Wrap(err, "perform MuAPI task request")
	}
	defer response.Body.Close()
	result := muAPIHTTPResponse{status: response.StatusCode, costHeaders: response.Header.Values("X-MuAPI-Cost-USD")}
	data, err := io.ReadAll(io.LimitReader(response.Body, dbmodel.MaxAsyncTaskBody+1))
	if len(data) > dbmodel.MaxAsyncTaskBody {
		return result, errors.New("MuAPI task response exceeds size limit")
	}
	result.body = data
	if err != nil {
		return result, errors.Wrap(err, "read MuAPI task response")
	}
	return result, nil
}

// muAPIChargedCost validates the documented actual wallet charge without binary
// floating-point rounding. Missing cost is allowed; invalid cost never refunds.
func muAPIChargedCost(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var cost struct {
		Amount json.Number `json:"amount_usd"`
	}
	if err := json.Unmarshal(raw, &cost); err != nil {
		return "", errors.Wrap(err, "decode MuAPI charged cost")
	}
	amount := cost.Amount.String()
	if amount == "" {
		return "", nil
	}
	if _, err := dbmodel.AsyncUpstreamCostQuota("1", amount); err != nil {
		return "", errors.Wrap(err, "invalid MuAPI charged cost")
	}
	return amount, nil
}
