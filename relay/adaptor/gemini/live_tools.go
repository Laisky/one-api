package gemini

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
)

// liveCall is one pending server function call: its declared name and the
// sequence of the turn that issued it (see liveToolCoverage.turn).
type liveCall struct {
	name string
	turn int64
}

// liveToolState binds client function responses to this connection's server
// calls. It stores bounded identifiers, never function arguments or results.
type liveToolState struct {
	mu      sync.Mutex
	pending map[string]liveCall
}

// observe records trusted server calls and cancellations. Parameters: raw is a
// validated server frame and turn the sequence of the turn the frame belongs
// to. Returns: an error for malformed or excessive calls.
func (s *liveToolState) observe(raw []byte, turn int64) error {
	var e struct {
		Call *struct {
			Calls []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"functionCalls"`
		} `json:"toolCall"`
		Cancel *struct {
			IDs []string `json:"ids"`
		} `json:"toolCallCancellation"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return errors.Wrap(ErrLiveProtocol, "invalid server tool envelope")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[string]liveCall)
	}
	if e.Cancel != nil {
		for _, id := range e.Cancel.IDs {
			delete(s.pending, id)
		}
	}
	if e.Call != nil {
		if len(e.Call.Calls) > 256 {
			return errors.Wrap(ErrLiveProtocol, "too many Live function calls")
		}
		for _, call := range e.Call.Calls {
			if call.ID == "" || len(call.ID) > 1024 || call.Name == "" || len(call.Name) > 1024 {
				return errors.Wrap(ErrLiveProtocol, "invalid Live function identity")
			}
			if len(s.pending) >= 256 {
				if _, exists := s.pending[call.ID]; !exists {
					return errors.Wrap(ErrLiveProtocol, "Live function capacity exceeded")
				}
			}
			s.pending[call.ID] = liveCall{name: call.Name, turn: turn}
		}
	}
	return nil
}

// accept validates a client function response against pending server calls.
// Parameters: raw is a validated client frame. Returns: the answered calls, nil
// when the frame is not a tool response, and an error for unsolicited,
// duplicate or mismatched identities. A tool response continues server work
// already tracked by the collector; it does not invent a new pending user turn
// or an additional transcript charge, but it can still need its own receipt.
func (s *liveToolState) accept(raw []byte) ([]liveCall, error) {
	// Decode with exact keys, as the provider does: encoding/json would also
	// match "ID" or "Name" and keep the last spelling, binding the response to
	// a different call than the one the provider resumes.
	var e struct {
		Response *struct {
			Responses []map[string]json.RawMessage `json:"functionResponses"`
		} `json:"toolResponse"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, errors.Wrap(ErrLiveProtocol, "invalid client tool envelope")
	}
	if e.Response == nil {
		return nil, nil
	}
	if len(e.Response.Responses) == 0 || len(e.Response.Responses) > 256 {
		return nil, errors.Wrap(ErrLiveProtocol, "invalid function response count")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool, len(e.Response.Responses))
	calls := make([]liveCall, 0, len(e.Response.Responses))
	for _, r := range e.Response.Responses {
		id, name, err := liveResponseIdentity(r)
		if err != nil {
			return nil, err
		}
		call, exists := s.pending[id]
		if !exists || seen[id] || (name != "" && name != call.name) {
			return nil, errors.Wrap(ErrLiveProtocol, "unrequested Live function response")
		}
		seen[id] = true
		calls = append(calls, call)
	}
	for id := range seen {
		delete(s.pending, id)
	}
	return calls, nil
}

// liveResponseIdentity reads one function response's exact "id" and optional
// "name". Parameters: response is the response object with exact keys.
// Returns: the identity, or an error when a value is not a string or another
// key spells "id" or "name" in a different letter case.
func liveResponseIdentity(response map[string]json.RawMessage) (string, string, error) {
	for key := range response {
		if (strings.EqualFold(key, "id") && key != "id") || (strings.EqualFold(key, "name") && key != "name") {
			return "", "", errors.Wrap(ErrLiveProtocol, "ambiguous Live function response identity")
		}
	}
	var id, name string
	if err := json.Unmarshal(response["id"], &id); err != nil {
		return "", "", errors.Wrap(ErrLiveProtocol, "invalid Live function response id")
	}
	if raw, ok := response["name"]; ok {
		if err := json.Unmarshal(raw, &name); err != nil {
			return "", "", errors.Wrap(ErrLiveProtocol, "invalid Live function response name")
		}
	}
	return id, name, nil
}
