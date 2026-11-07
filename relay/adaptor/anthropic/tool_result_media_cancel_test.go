package anthropic

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/model"
)

// blockingImageTransport stands in for a remote image host that never answers:
// each round trip waits until the request context ends or the hold elapses.
type blockingImageTransport struct {
	hold time.Duration
}

// RoundTrip blocks until the request is cancelled or the hold elapses. It
// returns the context error on cancellation and a hold-timeout error otherwise.
func (b blockingImageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		return nil, errors.Wrap(req.Context().Err(), "blocking image transport cancelled")
	case <-time.After(b.hold):
		return nil, errors.New("blocking image transport hold elapsed")
	}
}

// TestToolResultMediaRemoteFetchStopsOnCancel verifies that cancelling the
// request interrupts a remote tool-result image download in flight, instead of
// waiting for the shared image client's own timeout.
func TestToolResultMediaRemoteFetchStopsOnCancel(t *testing.T) {
	previous := client.UserContentRequestHTTPClient
	client.UserContentRequestHTTPClient = &http.Client{Transport: blockingImageTransport{hold: 5 * time.Second}}
	t.Cleanup(func() { client.UserContentRequestHTTPClient = previous })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)

	budget := newRequestMediaBudget(ctx, maxRequestMediaBytes)
	// A globally reachable IP literal passes the SSRF guard without DNS, and
	// the injected transport keeps the fetch pending without sending a packet.
	// Documentation ranges such as 203.0.113.0/24 are refused by the guard.
	started := time.Now()
	_, err := convertToolResultMediaBlocks(budget, []model.MessageContent{toolMediaImage("http://8.8.8.8/view.png")})
	elapsed := time.Since(started)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled, "the fetch must observe the request cancellation")
	require.Less(t, elapsed, 2*time.Second, "cancellation must interrupt the download in flight")
}
