package doubao

// init repairs units in the existing Seed 1.6 compatibility tariff. It takes no
// arguments and returns nothing. The source table and constants.go describe
// [0,32K], (32K,128K], and (128K,256K] bands, but the old thresholds contained
// 32 and 128. The resolver accepts raw token counts and inclusive lower bounds.
// Preserve the historic rates: this is not a new September 2026 price quote.
// Reference: https://www.volcengine.com/docs/82379/1544106
func init() {
	for _, id := range []string{
		"doubao-seed-1.6", "doubao-seed-1.6-flash", "doubao-seed-1.6-vision", "doubao-seed-1.6-lite",
	} {
		cfg, ok := ModelRatios[id]
		if !ok || len(cfg.Tiers) != 2 {
			panic("doubao: missing Seed 1.6 pricing bands for " + id)
		}
		cfg = cfg.Clone()
		cfg.Tiers[0].InputTokenThreshold = 32001
		cfg.Tiers[1].InputTokenThreshold = 128001
		ModelRatios[id] = cfg
	}
}
