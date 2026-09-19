package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"

	sharedconfig "github.com/Laisky/one-api/common/config"
)

const (
	defaultLiveModel          = "gemini-3.8-live"
	defaultLiveThinkingModel  = "gemini-3.8-live-extended-thinking"
	defaultLivePrompt         = "Say hello in five words."
	defaultLiveTimeout        = 90 * time.Second
	liveReadDeadline          = 45 * time.Second
	liveMaxFramesPerTurn      = 4096
	maxLiveTranscriptBytes    = 4 << 10
	liveInvalidSetupCloseCode = websocket.ClosePolicyViolation
	liveSettlementTimeout     = 30 * time.Second
)

// liveOptions captures CLI flags for the `live` command. Scenarios that only
// exercise proxy-side validation still need a reachable Gemini channel, because
// one-api dials Google before it upgrades the caller's socket.
type liveOptions struct {
	apiBase       string
	apiToken      string
	model         string
	thinkingModel string
	prompt        string
	timeout       time.Duration
	verifyBilling bool
	scenarios     []string
}

// liveScenario names one executable probe. Parameters: none. Returns: the
// ordered default suite, which is also the accepted value set for --scenarios.
func liveScenarioNames() []string {
	return []string{"conversation", "thinking", "setup-guard", "rest-guard", "subprotocol"}
}

// live runs the Gemini Live end-to-end probes. Parameters: ctx bounds the run,
// logger reports progress and args are the CLI tokens after `live`. Returns: a
// non-nil error when any selected scenario does not behave as documented.
func live(ctx context.Context, logger glog.Logger, args []string) error {
	opts, err := parseLiveArgs(args, loadLiveConfig())
	if err != nil {
		return errors.Wrap(err, "parse live arguments")
	}
	return runLiveProbe(ctx, logger, opts)
}

// loadLiveConfig reads only the settings needed by the standalone Live probe.
// Parameters: none. Returns: base URL and optional token defaults without
// validating model-suite settings that the Live command never consumes.
func loadLiveConfig() config {
	base := strings.TrimSpace(sharedconfig.APIBase)
	if base == "" {
		base = defaultAPIBase
	}
	return config{
		APIBase: strings.TrimSuffix(base, "/"),
		Token:   strings.TrimSpace(sharedconfig.APIToken),
	}
}

// parseLiveArgs parses `live` flags. Parameters: args are raw CLI tokens and cfg
// supplies harness defaults. Returns: normalized options or a validation error.
func parseLiveArgs(args []string, cfg config) (liveOptions, error) {
	fs := flag.NewFlagSet("live", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts liveOptions
	var scenarios string
	fs.StringVar(&opts.apiBase, "api-base", strings.TrimSuffix(cfg.APIBase, "/"),
		"one-api base URL, e.g. http://127.0.0.1:3000")
	fs.StringVar(&opts.apiToken, "token", strings.TrimSpace(cfg.Token), "API token; defaults to API_TOKEN")
	fs.StringVar(&opts.model, "model", defaultLiveModel, "Live model bound at the handshake")
	fs.StringVar(&opts.thinkingModel, "thinking-model", defaultLiveThinkingModel,
		"Live model used by the extended-thinking scenario")
	fs.StringVar(&opts.prompt, "prompt", defaultLivePrompt, "text turn sent to the model")
	fs.DurationVar(&opts.timeout, "timeout", defaultLiveTimeout, "per-scenario timeout")
	fs.BoolVar(&opts.verifyBilling, "verify-billing", true,
		"assert the conversation's persisted consume log matches the provider receipts")
	fs.StringVar(&scenarios, "scenarios", strings.Join(liveScenarioNames(), ","),
		"comma-separated subset of: "+strings.Join(liveScenarioNames(), ", "))

	if err := fs.Parse(args); err != nil {
		return liveOptions{}, errors.Wrap(err, "parse live flags")
	}

	opts.apiBase = strings.TrimSuffix(strings.TrimSpace(opts.apiBase), "/")
	opts.apiToken = strings.TrimSpace(opts.apiToken)
	opts.model = strings.TrimSpace(opts.model)
	opts.thinkingModel = strings.TrimSpace(opts.thinkingModel)
	opts.prompt = strings.TrimSpace(opts.prompt)

	if opts.apiBase == "" {
		return liveOptions{}, errors.New("api-base is required")
	}
	if opts.apiToken == "" {
		return liveOptions{}, errors.New("token is required")
	}
	if opts.model == "" {
		return liveOptions{}, errors.New("model is required")
	}
	if opts.prompt == "" {
		return liveOptions{}, errors.New("prompt is required")
	}
	if opts.timeout <= 0 {
		return liveOptions{}, errors.New("timeout must be positive")
	}

	known := make(map[string]bool, len(liveScenarioNames()))
	for _, name := range liveScenarioNames() {
		known[name] = true
	}
	selected := make(map[string]bool, len(liveScenarioNames()))
	for _, part := range strings.Split(scenarios, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		if !known[name] {
			return liveOptions{}, errors.Errorf("unknown scenario %q", name)
		}
		if !selected[name] {
			opts.scenarios = append(opts.scenarios, name)
			selected[name] = true
		}
	}
	if len(opts.scenarios) == 0 {
		return liveOptions{}, errors.New("no scenarios selected")
	}
	return opts, nil
}

// runLiveProbe executes each selected scenario and aggregates failures.
// Parameters: ctx, logger and opts describe the run. Returns: an aggregate error
// naming every scenario that failed, so one bad turn does not hide the others.
func runLiveProbe(ctx context.Context, logger glog.Logger, opts liveOptions) error {
	wsBase, err := resolveLiveWSEndpoint(opts.apiBase)
	if err != nil {
		return errors.Wrap(err, "resolve websocket endpoint")
	}
	logger.Info("live probe starting",
		zap.String("endpoint", wsBase),
		zap.String("model", opts.model),
		zap.Strings("scenarios", opts.scenarios))

	runners := map[string]func(context.Context, glog.Logger, string, liveOptions) error{
		"conversation": runLiveConversationScenario,
		"thinking":     runLiveThinkingScenario,
		"setup-guard":  runLiveSetupGuardScenario,
		"rest-guard":   runLiveRESTGuardScenario,
		"subprotocol":  runLiveSubprotocolScenario,
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
		return errors.Errorf("live probe failed scenarios: %s", strings.Join(failures, " | "))
	}
	logger.Info("live probe completed successfully")
	return nil
}

// runLiveConversationScenario holds one full native turn against the real model.
// Parameters: ctx, logger, wsBase and opts describe the run. Returns: an error
// when setup, streaming or the billing receipt does not match the Live contract.
// The receipt assertions are the point of this scenario: a turn that streams
// audio but reports no usage is a silent billing loss, not a success.
func runLiveConversationScenario(ctx context.Context, logger glog.Logger, wsBase string, opts liveOptions) error {
	conn, requestID, err := dialLiveWithRequestID(ctx, appendQueryModel(wsBase, opts.model), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	setup := map[string]any{"setup": map[string]any{
		"model":                    "models/" + opts.model,
		"generationConfig":         map[string]any{"responseModalities": []string{"AUDIO"}},
		"outputAudioTranscription": map[string]any{},
	}}
	if err := liveHandshake(conn, setup, opts.timeout); err != nil {
		return err
	}
	if err := writeLiveJSON(conn, map[string]any{"realtimeInput": map[string]any{"text": opts.prompt}}); err != nil {
		return errors.Wrap(err, "send text turn")
	}

	turn, err := readLiveTurn(conn, opts.timeout)
	if err != nil {
		return err
	}
	logger.Info("live turn observed",
		zap.Int("audio_chunks", turn.audioChunks),
		zap.Int("frames", turn.frames),
		zap.String("transcript", shorten(turn.transcript, 120)),
		zap.String("usage", turn.usageJSON))

	if turn.audioChunks == 0 {
		return errors.New("model returned no audio parts; AUDIO is the only supported Live response modality")
	}
	if !turn.turnComplete {
		return errors.New("turn ended without serverContent.turnComplete")
	}
	if turn.usage == nil {
		return errors.New("turn completed without a usageMetadata receipt; the session would settle unbilled")
	}
	if err := assertLiveReceipt(turn.usage); err != nil {
		return err
	}
	// A receipt the proxy cannot meter closes the socket with a policy violation
	// instead of a normal closure. Prove the conversation is still usable.
	if err := writeLiveJSON(conn, map[string]any{"realtimeInput": map[string]any{"text": "Now count from one to three."}}); err != nil {
		return errors.Wrap(err, "send second text turn")
	}
	second, err := readLiveTurn(conn, opts.timeout)
	if err != nil {
		return errors.Wrap(err, "second turn")
	}
	if second.usage == nil {
		return errors.New("second turn completed without a usageMetadata receipt")
	}
	if err := assertLiveReceipt(second.usage); err != nil {
		return errors.Wrap(err, "second turn")
	}
	logger.Info("second live turn billed", zap.String("usage", second.usageJSON))

	// Settlement happens after the socket closes, so end the session first.
	_ = conn.Close()
	if !opts.verifyBilling {
		return nil
	}
	if requestID == "" {
		return errors.New("live websocket handshake did not include X-Oneapi-Request-Id; cannot verify settlement")
	}
	return verifyLiveSettlement(ctx, logger, opts, requestID, []map[string]any{turn.usage, second.usage})
}

// verifyLiveSettlement matches the session against its persisted consume log.
// Parameters: ctx, logger and opts describe the run, requestID identifies the
// upgraded WebSocket request and receipts are the provider receipts the caller observed. Returns: an error
// when the settled row is missing, incomplete, or disagrees with those receipts.
// Settlement is asynchronous, so the row is polled for rather than read once.
func verifyLiveSettlement(ctx context.Context, logger glog.Logger, opts liveOptions,
	requestID string, receipts []map[string]any) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("request ID is required to verify Live settlement")
	}
	wantPrompt, wantCompletion := expectedLiveTokens(receipts)
	deadline := time.Now().Add(liveSettlementTimeout)
	var last string
	for {
		entries, err := fetchConsumeLogs(ctx, opts.apiBase, opts.apiToken, "")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.RequestID != requestID {
				continue
			}
			last = entry.Content
			// The matching row is created before the WebSocket session settles.
			// Keep polling only while it is the expected untouched reservation;
			// a partially reconciled row is a terminal accounting defect.
			if !entry.Metadata.BillingComplete {
				if entry.Metadata.Usage.Receipts == 0 && !entry.Metadata.Usage.UsageGap {
					continue
				}
				return errors.Errorf("settled log is incomplete with %d receipts and usage_gap=%t: %s",
					entry.Metadata.Usage.Receipts, entry.Metadata.Usage.UsageGap, entry.Content)
			}
			if entry.Metadata.Usage.Receipts != len(receipts) {
				return errors.Errorf("settled log reports %d receipts, want %d: %s",
					entry.Metadata.Usage.Receipts, len(receipts), entry.Content)
			}
			if entry.Metadata.Usage.UsageGap {
				return errors.Errorf("settled log reports a usage gap: %s", entry.Content)
			}
			if int64(entry.PromptTokens) != wantPrompt || int64(entry.CompletionTokens) != wantCompletion {
				return errors.Errorf("settled tokens %d/%d do not match the provider receipts %d/%d",
					entry.PromptTokens, entry.CompletionTokens, wantPrompt, wantCompletion)
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
			return errors.Errorf("no settled consume log for request %q appeared within %s (last seen: %s)",
				requestID, liveSettlementTimeout, last)
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for settlement")
		case <-time.After(time.Second):
		}
	}
}

// expectedLiveTokens restates the documented Live receipt rules independently of
// the server. Parameters: receipts are the observed usageMetadata objects.
// Returns: the prompt and completion totals the consume log must carry. Thinking
// tokens are billable output even when the provider's aggregate omits them.
func expectedLiveTokens(receipts []map[string]any) (prompt, completion int64) {
	for _, usage := range receipts {
		promptCount, _ := liveReceiptCount(usage, "promptTokenCount")
		response, _ := liveReceiptCount(usage, "responseTokenCount")
		thoughts, _ := liveReceiptCount(usage, "thoughtsTokenCount")
		total, _ := liveReceiptCount(usage, "totalTokenCount")
		toolPrompt, _ := liveReceiptCount(usage, "toolUsePromptTokenCount")
		extra := thoughts - liveModalityCount(usage, "responseTokensDetails", "TEXT")
		if total == promptCount+response+thoughts+toolPrompt && thoughts+toolPrompt > 0 {
			promptCount += toolPrompt
			extra = thoughts
		}
		if extra < 0 {
			extra = 0
		}
		prompt += promptCount
		completion += response + extra
	}
	return prompt, completion
}

// liveModalityCount reads one modality bucket from a receipt breakdown.
// Parameters: usage is the receipt, field the breakdown and modality its entry.
// Returns: the reported count, or zero when the provider omitted it.
func liveModalityCount(usage map[string]any, field, modality string) int64 {
	details, ok := usage[field].([]any)
	if !ok {
		return 0
	}
	for _, raw := range details {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := entry["modality"].(string); name != modality {
			continue
		}
		if count, ok := entry["tokenCount"].(float64); ok {
			return int64(count)
		}
	}
	return 0
}

// runLiveThinkingScenario exercises the configurable-thinking variant and the
// rejection of that same setting on the ordinary model. Parameters: ctx, logger,
// wsBase and opts describe the run. Returns: an error on a contract mismatch.
func runLiveThinkingScenario(ctx context.Context, logger glog.Logger, wsBase string, opts liveOptions) error {
	thinkingSetup := map[string]any{"setup": map[string]any{
		"model": "models/" + opts.thinkingModel,
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"thinkingConfig":     map[string]any{"thinkingLevel": "LOW"},
		},
		"outputAudioTranscription": map[string]any{},
	}}
	conn, err := dialLive(ctx, appendQueryModel(wsBase, opts.thinkingModel), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := liveHandshake(conn, thinkingSetup, opts.timeout); err != nil {
		return errors.Wrap(err, "extended-thinking setup")
	}
	if err := writeLiveJSON(conn, map[string]any{"realtimeInput": map[string]any{"text": opts.prompt}}); err != nil {
		return errors.Wrap(err, "send text turn")
	}
	turn, err := readLiveTurn(conn, opts.timeout)
	if err != nil {
		return err
	}
	if turn.usage == nil {
		return errors.New("extended-thinking turn completed without a usageMetadata receipt")
	}
	if err := assertLiveReceipt(turn.usage); err != nil {
		return err
	}
	logger.Info("extended-thinking turn billed",
		zap.String("usage", turn.usageJSON), zap.Int("audio_chunks", turn.audioChunks))

	// The ordinary model thinks automatically and must reject the same setting
	// before it reaches Google.
	rejected := map[string]any{"setup": map[string]any{
		"model": "models/" + opts.model,
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"thinkingConfig":     map[string]any{"thinkingLevel": "LOW"},
		},
	}}
	return expectLiveSetupRejected(ctx, logger, wsBase, opts, opts.model, rejected, "configurable thinking on the automatic-thinking model")
}

// runLiveSetupGuardScenario proves the setup frame cannot escape the model bound
// at the HTTP handshake. Parameters: ctx, logger, wsBase and opts describe the
// run. Returns: an error when any rejected setup is instead forwarded upstream.
func runLiveSetupGuardScenario(ctx context.Context, logger glog.Logger, wsBase string, opts liveOptions) error {
	cases := []struct {
		name  string
		setup map[string]any
	}{
		{"model_switch", map[string]any{"setup": map[string]any{"model": "models/gemini-3.5-flash"}}},
		{"text_modality", map[string]any{"setup": map[string]any{
			"model":            "models/" + opts.model,
			"generationConfig": map[string]any{"responseModalities": []string{"TEXT"}},
		}}},
		{"unsupported_field", map[string]any{"setup": map[string]any{
			"model": "models/" + opts.model, "sessionResumption": map[string]any{},
		}}},
		{"not_setup_first", map[string]any{"realtimeInput": map[string]any{"text": opts.prompt}}},
	}
	for _, tc := range cases {
		if err := expectLiveSetupRejected(ctx, logger, wsBase, opts, opts.model, tc.setup, tc.name); err != nil {
			return err
		}
	}
	return nil
}

// runLiveSubprotocolScenario checks the browser handshake. Parameters: ctx,
// logger, wsBase and opts describe the run. Returns: an error when the
// non-authentication protocol is not echoed back to the caller. The token-bearing
// protocol must never be selected, because a selected protocol is sent upstream.
func runLiveSubprotocolScenario(ctx context.Context, logger glog.Logger, wsBase string, opts liveOptions) error {
	protocols := []string{"gemini-live", "openai-insecure-api-key." + opts.apiToken}
	conn, err := dialLive(ctx, appendQueryModel(wsBase, opts.model), "", protocols, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if got := conn.Subprotocol(); got != "gemini-live" {
		return errors.Errorf("negotiated subprotocol %q, want %q", got, "gemini-live")
	}
	setup := map[string]any{"setup": map[string]any{
		"model":            "models/" + opts.model,
		"generationConfig": map[string]any{"responseModalities": []string{"AUDIO"}},
	}}
	if err := liveHandshake(conn, setup, opts.timeout); err != nil {
		return err
	}
	logger.Info("browser subprotocol handshake accepted", zap.String("subprotocol", conn.Subprotocol()))
	return nil
}

// expectLiveSetupRejected asserts one invalid first frame is refused after the
// upgrade. Parameters: ctx, logger, wsBase, opts, model, frame and name describe
// the case. Returns: an error when the socket stays open or closes with an
// unexpected code. one-api answers a bad setup with a policy-violation close,
// never by forwarding it to Google.
func expectLiveSetupRejected(ctx context.Context, logger glog.Logger, wsBase string, opts liveOptions,
	model string, frame map[string]any, name string) error {
	conn, err := dialLive(ctx, appendQueryModel(wsBase, model), opts.apiToken, nil, opts.timeout)
	if err != nil {
		return errors.Wrapf(err, "%s: dial", name)
	}
	defer func() { _ = conn.Close() }()
	if err := writeLiveJSON(conn, frame); err != nil {
		return errors.Wrapf(err, "%s: send setup", name)
	}
	if err := conn.SetReadDeadline(time.Now().Add(opts.timeout)); err != nil {
		return errors.Wrapf(err, "%s: set read deadline", name)
	}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				if closeErr.Code != liveInvalidSetupCloseCode {
					return errors.Errorf("%s: closed with %d (%s), want %d",
						name, closeErr.Code, closeErr.Text, liveInvalidSetupCloseCode)
				}
				logger.Info("invalid setup rejected as expected",
					zap.String("case", name), zap.String("reason", closeErr.Text))
				return nil
			}
			return errors.Wrapf(err, "%s: read after setup", name)
		}
		if isLiveSetupComplete(msg) {
			return errors.Errorf("%s: proxy acknowledged an invalid setup", name)
		}
	}
}

// liveTurn summarizes one server turn without retaining audio payloads.
type liveTurn struct {
	audioChunks           int
	frames                int
	transcript            string
	turnComplete          bool
	interactionIdle       bool
	interactionInProgress bool
	usage                 map[string]any
	usageJSON             string
}

// readLiveTurn consumes frames until the turn completes. Parameters: conn is an
// acknowledged socket and timeout bounds the turn. Returns: the turn summary, or
// an error on a transport failure or a proxy-initiated close.
func readLiveTurn(conn *websocket.Conn, timeout time.Duration) (liveTurn, error) {
	var turn liveTurn
	deadline := time.Now().Add(timeout)
	for turn.frames < liveMaxFramesPerTurn {
		read := time.Now().Add(liveReadDeadline)
		if read.After(deadline) {
			read = deadline
		}
		if err := conn.SetReadDeadline(read); err != nil {
			return turn, errors.Wrap(err, "set read deadline")
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return turn, errors.Errorf("session closed mid-turn with %d (%s)", closeErr.Code, closeErr.Text)
			}
			return turn, errors.Wrap(err, "read live frame")
		}
		turn.frames++
		var event struct {
			Usage             map[string]any `json:"usageMetadata"`
			InteractionStatus string         `json:"interactionStatus"`
			ServerContent     *struct {
				ModelTurn *struct {
					Parts []struct {
						InlineData *struct {
							MimeType string `json:"mimeType"`
						} `json:"inlineData"`
					} `json:"parts"`
				} `json:"modelTurn"`
				OutputTranscription *struct {
					Text string `json:"text"`
				} `json:"outputTranscription"`
				TurnComplete      bool   `json:"turnComplete"`
				InteractionStatus string `json:"interactionStatus"`
			} `json:"serverContent"`
		}
		if err := json.Unmarshal(msg, &event); err != nil {
			return turn, errors.Wrap(err, "decode live frame")
		}
		if event.Usage != nil {
			turn.usage = event.Usage
			turn.usageJSON = truncateString(string(msg), 512)
		}
		if event.ServerContent != nil && event.ServerContent.ModelTurn != nil {
			for _, part := range event.ServerContent.ModelTurn.Parts {
				if part.InlineData != nil {
					turn.audioChunks++
				}
			}
		}
		if event.ServerContent != nil && event.ServerContent.OutputTranscription != nil {
			turn.transcript = appendLiveTranscript(turn.transcript, event.ServerContent.OutputTranscription.Text)
		}
		if event.ServerContent != nil && event.ServerContent.TurnComplete {
			turn.turnComplete = true
		}
		status := event.InteractionStatus
		if event.ServerContent != nil && event.ServerContent.InteractionStatus != "" {
			status = event.ServerContent.InteractionStatus
		}
		if status != "" {
			switch strings.ToUpper(strings.TrimSpace(status)) {
			case "IN_PROGRESS":
				turn.interactionInProgress = true
				turn.interactionIdle = false
			case "IDLE":
				if turn.interactionInProgress {
					turn.interactionIdle = true
				}
			}
		}
		if turn.turnComplete && turn.usage != nil && (!turn.interactionInProgress || turn.interactionIdle) {
			return turn, nil
		}
	}
	return turn, errors.New("turn exceeded the frame budget without a receipt")
}

// appendLiveTranscript retains a bounded, rune-safe diagnostic transcript.
// Parameters: transcript is the current preview and text is the new provider
// fragment. Returns: a preview no larger than maxLiveTranscriptBytes.
func appendLiveTranscript(transcript, text string) string {
	remaining := maxLiveTranscriptBytes - len(transcript)
	if remaining <= 0 || text == "" {
		return transcript
	}
	if len(text) <= remaining {
		return transcript + text
	}
	for _, r := range text {
		fragment := string(r)
		if len(fragment) > remaining {
			break
		}
		transcript += fragment
		remaining -= len(fragment)
	}
	return transcript
}

// assertLiveReceipt validates the provider receipt the proxy must be able to
// meter. Parameters: usage is the decoded usageMetadata. Returns: an error when
// a receipt cannot support settlement. Modality details are deliberately NOT
// required to sum to their parent: real receipts leave a remainder, and demanding
// closure is exactly what previously discarded every turn's billing.
func assertLiveReceipt(usage map[string]any) error {
	total, ok := liveReceiptCount(usage, "totalTokenCount")
	if !ok || total <= 0 {
		return errors.Errorf("receipt has no usable totalTokenCount: %v", usage)
	}
	prompt, _ := liveReceiptCount(usage, "promptTokenCount")
	response, _ := liveReceiptCount(usage, "responseTokenCount")
	thoughts, _ := liveReceiptCount(usage, "thoughtsTokenCount")
	if prompt <= 0 {
		return errors.Errorf("receipt has no promptTokenCount: %v", usage)
	}
	if total != prompt+response && total != prompt+response+thoughts {
		return errors.Errorf("receipt total %d reconciles with neither prompt+response nor prompt+response+thoughts: %v",
			total, usage)
	}
	return nil
}

// liveReceiptCount reads one numeric receipt field. Parameters: usage is the
// decoded receipt and key the field. Returns: the value and whether it existed.
func liveReceiptCount(usage map[string]any, key string) (int64, bool) {
	value, exists := usage[key]
	if !exists {
		return 0, false
	}
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int64(number), true
}

// liveHandshake sends the setup frame and waits for Google's acknowledgement.
// Parameters: conn, setup and timeout describe the handshake. Returns: an error
// when the proxy rejects the setup or forwards something other than setupComplete.
func liveHandshake(conn *websocket.Conn, setup map[string]any, timeout time.Duration) error {
	if err := writeLiveJSON(conn, setup); err != nil {
		return errors.Wrap(err, "send setup")
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return errors.Wrap(err, "set read deadline")
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) {
			return errors.Errorf("setup rejected with close %d (%s)", closeErr.Code, closeErr.Text)
		}
		return errors.Wrap(err, "read setup acknowledgement")
	}
	if !isLiveSetupComplete(msg) {
		return errors.Errorf("first server frame is not setupComplete: %s", truncateString(string(msg), 256))
	}
	return nil
}

// isLiveSetupComplete reports whether a server frame acknowledges the setup.
// Parameters: msg is one native frame. Returns: true for a setupComplete object.
func isLiveSetupComplete(msg []byte) bool {
	var frame map[string]json.RawMessage
	if err := json.Unmarshal(msg, &frame); err != nil {
		return false
	}
	raw, exists := frame["setupComplete"]
	return exists && string(raw) != "null"
}

// writeLiveJSON sends one native JSON frame. Parameters: conn is the socket and
// frame the operation. Returns: a wrapped transport error.
func writeLiveJSON(conn *websocket.Conn, frame map[string]any) error {
	payload, err := json.Marshal(frame)
	if err != nil {
		return errors.Wrap(err, "encode live frame")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return errors.Wrap(err, "set write deadline")
	}
	return errors.Wrap(conn.WriteMessage(websocket.TextMessage, payload), "write live frame")
}
