package adaptor

// JoinModelCatalogs assembles disjoint model families without overriding any ID.
// It accepts independently constructed family maps and returns their combined
// catalog. Duplicate IDs panic at initialization rather than silently changing
// prices. Families retain their value fields; this function performs no parsing,
// unit conversion, schema interpretation, or field-level merging.
func JoinModelCatalogs(families ...map[string]ModelConfig) map[string]ModelConfig {
	result := make(map[string]ModelConfig)
	for _, family := range families {
		for id, config := range family {
			if _, exists := result[id]; exists {
				panic("duplicate model ID in catalog: " + id)
			}
			result[id] = config
		}
	}
	return result
}
