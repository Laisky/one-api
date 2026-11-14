package model

// NativePartReference identifies exactly one content part or tool call in an internally lowered message.
// These references never serialize as provider fields; native converters validate and consume them.
type NativePartReference struct {
	ContentIndex  *int
	ToolCallIndex *int
}
