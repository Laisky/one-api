package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

// videoFailoverRoutes lists every paid video-creation route together with a
// channel type that serves it, so the retry walk can select the backup channel.
var videoFailoverRoutes = []struct {
	path        string
	channelType int
}{
	{path: "/v1/videos", channelType: channeltype.OpenAI},
	{path: "/v1/videos/generations", channelType: channeltype.XAI},
	{path: "/v1/async/videos", channelType: channeltype.MuAPI},
}

// videoFailoverChannelIDs are the primary, backup and tertiary channel ids used
// by every scenario in this file, in descending priority.
const (
	videoFailoverPrimary   = 1
	videoFailoverBackup    = 2
	videoFailoverTertiary  = 3
	videoFailoverRetryTime = 3
)

// videoFailoverPool builds three enabled channels of channelType serving
// retryOrderModel at strictly descending priorities. Parameter channelType is
// the provider type able to serve the route under test. It returns the
// channels ordered primary, backup, tertiary.
func videoFailoverPool(channelType int) []*dbmodel.Channel {
	pool := []*dbmodel.Channel{
		retryOrderChannel(videoFailoverPrimary, "video-primary", 30),
		retryOrderChannel(videoFailoverBackup, "video-backup", 20),
		retryOrderChannel(videoFailoverTertiary, "video-tertiary", 10),
	}
	for _, ch := range pool {
		ch.Type = channelType
	}
	return pool
}

// issue625QuotaRejection returns the exact provider envelope reported in issue
// #625: an HTTP 429 AccountQuotaExceeded decoded from the provider body. The
// channelID parameter is ignored; the returned error is always provider-attributed.
func issue625QuotaRejection(int) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{
		StatusCode: http.StatusTooManyRequests,
		Error: model.Error{
			Message:      "You have exceeded the monthly usage quota",
			Type:         "TooManyRequests",
			Code:         "AccountQuotaExceeded",
			FromUpstream: true,
		},
	}
}

// providerStatus builds an outcome returning a provider-attributed error with
// the given HTTP status and code. Parameters: status is the provider status and
// code its error code. It returns a scripted outcome usable with failOnly.
func providerStatus(status int, code string) retryOrderOutcome {
	return func(int) *model.ErrorWithStatusCode {
		return &model.ErrorWithStatusCode{
			StatusCode: status,
			Error: model.Error{
				Message:      "provider answered " + http.StatusText(status),
				Type:         model.ErrorTypeUpstream,
				Code:         code,
				FromUpstream: true,
			},
		}
	}
}

// fixedOutcome adapts a prebuilt error into a scripted outcome. Parameter err is
// returned for every channel the outcome is applied to; a fresh copy is made per
// call so the relay loop's client-facing decoration never leaks between attempts.
func fixedOutcome(err *model.ErrorWithStatusCode) retryOrderOutcome {
	return func(int) *model.ErrorWithStatusCode {
		copied := *err
		return &copied
	}
}

// TestVideoFailover_ProviderRejectionTriesHealthyChannel reproduces issue #625:
// the provider definitively refused a paid video creation (it answered 429
// AccountQuotaExceeded, 401, 403 or 413 before admitting any job), so no job
// exists and a healthy backup channel serving the same model must be tried
// instead of returning the rejection to the client.
func TestVideoFailover_ProviderRejectionTriesHealthyChannel(t *testing.T) {
	rejections := []struct {
		name    string
		outcome retryOrderOutcome
	}{
		{name: "429_account_quota_exceeded", outcome: issue625QuotaRejection},
		{name: "401_invalid_api_key", outcome: providerStatus(http.StatusUnauthorized, "invalid_api_key")},
		{name: "403_account_suspended", outcome: providerStatus(http.StatusForbidden, "account_suspended")},
		{name: "413_payload_too_large", outcome: providerStatus(http.StatusRequestEntityTooLarge, "payload_too_large")},
	}
	for _, route := range videoFailoverRoutes {
		for _, rejection := range rejections {
			for _, path := range routingPaths {
				name := strings.ReplaceAll(route.path, "/", "_") + "/" + rejection.name + "/" + path.name
				t.Run(name, func(t *testing.T) {
					pool := videoFailoverPool(route.channelType)
					if strings.HasPrefix(rejection.name, "413") {
						// The 413 walk seeks a channel with a larger context window.
						pool[0].ModelConfigs = stringPtr(`{"gpt-4o-mini":{"max_tokens":4096}}`)
						pool[1].ModelConfigs = stringPtr(`{"gpt-4o-mini":{"max_tokens":8192}}`)
					}
					setupRetryOrderDB(t, pool)

					got := runRelayRetryScenarioForRequest(t, pool[0], retryOrderModel, http.MethodPost, route.path,
						failOnly(rejection.outcome, videoFailoverPrimary), path.memoryCache, videoFailoverRetryTime)

					require.Equal(t, []int{videoFailoverPrimary, videoFailoverBackup}, got.order,
						"a provider rejection created no job, so the healthy backup must serve the creation")
					require.Equal(t, http.StatusOK, got.status, "the backup's success must reach the client")
					require.Empty(t, got.body, "no error body may be written after a successful failover")
				})
			}
		}
	}
}

// TestVideoFailover_RejectionChainWalksEveryChannel pins that each further
// definitive rejection keeps the walk going: primary and backup both refuse the
// job before admission, and the tertiary channel serves it.
func TestVideoFailover_RejectionChainWalksEveryChannel(t *testing.T) {
	for _, route := range videoFailoverRoutes {
		for _, path := range routingPaths {
			t.Run(strings.ReplaceAll(route.path, "/", "_")+"/"+path.name, func(t *testing.T) {
				pool := videoFailoverPool(route.channelType)
				setupRetryOrderDB(t, pool)

				got := runRelayRetryScenarioForRequest(t, pool[0], retryOrderModel, http.MethodPost, route.path,
					failOnly(issue625QuotaRejection, videoFailoverPrimary, videoFailoverBackup), path.memoryCache, videoFailoverRetryTime)

				require.Equal(t, []int{videoFailoverPrimary, videoFailoverBackup, videoFailoverTertiary}, got.order)
				require.Equal(t, http.StatusOK, got.status)
			})
		}
	}
}

// TestVideoFailover_AmbiguousOutcomeAfterRejectionStopsWalk pins that the
// replay verdict belongs to the latest attempt only: once the backup answers
// with an ambiguous 5xx (its job may exist), the earlier primary rejection must
// not authorize a third paid creation.
func TestVideoFailover_AmbiguousOutcomeAfterRejectionStopsWalk(t *testing.T) {
	for _, route := range videoFailoverRoutes {
		for _, path := range routingPaths {
			t.Run(strings.ReplaceAll(route.path, "/", "_")+"/"+path.name, func(t *testing.T) {
				pool := videoFailoverPool(route.channelType)
				setupRetryOrderDB(t, pool)

				outcome := func(channelID int) *model.ErrorWithStatusCode {
					switch channelID {
					case videoFailoverPrimary:
						return issue625QuotaRejection(channelID)
					case videoFailoverBackup:
						return providerStatus(http.StatusBadGateway, "bad_gateway")(channelID)
					default:
						return nil
					}
				}
				got := runRelayRetryScenarioForRequest(t, pool[0], retryOrderModel, http.MethodPost, route.path,
					outcome, path.memoryCache, videoFailoverRetryTime)

				require.Equal(t, []int{videoFailoverPrimary, videoFailoverBackup}, got.order,
					"an ambiguous backup outcome must veto any further paid creation")
				require.Equal(t, http.StatusBadGateway, got.status)
			})
		}
	}
}

// TestVideoFailover_AmbiguousOutcomesNeverReplay is the false-positive guard:
// every failure that cannot prove the provider refused the job before admission
// keeps the paid-creation veto, so the healthy backup is never contacted.
func TestVideoFailover_AmbiguousOutcomesNeverReplay(t *testing.T) {
	controls := []struct {
		name    string
		outcome retryOrderOutcome
	}{
		{name: "provider_500", outcome: providerStatus(http.StatusInternalServerError, "server_error")},
		{name: "provider_502", outcome: providerStatus(http.StatusBadGateway, "bad_gateway")},
		{name: "provider_503", outcome: providerStatus(http.StatusServiceUnavailable, "overloaded")},
		{name: "provider_504_gateway_timeout", outcome: providerStatus(http.StatusGatewayTimeout, "gateway_timeout")},
		{name: "provider_408_request_timeout", outcome: providerStatus(http.StatusRequestTimeout, "request_timeout")},
		{name: "provider_409_conflict", outcome: providerStatus(http.StatusConflict, "idempotency_conflict")},
		{name: "deadline_exceeded", outcome: fixedOutcome(openai.ErrorWrapper(
			errors.Wrap(context.DeadlineExceeded, "await video creation response"), "do_request_failed", http.StatusGatewayTimeout))},
		{name: "transport_reset", outcome: fixedOutcome(openai.ErrorWrapper(
			errors.New("read tcp 10.0.0.1:443: connection reset by peer"), "do_request_failed", http.StatusInternalServerError))},
		{name: "local_429_not_from_provider", outcome: fixedOutcome(openai.ErrorWrapper(
			errors.New("local admission rate limit"), "rate_limit_exceeded", http.StatusTooManyRequests))},
		{name: "provider_400_generated_unparseable_output", outcome: fixedOutcome(&model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{Code: "output_parse_failed", Message: "the model generated output that could not be parsed",
				Type: model.ErrorTypeUpstream, FromUpstream: true},
		})},
	}
	for _, route := range videoFailoverRoutes {
		for _, control := range controls {
			for _, path := range routingPaths {
				name := strings.ReplaceAll(route.path, "/", "_") + "/" + control.name + "/" + path.name
				t.Run(name, func(t *testing.T) {
					pool := videoFailoverPool(route.channelType)
					setupRetryOrderDB(t, pool)

					got := runRelayRetryScenarioForRequest(t, pool[0], retryOrderModel, http.MethodPost, route.path,
						failOnly(control.outcome, videoFailoverPrimary), path.memoryCache, videoFailoverRetryTime)

					require.Equal(t, []int{videoFailoverPrimary}, got.order,
						"an outcome that may have created a paid job must never be replayed")
					require.NotEqual(t, http.StatusOK, got.status)
				})
			}
		}
	}
}
