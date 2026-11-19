package adaptor

import (
	"fmt"
	"slices"
)

// JoinModelCatalogs assembles disjoint model families without overriding any ID.
// It is the single helper every provider uses to combine its per-family catalog
// maps into ModelRatios. It accepts independently constructed family maps (nil
// families are allowed and contribute nothing) and returns a fresh combined
// catalog that callers own. Duplicate IDs panic at initialization rather than
// silently changing prices; the panic message names the duplicated ID and the
// zero-based argument indexes of both families that define it. Families retain
// their value fields; this function performs no parsing, unit conversion, schema
// interpretation, or field-level merging, and never modifies its arguments.
func JoinModelCatalogs(families ...map[string]ModelConfig) map[string]ModelConfig {
	total := 0
	for _, family := range families {
		total += len(family)
	}
	result := make(map[string]ModelConfig, total)
	for index, family := range families {
		for id, config := range family {
			if _, exists := result[id]; exists {
				first := slices.IndexFunc(families[:index], func(earlier map[string]ModelConfig) bool {
					_, defined := earlier[id]
					return defined
				})
				panic(fmt.Sprintf("duplicate model ID in catalog: %s (families %d and %d)", id, first, index))
			}
			result[id] = config
		}
	}
	return result
}
