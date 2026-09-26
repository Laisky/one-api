package observation

import (
	"context"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"runtime/trace"
	"strconv"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
)

// ClientTraceEnvironment is read only by the explicit diagnostic driver overlay.
const ClientTraceEnvironment = "STREAM_PERF_CLIENT_TRACE_LISTEN"

// ClientTrace owns one private trace listener and its bounded shutdown lifecycle.
type ClientTrace struct {
	server     *http.Server
	address    string
	done       chan error
	once       sync.Once
	closeError error
}

// StartClientTrace starts an IPv4-loopback trace-only server; an empty address is inert.
// The normal gateway and normal load-driver never invoke this diagnostic helper.
func StartClientTrace(address string) (*ClientTrace, error) {
	if address == "" {
		return nil, nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.Wrap(err, "parse client trace listener")
	}
	number, err := strconv.Atoi(port)
	if err != nil || host != "127.0.0.1" || number < 0 || number > 65535 || strconv.Itoa(number) != port {
		return nil, errors.New("client trace requires literal IPv4 loopback and a valid port")
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, errors.Wrap(err, "bind client trace listener")
	}
	server := &http.Server{Handler: http.HandlerFunc(clientTraceHandler), ReadHeaderTimeout: 2 * time.Second, IdleTimeout: 5 * time.Second}
	collector := &ClientTrace{server: server, address: listener.Addr().String(), done: make(chan error, 1)}
	go func() { collector.done <- server.Serve(listener) }()
	return collector, nil
}

// Address returns the bound listener address, including its allocated ephemeral port.
func (c *ClientTrace) Address() string {
	if c == nil {
		return ""
	}
	return c.address
}

// Close synchronously stops and reaps the server exactly once, reporting unexpected serve errors.
func (c *ClientTrace) Close() error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		closed := c.server.Close()
		served := <-c.done
		if closed != nil {
			c.closeError = errors.Wrap(closed, "close client trace listener")
		} else if !errors.Is(served, http.ErrServerClosed) {
			c.closeError = errors.Wrap(served, "serve client trace listener")
		}
	})
	return c.closeError
}

// clientTraceHandler exposes only a bounded execution trace, never the default pprof mux.
func clientTraceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/debug/pprof/trace" {
		http.Error(w, "unsupported diagnostic route", http.StatusNotFound)
		return
	}
	_, err := netQuerySeconds(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "trace duration must be an integer from 1 to 10", http.StatusBadRequest)
		return
	}
	pprof.Trace(w, r)
}

// ObserveClient records the same client timestamp plus a numeric trace marker for selected requests.
func ObserveClient(times []int64, request int) ([]int64, error) {
	return observeClient(times, request, MonotonicNS, trace.IsEnabled, trace.Log)
}

// observeClient validates identity before recording and shares one clock read with retained samples.
func observeClient(times []int64, request int, clock func() (int64, error), active func() bool, emit func(context.Context, string, string)) ([]int64, error) {
	if !Selected(request) {
		return times, errors.New("client trace request is outside the selected cohort")
	}
	observed, err := appendClient(times, clock)
	if err != nil {
		return times, errors.Wrap(err, "observe traced client event")
	}
	if active() {
		message := strconv.Itoa(request) + "/" + strconv.Itoa(len(times)) + "/observe/" + strconv.FormatInt(observed[len(observed)-1], 10)
		emit(context.Background(), "oneapi.sse.client", message)
	}
	return observed, nil
}

// netQuerySeconds rejects duplicate, extra, malformed or unbounded query parameters.
func netQuerySeconds(raw string) (int, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return 0, errors.Wrap(err, "parse trace query")
	}
	values := query["seconds"]
	if len(query) != 1 || len(values) != 1 {
		return 0, errors.New("trace requires exactly one duration")
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < 1 || n > 10 || strconv.Itoa(n) != values[0] {
		return 0, errors.New("invalid trace duration")
	}
	return n, nil
}
