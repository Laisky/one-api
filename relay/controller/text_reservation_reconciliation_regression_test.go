package controller

import "testing"

// TestSuccessfulTextRelayReconcilesOneReservation verifies exact accounting
// across HTTP and SDK providers, MCP and direct calls, and all receipt sizes.
// t supplies the test lifecycle and assertions; this test returns no value.
func TestSuccessfulTextRelayReconcilesOneReservation(t *testing.T) {
	checkSuccessfulRelayReconcilesOneReservation(t, false)
}
