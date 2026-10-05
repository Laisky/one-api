package openai

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
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

// authorize validates a client creation frame against meta and owner-scoped storage, returning the rewritten frame or a non-disclosing error.
func (s *responseWSSessionOwnership) authorize(ctx context.Context, meta *rmeta.Meta, frame []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(frame, &raw); err != nil {
		return nil, errors.Wrap(err, "invalid response websocket frame")
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
	outbound := frame
	if StripResponseBackgroundKeys(raw) {
		// Even an explicit false is removed so the provider never sees the flag.
		stripped, err := json.Marshal(raw)
		if err != nil {
			return nil, errors.Wrap(err, "encode response frame without background flag")
		}
		outbound = stripped
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
