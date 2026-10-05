package gemini

const (
	// These are transport resource limits, not token or monetary estimates.
	liveSessionInputBytes  = 8 << 20
	liveSessionInputFrames = 32768
)

// liveInputBudget bounds aggregate client traffic, including setup, media,
// function responses and activity controls. One client reader owns it after
// setup; the handler may inspect it only after runLivePump joins both readers.
// It does not claim that bytes map to provider tokens or a prepaid quota ceiling.
type liveInputBudget struct {
	byteLimit  int64
	frameLimit int64
	bytes      int64
	frames     int64
	exhausted  bool
}

// reserve accepts an entire frame before any bytes can reach the provider.
// Parameters: size is the encoded frame length. Returns: false if either
// aggregate allowance would be exceeded. Failed writes never restore allowance
// because the provider may already have accepted a partial or complete frame.
func (b *liveInputBudget) reserve(size int) bool {
	if b == nil || size < 0 {
		return false
	}
	if b.exhausted || b.frames >= b.frameLimit || int64(size) > b.byteLimit-b.bytes {
		b.exhausted = true
		return false
	}
	b.frames++
	b.bytes += int64(size)
	return true
}
