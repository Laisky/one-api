package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"
)

const (
	defaultChatResponsePrompt  = "Reply with exactly: pong"
	defaultChatResponseTimeout = 90 * time.Second
	maxWSProbeLogMessages      = 8
)

// chat runs interactive chat-focused probes from the command line.
//
// Parameters:
//   - ctx: command context for cancellation and deadlines.
//   - logger: structured logger for command diagnostics.
//   - args: command-line arguments after the `chat` token.
//
// Returns:
//   - error: non-nil when argument parsing or probe execution fails.
func chat(ctx context.Context, logger glog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("missing chat subcommand, expected: response")
	}

	subcommand := strings.ToLower(strings.TrimSpace(args[0]))
	if subcommand != "response" {
		return errors.Errorf("unknown chat subcommand %q, expected: response", subcommand)
	}

	cfg, err := loadConfig()
	if err != nil {
		return errors.Wrap(err, "load config")
	}

	opts, err := parseChatResponseArgs(args[1:], cfg)
	if err != nil {
		return errors.Wrap(err, "parse chat response arguments")
	}

	if !opts.useWebSocket {
		return errors.New("only websocket probe is supported by this command for now; pass --ws")
	}

	return runChatResponseWebSocketProbe(ctx, logger, opts)
}

// chatResponseOptions captures CLI options for `chat response` probes.
type chatResponseOptions struct {
	apiEndpoint  string
	apiToken     string
	model        string
	prompt       string
	timeout      time.Duration
	useWebSocket bool
}

// parseChatResponseArgs parses flags for the `chat response` subcommand.
//
// Parameters:
//   - args: raw CLI arguments after `chat response`.
//   - cfg: shared harness configuration used for defaults.
//
// Returns:
//   - chatResponseOptions: normalized options for the probe.
//   - error: parsing/validation error when arguments are invalid.
func parseChatResponseArgs(args []string, cfg config) (chatResponseOptions, error) {
	fs := flag.NewFlagSet("chat response", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	defaultModel := "gpt-5-mini"
	if len(cfg.Models) > 0 && strings.TrimSpace(cfg.Models[0]) != "" {
		defaultModel = strings.TrimSpace(cfg.Models[0])
	}

	defaultEndpoint := strings.TrimSuffix(cfg.APIBase, "/") + "/v1/responses"

	var opts chatResponseOptions
	fs.BoolVar(&opts.useWebSocket, "ws", false, "use websocket mode for Response API probe")
	fs.StringVar(&opts.apiEndpoint, "endpoint", defaultEndpoint, "Response API endpoint URL, e.g. https://host/v1/responses")
	fs.StringVar(&opts.model, "model", defaultModel, "model name")
	fs.StringVar(&opts.prompt, "prompt", defaultChatResponsePrompt, "user prompt for the probe")
	fs.DurationVar(&opts.timeout, "timeout", defaultChatResponseTimeout, "end-to-end probe timeout")
	fs.StringVar(&opts.apiToken, "token", strings.TrimSpace(cfg.Token), "API token; defaults to API_TOKEN")

	if err := fs.Parse(args); err != nil {
		return chatResponseOptions{}, err
	}

	opts.apiEndpoint = strings.TrimSpace(opts.apiEndpoint)
	opts.apiToken = strings.TrimSpace(opts.apiToken)
	opts.model = strings.TrimSpace(opts.model)
	opts.prompt = strings.TrimSpace(opts.prompt)

	if opts.apiEndpoint == "" {
		return chatResponseOptions{}, errors.New("endpoint is required")
	}
	if opts.apiToken == "" {
		return chatResponseOptions{}, errors.New("token is required")
	}
	if opts.model == "" {
		return chatResponseOptions{}, errors.New("model is required")
	}
	if opts.prompt == "" {
		return chatResponseOptions{}, errors.New("prompt is required")
	}
	if opts.timeout <= 0 {
		return chatResponseOptions{}, errors.New("timeout must be positive")
	}

	return opts, nil
}

// runChatResponseWebSocketProbe performs one websocket response.create request to validate
// whether the target endpoint supports Response API websocket mode.
//
// Parameters:
//   - ctx: command context.
//   - logger: structured logger.
//   - opts: probe options.
//
// Returns:
//   - error: non-nil when the websocket handshake/request/response fails.
func runChatResponseWebSocketProbe(ctx context.Context, logger glog.Logger, opts chatResponseOptions) error {
	wsEndpoint, err := resolveChatResponseWSEndpoint(opts.apiEndpoint)
	if err != nil {
		return errors.Wrap(err, "resolve websocket endpoint")
	}

	requestCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+opts.apiToken)

	conn, resp, err := dialer.DialContext(requestCtx, wsEndpoint, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return errors.Wrapf(err, "dial websocket endpoint failed (status=%d)", status)
	}
	defer func() { _ = conn.Close() }()

	requestPayload := responseAPIPayload(opts.model, false, expectationDefault)
	requestMap, ok := requestPayload.(map[string]any)
	if !ok {
		return errors.New("response payload has unexpected type")
	}

	delete(requestMap, "stream")
	delete(requestMap, "background")
	requestMap["type"] = "response.create"

	eventBody, err := json.Marshal(requestMap)
	if err != nil {
		return errors.Wrap(err, "marshal response.create payload")
	}

	if err := conn.WriteMessage(websocket.TextMessage, eventBody); err != nil {
		return errors.Wrap(err, "send response.create event")
	}

	logger.Info("response websocket probe started",
		zap.String("endpoint", wsEndpoint),
		zap.String("model", opts.model),
	)

	messageCount := 0
	var previews []string
	for {
		if err := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
			return errors.Wrap(err, "set websocket read deadline")
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
				return errors.Wrap(requestCtx.Err(), "response websocket probe timed out")
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logger.Info("response websocket probe completed by normal close",
					zap.String("endpoint", wsEndpoint),
					zap.Int("messages", messageCount),
				)
				return nil
			}
			return errors.Wrap(err, "read websocket message")
		}

		messageCount++
		if len(previews) < maxWSProbeLogMessages {
			previews = append(previews, truncateString(string(msg), 256))
		}

		if err := responseWebSocketMessageError(msg); err != nil {
			return err
		}

		if isResponseWebSocketTerminalMessage(msg) {
			logger.Info("response websocket probe completed",
				zap.String("endpoint", wsEndpoint),
				zap.Int("messages", messageCount),
				zap.Strings("preview", previews),
			)
			return nil
		}
	}
}

// resolveChatResponseWSEndpoint converts an HTTP(S)/WS(S) endpoint to a websocket URL
// targeting `/v1/responses`.
//
// Parameters:
//   - endpoint: user-provided endpoint URL.
//
// Returns:
//   - string: normalized websocket endpoint URL.
//   - error: URL parsing/validation error.
func resolveChatResponseWSEndpoint(endpoint string) (string, error) {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return "", errors.New("empty endpoint")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.Wrap(err, "parse endpoint")
	}

	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return "", errors.Errorf("unsupported endpoint scheme %q", u.Scheme)
	}

	if strings.TrimSpace(u.Host) == "" {
		return "", errors.New("endpoint host is required")
	}

	if strings.TrimSpace(u.Path) == "" || strings.TrimSpace(u.Path) == "/" {
		u.Path = "/v1/responses"
	}

	return u.String(), nil
}

// responseWebSocketMessageError parses websocket error payloads and returns an error when present.
//
// Parameters:
//   - msg: one websocket text payload.
//
// Returns:
//   - error: non-nil when the payload indicates an upstream error event.
func responseWebSocketMessageError(msg []byte) error {
	if len(msg) == 0 {
		return nil
	}

	var event map[string]any
	if err := json.Unmarshal(msg, &event); err != nil {
		return nil
	}

	eventType, _ := event["type"].(string)
	if strings.ToLower(strings.TrimSpace(eventType)) != "error" {
		return nil
	}

	status, _ := event["status"].(float64)
	errObj, _ := event["error"].(map[string]any)
	if errObj == nil {
		return errors.Errorf("websocket error event received (status=%.0f): %s", status, truncateString(string(msg), 256))
	}

	code, _ := errObj["code"].(string)
	message, _ := errObj["message"].(string)
	if strings.TrimSpace(message) == "" {
		message = truncateString(string(msg), 256)
	}

	return errors.Errorf("websocket error event received (status=%.0f, code=%s): %s", status, code, message)
}

// isResponseWebSocketTerminalMessage reports whether the websocket payload indicates
// a terminal state for one response.create request.
//
// Parameters:
//   - msg: one websocket text payload.
//
// Returns:
//   - bool: true when the payload is terminal.
func isResponseWebSocketTerminalMessage(msg []byte) bool {
	if len(msg) == 0 {
		return false
	}

	var event map[string]any
	if err := json.Unmarshal(msg, &event); err != nil {
		return false
	}

	if terminatingStreamType(event) {
		return true
	}

	if typ, ok := event["type"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(typ)) {
		case "error", "response.failed", "response.canceled":
			return true
		}
	}

	return false
}
