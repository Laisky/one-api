package state

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// CurrentSchemaVersion is the schema version stamped on every record written by
// this build. Readers reject a record whose version they cannot decode (S07).
const CurrentSchemaVersion = 1

// DefaultResponseTTL is the default lifetime of a stored response node (R6).
// Conversations do NOT inherit this TTL.
const DefaultResponseTTL = 30 * 24 * time.Hour

// Response status values recorded on a node.
const (
	StatusCompleted  = "completed"
	StatusIncomplete = "incomplete"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
	StatusInProgress = "in_progress"
)

// ItemEnvelope is one lossless entry in the item ledger. The Raw payload is
// authoritative: typed converter DTOs are projections and must never overwrite
// it (Section 5.3). The normalized index fields exist only for routing and
// portability decisions.
type ItemEnvelope struct {
	// GatewayItemID is the stable gateway-owned item identifier.
	GatewayItemID string `json:"gateway_item_id"`
	// UpstreamItemID is the raw provider item ID when one is known.
	UpstreamItemID string `json:"upstream_item_id,omitempty"`

	// Indexes derived from Raw for cheap routing/portability decisions.
	Kind   string `json:"kind"`
	Role   string `json:"role,omitempty"`
	Status string `json:"status,omitempty"`
	Phase  string `json:"phase,omitempty"`

	// Normalized links.
	CallID       string `json:"call_id,omitempty"`
	ReferencedID string `json:"referenced_id,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`

	// Provenance records the upstream family the item originated from.
	Provenance string `json:"provenance,omitempty"`

	// Portability classifies how the item may be replayed onto other formats.
	Portability PortabilityClass `json:"portability"`
	// DegradeReason records why an item could not be represented, if applicable.
	DegradeReason string `json:"degrade_reason,omitempty"`

	// Raw is the authoritative lossless canonical JSON, with every unknown field
	// retained verbatim.
	Raw json.RawMessage `json:"raw"`
}

// ProviderBinding is an acceleration handle, not canonical state. It lets a
// same-provider continuation reuse the upstream response/conversation handle.
// It must never contain API keys.
type ProviderBinding struct {
	ChannelID              int    `json:"channel_id"`
	APIType                int    `json:"api_type"`
	EndpointFamily         string `json:"endpoint_family,omitempty"`
	ActualModel            string `json:"actual_model,omitempty"`
	UpstreamResponseID     string `json:"upstream_response_id,omitempty"`
	UpstreamConversationID string `json:"upstream_conversation_id,omitempty"`
}

// ResponseStateRecord is the immutable canonical record for one completed turn.
type ResponseStateRecord struct {
	SchemaVersion int `json:"schema_version"`

	// Identity.
	GatewayResponseID string     `json:"gateway_response_id"`
	Owner             OwnerScope `json:"owner"`
	CreatedAt         int64      `json:"created_at"`
	Status            string     `json:"status"`

	// Graph.
	ParentResponseID string `json:"parent_response_id,omitempty"`
	ConversationID   string `json:"conversation_id,omitempty"`
	TurnSequence     int64  `json:"turn_sequence"`

	// Request. Instructions are stored separately from transcript items so a
	// previous_response_id hydration does not inherit prior request instructions
	// (R4).
	InputItems     []ItemEnvelope  `json:"input_items"`
	Instructions   *string         `json:"instructions,omitempty"`
	RequestedModel string          `json:"requested_model,omitempty"`
	Tools          json.RawMessage `json:"tools,omitempty"`
	StoreMode      bool            `json:"store_mode"`

	// Result.
	OutputItems      []ItemEnvelope  `json:"output_items"`
	Usage            json.RawMessage `json:"usage,omitempty"`
	CompletionStatus string          `json:"completion_status,omitempty"`
	IncompleteReason string          `json:"incomplete_reason,omitempty"`
	ErrorMetadata    json.RawMessage `json:"error_metadata,omitempty"`

	// Provider binding (acceleration handle only).
	Binding *ProviderBinding `json:"binding,omitempty"`

	// Retention.
	ExpiresAt     int64 `json:"expires_at,omitempty"`
	PayloadSize   int   `json:"payload_size,omitempty"`
	ItemCount     int   `json:"item_count,omitempty"`
	TokenEstimate int   `json:"token_estimate,omitempty"`
}

// ConversationStateRecord is the canonical record for a gateway Conversation. It
// contains only items that were explicitly added or automatically appended under
// Conversation semantics.
type ConversationStateRecord struct {
	SchemaVersion int `json:"schema_version"`

	GatewayConversationID string           `json:"gateway_conversation_id"`
	Owner                 OwnerScope       `json:"owner"`
	CreatedAt             int64            `json:"created_at"`
	Version               int64            `json:"version"`
	Items                 []ItemEnvelope   `json:"items"`
	Metadata              json.RawMessage  `json:"metadata,omitempty"`
	Binding               *ProviderBinding `json:"binding,omitempty"`

	// ExpiresAt of zero means no automatic TTL: Conversations remain until
	// explicit deletion or an administrator-defined retention policy (R6, S03).
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

// CheckpointRecord is an exact-transcript continuation checkpoint for stateless
// (Chat/Claude) clients (Section 5.7). Its key is a deterministic hash over the
// full downstream-visible transcript; matching is an optimization that always
// fails open to explicit replay.
type CheckpointRecord struct {
	SchemaVersion int `json:"schema_version"`

	Key          string           `json:"key"`
	Owner        OwnerScope       `json:"owner"`
	ClientFamily string           `json:"client_family"`
	PublicModel  string           `json:"public_model"`
	Binding      *ProviderBinding `json:"binding,omitempty"`
	ResponseID   string           `json:"response_id"`
	// Ambiguous marks a key that maps to more than one distinct continuation, so
	// the optimization must be disabled for it (CP07).
	Ambiguous bool  `json:"ambiguous,omitempty"`
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

// itemIndexProbe captures the routing/portability index fields from a raw item.
type itemIndexProbe struct {
	Type          string `json:"type"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	Phase         string `json:"phase"`
	Id            string `json:"id"`
	CallId        string `json:"call_id"`
	Name          string `json:"name"`
	ReferenceId   string `json:"reference_id"`
	ReferencedRef string `json:"ref"`
}

// NewItemEnvelope builds a lossless envelope from a raw canonical item. It parses
// only the index fields and classifies portability; the raw payload is retained
// verbatim so every unknown field survives a store round trip byte-for-byte
// (I01). A fresh gateway item ID is minted when the caller does not supply one.
func NewItemEnvelope(raw json.RawMessage, provenance string) (ItemEnvelope, error) {
	env := ItemEnvelope{
		Raw:        cloneRaw(raw),
		Provenance: provenance,
	}
	if len(raw) > 0 {
		var probe itemIndexProbe
		if err := json.Unmarshal(raw, &probe); err != nil {
			// A non-object item (e.g. a bare string input) is still retained; it is
			// portable and needs no index fields.
			env.Kind = KindMessage
			env.Role = "user"
			env.Portability = PortabilityPortable
		} else {
			env.Kind = strings.TrimSpace(probe.Type)
			env.Role = probe.Role
			env.Status = probe.Status
			env.Phase = probe.Phase
			env.UpstreamItemID = probe.Id
			env.CallID = probe.CallId
			env.ToolName = probe.Name
			env.ReferencedID = firstNonEmpty(probe.ReferenceId, probe.ReferencedRef, referencedIDForKind(probe))
			env.Portability = classifyRawItem(env.Kind, raw)
		}
	} else {
		env.Kind = KindMessage
		env.Portability = PortabilityPortable
	}

	gid, err := NewItemID()
	if err != nil {
		return ItemEnvelope{}, errors.Wrap(err, "mint gateway item id")
	}
	env.GatewayItemID = gid
	return env, nil
}

// referencedIDForKind extracts the referenced item id for an item_reference item,
// whose target lives in the "id" field rather than a dedicated reference field.
func referencedIDForKind(probe itemIndexProbe) string {
	if strings.EqualFold(strings.TrimSpace(probe.Type), KindItemReference) {
		return probe.Id
	}
	return ""
}

// NewStringInputEnvelope builds an envelope for a bare string input item, which
// the Responses API treats as a user message.
func NewStringInputEnvelope(text string) (ItemEnvelope, error) {
	raw, err := json.Marshal(map[string]any{
		"type":    KindMessage,
		"role":    "user",
		"content": text,
	})
	if err != nil {
		return ItemEnvelope{}, errors.Wrap(err, "marshal string input item")
	}
	return NewItemEnvelope(raw, "")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
	return out
}
