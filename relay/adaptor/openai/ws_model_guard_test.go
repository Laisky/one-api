package openai

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

func TestEnforceResponseCreateModel_NonCreateFramePassThrough(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"response.output_text.delta","delta":"hi"}`)
	out, err := enforceResponseCreateModel(in, "gpt-4o-mini", "gpt-4o-mini")
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestEnforceResponseCreateModel_MissingModelFieldInjects(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"response.create","input":[]}`)
	out, err := enforceResponseCreateModel(in, "", "gpt-4o-mini")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-mini", parsed["model"])
}

func TestEnforceResponseCreateModel_EmptyStringModelInjects(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"response.create","model":"","input":[]}`)
	out, err := enforceResponseCreateModel(in, "", "gpt-4o-mini")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-mini", parsed["model"])
}

func TestEnforceResponseCreateModel_MatchingActualForwardsUnchanged(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)
	out, err := enforceResponseCreateModel(in, "gpt-4o-mini", "gpt-4o-mini")
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestEnforceResponseCreateModel_OriginNameRewrittenToActual(t *testing.T) {
	t.Parallel()

	// User-facing alias `gpt-mini-alias` maps to upstream `gpt-4o-mini` via channel model mapping.
	in := []byte(`{"type":"response.create","model":"gpt-mini-alias","input":[]}`)
	out, err := enforceResponseCreateModel(in, "gpt-mini-alias", "gpt-4o-mini")
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-mini", parsed["model"])
}

// CORE SECURITY TEST: a client cannot upgrade to a different (more expensive) model
// once the WS handshake has pinned a model.
func TestEnforceResponseCreateModel_MismatchedModelDenied(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"response.create","model":"gpt-5","input":[]}`)
	out, err := enforceResponseCreateModel(in, "gpt-4o-mini", "gpt-4o-mini")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrModelSwitchDenied))
	// The frame should not be forwarded on rejection; the caller is responsible
	// for emitting an error event and closing the connection.
	require.Equal(t, in, out)
}

func TestEnforceResponseCreateModel_NoEnforcementWhenActualEmpty(t *testing.T) {
	t.Parallel()

	// Backward compat: if the caller could not resolve a bound model (e.g. WS
	// handshake without `?model=`), the guard returns the frame unchanged so
	// downstream behavior is preserved while billing remains the legacy gap.
	in := []byte(`{"type":"response.create","model":"gpt-5","input":[]}`)
	out, err := enforceResponseCreateModel(in, "", "")
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestEnforceResponseCreateModel_MalformedJSONPassThrough(t *testing.T) {
	t.Parallel()

	in := []byte(`{not json`)
	out, err := enforceResponseCreateModel(in, "gpt-4o-mini", "gpt-4o-mini")
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestEnforceResponseCreateModel_EmptyFrame(t *testing.T) {
	t.Parallel()

	out, err := enforceResponseCreateModel(nil, "gpt-4o-mini", "gpt-4o-mini")
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestBuildModelSwitchErrorEventShape(t *testing.T) {
	t.Parallel()

	payload := buildModelSwitchErrorEvent("client model \"gpt-5\" does not match handshake-bound model \"gpt-4o-mini\"")

	var parsed wsErrorEvent
	require.NoError(t, json.Unmarshal(payload, &parsed))
	require.Equal(t, "error", parsed.Type)
	require.Equal(t, "invalid_request_error", parsed.Error.Type)
	require.Equal(t, "model_switch_denied", parsed.Error.Code)
	require.Contains(t, parsed.Error.Message, "gpt-5")
	require.Contains(t, parsed.Error.Message, "gpt-4o-mini")
}

func TestEnforceRealtimeSessionUpdate_NonSessionUpdatePassThrough(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"conversation.item.create","item":{"type":"message"}}`)
	out, err := enforceRealtimeSessionUpdate(in)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestEnforceRealtimeSessionUpdate_WithoutModelFieldAllowed(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"session.update","session":{"instructions":"be brief"}}`)
	out, err := enforceRealtimeSessionUpdate(in)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

// CORE SECURITY TEST: realtime session.update cannot change model field.
func TestEnforceRealtimeSessionUpdate_WithModelFieldDenied(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"session.update","session":{"model":"gpt-4o-realtime-preview","instructions":"x"}}`)
	out, err := enforceRealtimeSessionUpdate(in)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrModelSwitchDenied))
	require.Equal(t, in, out)
}

func TestEnforceRealtimeSessionUpdate_MalformedJSONPassThrough(t *testing.T) {
	t.Parallel()

	in := []byte(`{not json`)
	out, err := enforceRealtimeSessionUpdate(in)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestEnforceRealtimeSessionUpdate_NoSessionFieldPassThrough(t *testing.T) {
	t.Parallel()

	in := []byte(`{"type":"session.update"}`)
	out, err := enforceRealtimeSessionUpdate(in)
	require.NoError(t, err)
	require.Equal(t, in, out)
}
