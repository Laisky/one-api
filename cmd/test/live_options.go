package main

import (
	"flag"
	"io"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
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

// liveOptions captures CLI flags. Even setup validation needs a reachable
// Gemini channel because one-api dials Google before upgrading the socket.
type liveOptions struct {
	apiBase       string
	apiToken      string
	model         string
	thinkingModel string
	prompt        string
	timeout       time.Duration
	verifyBilling bool
	restChannel   int
	scenarios     []string
}

// liveScenarioNames returns the ordered supported suite. Parameters: none.
// Returns: the accepted scenario names for --scenarios.
func liveScenarioNames() []string {
	return []string{"conversation", "thinking", "setup-guard", "rest-guard", "subprotocol"}
}

// loadLiveConfig reads only the standalone probe settings. Parameters: none.
// Returns: base URL and optional token defaults without premature validation.
func loadLiveConfig() config {
	base := strings.TrimSpace(sharedconfig.APIBase)
	if base == "" {
		base = defaultAPIBase
	}
	return config{APIBase: strings.TrimSuffix(base, "/"), Token: strings.TrimSpace(sharedconfig.APIToken)}
}

// parseLiveArgs parses live flags. Parameters: args are CLI tokens and cfg
// supplies defaults. Returns: normalized options or a validation error.
func parseLiveArgs(args []string, cfg config) (liveOptions, error) {
	fs := flag.NewFlagSet("live", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts liveOptions
	var scenarios string
	fs.StringVar(&opts.apiBase, "api-base", strings.TrimSuffix(cfg.APIBase, "/"), "one-api base URL")
	fs.StringVar(&opts.apiToken, "token", strings.TrimSpace(cfg.Token), "API token; defaults to API_TOKEN")
	fs.StringVar(&opts.model, "model", defaultLiveModel, "Live model bound at the handshake")
	fs.StringVar(&opts.thinkingModel, "thinking-model", defaultLiveThinkingModel, "Extended Thinking model")
	fs.StringVar(&opts.prompt, "prompt", defaultLivePrompt, "text turn sent to the model")
	fs.DurationVar(&opts.timeout, "timeout", defaultLiveTimeout, "per-scenario timeout")
	fs.BoolVar(&opts.verifyBilling, "verify-billing", true, "verify persisted conversation token accounting")
	fs.IntVar(&opts.restChannel, "rest-channel", 0, "Google channel ID for rest-guard (requires an admin token)")
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
	if opts.restChannel < 0 {
		return liveOptions{}, errors.New("rest-channel must be a positive channel ID")
	}
	explicitScenarios := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "scenarios" {
			explicitScenarios = true
		}
	})
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
		if name == "rest-guard" && opts.restChannel == 0 {
			if explicitScenarios {
				return liveOptions{}, errors.New("rest-guard requires --rest-channel with a Google channel ID and an admin token")
			}
			continue
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
