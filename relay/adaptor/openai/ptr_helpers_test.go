package openai

// boolPtr returns a pointer to the provided bool value.
//
// b: The bool value to take the address of.
// Returns: A pointer to b.
func boolPtr(b bool) *bool {
	return &b
}

// floatPtr returns a pointer to the provided float64 value.
//
// f: The float64 value to take the address of.
// Returns: A pointer to f.
func floatPtr(f float64) *float64 {
	return &f
}

// stringPtr returns a pointer to the provided string value.
//
// s: The string value to take the address of.
// Returns: A pointer to s.
func stringPtr(s string) *string {
	return &s
}
