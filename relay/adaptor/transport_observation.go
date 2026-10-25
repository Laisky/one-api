package adaptor

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"sync"
	"syscall"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// transportObservation retains only bounded local callback counts, never sockets or request data.
// Its callbacks can race a Client.Do return, so every update and snapshot is synchronized.
type transportObservation struct {
	mu                sync.Mutex
	gotConn           int
	reusedConn        int
	writeCompleted    int
	writeFailed       int
	firstResponseByte int
}

// transportObservationLimit bounds diagnostic counters without retaining an event history.
const transportObservationLimit = 1_000_000

// observeTransportRequest composes native transport tracing while preserving the request's cancellation channel.
// Custom transports keep their original context and callbacks; absent observations remain unknown.
func observeTransportRequest(req *http.Request, transport http.RoundTripper) (*http.Request, *transportObservation) {
	observation := &transportObservation{}
	if transport == nil {
		transport = http.DefaultTransport
	}
	if _, native := transport.(*http.Transport); !native || req == nil {
		return req, observation
	}
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			observation.mu.Lock()
			defer observation.mu.Unlock()
			observation.gotConn = min(observation.gotConn+1, transportObservationLimit)
			if info.Reused {
				observation.reusedConn = min(observation.reusedConn+1, transportObservationLimit)
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			observation.mu.Lock()
			defer observation.mu.Unlock()
			if info.Err == nil {
				observation.writeCompleted = min(observation.writeCompleted+1, transportObservationLimit)
			} else {
				observation.writeFailed = min(observation.writeFailed+1, transportObservationLimit)
			}
		},
		GotFirstResponseByte: func() {
			observation.mu.Lock()
			defer observation.mu.Unlock()
			observation.firstResponseByte = min(observation.firstResponseByte+1, transportObservationLimit)
		},
	}
	return req.WithContext(httptrace.WithClientTrace(req.Context(), trace)), observation
}

// complete emits one content-free snapshot when Client.Do returns, before response-body processing.
// A completed write is only a local transport callback, not evidence of accepted provider work.
// Counts saturate at transportObservationLimit and may exclude callbacks arriving after this snapshot.
func (o *transportObservation) complete(logger glog.Logger, ctx context.Context, resp *http.Response, err error) {
	o.mu.Lock()
	gotConn, reusedConn, wrote, writeFailed, firstByte := o.gotConn, o.reusedConn, o.writeCompleted, o.writeFailed, o.firstResponseByte
	o.mu.Unlock()

	traceState := "unknown"
	if gotConn > 0 || wrote > 0 || writeFailed > 0 || firstByte > 0 {
		traceState = "observed"
	}
	phase := "none"
	if err != nil {
		switch {
		case resp != nil:
			phase = "after_response_headers"
		case firstByte > 0:
			phase = "response_headers"
		case traceState == "observed":
			phase = "before_response_headers"
		default:
			phase = "unknown"
		}
	}
	fields := []zap.Field{
		zap.String("log_stage", "transport_attempt"),
		zap.Bool("response_headers_received", resp != nil),
		zap.String("transport_failure_phase", phase),
		zap.String("transport_failure_class", transportFailureClass(err)),
		zap.String("request_context_state", transportContextState(ctx)),
		zap.String("transport_trace_state", traceState),
		zap.Int("transport_got_conn_count", gotConn),
		zap.Int("transport_reused_conn_count", reusedConn),
		zap.Int("transport_write_completed_count", wrote),
		zap.Int("transport_write_failed_count", writeFailed),
		zap.Int("transport_first_response_byte_count", firstByte),
	}
	if resp != nil {
		fields = append(fields, zap.Int("upstream_http_status", resp.StatusCode))
	}
	switch {
	case err != nil:
		logger.Warn("upstream transport completed", fields...)
	case resp != nil && resp.StatusCode >= http.StatusBadRequest && resp.StatusCode <= 599:
		// Client.Do returns nil error for HTTP failures. Keep their actual status
		// visible at the normal INFO floor without labeling them network failures.
		logger.Info("upstream transport completed", fields...)
	default:
		logger.Debug("upstream transport completed", fields...)
	}
}

// transportFailureClass classifies the returned cause without copying error text or network addresses.
// Independent reset evidence remains visible when cancellation is also present in a joined error.
func transportFailureClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "other"
	}
}

// transportContextState reports the request's current cancellation state without inferring its cause or history.
func transportContextState(ctx context.Context) string {
	if ctx == nil || ctx.Err() == nil {
		return "active"
	}
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return "canceled"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "other"
	}
}
