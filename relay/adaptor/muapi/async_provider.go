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
	if status < 200 || status >= 300 {
		// 408/409/5xx and redirects cannot prove no paid job was created.
		rejected := status >= 400 && status < 500 && status != 408 && status != 409
		return asyncvideo.Submission{Rejected: rejected}, errors.Errorf("MuAPI submission returned HTTP %d", status)
	}
	var response struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return asyncvideo.Submission{}, errors.Wrap(err, "decode MuAPI submission receipt")
	}
	if !validMuAPITaskID(response.RequestID) {
		return asyncvideo.Submission{}, errors.New("MuAPI submission receipt has no valid task ID")
	}
	return asyncvideo.Submission{ID: response.RequestID}, nil
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
	if status < 200 || status >= 300 {
		return asyncvideo.Observation{}, errors.Errorf("MuAPI poll returned HTTP %d", status)
	}
	return normalizeAsyncObservation(body, id)
}

// normalizeAsyncObservation validates the provider's documented ID, status and
// outputs. Only an explicit refund flag authorizes releasing held quota.
func normalizeAsyncObservation(body []byte, expectedID string) (asyncvideo.Observation, error) {
	var response struct {
		ID        string   `json:"id"`
		RequestID string   `json:"request_id"`
		Status    string   `json:"status"`
		Outputs   []string `json:"outputs"`
		Cost      struct {
			Refunded bool `json:"refunded"`
		} `json:"cost"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return asyncvideo.Observation{}, errors.Wrap(err, "decode MuAPI result")
	}
	if (response.ID != "" && response.ID != expectedID) || (response.RequestID != "" && response.RequestID != expectedID) {
		return asyncvideo.Observation{}, errors.New("MuAPI result task ID mismatch")
	}
	observed := asyncvideo.Observation{Refunded: response.Cost.Refunded}
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "queued", "pending":
		observed.State = dbmodel.AsyncTaskQueued
	case "processing", "running":
		observed.State = dbmodel.AsyncTaskRunning
	case "completed", "succeeded", "success":
		observed.State = dbmodel.AsyncTaskCompleted
		if len(response.Outputs) == 0 || len(response.Outputs) > 32 {
			return asyncvideo.Observation{}, errors.New("MuAPI completed result has no valid outputs")
		}
		observed.Result = &asyncvideo.Result{Videos: make([]asyncvideo.Video, 0, len(response.Outputs))}
		for _, output := range response.Outputs {
			parsed, err := url.Parse(output)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || len(output) > 8192 {
				return asyncvideo.Observation{}, errors.New("MuAPI output URL is invalid")
			}
			observed.Result.Videos = append(observed.Result.Videos, asyncvideo.Video{URL: output})
		}
	case "failed", "error":
		observed.State = dbmodel.AsyncTaskFailed
	case "cancelled", "canceled":
		observed.State = dbmodel.AsyncTaskCancelled
	default:
		return asyncvideo.Observation{}, errors.New("MuAPI returned an unknown task status")
	}
	if observed.Refunded && observed.State != dbmodel.AsyncTaskFailed && observed.State != dbmodel.AsyncTaskCancelled {
		return asyncvideo.Observation{}, errors.New("MuAPI refund flag conflicts with task status")
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
