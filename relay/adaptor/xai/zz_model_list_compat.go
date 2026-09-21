package xai

import "github.com/Laisky/one-api/relay/adaptor"

// init runs after catalog_refresh_20260921.go in the standard Go file order.
// Rebuild the exported compatibility list after that file mutates ModelRatios.
func init() {
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
