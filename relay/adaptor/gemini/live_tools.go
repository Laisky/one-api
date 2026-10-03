package gemini

import (
	"encoding/json"
	"sync"

	"github.com/Laisky/errors/v2"
)

// liveToolState binds client function responses to this connection's server
// calls. It stores bounded identifiers, never function arguments or results.
type liveToolState struct {
	mu      sync.Mutex
	pending map[string]string
}

// observe records trusted server calls and cancellations. Parameters: raw is a
// validated server frame. Returns: an error for malformed or excessive calls.
func (s *liveToolState) observe(raw []byte) error {
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
		s.pending = make(map[string]string)
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
			s.pending[call.ID] = call.Name
		}
	}
	return nil
}

// accept validates a client function response against pending server calls.
// Parameters: raw is a validated client frame. Returns: whether it is a tool
// response and an error for unsolicited, duplicate or mismatched identities.
// A tool response continues server work already tracked by the collector; it
// does not invent a new pending user turn or an additional transcript charge.
func (s *liveToolState) accept(raw []byte) (bool, error) {
	var e struct {
		Response *struct {
			Responses []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"functionResponses"`
		} `json:"toolResponse"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return false, errors.Wrap(ErrLiveProtocol, "invalid client tool envelope")
	}
	if e.Response == nil {
		return false, nil
	}
	if len(e.Response.Responses) == 0 || len(e.Response.Responses) > 256 {
		return true, errors.Wrap(ErrLiveProtocol, "invalid function response count")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool, len(e.Response.Responses))
	for _, r := range e.Response.Responses {
		name, exists := s.pending[r.ID]
		if !exists || seen[r.ID] || (r.Name != "" && r.Name != name) {
			return true, errors.Wrap(ErrLiveProtocol, "unrequested Live function response")
		}
		seen[r.ID] = true
	}
	for id := range seen {
		delete(s.pending, id)
	}
	return true, nil
}
