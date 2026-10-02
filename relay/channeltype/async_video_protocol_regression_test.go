package channeltype

import (
 "testing"
 "github.com/Laisky/one-api/relay/relaymode"
)

// Native capability must not claim legacy wire compatibility. The synchronous
// bridge is a separate, explicit routing decision, never a default alias.
func TestMuAPINativeVideoCapabilityIsIsolated(t *testing.T) {
 endpoints := DefaultEndpointsForChannelType(MuAPI)
 if IsEndpointSupported(relaymode.Videos, endpoints) {
  t.Fatal("MuAPI native submit/poll must not advertise legacy EndpointVideos")
 }
 if len(endpoints) == 0 {
  t.Fatal("MuAPI must retain an explicit native video capability")
 }
 if !IsEndpointSupported(relaymode.Videos, DefaultEndpointsForChannelType(OpenAI)) {
  t.Fatal("legacy OpenAI video capability changed")
 }
}
