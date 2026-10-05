package openai

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

// responseWSSessionOwnership retains a bounded set of provider-confirmed IDs for one authenticated socket.
type responseWSSessionOwnership struct {
	mu         sync.RWMutex
	ids        map[string]struct{}
	denied     atomic.Bool
	dispatched atomic.Bool
	// creates counts response.create frames forwarded upstream, so settlement can
	// tell when dispatched work never produced an authoritative terminal receipt.
	creates atomic.Int64
}

// observe records a provider-confirmed response ID before forwarding it to the client and returns no value.
func (s *responseWSSessionOwnership) observe(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ids) >= 1024 {
		return
	} // Older sessions can still use persisted owner bindings after reconnecting.
	if s.ids == nil {
		s.ids = make(map[string]struct{})
	}
	s.ids[id] = struct{}{}
}

// ErrResponseWSConversationUnsupported marks a response.create frame that names
// a conversation. Gateway conversations exist only in the owner-scoped state
// store and cannot be hydrated on the passthrough socket, and a provider
// conversation ID carries no owner binding, so no conversation selector is ever
// forwarded upstream. Clients continue a conversation over HTTP /v1/responses.
var ErrResponseWSConversationUnsupported = errors.New("conversation is not supported on the Responses WebSocket; send this turn over HTTP /v1/responses")

// responseWSConversationCloseReason is the client-visible close reason for
// ErrResponseWSConversationUnsupported; WebSocket close reasons are capped at
// 123 bytes, so it is a short form of the error text.
const responseWSConversationCloseReason = "conversation is not supported over websocket; use HTTP /v1/responses"

// responseWSAmbiguousKeyCloseReason is the client-visible close reason for a
// frame that names a guarded key more than once or in non-canonical case.
const responseWSAmbiguousKeyCloseReason = "ambiguous request parameter: duplicate or case-folded key"

// responseWSAuthorizationCloseReason maps an authorize error to a close reason.
// The parameter err is the authorization failure. It returns a specific reason
// for client-correctable rejections and the generic non-disclosing reason for
// every ownership failure.
func responseWSAuthorizationCloseReason(err error) string {
	switch {
	case errors.Is(err, ErrResponseWSConversationUnsupported):
		return responseWSConversationCloseReason
	case errors.Is(err, common.ErrAmbiguousJSONKey):
		return responseWSAmbiguousKeyCloseReason
	default:
		return "response authorization denied"
	}
}

// stripEmptyResponseWSConversation enforces the WebSocket conversation policy on
// a decoded response.create root. The parameter raw is modified in place. It
// removes an absent-in-effect selector (null, "", {}, or an empty id) and
// reports true when it did; it returns an error for a malformed selector, and
// ErrResponseWSConversationUnsupported for any selector that names an ID.
func stripEmptyResponseWSConversation(raw map[string]json.RawMessage) (bool, error) {
	value, ok := raw["conversation"]
	if !ok {
		return false, nil
	}
	var selector ResponseAPIConversation
	if err := json.Unmarshal(value, &selector); err != nil {
		return false, errors.Wrap(err, "invalid conversation selector")
	}
	if strings.TrimSpace(selector.ConversationID()) != "" {
		return false, errors.WithStack(ErrResponseWSConversationUnsupported)
	}
	delete(raw, "conversation")
	return true, nil
}

// authorize validates a client creation frame against meta and owner-scoped storage, returning the rewritten frame or a non-disclosing error.
func (s *responseWSSessionOwnership) authorize(ctx context.Context, meta *rmeta.Meta, frame []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(frame, &raw); err != nil {
		return nil, errors.Wrap(err, "invalid response websocket frame")
	}
	// The map keeps only the last exact key while a provider may read the first
	// duplicate or fold case. Reject ambiguity in every key this guard and the
	// model guard decide on, so the authorized value is the forwarded value.
	if err := validateWSFrameGuardedKeys(frame, responseWSGuardedKeys...); err != nil {
		return nil, err
	}
	var eventType string
	if err := json.Unmarshal(raw["type"], &eventType); err != nil {
		return nil, errors.Wrap(err, "invalid response websocket event type")
	}
	if eventType != "response.create" {
		return nil, errors.New("unsupported response websocket event")
	}
	// Scan the raw frame rather than the decoded map: map decoding keeps only the
	// last duplicate and exact-key lookups miss case-folded spellings (#483).
	if err := ValidateResponseBackgroundPayload(frame); err != nil {
		return nil, err
	}
	// Even an explicit false is removed so the provider never sees the flag.
	backgroundStripped := StripResponseBackgroundKeys(raw)
	conversationStripped, err := stripEmptyResponseWSConversation(raw)
	if err != nil {
		return nil, err
	}
	outbound := frame
	if backgroundStripped || conversationStripped {
		encoded, encodeErr := json.Marshal(raw)
		if encodeErr != nil {
			return nil, errors.Wrap(encodeErr, "encode response frame without stripped selectors")
		}
		outbound = encoded
	}
	previous, ok := raw["previous_response_id"]
	if !ok || string(previous) == "null" {
		return outbound, nil
	}
	var id string
	if err := json.Unmarshal(previous, &id); err != nil {
		return nil, errors.Wrap(err, "invalid previous response identifier")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return outbound, nil
	}
	s.mu.RLock()
	_, local := s.ids[id]
	s.mu.RUnlock()
	if local {
		return outbound, nil
	}
	if meta == nil || !state.Enabled() || state.Store() == nil {
		return nil, errors.New("response ownership storage is unavailable")
	}
	owner := state.OwnerScope{UserID: meta.UserId, TokenID: meta.TokenId}
	if !owner.Valid() {
		return nil, errors.New("previous response not found")
	}
	binding, err := state.Store().GetResponseBinding(ctx, owner, id)
	if err != nil {
		return nil, errors.Wrap(err, "previous response authorization failed")
	}
	if binding == nil || binding.ChannelID != meta.ChannelId || binding.APIType != meta.APIType || binding.UpstreamResponseID == "" {
		return nil, errors.New("previous response is unavailable on this channel")
	}
	encoded, err := json.Marshal(binding.UpstreamResponseID)
	if err != nil {
		return nil, errors.Wrap(err, "encode bound response identifier")
	}
	raw["previous_response_id"] = encoded
	rewritten, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.Wrap(err, "encode authorized response frame")
	}
	return rewritten, nil
}
