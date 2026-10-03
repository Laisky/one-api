package main

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/websocket"
)

// runLiveConversationScenario observes two native turns and their settlement.
// Parameters: ctx, logger, wsBase and opts describe the run. Returns: an error
// when setup, streaming, receipt validation or persisted accounting fails.
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
		zap.Int("audio_chunks", turn.audioChunks), zap.Int("frames", turn.frames),
		zap.String("transcript", shorten(turn.transcript, 120)), zap.String("usage", turn.usageJSON))
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
	// A metering rejection closes the socket. A second turn verifies usability.
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
	logger.Info("second live receipt observed", zap.String("usage", second.usageJSON))
	_ = conn.Close()
	if !opts.verifyBilling {
		return nil
	}
	if requestID == "" {
		return errors.New("live websocket handshake did not include X-Oneapi-Request-Id; cannot verify settlement")
	}
	return verifyLiveSettlement(ctx, logger, opts, requestID, []map[string]any{turn.usage, second.usage})
}

// runLiveThinkingScenario exercises configurable thinking and ordinary-model
// rejection. Parameters: ctx, logger, wsBase and opts describe the run.
// Returns: an error on a lifecycle, receipt or setup-validation mismatch.
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
	logger.Info("extended-thinking receipt observed",
		zap.String("usage", turn.usageJSON), zap.Int("audio_chunks", turn.audioChunks))
	// A receipt is not proof that asynchronous settlement has completed.
	_ = conn.Close()
	rejected := map[string]any{"setup": map[string]any{
		"model": "models/" + opts.model,
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"thinkingConfig":     map[string]any{"thinkingLevel": "LOW"},
		},
	}}
	return expectLiveSetupRejected(ctx, logger, wsBase, opts, opts.model, rejected, "configurable thinking on the automatic-thinking model")
}

// runLiveSetupGuardScenario checks model binding and setup validation.
// Parameters: ctx, logger, wsBase and opts describe the run. Returns: an error
// when an invalid first frame is accepted instead of rejected locally.
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
// logger, wsBase and opts describe the run. Returns: an error unless the server
// selects the non-secret protocol and acknowledges a valid setup frame.
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

// expectLiveSetupRejected asserts that an invalid first frame is refused.
// Parameters: ctx, logger, wsBase, opts, model, frame and name describe the case.
// Returns: an error unless the server rejects setup with the expected close code.
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
