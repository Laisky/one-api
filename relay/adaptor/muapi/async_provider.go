package muapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/client"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
)

var _ asyncvideo.Provider = (*Adaptor)(nil)

// SubmitVideo submits one normalized request without retries or response writes.
// A response must positively identify an accepted job or an explicit rejection;
// network errors and malformed successful responses are unknown, never retryable.
func (a *Adaptor) SubmitVideo(ctx context.Context, info *meta.Meta, body []byte) (asyncvideo.Submission, error) {
	if info == nil || !validMuAPIModelName(info.ActualModelName) || !json.Valid(body) || len(body) > dbmodel.MaxAsyncTaskBody {
		return asyncvideo.Submission{Rejected: true}, errors.New("invalid MuAPI submission")
	}
	status, data, err := a.asyncVideoHTTP(ctx, info, http.MethodPost, muAPICoreBaseURL(info.BaseURL)+"/"+info.ActualModelName, body)
	if err != nil {
		return asyncvideo.Submission{}, err
	}

	var response struct {
		RequestID string          `json:"request_id"`
		Cost      json.RawMessage `json:"cost"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return asyncvideo.Submission{}, errors.Wrap(err, "decode MuAPI submission receipt")
	}
	receipt := asyncvideo.Submission{}
	if validMuAPITaskID(response.RequestID) {
		receipt.ID = response.RequestID
	}
	cost, costErr := muAPIChargedCost(response.Cost)
	if costErr != nil {
		return receipt, costErr // never discard an accepted ID
	}
	receipt.CostUSD = cost
	if receipt.ID == "" {
		// A directly observed charge still belongs to this paid POST even when
		// its task identifier is unusable. Preserve it without authorizing replay.
		return receipt, errors.New("MuAPI submission receipt has no valid task ID")
	}
	if status < 200 || status >= 300 {
		return receipt, errors.Errorf("MuAPI accepted task with HTTP %d", status)
	}
	return receipt, nil
}

// PollVideo maps the documented result schema to one-api's public result. A
// malformed/unknown response is an observation error, not a failed paid job.
func (a *Adaptor) PollVideo(ctx context.Context, info *meta.Meta, id string) (asyncvideo.Observation, error) {
	if info == nil || !validMuAPITaskID(id) {
		return asyncvideo.Observation{}, errors.New("invalid MuAPI polling request")
	}
	status, body, err := a.asyncVideoHTTP(ctx, info, http.MethodGet, muAPICoreBaseURL(info.BaseURL)+"/predictions/"+id+"/result", nil)
	if err != nil {
		return asyncvideo.Observation{}, err
	}
	observation, observationErr := normalizeAsyncObservation(body, id)
	if status < 200 || status >= 300 {
		// A failed HTTP envelope cannot authorize a result or refund. A valid
		// charge tied to this task still increases its already-reserved debit.
		return asyncvideo.Observation{CostUSD: observation.CostUSD}, errors.Errorf("MuAPI poll returned HTTP %d", status)
	}
	return observation, observationErr
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
		return asyncvideo.Observation{}, errors.New("MuAPI result task ID mismatch")
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

// asyncVideoHTTP performs one bounded provider operation with redirects disabled.
// It reuses the shared transport without mutating global clients or forwarding
// caller headers; a local copy enforces a timeout even when the shared one lacks it.
func (a *Adaptor) asyncVideoHTTP(ctx context.Context, info *meta.Meta, method, endpoint string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.Wrap(err, "build MuAPI task request")
	}
	request.Header.Set("x-api-key", info.APIKey)
	request.Header.Set("Content-Type", "application/json")
	source := client.HTTPClient
	if source == nil {
		source = http.DefaultClient
	}
	bounded := *source
	bounded.Timeout = 30 * time.Second
	bounded.CheckRedirect = a.CheckRedirect
	response, err := bounded.Do(request)
	if err != nil {
		return 0, nil, errors.Wrap(err, "perform MuAPI task request")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, dbmodel.MaxAsyncTaskBody+1))
	if err != nil {
		return 0, nil, errors.Wrap(err, "read MuAPI task response")
	}
	if len(data) > dbmodel.MaxAsyncTaskBody {
		return 0, nil, errors.New("MuAPI task response exceeds size limit")
	}
	return response.StatusCode, data, nil
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
	if len(amount) > 128 {
		return "", errors.New("MuAPI charged cost exceeds numeric limit")
	}
	if _, err := dbmodel.AsyncUpstreamCostQuota("1", amount); err != nil {
		return "", errors.Wrap(err, "invalid MuAPI charged cost")
	}
	number, err := strconv.ParseFloat(amount, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return "", errors.New("MuAPI charged cost is invalid")
	}
	return amount, nil
}
