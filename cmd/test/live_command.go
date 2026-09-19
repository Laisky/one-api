package main

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// live runs the Gemini Live end-to-end probes. Parameters: ctx bounds the run,
// logger reports progress and args are the CLI tokens after live. Returns: an
// error when argument validation or any selected scenario fails.
func live(ctx context.Context, logger glog.Logger, args []string) error {
	opts, err := parseLiveArgs(args, loadLiveConfig())
	if err != nil {
		return errors.Wrap(err, "parse live arguments")
	}
	return runLiveProbe(ctx, logger, opts)
}

// runLiveProbe executes each selected scenario and aggregates failures.
// Parameters: ctx, logger and opts describe the run. Returns: one aggregate
// error for the caller to log, without logging the same failures twice.
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
		runner, ok := runners[name]
		if !ok {
			return errors.Errorf("unknown scenario %q", name)
		}
		if err := runner(ctx, logger, wsBase, opts); err != nil {
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
