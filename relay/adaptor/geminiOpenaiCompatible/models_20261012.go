package geminiOpenaiCompatible

// init updates provider lifecycle descriptions from
// Google's October 6 and October 8 release notes. It accepts no parameters and
// returns no values. Historical identifiers and pricing remain available for
// existing channel configurations; this function does not redirect requests.
// Source verified 2026-10-12: https://ai.google.dev/gemini-api/docs/changelog
func init() {
	for id, description := range map[string]string{
		"gemini-3.7-flash":       "Deprecated in the Gemini Developer API on 2026-10-08. Google automatically routes requests for this ID to gemini-3.8-flash. Use the replacement ID and its current pricing explicitly; retaining this legacy configuration does not promise a separately served model.",
		"gemini-3.5-flash":       "Deprecated in the Gemini Developer API on 2026-10-08. Google automatically routes requests for this ID to gemini-3.6-flash. Use the replacement ID and its current pricing explicitly; this entry retains historical pricing for existing configurations, not a verified current tariff for redirected requests.",
		"gemini-3.1-flash-image": "Deprecated in the Gemini Developer API on 2026-10-06; no shutdown date announced. Google recommends gemini-nano-banana-2.1. That replacement requires separate gateway image-routing and billing qualification; this legacy ID is not silently remapped and retains its original tariff.",
	} {
		cfg, exists := ModelRatios[id]
		if !exists {
			panic("gemini: missing legacy model for lifecycle update " + id)
		}
		cfg.Description = description
		ModelRatios[id] = cfg
	}
}
