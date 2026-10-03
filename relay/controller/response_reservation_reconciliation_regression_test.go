package controller

import "testing"

// TestSuccessfulResponseRelayReconcilesOneReservation verifies exact accounting
// across HTTP and SDK providers, MCP and direct calls, and all receipt sizes.
// t supplies the test lifecycle and assertions; this test returns no value.
func TestSuccessfulResponseRelayReconcilesOneReservation(t *testing.T) {
	checkSuccessfulRelayReconcilesOneReservation(t, true)
}
