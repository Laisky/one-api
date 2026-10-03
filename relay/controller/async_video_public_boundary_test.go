package controller

import (
	"strings"
	"testing"

	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
)

// TestAsyncVideoPublicResultRequiresSettlement validates the public serializer,
// independently of a provider's status handling. Partial output cannot expose
// a result while its financial state is still held or already refunded.
func TestAsyncVideoPublicResultRequiresSettlement(t *testing.T) {
	for _, state := range []string{model.AsyncTaskQueued, model.AsyncTaskRunning, model.AsyncTaskFailed, model.AsyncTaskCompleted} {
		for _, billing := range []string{model.AsyncBillingHeld, model.AsyncBillingRefunded, model.AsyncBillingSettled} {
			t.Run(state+"/"+billing, func(t *testing.T) {
				task := &model.AsyncTask{ID: "at_public", State: state, BillingState: billing,
					ResultJSON: `{"videos":[{"url":"https://media.example/only-after-settlement.mp4"}]}`}
				response, err := publicAsyncVideoTask(task)
				if state == model.AsyncTaskCompleted && billing == model.AsyncBillingSettled {
					require.NoError(t, err)
					require.NotNil(t, response.Result)
					return
				}
				require.Nil(t, response.Result, "non-settled output must never be public")
			})
		}
	}
}

// TestVideoTotalQuoteHasBoundedDecimalSyntax checks the shared exact-quote
// conversion before arbitrary precision parsing, with safe-sized negative
// controls. A ratio expression is not a provider USD decimal quote.
func TestVideoTotalQuoteHasBoundedDecimalSyntax(t *testing.T) {
	for _, value := range []string{"1/2", "0x1p-1", "1e-309", "0." + strings.Repeat("0", 130) + "1"} {
		t.Run(value, func(t *testing.T) {
			_, err := videoQuotaFromTotalDecimal(value, 1)
			require.Error(t, err)
		})
	}
	for _, test := range []struct {
		value string
		want  int64
	}{{"0.40", 200000}, {"1e-6", 1}, {"1e-308", 1}} {
		quota, err := videoQuotaFromTotalDecimal(test.value, 1)
		require.NoError(t, err)
		require.Equal(t, test.want, quota)
	}
}
