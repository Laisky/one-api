package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"
)

const (
	defaultWSGuardBoundModel  = "gpt-5-mini"
	defaultWSGuardAttackModel = "gpt-5"
	defaultWSGuardTimeout     = 30 * time.Second
	wsGuardReadDeadline       = 10 * time.Second
	wsGuardErrorCode          = "model_switch_denied"
)

// wsGuardOptions captures CLI flags for `chat ws-guard`. The default invocation
// runs only the security-critical attack scenario, which does not require a
// working upstream channel because the proxy must reject before forwarding.
type wsGuardOptions struct {
	apiEndpoint  string
	apiToken     string
	boundModel   string
	attackModel  string
	timeout      time.Duration
	runHappyPath bool
	runAttack    bool
}

// parseWSGuardArgs parses `chat ws-guard` flags.
//
// Parameters:
//   - args: raw CLI tokens after `chat ws-guard`.
//   - cfg: shared harness configuration providing defaults.
//
// Returns:
//   - wsGuardOptions: normalized options.
//   - error: parsing/validation error.
func parseWSGuardArgs(args []string, cfg config) (wsGuardOptions, error) {
	fs := flag.NewFlagSet("chat ws-guard", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	defaultEndpoint := strings.TrimSuffix(cfg.APIBase, "/") + "/v1/responses"

	var opts wsGuardOptions
	fs.StringVar(&opts.apiEndpoint, "endpoint", defaultEndpoint,
		"Response API endpoint, e.g. http://host:port/v1/responses")
	fs.StringVar(&opts.apiToken, "token", strings.TrimSpace(cfg.Token),
		"API token; defaults to API_TOKEN")
	fs.StringVar(&opts.boundModel, "bound-model", defaultWSGuardBoundModel,
		"model bound at WS handshake via ?model=")
	fs.StringVar(&opts.attackModel, "attack-model", defaultWSGuardAttackModel,
		"model the attacker tries to swap to inside response.create")
	fs.DurationVar(&opts.timeout, "timeout", defaultWSGuardTimeout,
		"per-scenario timeout")
	fs.BoolVar(&opts.runHappyPath, "happy-path", false,
		"also run happy-path scenarios (matching model + missing-model injection); requires a working upstream channel")
	skipAttack := fs.Bool("skip-attack", false,
		"skip the model-switch attack scenario (e.g. when only sanity-checking the happy path)")

	if err := fs.Parse(args); err != nil {
		return wsGuardOptions{}, errors.Wrap(err, "parse chat ws-guard flags")
	}

	opts.runAttack = !*skipAttack

	opts.apiEndpoint = strings.TrimSpace(opts.apiEndpoint)
	opts.apiToken = strings.TrimSpace(opts.apiToken)
	opts.boundModel = strings.TrimSpace(opts.boundModel)
	opts.attackModel = strings.TrimSpace(opts.attackModel)

	if opts.apiEndpoint == "" {
		return wsGuardOptions{}, errors.New("endpoint is required")
	}
	if opts.apiToken == "" {
		return wsGuardOptions{}, errors.New("token is required")
	}
	if opts.boundModel == "" {
		return wsGuardOptions{}, errors.New("bound-model is required")
	}
	if opts.attackModel == "" {
		return wsGuardOptions{}, errors.New("attack-model is required")
	}
	if opts.boundModel == opts.attackModel {
		return wsGuardOptions{}, errors.New("attack-model must differ from bound-model")
	}
	if opts.timeout <= 0 {
		return wsGuardOptions{}, errors.New("timeout must be positive")
	}
	if !opts.runAttack && !opts.runHappyPath {
		return wsGuardOptions{}, errors.New("nothing to run: do not combine --skip-attack with the default (no --happy-path)")
	}

	return opts, nil
}

// runChatResponseWSGuardProbe executes the configured guard scenarios against
// a live one-api server and returns an aggregate error.
//
// Parameters:
//   - ctx: command context.
//   - logger: structured logger.
//   - opts: probe options.
//
// Returns:
//   - error: non-nil when any scenario does not behave as expected.
func runChatResponseWSGuardProbe(ctx context.Context, logger glog.Logger, opts wsGuardOptions) error {
	wsEndpoint, err := resolveChatResponseWSEndpoint(opts.apiEndpoint)
	if err != nil {
		return errors.Wrap(err, "resolve websocket endpoint")
	}

	logger.Info("ws-guard probe starting",
		zap.String("endpoint", wsEndpoint),
		zap.String("bound_model", opts.boundModel),
		zap.String("attack_model", opts.attackModel),
		zap.Bool("run_attack", opts.runAttack),
		zap.Bool("run_happy_path", opts.runHappyPath),
	)

	var failures []string

	if opts.runAttack {
		if err := runWSGuardAttackScenario(ctx, logger, wsEndpoint, opts); err != nil {
			logger.Error("scenario FAILED: attack", zap.Error(err))
			failures = append(failures, "attack: "+err.Error())
		} else {
			logger.Info("scenario PASSED: attack")
		}
	}

	if opts.runHappyPath {
		if err := runWSGuardHappyMatchingScenario(ctx, logger, wsEndpoint, opts); err != nil {
			logger.Error("scenario FAILED: happy-matching", zap.Error(err))
			failures = append(failures, "happy-matching: "+err.Error())
		} else {
			logger.Info("scenario PASSED: happy-matching")
		}

		if err := runWSGuardHappyMissingScenario(ctx, logger, wsEndpoint, opts); err != nil {
			logger.Error("scenario FAILED: happy-missing-model", zap.Error(err))
			failures = append(failures, "happy-missing-model: "+err.Error())
		} else {
			logger.Info("scenario PASSED: happy-missing-model")
		}
	}

	if len(failures) > 0 {
		return errors.Errorf("ws-guard probe failed scenarios: %s", strings.Join(failures, " | "))
	}

	logger.Info("ws-guard probe completed successfully")
	return nil
}

// runWSGuardAttackScenario dials the proxy with `?model=<boundModel>` and
// sends a `response.create` event carrying `<attackModel>`. The proxy MUST
// reply with a `model_switch_denied` error event and close with
// `ClosePolicyViolation`. Any other outcome counts as a regression of the
// security fix.
func runWSGuardAttackScenario(ctx context.Context, logger glog.Logger, wsEndpoint string, opts wsGuardOptions) error {
	dialURL := appendQueryModel(wsEndpoint, opts.boundModel)

	conn, err := dialWSGuard(ctx, dialURL, opts.apiToken, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	attack := map[string]any{
		"type":  "response.create",
		"model": opts.attackModel,
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "ping"},
				},
			},
		},
	}
	attackBytes, _ := json.Marshal(attack)

	if err := conn.WriteMessage(websocket.TextMessage, attackBytes); err != nil {
		return errors.Wrap(err, "send attack frame")
	}

	logger.Info("attack frame sent",
		zap.String("bound_model", opts.boundModel),
		zap.String("attack_model", opts.attackModel),
	)

	deadline := time.Now().Add(opts.timeout)
	var sawErrorEvent bool
	var sawCorrectClose bool
	var observedCloseCode int

	for {
		readDeadline := time.Now().Add(wsGuardReadDeadline)
		if !readDeadline.Before(deadline) {
			readDeadline = deadline
		}
		if err := conn.SetReadDeadline(readDeadline); err != nil {
			return errors.Wrap(err, "set read deadline")
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				observedCloseCode = closeErr.Code
				if closeErr.Code == websocket.ClosePolicyViolation {
					sawCorrectClose = true
				}
				break
			}
			return errors.Wrap(err, "read message after attack")
		}

		event, parseErr := decodeWSEvent(msg)
		if parseErr != nil {
			continue
		}
		if eventType, _ := event["type"].(string); eventType == "error" {
			errBody, _ := event["error"].(map[string]any)
			code, _ := errBody["code"].(string)
			if code == wsGuardErrorCode {
				sawErrorEvent = true
				logger.Info("received expected error event",
					zap.String("code", code),
					zap.Any("message", errBody["message"]),
				)
			} else {
				return errors.Errorf("unexpected error event code: %q (full payload: %s)",
					code, truncateString(string(msg), 256))
			}
		} else if isResponseWebSocketTerminalMessage(msg) {
			return errors.Errorf("received non-error terminal event during attack: %s",
				truncateString(string(msg), 256))
		}
	}

	if !sawErrorEvent {
		return errors.Errorf("did not receive %q error event before close (close_code=%d)",
			wsGuardErrorCode, observedCloseCode)
	}
	if !sawCorrectClose {
		return errors.Errorf("expected close code %d (ClosePolicyViolation), got %d",
			websocket.ClosePolicyViolation, observedCloseCode)
	}

	return nil
}

// runWSGuardHappyMatchingScenario verifies that sending `response.create` with
// the same model as the bound handshake succeeds end-to-end. Requires a
// working upstream channel.
func runWSGuardHappyMatchingScenario(ctx context.Context, logger glog.Logger, wsEndpoint string, opts wsGuardOptions) error {
	dialURL := appendQueryModel(wsEndpoint, opts.boundModel)
	return runWSGuardHappyScenario(ctx, logger, dialURL, opts, opts.boundModel)
}

// runWSGuardHappyMissingScenario verifies that sending `response.create` with
// no `model` field has the bound model injected and reaches a normal terminal
// state. Requires a working upstream channel.
func runWSGuardHappyMissingScenario(ctx context.Context, logger glog.Logger, wsEndpoint string, opts wsGuardOptions) error {
	dialURL := appendQueryModel(wsEndpoint, opts.boundModel)
	return runWSGuardHappyScenario(ctx, logger, dialURL, opts, "")
}

// runWSGuardHappyScenario sends one `response.create` with the supplied
// (possibly empty) model field and waits for a normal terminal event. Used
// by both happy-path scenarios.
func runWSGuardHappyScenario(
	ctx context.Context,
	logger glog.Logger,
	dialURL string,
	opts wsGuardOptions,
	createModel string,
) error {
	conn, err := dialWSGuard(ctx, dialURL, opts.apiToken, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	createEvent := map[string]any{
		"type": "response.create",
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": defaultChatResponsePrompt},
				},
			},
		},
	}
	if createModel != "" {
		createEvent["model"] = createModel
	}
	payload, _ := json.Marshal(createEvent)

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return errors.Wrap(err, "send response.create")
	}

	deadline := time.Now().Add(opts.timeout)
	for {
		readDeadline := time.Now().Add(wsGuardReadDeadline)
		if !readDeadline.Before(deadline) {
			readDeadline = deadline
		}
		if err := conn.SetReadDeadline(readDeadline); err != nil {
			return errors.Wrap(err, "set read deadline")
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return errors.Wrap(err, "read message")
		}

		if err := responseWebSocketMessageError(msg); err != nil {
			return errors.Wrap(err, "upstream error event")
		}
		if isResponseWebSocketTerminalMessage(msg) {
			logger.Info("happy-path terminal event received",
				zap.String("create_model", createModel),
				zap.String("preview", truncateString(string(msg), 192)),
			)
			return nil
		}
	}
}

// dialWSGuard opens one WS connection to the given URL using the supplied
// bearer token.
func dialWSGuard(ctx context.Context, wsURL, token string, timeout time.Duration) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	conn, resp, err := dialer.DialContext(dialCtx, wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, errors.Wrapf(err, "dial websocket (status=%d)", status)
	}
	return conn, nil
}

// appendQueryModel adds `?model=<model>` to the WS URL, preserving any
// existing query parameters.
func appendQueryModel(wsEndpoint, model string) string {
	if model == "" {
		return wsEndpoint
	}
	sep := "?"
	if strings.Contains(wsEndpoint, "?") {
		sep = "&"
	}
	return wsEndpoint + sep + "model=" + model
}

// decodeWSEvent decodes a WS text payload into a generic event map.
func decodeWSEvent(msg []byte) (map[string]any, error) {
	var ev map[string]any
	if err := json.Unmarshal(msg, &ev); err != nil {
		return nil, errors.Wrap(err, "decode ws event")
	}
	return ev, nil
}
