package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io"
	"math"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"
)

const (
	defaultRealtimeModel      = "gpt-realtime"
	defaultRealtimePrompt     = "Reply with exactly: pong"
	defaultRealtimeTimeout    = 90 * time.Second
	realtimeIdleObservation   = 3 * time.Second
	realtimeTranscriptionName = "whisper-1"
	realtimeInputSampleRate   = 24000
)

// realtimeOptions captures CLI flags for the `realtime` command, which speaks
// the OpenAI Realtime event protocol rather than Gemini's native Live frames.
type realtimeOptions struct {
	apiBase       string
	apiToken      string
	model         string
	switchModel   string
	prompt        string
	timeout       time.Duration
	verifyBilling bool
	scenarios     []string
}

// realtimeScenarioNames returns the ordered default suite. Parameters: none.
// Returns: the accepted values for --scenarios.
func realtimeScenarioNames() []string {
	return []string{"conversation", "audio", "model-guard", "idle"}
}

// realtime runs the OpenAI Realtime end-to-end probes. Parameters: ctx bounds
// the run, logger reports progress and args are the CLI tokens after
// `realtime`. Returns: an error when any selected scenario misbehaves.
func realtime(ctx context.Context, logger glog.Logger, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return errors.Wrap(err, "load config")
	}
	opts, err := parseRealtimeArgs(args, cfg)
	if err != nil {
		return errors.Wrap(err, "parse realtime arguments")
	}
	return runRealtimeProbe(ctx, logger, opts)
}

// parseRealtimeArgs parses `realtime` flags. Parameters: args are raw CLI
// tokens and cfg supplies defaults. Returns: options or a validation error.
func parseRealtimeArgs(args []string, cfg config) (realtimeOptions, error) {
	fs := flag.NewFlagSet("realtime", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts realtimeOptions
	var scenarios string
	fs.StringVar(&opts.apiBase, "api-base", strings.TrimSuffix(cfg.APIBase, "/"),
		"one-api base URL, e.g. http://127.0.0.1:3000")
	fs.StringVar(&opts.apiToken, "token", strings.TrimSpace(cfg.Token), "API token; defaults to API_TOKEN")
	fs.StringVar(&opts.model, "model", defaultRealtimeModel, "realtime model bound at the handshake")
	fs.StringVar(&opts.switchModel, "switch-model", "gpt-realtime-2",
		"model the model-guard scenario tries to switch to inside session.update")
	fs.StringVar(&opts.prompt, "prompt", defaultRealtimePrompt, "text turn sent to the model")
	fs.DurationVar(&opts.timeout, "timeout", defaultRealtimeTimeout, "per-scenario timeout")
	fs.BoolVar(&opts.verifyBilling, "verify-billing", true,
		"assert each session's persisted consume log matches the provider receipts")
	fs.StringVar(&scenarios, "scenarios", strings.Join(realtimeScenarioNames(), ","),
		"comma-separated subset of: "+strings.Join(realtimeScenarioNames(), ", "))

	if err := fs.Parse(args); err != nil {
		return realtimeOptions{}, errors.Wrap(err, "parse realtime flags")
	}

	opts.apiBase = strings.TrimSuffix(strings.TrimSpace(opts.apiBase), "/")
	opts.apiToken = strings.TrimSpace(opts.apiToken)
	opts.model = strings.TrimSpace(opts.model)
	opts.switchModel = strings.TrimSpace(opts.switchModel)
	opts.prompt = strings.TrimSpace(opts.prompt)

	switch {
	case opts.apiBase == "":
		return realtimeOptions{}, errors.New("api-base is required")
	case opts.apiToken == "":
		return realtimeOptions{}, errors.New("token is required")
	case opts.model == "":
		return realtimeOptions{}, errors.New("model is required")
	case opts.prompt == "":
		return realtimeOptions{}, errors.New("prompt is required")
	case opts.timeout <= 0:
		return realtimeOptions{}, errors.New("timeout must be positive")
	case opts.switchModel == opts.model:
		return realtimeOptions{}, errors.New("switch-model must differ from model")
	}

	known := make(map[string]bool, len(realtimeScenarioNames()))
	for _, name := range realtimeScenarioNames() {
		known[name] = true
	}
	for _, part := range strings.Split(scenarios, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		if !known[name] {
			return realtimeOptions{}, errors.Errorf("unknown scenario %q", name)
		}
		opts.scenarios = append(opts.scenarios, name)
	}
	if len(opts.scenarios) == 0 {
		return realtimeOptions{}, errors.New("no scenarios selected")
	}
	return opts, nil
}

// runRealtimeProbe executes each selected scenario. Parameters: ctx, logger and
// opts describe the run. Returns: an aggregate error naming every failure.
func runRealtimeProbe(ctx context.Context, logger glog.Logger, opts realtimeOptions) error {
	wsBase, err := resolveLiveWSEndpoint(opts.apiBase)
	if err != nil {
		return errors.Wrap(err, "resolve websocket endpoint")
	}
	logger.Info("realtime probe starting",
		zap.String("endpoint", wsBase),
		zap.String("model", opts.model),
		zap.Strings("scenarios", opts.scenarios))

	runners := map[string]func(context.Context, glog.Logger, string, realtimeOptions) error{
		"conversation": runRealtimeConversationScenario,
		"audio":        runRealtimeAudioScenario,
		"model-guard":  runRealtimeModelGuardScenario,
		"idle":         runRealtimeIdleScenario,
	}
	var failures []string
	for _, name := range opts.scenarios {
		if err := runners[name](ctx, logger, wsBase, opts); err != nil {
			logger.Error("scenario FAILED", zap.String("scenario", name), zap.Error(err))
			failures = append(failures, name+": "+err.Error())
			continue
		}
		logger.Info("scenario PASSED", zap.String("scenario", name))
	}
	if len(failures) > 0 {
		return errors.Errorf("realtime probe failed scenarios: %s", strings.Join(failures, " | "))
	}
	logger.Info("realtime probe completed successfully")
	return nil
}

// runRealtimeConversationScenario holds two text turns and settles them.
// Parameters: ctx, logger, wsBase and opts describe the run. Returns: an error
// when a turn, a receipt, or the persisted settlement disagrees. It opens with
// a session.update that echoes the bound model, which is what the server's own
// session.created carries and what conformant clients send back; rejecting it
// closed every such session with a policy violation.
func runRealtimeConversationScenario(ctx context.Context, logger glog.Logger, wsBase string, opts realtimeOptions) error {
	startedAt := time.Now().Unix()
	conn, err := dialLive(ctx, appendQueryModel(wsBase, opts.model), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	created, err := awaitRealtimeEvent(conn, opts.timeout, "session.created")
	if err != nil {
		return err
	}
	boundModel := realtimeSessionModel(created)
	if boundModel == "" {
		return errors.New("session.created carried no session model")
	}
	if err := writeLiveJSON(conn, map[string]any{"type": "session.update", "session": map[string]any{
		"type": "realtime", "model": boundModel, "output_modalities": []string{"text"},
	}}); err != nil {
		return errors.Wrap(err, "send session.update")
	}
	if _, err := awaitRealtimeEvent(conn, opts.timeout, "session.updated"); err != nil {
		return errors.Wrap(err, "echoing the bound model in session.update must be accepted")
	}

	var receipts []map[string]any
	for _, text := range []string{opts.prompt, "Now say: ping"} {
		usage, err := runRealtimeTextTurn(conn, opts, text)
		if err != nil {
			return err
		}
		receipts = append(receipts, usage)
		logger.Info("realtime turn billed", zap.String("usage", truncateString(marshalCompact(usage), 320)))
	}

	_ = conn.Close()
	if !opts.verifyBilling {
		return nil
	}
	return verifyRealtimeSettlement(ctx, logger, opts, startedAt, receipts, 0)
}

// runRealtimeAudioScenario sends PCM audio with transcription enabled.
// Parameters: ctx, logger, wsBase and opts describe the run. Returns: an error
// when the audio turn is not billed, or when the transcription receipt, which
// the provider reports as an audio duration rather than tokens, is lost.
func runRealtimeAudioScenario(ctx context.Context, logger glog.Logger, wsBase string, opts realtimeOptions) error {
	startedAt := time.Now().Unix()
	conn, err := dialLive(ctx, appendQueryModel(wsBase, opts.model), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err := awaitRealtimeEvent(conn, opts.timeout, "session.created"); err != nil {
		return err
	}
	if err := writeLiveJSON(conn, map[string]any{"type": "session.update", "session": map[string]any{
		"type":              "realtime",
		"output_modalities": []string{"audio"},
		"audio": map[string]any{
			"input": map[string]any{
				"format":         map[string]any{"type": "audio/pcm", "rate": realtimeInputSampleRate},
				"transcription":  map[string]any{"model": realtimeTranscriptionName},
				"turn_detection": nil,
			},
			"output": map[string]any{"format": map[string]any{"type": "audio/pcm", "rate": realtimeInputSampleRate}},
		},
	}}); err != nil {
		return errors.Wrap(err, "send session.update")
	}
	if _, err := awaitRealtimeEvent(conn, opts.timeout, "session.updated"); err != nil {
		return err
	}
	if err := writeLiveJSON(conn, map[string]any{
		"type": "input_audio_buffer.append", "audio": realtimeVoicedPCM16(1200 * time.Millisecond),
	}); err != nil {
		return errors.Wrap(err, "append audio")
	}
	if err := writeLiveJSON(conn, map[string]any{"type": "input_audio_buffer.commit"}); err != nil {
		return errors.Wrap(err, "commit audio")
	}
	if err := writeLiveJSON(conn, map[string]any{"type": "response.create"}); err != nil {
		return errors.Wrap(err, "request response")
	}

	done, err := awaitRealtimeEvent(conn, opts.timeout, "response.done")
	if err != nil {
		return err
	}
	usage, err := realtimeResponseUsage(done)
	if err != nil {
		return err
	}
	if audio := realtimeDetailCount(usage, "output_token_details", "audio_tokens"); audio <= 0 {
		return errors.Errorf("audio turn reported no output audio tokens: %s", marshalCompact(usage))
	}
	logger.Info("realtime audio turn billed", zap.String("usage", truncateString(marshalCompact(usage), 320)))

	_ = conn.Close()
	if !opts.verifyBilling {
		return nil
	}
	// The transcription receipt is a second, duration-priced record: one audio
	// turn with transcription enabled must settle two receipts, not one.
	return verifyRealtimeSettlement(ctx, logger, opts, startedAt, []map[string]any{usage}, 1)
}

// runRealtimeModelGuardScenario proves a session cannot be moved to another
// model. Parameters: ctx, logger, wsBase and opts describe the run. Returns: an
// error when the switch is accepted, which would let a cheap channel serve an
// expensive model.
func runRealtimeModelGuardScenario(ctx context.Context, logger glog.Logger, wsBase string, opts realtimeOptions) error {
	conn, err := dialLive(ctx, appendQueryModel(wsBase, opts.model), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err := awaitRealtimeEvent(conn, opts.timeout, "session.created"); err != nil {
		return err
	}
	if err := writeLiveJSON(conn, map[string]any{"type": "session.update", "session": map[string]any{
		"type": "realtime", "model": opts.switchModel,
	}}); err != nil {
		return errors.Wrap(err, "send session.update")
	}

	deadline := time.Now().Add(opts.timeout)
	var sawDenial bool
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return errors.Wrap(err, "set read deadline")
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				if !sawDenial {
					return errors.Errorf("session closed with %d (%s) but no model_switch_denied error event",
						closeErr.Code, closeErr.Text)
				}
				if closeErr.Code != websocket.ClosePolicyViolation {
					return errors.Errorf("expected close %d, got %d (%s)",
						websocket.ClosePolicyViolation, closeErr.Code, closeErr.Text)
				}
				logger.Info("model switch denied as expected", zap.String("reason", closeErr.Text))
				return nil
			}
			return errors.Wrap(err, "read after model switch")
		}
		event, err := decodeWSEvent(msg)
		if err != nil {
			continue
		}
		if kind, _ := event["type"].(string); kind != "error" {
			if kind == "session.updated" {
				return errors.Errorf("session.update switched the model to %q", opts.switchModel)
			}
			continue
		}
		body, _ := event["error"].(map[string]any)
		code, _ := body["code"].(string)
		if code != wsGuardErrorCode {
			return errors.Errorf("unexpected error code %q: %s", code, truncateString(string(msg), 256))
		}
		sawDenial = true
	}
}

// runRealtimeIdleScenario connects without sending any work. Parameters: ctx,
// logger, wsBase and opts describe the run. Returns: an error when the idle
// session is charged. A reservation is a hold, not a minimum session fee.
func runRealtimeIdleScenario(ctx context.Context, logger glog.Logger, wsBase string, opts realtimeOptions) error {
	startedAt := time.Now().Unix()
	conn, err := dialLive(ctx, appendQueryModel(wsBase, opts.model), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return err
	}
	if _, err := awaitRealtimeEvent(conn, opts.timeout, "session.created"); err != nil {
		_ = conn.Close()
		return err
	}
	time.Sleep(realtimeIdleObservation)
	_ = conn.Close()
	if !opts.verifyBilling {
		return nil
	}

	deadline := time.Now().Add(liveSettlementTimeout)
	for {
		entries, err := fetchConsumeLogs(ctx, opts.apiBase, opts.apiToken, opts.model)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.CreatedAt+1 < startedAt || entry.Metadata.Usage.Receipts != 0 {
				continue
			}
			if entry.Quota != 0 || entry.PromptTokens != 0 || entry.CompletionTokens != 0 {
				return errors.Errorf("idle session charged quota=%d prompt=%d completion=%d",
					entry.Quota, entry.PromptTokens, entry.CompletionTokens)
			}
			logger.Info("idle session settled free", zap.String("content", entry.Content))
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Errorf("no settled consume log for the idle session appeared within %s", liveSettlementTimeout)
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for settlement")
		case <-time.After(time.Second):
		}
	}
}

// verifyRealtimeSettlement matches a session against its persisted consume log.
// Parameters: ctx, logger and opts describe the run, startedAt bounds the
// search, receipts are the observed response usages and extraReceipts counts
// provider receipts the client cannot read, such as a duration-priced
// transcription. Returns: an error on any disagreement.
func verifyRealtimeSettlement(ctx context.Context, logger glog.Logger, opts realtimeOptions,
	startedAt int64, receipts []map[string]any, extraReceipts int) error {
	var wantPrompt, wantCompletion int64
	for _, usage := range receipts {
		input, _ := liveReceiptCount(usage, "input_tokens")
		output, _ := liveReceiptCount(usage, "output_tokens")
		wantPrompt += input
		wantCompletion += output
	}
	wantReceipts := len(receipts) + extraReceipts

	deadline := time.Now().Add(liveSettlementTimeout)
	var last string
	for {
		entries, err := fetchConsumeLogs(ctx, opts.apiBase, opts.apiToken, opts.model)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.CreatedAt+1 < startedAt || entry.Metadata.Usage.Receipts != wantReceipts {
				continue
			}
			last = entry.Content
			if entry.Metadata.Usage.UsageGap {
				return errors.Errorf("settled log reports a usage gap: %s", entry.Content)
			}
			if !entry.Metadata.BillingComplete {
				return errors.Errorf("settled log is not marked billing-complete: %s", entry.Content)
			}
			if int64(entry.PromptTokens) != wantPrompt || int64(entry.CompletionTokens) != wantCompletion {
				return errors.Errorf("settled tokens %d/%d do not match the provider receipts %d/%d",
					entry.PromptTokens, entry.CompletionTokens, wantPrompt, wantCompletion)
			}
			if entry.Quota <= 0 {
				return errors.Errorf("a session that consumed %d/%d tokens settled at quota %d",
					wantPrompt, wantCompletion, entry.Quota)
			}
			logger.Info("session settled",
				zap.Int64("quota", entry.Quota),
				zap.Int("prompt_tokens", entry.PromptTokens),
				zap.Int("completion_tokens", entry.CompletionTokens),
				zap.Int("receipts", entry.Metadata.Usage.Receipts),
				zap.String("content", entry.Content))
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Errorf("no settled consume log with %d receipts appeared within %s (last seen: %s)",
				wantReceipts, liveSettlementTimeout, last)
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for settlement")
		case <-time.After(time.Second):
		}
	}
}

// runRealtimeTextTurn sends one text message and waits for its receipt.
// Parameters: conn is an established session, opts bounds the turn and text is
// the user message. Returns: the response usage object or an error.
func runRealtimeTextTurn(conn *websocket.Conn, opts realtimeOptions, text string) (map[string]any, error) {
	if err := writeLiveJSON(conn, map[string]any{"type": "conversation.item.create", "item": map[string]any{
		"type": "message", "role": "user",
		"content": []map[string]any{{"type": "input_text", "text": text}},
	}}); err != nil {
		return nil, errors.Wrap(err, "create conversation item")
	}
	if err := writeLiveJSON(conn, map[string]any{"type": "response.create"}); err != nil {
		return nil, errors.Wrap(err, "request response")
	}
	done, err := awaitRealtimeEvent(conn, opts.timeout, "response.done")
	if err != nil {
		return nil, err
	}
	return realtimeResponseUsage(done)
}

// awaitRealtimeEvent reads until the named event arrives. Parameters: conn is
// the session, timeout bounds the wait and want is the event type. Returns: the
// decoded event, or an error on a close, an upstream error event, or a timeout.
func awaitRealtimeEvent(conn *websocket.Conn, timeout time.Duration, want string) (map[string]any, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, errors.Wrap(err, "set read deadline")
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return nil, errors.Errorf("session closed with %d (%s) while waiting for %s",
					closeErr.Code, closeErr.Text, want)
			}
			return nil, errors.Wrapf(err, "read while waiting for %s", want)
		}
		event, err := decodeWSEvent(msg)
		if err != nil {
			continue
		}
		kind, _ := event["type"].(string)
		if kind == want {
			return event, nil
		}
		if kind == "error" {
			return nil, errors.Errorf("upstream error event while waiting for %s: %s",
				want, truncateString(string(msg), 256))
		}
	}
}

// realtimeResponseUsage extracts the usage object from a response.done event.
// Parameters: event is the decoded frame. Returns: the usage map or an error
// when the response completed without one, which would settle unbilled.
func realtimeResponseUsage(event map[string]any) (map[string]any, error) {
	response, ok := event["response"].(map[string]any)
	if !ok {
		return nil, errors.New("response.done carried no response object")
	}
	if status, _ := response["status"].(string); status != "completed" {
		return nil, errors.Errorf("response ended with status %q: %s", status, truncateString(marshalCompact(response), 256))
	}
	usage, ok := response["usage"].(map[string]any)
	if !ok {
		return nil, errors.New("response.done carried no usage; the turn would settle unbilled")
	}
	return usage, nil
}

// realtimeSessionModel reads the model a session acknowledgement reports.
// Parameters: event is session.created or session.updated. Returns: the model.
func realtimeSessionModel(event map[string]any) string {
	session, ok := event["session"].(map[string]any)
	if !ok {
		return ""
	}
	model, _ := session["model"].(string)
	return model
}

// realtimeDetailCount reads one nested usage counter. Parameters: usage is the
// receipt, section the detail object and field its counter. Returns: the value.
func realtimeDetailCount(usage map[string]any, section, field string) int64 {
	details, ok := usage[section].(map[string]any)
	if !ok {
		return 0
	}
	value, ok := details[field].(float64)
	if !ok {
		return 0
	}
	return int64(value)
}

// realtimeVoicedPCM16 synthesizes a deterministic voiced sweep. Parameters:
// duration is the clip length. Returns: base64 PCM16 mono at the session rate.
// A pure tone is often discarded as non-speech; a harmonic stack with an
// envelope reliably produces a committed audio item.
func realtimeVoicedPCM16(duration time.Duration) string {
	samples := int(duration.Seconds() * realtimeInputSampleRate)
	raw := make([]byte, 0, samples*2)
	frame := make([]byte, 2)
	for i := range samples {
		t := float64(i) / realtimeInputSampleRate
		fundamental := 140 + 60*math.Sin(2*math.Pi*3*t)
		envelope := 0.5 * (1 - math.Cos(2*math.Pi*t/duration.Seconds()))
		var value float64
		for harmonic, amplitude := range map[float64]float64{1: 1, 2: 0.5, 3: 0.33, 4: 0.25} {
			value += amplitude * math.Sin(2*math.Pi*fundamental*harmonic*t)
		}
		binary.LittleEndian.PutUint16(frame, uint16(int16(9000*envelope*value/2.08)))
		raw = append(raw, frame...)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// marshalCompact renders a value for a log line. Parameters: value is any
// JSON-encodable object. Returns: its compact encoding, or a placeholder.
func marshalCompact(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "<unencodable>"
	}
	return string(encoded)
}
