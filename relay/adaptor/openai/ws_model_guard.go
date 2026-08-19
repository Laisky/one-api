package openai

import (
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
)

// ErrModelSwitchDenied is returned when a client WebSocket frame attempts to
// use a model that does not match the model bound at WS handshake. Surfacing
// it as a sentinel lets the proxy distinguish billing-relevant rejections
// from generic I/O errors.
var ErrModelSwitchDenied = errors.New("ws_model_switch_denied")

// wsErrorEvent is the JSON shape both OpenAI Realtime and Response WS APIs use
// for server-emitted error events. The proxy emits the same shape so that
// existing client SDKs (OpenAI Realtime / Responses) can parse it normally.
type wsErrorEvent struct {
	Type  string           `json:"type"`
	Error wsErrorEventBody `json:"error"`
}

type wsErrorEventBody struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// buildModelSwitchErrorEvent returns a JSON `error` event payload describing a
// rejected model-switch attempt. The shape matches OpenAI's WS error event so
// downstream clients can surface it without custom parsing.
func buildModelSwitchErrorEvent(message string) []byte {
	body, err := json.Marshal(wsErrorEvent{
		Type: "error",
		Error: wsErrorEventBody{
			Type:    "invalid_request_error",
			Code:    "model_switch_denied",
			Message: message,
		},
	})
	if err != nil {
		// Should never happen for a static struct; fall back to a minimal payload.
		return []byte(`{"type":"error","error":{"type":"invalid_request_error","code":"model_switch_denied","message":"model_switch_denied"}}`)
	}
	return body
}

// enforceResponseCreateModel inspects one client-to-upstream WebSocket frame
// for the `/v1/responses` API. When the frame is a `response.create` event,
// the function enforces strict model pinning against the handshake-bound
// `boundOriginModel` (user-facing name) and `boundActualModel` (upstream name
// after model mapping).
//
// Rules:
//   - non-`response.create` frames are returned unchanged
//   - frames with no `model` field have `boundActualModel` injected
//   - frames whose `model` equals `boundActualModel` are forwarded as-is
//   - frames whose `model` equals `boundOriginModel` (when origin differs from
//     actual due to model mapping) are rewritten to `boundActualModel`
//   - frames whose `model` differs from both bound names return ErrModelSwitchDenied
//     so the caller can emit an error event and close the connection
//
// Parameters:
//   - frame: raw text frame bytes received from the client.
//   - boundOriginModel: user-facing model name pinned at WS handshake; may be empty.
//   - boundActualModel: upstream model name pinned at WS handshake; must be non-empty
//     when callers enable enforcement.
//
// Returns:
//   - rewritten frame bytes to forward to upstream (may equal the input).
//   - error: ErrModelSwitchDenied (wrapped) on model mismatch; nil otherwise.
func enforceResponseCreateModel(frame []byte, boundOriginModel, boundActualModel string) ([]byte, error) {
	if len(frame) == 0 || boundActualModel == "" {
		return frame, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(frame, &raw); err != nil {
		// Non-JSON or malformed payload; forward unchanged. OpenAI will reject
		// malformed frames on its side and a malformed frame cannot itself
		// switch the model.
		return frame, nil
	}

	eventType, _ := raw["type"].(string)
	if eventType != "response.create" {
		return frame, nil
	}

	rawModel, exists := raw["model"]
	if !exists {
		raw["model"] = boundActualModel
		out, err := json.Marshal(raw)
		if err != nil {
			return nil, errors.Wrap(err, "marshal response.create with injected model")
		}
		return out, nil
	}

	clientModel, _ := rawModel.(string)
	switch {
	case clientModel == "":
		raw["model"] = boundActualModel
	case clientModel == boundActualModel:
		return frame, nil
	case boundOriginModel != "" && clientModel == boundOriginModel:
		raw["model"] = boundActualModel
	default:
		return frame, errors.Wrapf(ErrModelSwitchDenied,
			"client model %q does not match handshake-bound model %q",
			clientModel, boundActualModel)
	}

	out, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.Wrap(err, "marshal response.create with rewritten model")
	}
	return out, nil
}

// enforceRealtimeSessionsBodyModel validates and normalizes the JSON body of a
// `POST /v1/realtime/sessions` request before it is forwarded upstream. The
// body controls which model OpenAI mints the ephemeral token for, and that
// token is used over WebRTC where the proxy never sees the realtime traffic —
// so an unvalidated body lets a cheap channel mint a token for an expensive
// model and have the channel's upstream API key charged for it.
//
// Rules mirror enforceResponseCreateModel:
//   - empty/missing model field -> inject meta.ActualModelName
//   - model == meta.ActualModelName -> forward as-is
//   - model == meta.OriginModelName (alias) -> rewrite to meta.ActualModelName
//   - any other value -> return a 400 error and do NOT forward upstream
//
// When meta is nil or meta.ActualModelName is empty (legacy code path where
// the proxy could not resolve a bound model) the function is a no-op and
// returns the body unchanged.
//
// Parameters:
//   - body: raw request bytes from the client. May be empty.
//   - meta: relay metadata; OriginModelName/ActualModelName drive the policy.
//
// Returns:
//   - the possibly-rewritten body bytes to send upstream.
//   - a business error (with 400 status) when the policy rejects the request;
//     nil on success or when enforcement is skipped.
func enforceRealtimeSessionsBodyModel(body []byte, meta *rmeta.Meta) ([]byte, *rmodel.ErrorWithStatusCode) {
	if meta == nil || meta.ActualModelName == "" {
		return body, nil
	}
	if len(body) == 0 {
		// OpenAI rejects empty bodies anyway, but we still inject the bound
		// model so the user gets the channel's model rather than a generic
		// upstream failure.
		out, err := json.Marshal(map[string]any{"model": meta.ActualModelName})
		if err != nil {
			return nil, &rmodel.ErrorWithStatusCode{
				Error: rmodel.Error{
					Message:  "marshal realtime sessions body failed",
					Type:     rmodel.ErrorTypeInternal,
					Code:     "marshal_failed",
					RawError: errors.Wrap(err, "marshal realtime sessions body"),
				},
				StatusCode: http.StatusInternalServerError,
			}
		}
		return out, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &rmodel.ErrorWithStatusCode{
			Error: rmodel.Error{
				Message:  "invalid realtime sessions body: not JSON",
				Type:     rmodel.ErrorTypeOneAPI,
				Code:     "invalid_request_body",
				RawError: errors.Wrap(err, "unmarshal realtime sessions body"),
			},
			StatusCode: http.StatusBadRequest,
		}
	}

	rawModel, exists := raw["model"]
	rewriteOrInject := func(target string) ([]byte, *rmodel.ErrorWithStatusCode) {
		raw["model"] = target
		out, err := json.Marshal(raw)
		if err != nil {
			return nil, &rmodel.ErrorWithStatusCode{
				Error: rmodel.Error{
					Message:  "marshal realtime sessions body failed",
					Type:     rmodel.ErrorTypeInternal,
					Code:     "marshal_failed",
					RawError: errors.Wrap(err, "marshal realtime sessions body"),
				},
				StatusCode: http.StatusInternalServerError,
			}
		}
		return out, nil
	}

	if !exists {
		return rewriteOrInject(meta.ActualModelName)
	}

	clientModel, _ := rawModel.(string)
	switch {
	case clientModel == "":
		return rewriteOrInject(meta.ActualModelName)
	case clientModel == meta.ActualModelName:
		return body, nil
	case meta.OriginModelName != "" && clientModel == meta.OriginModelName:
		return rewriteOrInject(meta.ActualModelName)
	default:
		denyMessage := errors.Wrapf(ErrModelSwitchDenied,
			"realtime sessions body model %q does not match channel-bound model %q",
			clientModel, meta.ActualModelName).Error()
		return nil, &rmodel.ErrorWithStatusCode{
			Error: rmodel.Error{
				Message:  denyMessage,
				Type:     rmodel.ErrorTypeOneAPI,
				Code:     "model_switch_denied",
				RawError: ErrModelSwitchDenied,
			},
			StatusCode: http.StatusBadRequest,
		}
	}
}

// enforceRealtimeSessionUpdate inspects one client-to-upstream WebSocket frame
// for the `/v1/realtime` API. When the frame is a `session.update` event that
// attempts to change the `model` field of the session, it returns
// ErrModelSwitchDenied. OpenAI itself rejects model changes on Realtime
// sessions, but defense-in-depth keeps the proxy authoritative.
//
// Parameters:
//   - frame: raw text frame bytes received from the client.
//
// Returns:
//   - the original frame (the function never rewrites Realtime frames).
//   - error: ErrModelSwitchDenied (wrapped) if `session.update.session.model`
//     is present; nil otherwise.
func enforceRealtimeSessionUpdate(frame []byte) ([]byte, error) {
	if len(frame) == 0 {
		return frame, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(frame, &raw); err != nil {
		return frame, nil
	}

	if raw["type"] != "session.update" {
		return frame, nil
	}

	session, ok := raw["session"].(map[string]any)
	if !ok {
		return frame, nil
	}

	if _, hasModel := session["model"]; hasModel {
		return frame, errors.Wrap(ErrModelSwitchDenied,
			"realtime session.update cannot change session model")
	}

	return frame, nil
}
