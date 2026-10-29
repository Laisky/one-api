package model

// ClaudeImageReceipt tracks complete image text blocks accepted by a downstream writer.
type ClaudeImageReceipt struct {
	ImageEndOffsets      []int
	CandidateImageTokens int
	Incomplete           bool
}
