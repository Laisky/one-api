package muapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestAsyncBillingPollErrorsPreserveKnownCharge prevents HTTP or malformed
// refund metadata from erasing otherwise valid charge evidence for the task.
func TestAsyncBillingPollErrorsPreserveKnownCharge(t *testing.T) {
	for _, status := range []int{200, 400, 429, 500, 502} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := `{"id":"paid-job","status":"completed","outputs":["https://media.example/v.mp4"],"cost":{"amount_usd":0.8,"refunded":false}}`
			if status == 200 {
				body = `{"id":"paid-job","status":"failed","cost":{"amount_usd":0.8,"refunded":"not-a-boolean"}}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			observation, err := (&Adaptor{}).PollVideo(context.Background(), &meta.Meta{BaseURL: server.URL}, "paid-job")
			require.Error(t, err)
			require.Equal(t, "0.8", observation.CostUSD)
			require.False(t, observation.Refunded)
			require.Nil(t, observation.Result)
		})
	}
}
