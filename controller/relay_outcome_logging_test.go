package controller

import (
	"context"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestRelayOutcomeLogging observes final outcomes after real controller retry decisions without inferring client success from an attempt failure.
func TestRelayOutcomeLogging(t *testing.T) {
	for _, path := range routingPaths {
		for _, scenario := range []string{"initial_success", "retry_success", "no_alternative", "budget_exhausted", "canceled"} {
			t.Run(path.name+"/"+scenario, func(t *testing.T) {
				a := retryOrderChannel(1, "fixture-a", 10)
				channels := []*dbmodel.Channel{a}
				if scenario == "retry_success" {
					channels = append(channels, retryOrderChannel(2, "fixture-b", 5))
				}
				setupRetryOrderDB(t, channels)
				budget := 2
				if scenario == "budget_exhausted" {
					budget = 0
				}
				outcome := func(id int) *model.ErrorWithStatusCode {
					if scenario == "initial_success" || id == 2 {
						return nil
					}
					e := serverError(id)
					if scenario == "canceled" {
						e.RawError = context.Canceled
					}
					return e
				}
				result := runRelayRetryScenario(t, a, outcome, path.memoryCache, budget)
				final := result.logs.FilterMessage("relay request completed").All()
				require.Len(t, final, 1, "one final outcome must distinguish attempt logs from the client result")
				fields := final[0].ContextMap()
				require.Equal(t, "relay_final", fields["log_stage"])
				require.EqualValues(t, len(result.order), fields["relay_attempts"])
				expected := "helper_failed"
				decision := "no_alternative"
				if scenario == "initial_success" {
					expected = "helper_succeeded"
					decision = "not_needed"
				}
				if scenario == "retry_success" {
					expected = "helper_succeeded"
					decision = "succeeded"
					require.Equal(t, []int{1, 2}, result.order)
				}
				if scenario == "budget_exhausted" {
					decision = "budget_exhausted"
				}
				if scenario == "canceled" {
					decision = "policy_veto"
				}
				require.Equal(t, expected, fields["relay_outcome"])
				require.Equal(t, decision, fields["retry_outcome"])
				if expected == "helper_failed" {
					require.Equal(t, http.StatusInternalServerError, result.status)
					require.Equal(t, "error_response_written", fields["client_outcome"])
				} else {
					require.Equal(t, http.StatusOK, result.status)
					require.Equal(t, "uncommitted", fields["client_outcome"])
					require.NotContains(t, fields, "client_status_code", "a scripted helper success does not prove bytes committed")
				}
				attempts := result.logs.FilterMessage("relay attempt completed").All()
				require.Len(t, attempts, len(result.order))
				for i, event := range attempts {
					require.EqualValues(t, i+1, event.ContextMap()["relay_attempt"])
				}
			})
		}
	}
}

// TestRelayOutcomeCommittedResponse preserves actual partial output and reports its status separately from the helper failure.
func TestRelayOutcomeCommittedResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	gmw.SetLogger(c, lg)
	_, err = c.Writer.WriteString("data: synthetic-partial\n\n")
	require.NoError(t, err)
	failure := serverError(1)
	o := &relayOutcomeObservation{attempts: 1, failure: failure, retryOutcome: "output_committed"}
	o.errorResponseWritten = writeRelayFinalError(c, failure)
	require.False(t, o.errorResponseWritten)
	o.logFinal(c)
	final := logs.FilterMessage("relay request completed").All()
	require.Len(t, final, 1)
	fields := final[0].ContextMap()
	require.Equal(t, "helper_failed", fields["relay_outcome"])
	require.Equal(t, "committed_response_preserved", fields["client_outcome"])
	require.EqualValues(t, 200, fields["client_status_code"])
	require.EqualValues(t, 500, fields["normalized_status_code"])
	require.Equal(t, "data: synthetic-partial\n\n", recorder.Body.String())
}
