package openai

import (
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
)

// openAIOctoberLifecycle records exact published IDs, without prefix matching
// or extrapolating a snapshot's date to every alias in its family. Dates describe
// OpenAI's API, not Azure or another provider's independently operated endpoint.
// Reviewed 2026-10-13 America/Toronto (2026-10-14 UTC):
// https://developers.openai.com/api/docs/deprecations
var openAIOctoberLifecycle = []struct {
	models      string
	shutdown    string
	replacement string
}{
	// The October 14 notice was visible during the October 13 Toronto audit.
	{"gpt-4o-2024-08-06 gpt-4o-2024-11-20 gpt-4o-mini-2024-07-18 gpt-4.1-2025-04-14 gpt-4.1-mini-2025-04-14", "2027-01-14", "gpt-6-luna"},
	{"gpt-5.3-codex gpt-5.1", "2027-04-01", "gpt-6-sol"},
	{"gpt-5.4-nano", "2027-04-01", "gpt-6-luna"},
	{"tts-1 tts-1-hd gpt-4o-mini-tts-2025-12-15", "2027-01-06", "gpt-realtime-2.1-mini (Realtime API; not a drop-in audio/speech replacement)"},
	{"whisper-1 gpt-4o-transcribe gpt-4o-mini-transcribe", "2027-02-26", "gpt-transcribe or gpt-live-transcribe; choose the appropriate transcription API"},
	{"gpt-realtime", "2027-01-20", "gpt-realtime-2.1"},
	{"gpt-audio gpt-audio-mini", "2027-01-20", "gpt-audio-1.5"},
	{"gpt-realtime-mini", "2027-01-20", "gpt-realtime-2.1-mini"},
	{"gpt-4o-mini-transcribe-2025-03-20", "2027-01-20", "gpt-4o-mini-transcribe-2025-12-15"},
	{"gpt-5-2025-08-07 o3-2025-04-16", "2026-12-11", "gpt-5.6-sol"},
	{"gpt-5-mini-2025-08-07", "2026-12-11", "gpt-5.6-terra"},
	{"gpt-5-nano-2025-08-07", "2026-12-11", "gpt-5.6-luna"},
	{"gpt-5-pro-2025-10-06 o3-pro-2025-06-10", "2026-12-11", "gpt-5.6-sol with reasoning.mode=pro (not a separate native model ID)"},
	{"gpt-image-1-mini gpt-image-1.5 chatgpt-image-latest", "2026-12-01", "gpt-image-2.5-sunburst or gpt-image-2.5-flare"},
	{"gpt-3.5-turbo-0125 gpt-3.5-turbo", "2026-10-23", "gpt-5.6-terra"},
	{"gpt-4-0613 gpt-4 gpt-4-turbo gpt-4-turbo-2024-04-09 gpt-4o-2024-05-13 o1 o1-2024-12-17 o3-mini o3-mini-2025-01-31", "2026-10-23", "gpt-5.6-sol"},
	{"gpt-4.1-nano gpt-4.1-nano-2025-04-14", "2026-10-23", "gpt-5.6-luna"},
	{"gpt-image-1", "2026-10-23", "gpt-image-2.5-sunburst or gpt-image-2.5-flare"},
	{"o1-pro o1-pro-2025-03-19", "2026-10-23", "gpt-5.6-sol with reasoning.mode=pro (not a separate native model ID)"},
	{"o4-mini o4-mini-2025-04-16", "2026-10-23", "gpt-5.6-terra"},
	{"gpt-5.3-chat-latest", "2026-08-10", "gpt-5.6-sol"},
	{"gpt-5-chat-latest gpt-5-codex gpt-5.1-chat-latest gpt-5.1-codex gpt-5.1-codex-max gpt-5.2-codex o3-deep-research o3-deep-research-2025-06-26 o4-mini-deep-research o4-mini-deep-research-2025-06-26", "2026-07-23", "gpt-5.6-sol"},
	{"gpt-5.1-codex-mini computer-use-preview computer-use-preview-2025-03-11 gpt-4o-mini-search-preview-2025-03-11 gpt-4o-search-preview-2025-03-11", "2026-07-23", "gpt-5.6-terra"},
	{"gpt-audio-mini-2025-10-06", "2026-07-23", "gpt-audio-1.5"},
	{"sora-2 sora-2-pro", "2026-09-24", "none announced; the Videos API was also shut down"},
	{"gpt-3.5-turbo-instruct babbage-002 davinci-002 gpt-3.5-turbo-1106", "2026-09-28", "gpt-5.6-terra"},
	{"gpt-4-0314 gpt-4-0125-preview gpt-4-turbo-preview", "2026-03-26", "gpt-5 or gpt-4.1; verify the replacement's current lifecycle"},
	{"dall-e-2 dall-e-3", "2026-05-12", "gpt-image-2, gpt-image-1, or gpt-image-1-mini; verify the replacement's current lifecycle"},
}

// applyOpenAIOctoberLifecycle updates descriptions in the supplied catalog and
// returns nothing. It preserves pricing, format warnings and model membership;
// repeated calls replace this audit's annotation rather than duplicating it.
func applyOpenAIOctoberLifecycle(models map[string]adaptor.ModelConfig) {
	const marker = " [OpenAI lifecycle: "
	for _, notice := range openAIOctoberLifecycle {
		for _, name := range strings.Fields(notice.models) {
			cfg, ok := models[name]
			if !ok {
				continue
			}
			description, _, _ := strings.Cut(cfg.Description, marker)
			// Replace older catalog guidance that recommended a superseded model.
			description, _, _ = strings.Cut(description, " (API retirement ")
			cfg.Description = description + marker + "announced shutdown date " + notice.shutdown +
				"; recommended replacement: " + notice.replacement +
				". Retained catalog pricing does not guarantee upstream availability.]"
			models[name] = cfg
		}
	}
}
