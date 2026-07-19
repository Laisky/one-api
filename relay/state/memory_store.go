package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
)

// MemoryStore is an in-process ResponseStateStore used for tests and the
// conformance harness. It is never selected in production: the feature flag
// refuses to enable without a healthy Redis backend (Section 5.4). It is
// concurrency-safe and enforces owner scoping, tombstones, idempotency, CAS
// versioning, and leases so it can validate the same contract as the Redis
// backend.
type MemoryStore struct {
	mu sync.Mutex

	limits Limits
	clock  func() time.Time

	responses      map[string]*ResponseStateRecord
	respTombstones map[string]struct{}
	respIdem       map[string]string // idempotencyKey -> gateway response id

	conversations  map[string]*ConversationStateRecord
	convTombstones map[string]struct{}
	convIdem       map[string]string   // idempotencyKey -> gateway conversation id
	convAppendIdem map[string]struct{} // applied append idempotency keys
	leases         map[string]leaseState

	items       map[string]itemIndexEntry // itemID -> entry
	checkpoints map[string]*CheckpointRecord
}

type leaseState struct {
	token     string
	expiresAt time.Time
}

type itemIndexEntry struct {
	owner OwnerScope
	env   ItemEnvelope
}

// NewMemoryStore builds an empty in-memory store with the given limits. A nil or
// zero-value Limits uses DefaultLimits.
func NewMemoryStore(limits Limits) *MemoryStore {
	if (limits == Limits{}) {
		limits = DefaultLimits()
	}
	return &MemoryStore{
		limits:         limits,
		clock:          time.Now,
		responses:      make(map[string]*ResponseStateRecord),
		respTombstones: make(map[string]struct{}),
		respIdem:       make(map[string]string),
		conversations:  make(map[string]*ConversationStateRecord),
		convTombstones: make(map[string]struct{}),
		convIdem:       make(map[string]string),
		convAppendIdem: make(map[string]struct{}),
		leases:         make(map[string]leaseState),
		items:          make(map[string]itemIndexEntry),
		checkpoints:    make(map[string]*CheckpointRecord),
	}
}

// SetClock overrides the store clock; tests use it to exercise TTL expiry.
func (s *MemoryStore) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

func (s *MemoryStore) now() time.Time { return s.clock() }

// Ping always succeeds for the in-memory store.
func (s *MemoryStore) Ping(context.Context) error { return nil }

// --- Response nodes ---------------------------------------------------------

// CreateResponse stores an immutable response node.
func (s *MemoryStore) CreateResponse(_ context.Context, record *ResponseStateRecord, idempotencyKey string) (*ResponseStateRecord, error) {
	if record == nil {
		return nil, errors.New("state: nil response record")
	}
	if !record.Owner.Valid() {
		return nil, ErrInvalidOwner
	}
	if err := s.validateResponseLimits(record); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if idempotencyKey != "" {
		if existingID, ok := s.respIdem[idempotencyKey]; ok {
			if existing, ok := s.responses[existingID]; ok {
				return cloneResponseRecord(existing)
			}
		}
	}

	stored, err := cloneResponseRecord(record)
	if err != nil {
		return nil, errors.Wrap(err, "clone response record")
	}
	if stored.SchemaVersion == 0 {
		stored.SchemaVersion = CurrentSchemaVersion
	}
	s.responses[stored.GatewayResponseID] = stored
	if idempotencyKey != "" {
		s.respIdem[idempotencyKey] = stored.GatewayResponseID
	}
	s.indexItems(stored.Owner, stored.InputItems)
	s.indexItems(stored.Owner, stored.OutputItems)

	return cloneResponseRecord(stored)
}

// GetResponse returns the owner's node.
func (s *MemoryStore) GetResponse(_ context.Context, owner OwnerScope, id string) (*ResponseStateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupResponseLocked(owner, id)
	if err != nil {
		return nil, err
	}
	return cloneResponseRecord(rec)
}

// GetResponseBinding returns the provider binding only.
func (s *MemoryStore) GetResponseBinding(_ context.Context, owner OwnerScope, id string) (*ProviderBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupResponseLocked(owner, id)
	if err != nil {
		return nil, err
	}
	if rec.Binding == nil {
		return nil, nil
	}
	binding := *rec.Binding
	return &binding, nil
}

// DeleteResponse tombstones the node.
func (s *MemoryStore) DeleteResponse(_ context.Context, owner OwnerScope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupResponseLocked(owner, id)
	if err != nil {
		return err
	}
	delete(s.responses, id)
	s.respTombstones[id] = struct{}{}
	// Remove the node's items from the reference index so a stale reference cannot
	// resolve after deletion (S06).
	for _, env := range rec.InputItems {
		delete(s.items, env.GatewayItemID)
	}
	for _, env := range rec.OutputItems {
		delete(s.items, env.GatewayItemID)
	}
	return nil
}

// BatchGetResponses returns nodes in order, with nil for missing/foreign nodes.
func (s *MemoryStore) BatchGetResponses(_ context.Context, owner OwnerScope, ids []string) ([]*ResponseStateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*ResponseStateRecord, len(ids))
	for i, id := range ids {
		rec, err := s.lookupResponseLocked(owner, id)
		if err != nil {
			out[i] = nil
			continue
		}
		clone, err := cloneResponseRecord(rec)
		if err != nil {
			return nil, err
		}
		out[i] = clone
	}
	return out, nil
}

func (s *MemoryStore) lookupResponseLocked(owner OwnerScope, id string) (*ResponseStateRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	rec, ok := s.responses[id]
	if !ok {
		return nil, ErrNotFound
	}
	if !rec.Owner.Matches(owner) {
		return nil, ErrNotFound
	}
	if s.isResponseExpiredLocked(rec) {
		return nil, ErrNotFound
	}
	return rec, nil
}

func (s *MemoryStore) isResponseExpiredLocked(rec *ResponseStateRecord) bool {
	return rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt
}

// --- Item lookup ------------------------------------------------------------

// GetItem resolves a stored item under owner scope for item_reference hydration.
func (s *MemoryStore) GetItem(_ context.Context, owner OwnerScope, itemID string) (*ItemEnvelope, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[itemID]
	if !ok || !entry.owner.Matches(owner) {
		return nil, ErrNotFound
	}
	env := entry.env
	env.Raw = cloneRaw(entry.env.Raw)
	return &env, nil
}

func (s *MemoryStore) indexItems(owner OwnerScope, items []ItemEnvelope) {
	for _, env := range items {
		if env.GatewayItemID == "" {
			continue
		}
		clone := env
		clone.Raw = cloneRaw(env.Raw)
		s.items[env.GatewayItemID] = itemIndexEntry{owner: owner, env: clone}
		if env.UpstreamItemID != "" {
			// Also index by upstream id so an item_reference expressed with the raw
			// provider id resolves.
			if _, exists := s.items[env.UpstreamItemID]; !exists {
				s.items[env.UpstreamItemID] = itemIndexEntry{owner: owner, env: clone}
			}
		}
	}
}

// --- Conversations ----------------------------------------------------------

// CreateConversation stores a new conversation record.
func (s *MemoryStore) CreateConversation(_ context.Context, record *ConversationStateRecord, idempotencyKey string) (*ConversationStateRecord, error) {
	if record == nil {
		return nil, errors.New("state: nil conversation record")
	}
	if !record.Owner.Valid() {
		return nil, ErrInvalidOwner
	}
	if err := s.validateConversationLimits(record); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if idempotencyKey != "" {
		if existingID, ok := s.convIdem[idempotencyKey]; ok {
			if existing, ok := s.conversations[existingID]; ok {
				return cloneConversationRecord(existing)
			}
		}
	}

	stored, err := cloneConversationRecord(record)
	if err != nil {
		return nil, errors.Wrap(err, "clone conversation record")
	}
	if stored.SchemaVersion == 0 {
		stored.SchemaVersion = CurrentSchemaVersion
	}
	s.conversations[stored.GatewayConversationID] = stored
	if idempotencyKey != "" {
		s.convIdem[idempotencyKey] = stored.GatewayConversationID
	}
	s.indexItems(stored.Owner, stored.Items)
	return cloneConversationRecord(stored)
}

// GetConversation returns the owner's conversation.
func (s *MemoryStore) GetConversation(_ context.Context, owner OwnerScope, id string) (*ConversationStateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupConversationLocked(owner, id)
	if err != nil {
		return nil, err
	}
	return cloneConversationRecord(rec)
}

// DeleteConversation tombstones the conversation and its items.
func (s *MemoryStore) DeleteConversation(_ context.Context, owner OwnerScope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupConversationLocked(owner, id)
	if err != nil {
		return err
	}
	delete(s.conversations, id)
	s.convTombstones[id] = struct{}{}
	delete(s.leases, id)
	for _, env := range rec.Items {
		delete(s.items, env.GatewayItemID)
	}
	return nil
}

// AppendConversationItems atomically appends items and advances the version.
func (s *MemoryStore) AppendConversationItems(_ context.Context, owner OwnerScope, id string, expectedVersion int64, items []ItemEnvelope, idempotencyKey string) (*ConversationStateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, err := s.lookupConversationLocked(owner, id)
	if err != nil {
		return nil, err
	}

	if idempotencyKey != "" {
		if _, applied := s.convAppendIdem[appendIdemKey(id, idempotencyKey)]; applied {
			// Already applied: return current state without a second append (S05).
			return cloneConversationRecord(rec)
		}
	}

	if expectedVersion != AnyVersion && expectedVersion != rec.Version {
		return nil, ErrVersionConflict
	}

	projected := len(rec.Items) + len(items)
	if s.limits.itemCountExceeded(projected) {
		return nil, errors.Wrapf(ErrLimitExceeded, "conversation item count %d", projected)
	}

	appended := make([]ItemEnvelope, 0, len(items))
	for _, env := range items {
		clone := env
		clone.Raw = cloneRaw(env.Raw)
		appended = append(appended, clone)
	}
	rec.Items = append(rec.Items, appended...)
	rec.Version++
	if idempotencyKey != "" {
		s.convAppendIdem[appendIdemKey(id, idempotencyKey)] = struct{}{}
	}
	s.indexItems(owner, appended)
	return cloneConversationRecord(rec)
}

// UpdateConversationMetadata updates metadata only and advances the version.
func (s *MemoryStore) UpdateConversationMetadata(_ context.Context, owner OwnerScope, id string, expectedVersion int64, metadata json.RawMessage) (*ConversationStateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupConversationLocked(owner, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion != AnyVersion && expectedVersion != rec.Version {
		return nil, ErrVersionConflict
	}
	rec.Metadata = cloneRaw(metadata)
	rec.Version++
	return cloneConversationRecord(rec)
}

// DeleteConversationItem removes one item and advances the version.
func (s *MemoryStore) DeleteConversationItem(_ context.Context, owner OwnerScope, id, itemID string, expectedVersion int64) (*ConversationStateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.lookupConversationLocked(owner, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion != AnyVersion && expectedVersion != rec.Version {
		return nil, ErrVersionConflict
	}
	filtered := rec.Items[:0:0]
	removed := false
	for _, env := range rec.Items {
		if env.GatewayItemID == itemID || (env.UpstreamItemID != "" && env.UpstreamItemID == itemID) {
			removed = true
			delete(s.items, env.GatewayItemID)
			continue
		}
		filtered = append(filtered, env)
	}
	if !removed {
		return nil, ErrNotFound
	}
	rec.Items = filtered
	rec.Version++
	return cloneConversationRecord(rec)
}

func (s *MemoryStore) lookupConversationLocked(owner OwnerScope, id string) (*ConversationStateRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	rec, ok := s.conversations[id]
	if !ok {
		return nil, ErrNotFound
	}
	if !rec.Owner.Matches(owner) {
		return nil, ErrNotFound
	}
	if rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt {
		return nil, ErrNotFound
	}
	return rec, nil
}

// --- Conversation lease -----------------------------------------------------

// AcquireConversationLease grants an exclusive mutation lease or returns
// ErrLeaseHeld if another live lease exists.
func (s *MemoryStore) AcquireConversationLease(_ context.Context, owner OwnerScope, id string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.lookupConversationLocked(owner, id); err != nil {
		return "", err
	}
	if lease, ok := s.leases[id]; ok && s.now().Before(lease.expiresAt) {
		return "", ErrLeaseHeld
	}
	token, err := randomHex(16)
	if err != nil {
		return "", err
	}
	s.leases[id] = leaseState{token: token, expiresAt: s.now().Add(ttl)}
	return token, nil
}

// RenewConversationLease extends a held lease.
func (s *MemoryStore) RenewConversationLease(_ context.Context, owner OwnerScope, id, leaseToken string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.lookupConversationLocked(owner, id); err != nil {
		return err
	}
	lease, ok := s.leases[id]
	if !ok || lease.token != leaseToken || !s.now().Before(lease.expiresAt) {
		return ErrLeaseInvalid
	}
	lease.expiresAt = s.now().Add(ttl)
	s.leases[id] = lease
	return nil
}

// ReleaseConversationLease releases a held lease.
func (s *MemoryStore) ReleaseConversationLease(_ context.Context, owner OwnerScope, id, leaseToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[id]
	if !ok {
		return nil
	}
	if lease.token != leaseToken {
		return ErrLeaseInvalid
	}
	delete(s.leases, id)
	return nil
}

// --- Checkpoints ------------------------------------------------------------

// PutCheckpoint stores or overwrites a checkpoint.
func (s *MemoryStore) PutCheckpoint(_ context.Context, record *CheckpointRecord) error {
	if record == nil {
		return errors.New("state: nil checkpoint record")
	}
	if !record.Owner.Valid() {
		return ErrInvalidOwner
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := cloneCheckpointRecord(record)
	if clone.SchemaVersion == 0 {
		clone.SchemaVersion = CurrentSchemaVersion
	}
	s.checkpoints[checkpointStoreKey(record.Owner, record.Key)] = clone
	return nil
}

// GetCheckpoint returns a checkpoint for the owner scope, or ErrNotFound.
func (s *MemoryStore) GetCheckpoint(_ context.Context, owner OwnerScope, key string) (*CheckpointRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.checkpoints[checkpointStoreKey(owner, key)]
	if !ok {
		return nil, ErrNotFound
	}
	if rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt {
		return nil, ErrNotFound
	}
	return cloneCheckpointRecord(rec), nil
}

// --- Limits -----------------------------------------------------------------

func (s *MemoryStore) validateResponseLimits(record *ResponseStateRecord) error {
	count := len(record.InputItems) + len(record.OutputItems)
	if s.limits.itemCountExceeded(count) {
		return errors.Wrapf(ErrLimitExceeded, "response item count %d", count)
	}
	if s.limits.MaxRecordBytes > 0 {
		data, err := json.Marshal(record)
		if err != nil {
			return errors.Wrap(err, "measure response record")
		}
		if s.limits.recordBytesExceeded(len(data)) {
			return errors.Wrapf(ErrLimitExceeded, "response record bytes %d", len(data))
		}
	}
	return nil
}

func (s *MemoryStore) validateConversationLimits(record *ConversationStateRecord) error {
	if s.limits.itemCountExceeded(len(record.Items)) {
		return errors.Wrapf(ErrLimitExceeded, "conversation item count %d", len(record.Items))
	}
	if s.limits.MaxRecordBytes > 0 {
		data, err := json.Marshal(record)
		if err != nil {
			return errors.Wrap(err, "measure conversation record")
		}
		if s.limits.recordBytesExceeded(len(data)) {
			return errors.Wrapf(ErrLimitExceeded, "conversation record bytes %d", len(data))
		}
	}
	return nil
}

func appendIdemKey(conversationID, idempotencyKey string) string {
	return conversationID + "\x00" + idempotencyKey
}

func checkpointStoreKey(owner OwnerScope, key string) string {
	return fmt.Sprintf("%d:%d:%s", owner.UserID, owner.TokenID, key)
}

// compile-time assertion that MemoryStore satisfies the interface.
var _ ResponseStateStore = (*MemoryStore)(nil)
