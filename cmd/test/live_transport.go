package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"
)

// runLiveRESTGuardScenario confirms a Live-only model cannot be dispatched over
// chat completions. Parameters: ctx, logger, wsBase and opts describe the run.
// Returns: an error unless the route returns the documented transport mismatch.
func runLiveRESTGuardScenario(ctx context.Context, logger glog.Logger, _ string, opts liveOptions) error {
	body, err := json.Marshal(map[string]any{
		"model":    opts.model,
		"messages": []map[string]any{{"role": "user", "content": opts.prompt}},
	})
	if err != nil {
		return errors.Wrap(err, "encode chat request")
	}
	reqCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, opts.apiBase+"/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return errors.Wrap(err, "build chat request")
	}
	token := opts.apiToken
	if opts.restChannel > 0 {
		// TokenAuth's existing admin-only channel suffix pins this negative test
		// without changing the credentials used by the actual Live scenarios.
		token += "-" + strconv.Itoa(opts.restChannel)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "send chat request")
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return errors.Wrap(err, "read chat response")
	}
	if err := validateLiveRESTGuardResponse(resp.StatusCode, payload); err != nil {
		return errors.Wrapf(err, "REST dispatch of Live-only model %q", opts.model)
	}
	logger.Info("REST dispatch rejected as expected",
		zap.Int("status", resp.StatusCode), zap.String("body", snippet(payload)))
	return nil
}

// validateLiveRESTGuardResponse verifies the exact public error contract for a
// Live-only model sent to REST. Parameters: status and payload are the HTTP
// response. Returns: nil only for HTTP 400 with unsupported_model_transport.
func validateLiveRESTGuardResponse(status int, payload []byte) error {
	if status != http.StatusBadRequest {
		return errors.Errorf("returned HTTP %d, want %d: %s", status, http.StatusBadRequest, snippet(payload))
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return errors.Wrap(err, "decode error response")
	}
	if body.Error.Code != "unsupported_model_transport" {
		return errors.Errorf("returned error code %q, want %q: %s", body.Error.Code,
			"unsupported_model_transport", snippet(payload))
	}
	return nil
}

// dialLive opens one authenticated Live socket. Parameters: ctx, wsURL, token,
// protocols and timeout describe the handshake; an empty token means the caller
// authenticates through a subprotocol instead. Returns: the socket or an error
// carrying the rejected HTTP status.
func dialLive(ctx context.Context, wsURL, token string, protocols []string, timeout time.Duration) (*websocket.Conn, error) {
	conn, _, err := dialLiveWithRequestID(ctx, wsURL, token, protocols, timeout)
	return conn, err
}

// dialLiveWithRequestID opens one authenticated Live socket and captures the
// server request ID from the successful upgrade response. Parameters: ctx,
// wsURL, token, protocols and timeout describe the handshake. Returns: the
// socket, server request ID, or an error carrying the rejected HTTP response.
func dialLiveWithRequestID(ctx context.Context, wsURL, token string, protocols []string, timeout time.Duration) (*websocket.Conn, string, error) {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := websocket.Dialer{
		HandshakeTimeout: 20 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
		Subprotocols:     protocols,
		ReadBufferSize:   1 << 16,
	}
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	conn, resp, err := dialer.DialContext(dialCtx, wsURL, headers)
	if err != nil {
		status := 0
		var body string
		if resp != nil {
			status = resp.StatusCode
			if resp.Body != nil {
				if payload, readErr := io.ReadAll(io.LimitReader(resp.Body, maxLoggedBodyBytes)); readErr == nil {
					body = snippet(payload)
				}
				_ = resp.Body.Close()
			}
		}
		return nil, "", errors.Wrapf(err, "dial live websocket (status=%d body=%s)", status, body)
	}
	conn.SetReadLimit(4 << 20)
	requestID := ""
	if resp != nil {
		requestID = strings.TrimSpace(resp.Header.Get("X-Oneapi-Request-Id"))
	}
	return conn, requestID, nil
}

// resolveLiveWSEndpoint converts a one-api base URL into the realtime websocket
// URL. Parameters: apiBase is an HTTP(S) or WS(S) base. Returns: the normalized
// /v1/realtime websocket endpoint or a validation error.
func resolveLiveWSEndpoint(apiBase string) (string, error) {
	raw := strings.TrimSpace(apiBase)
	if raw == "" {
		return "", errors.New("empty api-base")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.Wrap(err, "parse api-base")
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return "", errors.Errorf("unsupported api-base scheme %q", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", errors.New("api-base host is required")
	}
	path := strings.TrimSuffix(strings.TrimSpace(u.Path), "/")
	if !strings.HasSuffix(path, "/v1/realtime") {
		path = strings.TrimSuffix(path, "/v1") + "/v1/realtime"
	}
	u.Path = path
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}
