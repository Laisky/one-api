package openai

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
)

func TestEnforceRealtimeSessionsBodyModel_NilMetaPassThrough(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"anything"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, nil)
	require.Nil(t, bizErr)
	require.Equal(t, body, out)
}

func TestEnforceRealtimeSessionsBodyModel_EmptyActualSkipsEnforcement(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{ActualModelName: ""}
	body := []byte(`{"model":"gpt-5"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.Nil(t, bizErr)
	require.Equal(t, body, out)
}

func TestEnforceRealtimeSessionsBodyModel_EmptyBodyInjects(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{ActualModelName: "gpt-4o-realtime-preview"}
	out, bizErr := enforceRealtimeSessionsBodyModel(nil, meta)
	require.Nil(t, bizErr)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-realtime-preview", parsed["model"])
}

func TestEnforceRealtimeSessionsBodyModel_MissingModelFieldInjects(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{ActualModelName: "gpt-4o-realtime-preview"}
	body := []byte(`{"voice":"verse"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.Nil(t, bizErr)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-realtime-preview", parsed["model"])
	require.Equal(t, "verse", parsed["voice"], "other fields must be preserved")
}

func TestEnforceRealtimeSessionsBodyModel_MatchingActualKeepsBody(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{
		OriginModelName: "gpt-4o-realtime-preview",
		ActualModelName: "gpt-4o-realtime-preview",
	}
	body := []byte(`{"model":"gpt-4o-realtime-preview","voice":"verse"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.Nil(t, bizErr)
	require.Equal(t, body, out, "matching model must forward as-is")
}

func TestEnforceRealtimeSessionsBodyModel_OriginAliasRewritten(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{
		OriginModelName: "my-alias",
		ActualModelName: "gpt-4o-realtime-preview",
	}
	body := []byte(`{"model":"my-alias","voice":"verse"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.Nil(t, bizErr)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-realtime-preview", parsed["model"])
}

func TestEnforceRealtimeSessionsBodyModel_MismatchedModelDenied(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{
		OriginModelName: "gpt-4o-realtime-preview",
		ActualModelName: "gpt-4o-realtime-preview",
	}
	body := []byte(`{"model":"gpt-realtime-2"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
	require.Equal(t, "model_switch_denied", bizErr.Error.Code)
	require.True(t, errors.Is(bizErr.Error.RawError, ErrModelSwitchDenied))
	require.Nil(t, out)
}

func TestEnforceRealtimeSessionsBodyModel_EmptyStringModelInjects(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{ActualModelName: "gpt-4o-realtime-preview"}
	body := []byte(`{"model":"","voice":"verse"}`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.Nil(t, bizErr)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "gpt-4o-realtime-preview", parsed["model"])
}

func TestEnforceRealtimeSessionsBodyModel_MalformedJSONRejected(t *testing.T) {
	t.Parallel()

	meta := &rmeta.Meta{ActualModelName: "gpt-4o-realtime-preview"}
	body := []byte(`{not json`)
	out, bizErr := enforceRealtimeSessionsBodyModel(body, meta)
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
	require.Equal(t, "invalid_request_body", bizErr.Error.Code)
	require.Nil(t, out)
}

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
