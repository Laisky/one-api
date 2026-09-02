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

	limits      Limits
	convIdleTTL time.Duration
	clock       func() time.Time

	responses      map[string]*ResponseStateRecord
	respTombstones map[string]struct{}
	respIdem       map[string]string // idempotencyKey -> gateway response id
	respByUser     map[int][]userRespEntry

	conversations  map[string]*ConversationStateRecord
	convTombstones map[string]struct{}
	convIdem       map[string]string        // idempotencyKey -> gateway conversation id
	convAppendIdem map[string]struct{}      // applied append idempotency keys
	convByUser     map[int]map[string]int64 // userID -> convID -> last-activity unix
	leases         map[string]leaseState

	items       map[itemIndexKey]itemIndexEntry // {owner user, itemID} -> entry
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

// userRespEntry tracks one of a user's response records for the per-user cap.
// Entries are appended in creation order, so index 0 is the oldest (TTL+LRU
// eviction pops from the front, row L06).
type userRespEntry struct {
	id        string
	createdAt int64
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
		respByUser:     make(map[int][]userRespEntry),
		conversations:  make(map[string]*ConversationStateRecord),
		convTombstones: make(map[string]struct{}),
		convIdem:       make(map[string]string),
		convAppendIdem: make(map[string]struct{}),
		convByUser:     make(map[int]map[string]int64),
		leases:         make(map[string]leaseState),
		items:          make(map[itemIndexKey]itemIndexEntry),
		checkpoints:    make(map[string]*CheckpointRecord),
	}
}

// SetClock overrides the store clock; tests use it to exercise TTL expiry.
func (s *MemoryStore) SetClock(clock func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

// SetConversationIdleTTL configures the sliding idle time-to-live applied to
// conversations (row L08). Zero retains conversations until explicit deletion
// (today's S03 default).
func (s *MemoryStore) SetConversationIdleTTL(ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.convIdleTTL = ttl
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
	s.trackAndEvictUserResponseLocked(stored)

	return cloneResponseRecord(stored)
}

// trackAndEvictUserResponseLocked records a newly stored response against its
// owner's per-user budget and prunes the owner's oldest records when the budget
// is exceeded (TTL+LRU, row L06). An evicted parent degrades to the standard
// previous_response_not_found contract because it is tombstoned like an explicit
// delete. A non-positive cap disables the accounting entirely so behavior is
// unchanged when the feature is off (row L05).
func (s *MemoryStore) trackAndEvictUserResponseLocked(rec *ResponseStateRecord) {
	limit := s.limits.MaxResponsesPerUser
	if limit <= 0 {
		return
	}
	uid := rec.Owner.UserID
	s.respByUser[uid] = append(s.respByUser[uid], userRespEntry{id: rec.GatewayResponseID, createdAt: rec.CreatedAt})
	for len(s.respByUser[uid]) > limit {
		oldest := s.respByUser[uid][0]
		s.respByUser[uid] = s.respByUser[uid][1:]
		s.evictResponseLocked(oldest.id)
	}
	if len(s.respByUser[uid]) == 0 {
		delete(s.respByUser, uid)
	}
}

// evictResponseLocked removes a response node by id during LRU pruning: it drops
// the record, tombstones the id, and purges its item-index entries (both gateway
// and upstream ids) so nothing resolves after eviction (rows L06, S06).
func (s *MemoryStore) evictResponseLocked(id string) {
	rec, ok := s.responses[id]
	if !ok {
		return
	}
	delete(s.responses, id)
	s.respTombstones[id] = struct{}{}
	s.removeItemIndexLocked(rec.Owner, rec.InputItems)
	s.removeItemIndexLocked(rec.Owner, rec.OutputItems)
}

// itemIndexKey scopes an item-index entry to its owner. UpstreamItemID comes
// verbatim from a client-supplied "id", so a flat itemID-keyed map let one tenant
// squat or delete another tenant's index entry. This mirrors RedisStore.itemKey.
type itemIndexKey struct {
	userID int
	itemID string
}

// removeItemIndexLocked deletes both the gateway-id and upstream-id index entries
// for each item, closing the UpstreamItemID remanence gap (ST-018).
//
// Parameters:
//   - owner: the owner whose entries are being removed.
//   - items: the envelopes whose index entries to drop.
func (s *MemoryStore) removeItemIndexLocked(owner OwnerScope, items []ItemEnvelope) {
	for _, env := range items {
		if env.GatewayItemID != "" {
			delete(s.items, itemIndexKey{userID: owner.UserID, itemID: env.GatewayItemID})
		}
		if env.UpstreamItemID != "" {
			delete(s.items, itemIndexKey{userID: owner.UserID, itemID: env.UpstreamItemID})
		}
	}
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
	// Remove the node's items from the reference index (both gateway and upstream
	// ids) so a stale reference cannot resolve after deletion (S06, ST-018).
	s.removeItemIndexLocked(rec.Owner, rec.InputItems)
	s.removeItemIndexLocked(rec.Owner, rec.OutputItems)
	s.untrackUserResponseLocked(rec.Owner.UserID, id)
	return nil
}

// untrackUserResponseLocked drops a response id from its owner's per-user budget.
func (s *MemoryStore) untrackUserResponseLocked(userID int, id string) {
	entries, ok := s.respByUser[userID]
	if !ok {
		return
	}
	for i, e := range entries {
		if e.id == id {
			s.respByUser[userID] = append(entries[:i], entries[i+1:]...)
			break
		}
	}
	if len(s.respByUser[userID]) == 0 {
		delete(s.respByUser, userID)
	}
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
	// Consult the tombstone first so a deleted or evicted id can never resolve,
	// even against a record that was somehow re-added (ST-018: tombstones are read,
	// not just written).
	if _, dead := s.respTombstones[id]; dead {
		return nil, ErrNotFound
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

// ResponseTombstoned reports whether a response id was explicitly deleted or
// LRU-evicted. The resolve layer consults it so legacy passthrough never
// forwards a known gateway id upstream after deletion (row S06, ST-018).
func (s *MemoryStore) ResponseTombstoned(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, dead := s.respTombstones[id]
	return dead, nil
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
	entry, ok := s.items[itemIndexKey{userID: owner.UserID, itemID: itemID}]
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
		s.items[itemIndexKey{userID: owner.UserID, itemID: env.GatewayItemID}] = itemIndexEntry{owner: owner, env: clone}
		if env.UpstreamItemID != "" {
			// Also index by upstream id so an item_reference expressed with the raw
			// provider id resolves.
			upstreamKey := itemIndexKey{userID: owner.UserID, itemID: env.UpstreamItemID}
			if _, exists := s.items[upstreamKey]; !exists {
				s.items[upstreamKey] = itemIndexEntry{owner: owner, env: clone}
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

	// Enforce the per-user active-conversation cap before writing. Idle-expired
	// conversations are pruned first so the count reflects only live ones; on
	// overflow the create fails explicitly — conversations are never silently
	// evicted, which would corrupt continuation semantics (row L07).
	if s.limits.MaxConversationsPerUser > 0 {
		uid := record.Owner.UserID
		s.pruneIdleConversationsLocked(uid)
		if s.activeConversationCountLocked(uid) >= s.limits.MaxConversationsPerUser {
			return nil, errors.Wrapf(ErrLimitExceeded, "active conversations per user %d", s.limits.MaxConversationsPerUser)
		}
	}

	stored, err := cloneConversationRecord(record)
	if err != nil {
		return nil, errors.Wrap(err, "clone conversation record")
	}
	if stored.SchemaVersion == 0 {
		stored.SchemaVersion = CurrentSchemaVersion
	}
	// Apply the sliding idle TTL to a conversation that did not carry an explicit
	// expiry (row L08). A zero idle TTL leaves ExpiresAt untouched (S03 default).
	if s.convIdleTTL > 0 && stored.ExpiresAt == 0 {
		stored.ExpiresAt = s.now().Add(s.convIdleTTL).Unix()
	}
	s.conversations[stored.GatewayConversationID] = stored
	if idempotencyKey != "" {
		s.convIdem[idempotencyKey] = stored.GatewayConversationID
	}
	s.indexItems(stored.Owner, stored.Items)
	s.trackConversationActivityLocked(stored.Owner.UserID, stored.GatewayConversationID)
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
	// Reading is activity: slide the idle TTL forward (row L08).
	s.touchConversationLocked(rec)
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
	// Purge both gateway-id and upstream-id item index entries (ST-018).
	s.removeItemIndexLocked(rec.Owner, rec.Items)
	s.untrackConversationLocked(rec.Owner.UserID, id)
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
	if s.limits.ItemCountExceeded(projected) {
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
	// Appending is activity: slide the idle TTL forward (row L08).
	s.touchConversationLocked(rec)
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
			// Purge both gateway-id and upstream-id index entries (ST-018).
			s.removeItemIndexLocked(rec.Owner, []ItemEnvelope{env})
			continue
		}
		filtered = append(filtered, env)
	}
	if !removed {
		return nil, ErrNotFound
	}
	rec.Items = filtered
	rec.Version++
	s.touchConversationLocked(rec)
	return cloneConversationRecord(rec)
}

func (s *MemoryStore) lookupConversationLocked(owner OwnerScope, id string) (*ConversationStateRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	if _, dead := s.convTombstones[id]; dead {
		return nil, ErrNotFound
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

// --- Per-user conversation governance (rows L07, L08) -----------------------

// touchConversationLocked slides a conversation's idle TTL forward on any read
// or write, and refreshes its per-user activity timestamp. A zero idle TTL means
// conversations never expire on idle (S03 default) and only the activity index is
// refreshed.
func (s *MemoryStore) touchConversationLocked(rec *ConversationStateRecord) {
	if rec == nil {
		return
	}
	if s.convIdleTTL > 0 {
		rec.ExpiresAt = s.now().Add(s.convIdleTTL).Unix()
	}
	s.trackConversationActivityLocked(rec.Owner.UserID, rec.GatewayConversationID)
}

// trackConversationActivityLocked records the last-activity time for a
// conversation so the per-user cap can prune idle entries.
func (s *MemoryStore) trackConversationActivityLocked(userID int, id string) {
	if s.limits.MaxConversationsPerUser <= 0 {
		return
	}
	m := s.convByUser[userID]
	if m == nil {
		m = make(map[string]int64)
		s.convByUser[userID] = m
	}
	m[id] = s.now().Unix()
}

// untrackConversationLocked drops a conversation from its owner's activity index.
func (s *MemoryStore) untrackConversationLocked(userID int, id string) {
	if m, ok := s.convByUser[userID]; ok {
		delete(m, id)
		if len(m) == 0 {
			delete(s.convByUser, userID)
		}
	}
}

// pruneIdleConversationsLocked removes activity-index entries whose idle TTL has
// elapsed so the per-user active count reflects only live conversations. When the
// idle TTL is zero, entries are retained until explicit deletion (S03).
func (s *MemoryStore) pruneIdleConversationsLocked(userID int) {
	if s.convIdleTTL <= 0 {
		return
	}
	m, ok := s.convByUser[userID]
	if !ok {
		return
	}
	cutoff := s.now().Add(-s.convIdleTTL).Unix()
	for id, last := range m {
		if last <= cutoff {
			delete(m, id)
		}
	}
	if len(m) == 0 {
		delete(s.convByUser, userID)
	}
}

// activeConversationCountLocked returns the owner's live conversation count.
func (s *MemoryStore) activeConversationCountLocked(userID int) int {
	return len(s.convByUser[userID])
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
	if s.limits.ItemCountExceeded(count) {
		return errors.Wrapf(ErrLimitExceeded, "response item count %d", count)
	}
	if s.limits.MaxRecordBytes > 0 {
		data, err := json.Marshal(record)
		if err != nil {
			return errors.Wrap(err, "measure response record")
		}
		if s.limits.RecordBytesExceeded(len(data)) {
			return errors.Wrapf(ErrLimitExceeded, "response record bytes %d", len(data))
		}
	}
	return nil
}

func (s *MemoryStore) validateConversationLimits(record *ConversationStateRecord) error {
	if s.limits.ItemCountExceeded(len(record.Items)) {
		return errors.Wrapf(ErrLimitExceeded, "conversation item count %d", len(record.Items))
	}
	if s.limits.MaxRecordBytes > 0 {
		data, err := json.Marshal(record)
		if err != nil {
			return errors.Wrap(err, "measure conversation record")
		}
		if s.limits.RecordBytesExceeded(len(data)) {
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
