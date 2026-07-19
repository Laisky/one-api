package state

import (
	"context"
	"encoding/json"
	"time"
)

// AnyVersion is passed as the expected version to skip the compare-and-set check
// on a conversation mutation.
const AnyVersion int64 = -1

// ResponseStateStore is the pluggable backend for gateway Responses state. The
// production backend is Redis-backed and encrypted at rest; an in-memory backend
// is provided for tests and conformance only and is never selected in production
// (Section 5.4). All lookups validate owner scope before returning a record.
type ResponseStateStore interface {
	// --- Immutable response nodes ---------------------------------------------

	// CreateResponse stores an immutable response node. When idempotencyKey is
	// non-empty and a node was already created under it, the existing node is
	// returned unchanged and no duplicate is written (S05, F02, F04).
	CreateResponse(ctx context.Context, record *ResponseStateRecord, idempotencyKey string) (*ResponseStateRecord, error)
	// GetResponse returns the owner's node, or ErrNotFound for unknown, expired,
	// tombstoned, or foreign-owner IDs (SEC03, S06).
	GetResponse(ctx context.Context, owner OwnerScope, id string) (*ResponseStateRecord, error)
	// DeleteResponse tombstones a node so the ID cannot be confused with a cache
	// miss and its provider binding is never reused (S06).
	DeleteResponse(ctx context.Context, owner OwnerScope, id string) error
	// BatchGetResponses returns nodes in the requested order, preserving nil holes
	// for missing middle nodes so chain hydration can detect gaps (S08).
	BatchGetResponses(ctx context.Context, owner OwnerScope, ids []string) ([]*ResponseStateRecord, error)
	// GetResponseBinding returns just the provider binding for pre-routing affinity
	// without deserializing the full record (PERF02).
	GetResponseBinding(ctx context.Context, owner OwnerScope, id string) (*ProviderBinding, error)

	// --- Item lookup ----------------------------------------------------------

	// GetItem resolves an item_reference to a stored item under the owner scope.
	GetItem(ctx context.Context, owner OwnerScope, itemID string) (*ItemEnvelope, error)

	// --- Conversations --------------------------------------------------------

	CreateConversation(ctx context.Context, record *ConversationStateRecord, idempotencyKey string) (*ConversationStateRecord, error)
	GetConversation(ctx context.Context, owner OwnerScope, id string) (*ConversationStateRecord, error)
	DeleteConversation(ctx context.Context, owner OwnerScope, id string) error
	// AppendConversationItems atomically appends items and advances the version.
	// expectedVersion enables compare-and-set concurrency control (AnyVersion to
	// skip); idempotencyKey makes a retried append a no-op (CON02, S05, S11).
	AppendConversationItems(ctx context.Context, owner OwnerScope, id string, expectedVersion int64, items []ItemEnvelope, idempotencyKey string) (*ConversationStateRecord, error)
	UpdateConversationMetadata(ctx context.Context, owner OwnerScope, id string, expectedVersion int64, metadata json.RawMessage) (*ConversationStateRecord, error)
	DeleteConversationItem(ctx context.Context, owner OwnerScope, id, itemID string, expectedVersion int64) (*ConversationStateRecord, error)

	// --- Conversation lease (serialize writes, CON04/CON05) -------------------

	AcquireConversationLease(ctx context.Context, owner OwnerScope, id string, ttl time.Duration) (leaseToken string, err error)
	RenewConversationLease(ctx context.Context, owner OwnerScope, id, leaseToken string, ttl time.Duration) error
	ReleaseConversationLease(ctx context.Context, owner OwnerScope, id, leaseToken string) error

	// --- Checkpoints (stateless-client optimization) --------------------------

	PutCheckpoint(ctx context.Context, record *CheckpointRecord) error
	GetCheckpoint(ctx context.Context, owner OwnerScope, key string) (*CheckpointRecord, error)

	// --- Health ---------------------------------------------------------------

	// Ping reports whether the backend can currently be read and written.
	Ping(ctx context.Context) error
}

// cloneResponseRecord returns a deep copy so stored records stay immutable even
// if a caller mutates the returned value (F07: an immutable parent is never
// changed by a fork). It round-trips through JSON, which is acceptable on the
// cold state path and guarantees no shared slices or maps.
func cloneResponseRecord(record *ResponseStateRecord) (*ResponseStateRecord, error) {
	if record == nil {
		return nil, nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	var out ResponseStateRecord
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func cloneConversationRecord(record *ConversationStateRecord) (*ConversationStateRecord, error) {
	if record == nil {
		return nil, nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	var out ConversationStateRecord
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func cloneCheckpointRecord(record *CheckpointRecord) *CheckpointRecord {
	if record == nil {
		return nil
	}
	out := *record
	if record.Binding != nil {
		binding := *record.Binding
		out.Binding = &binding
	}
	return &out
}
