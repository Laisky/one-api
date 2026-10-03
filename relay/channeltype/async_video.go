package channeltype

// NativeAsyncVideoEndpoint returns the explicit native capability registered
// for a durable provider. Adding another provider changes this registry and its
// adapter, not controller billing, worker orchestration or public response DTOs.
func NativeAsyncVideoEndpoint(channelType int) (Endpoint, bool) {
	switch channelType {
	case MuAPI:
		return MuAsyncEndpointVideos, true
	default:
		return 0, false
	}
}
