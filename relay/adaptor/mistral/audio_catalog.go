package mistral

import "github.com/Laisky/one-api/relay/adaptor"

// init registers the published Voxtral speech and transcription aliases without
// removing historical administrator-configured slugs. These aliases share exact
// tariff metadata, not independent estimates.
func init() {
	ModelRatios["voxtral-mini-2602"] = ModelRatios["voxtral-mini-transcribe-2602"].Clone()
	for _, name := range []string{"voxtral-mini-tts-2603", "voxtral-mini-tts-latest"} {
		cfg := ModelRatios["voxtral-tts-2603"].Clone()
		ModelRatios[name] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
