package model

// PreserveNativeConstraintsFor records the destination channel for an internally
// translated schema. It changes no schema data and has no wire representation.
func (s *JSONSchema) PreserveNativeConstraintsFor(channelType int) {
	if s != nil {
		s.nativeSchemaChannel = channelType
	}
}

// PreservesNativeConstraintsFor reports whether this schema carries trusted
// lossless-conversion provenance for channelType. Nil schemas return false.
func (s *JSONSchema) PreservesNativeConstraintsFor(channelType int) bool {
	return s != nil && channelType != 0 && s.nativeSchemaChannel == channelType
}
