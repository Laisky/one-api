package model

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

// RerankRequest represents the canonical request payload for /v1/rerank operations.
type RerankRequest struct {
	Model           string   `json:"model,omitempty"`
	Query           string   `json:"query,omitempty"`
	Documents       []string `json:"documents,omitempty"`
	TopN            *int     `json:"top_n,omitempty"`
	MaxTokensPerDoc *int     `json:"max_tokens_per_doc,omitempty"`
	Priority        *int     `json:"priority,omitempty"`

	// NativeQuery and NativeDocuments preserve structured input for adaptors that
	// explicitly opt in. Query/Documents remain text-only accounting estimates;
	// image bytes are never treated as tokenizer input or sent as placeholders.
	NativeQuery     json.RawMessage   `json:"-"`
	NativeDocuments []json.RawMessage `json:"-"`

	// Legacy compatibility fields accepted by prior OpenAI-style DTOs.
	Input any `json:"input,omitempty"`
}

// Normalize ensures the rerank request has all required computed fields populated.
// It derives Query from legacy Input payloads when absent and trims surrounding whitespace.
func (r *RerankRequest) Normalize() error {
	if r == nil {
		return errors.New("nil rerank request")
	}

	query := strings.TrimSpace(r.Query)
	if query == "" && r.Input != nil {
		switch v := r.Input.(type) {
		case string:
			query = strings.TrimSpace(v)
		case json.Number:
			query = strings.TrimSpace(v.String())
		case []byte:
			query = strings.TrimSpace(string(v))
		default:
			if raw, err := json.Marshal(v); err == nil {
				query = strings.TrimSpace(string(raw))
			}
		}
	}
	if query == "" {
		return errors.New("field query is required")
	}

	r.Query = query
	return nil
}

// HasStructuredInput reports whether forwarding requires an opted-in adaptor.
func (r *RerankRequest) HasStructuredInput() bool {
	return r != nil && (len(r.NativeQuery) > 0 || len(r.NativeDocuments) > 0)
}

// Clone returns a deep copy of the rerank request to avoid mutating user-supplied slices.
func (r *RerankRequest) Clone() *RerankRequest {
	if r == nil {
		return nil
	}
	clone := *r
	if len(r.Documents) > 0 {
		clone.Documents = append([]string(nil), r.Documents...)
	}
	clone.NativeQuery = append(json.RawMessage(nil), r.NativeQuery...)
	if len(r.NativeDocuments) > 0 {
		clone.NativeDocuments = make([]json.RawMessage, len(r.NativeDocuments))
		for i, document := range r.NativeDocuments {
			clone.NativeDocuments[i] = append(json.RawMessage(nil), document...)
		}
	}
	return &clone
}
