package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/go-redis/redis/v8"
)

// RedisStore is the production ResponseStateStore backend. Payloads are encrypted
// with a versioned application key before storage; Redis keys carry only random
// gateway IDs or opaque hashes, never user content, prompts, or credentials
// (SEC01). It follows the pluggable-backend pattern established by
// relay/adaptor/anthropic/signature_cache.go, with the difference that this
// backend is the one used in production.
type RedisStore struct {
	rdb    redis.Cmdable
	ring   *KeyRing
	limits Limits
	ttl    time.Duration
	ns     string
	clock  func() time.Time
}

// NewRedisStore builds a Redis-backed store. responseTTL is the default lifetime
// applied to response nodes and their item index entries; a non-positive value
// means no TTL. The KeyRing must be non-nil so payloads are always encrypted.
func NewRedisStore(rdb redis.Cmdable, ring *KeyRing, limits Limits, responseTTL time.Duration) (*RedisStore, error) {
	if rdb == nil {
		return nil, errors.New("state: nil redis client")
	}
	if ring == nil {
		return nil, errors.New("state: nil key ring")
	}
	if (limits == Limits{}) {
		limits = DefaultLimits()
	}
	return &RedisStore{
		rdb:    rdb,
		ring:   ring,
		limits: limits,
		ttl:    responseTTL,
		ns:     "rs",
		clock:  time.Now,
	}, nil
}

func (s *RedisStore) now() time.Time { return s.clock() }

// Ping verifies the backend is reachable.
func (s *RedisStore) Ping(ctx context.Context) error {
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return nil
}

// --- key helpers ------------------------------------------------------------

func (s *RedisStore) respKey(id string) string     { return s.ns + ":resp:" + id }
func (s *RedisStore) respIdemKey(k string) string  { return s.ns + ":idem:resp:" + k }
func (s *RedisStore) respTombKey(id string) string { return s.ns + ":tomb:resp:" + id }
func (s *RedisStore) convKey(id string) string     { return s.ns + ":conv:" + id }
func (s *RedisStore) convIdemKey(k string) string  { return s.ns + ":idem:conv:" + k }
func (s *RedisStore) convAppendIdemKey(id, k string) string {
	return s.ns + ":idem:convapp:" + id + ":" + k
}
func (s *RedisStore) leaseKey(id string) string    { return s.ns + ":lease:" + id }
func (s *RedisStore) itemKey(itemID string) string { return s.ns + ":item:" + itemID }
func (s *RedisStore) checkpointKey(owner OwnerScope, key string) string {
	sum := sha256.Sum256([]byte(checkpointStoreKey(owner, key)))
	return s.ns + ":cp:" + hex.EncodeToString(sum[:])
}

// --- serialization ----------------------------------------------------------

func (s *RedisStore) encode(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", errors.Wrap(err, "state: marshal record")
	}
	if s.limits.recordBytesExceeded(len(data)) {
		return "", errors.Wrapf(ErrLimitExceeded, "record bytes %d", len(data))
	}
	token, err := s.ring.Encrypt(data)
	if err != nil {
		return "", err
	}
	return token, nil
}

// schemaProbe reads only the schema version so an unsupported record fails with a
// typed error instead of silently dropping fields (S07).
type schemaProbe struct {
	SchemaVersion int `json:"schema_version"`
}

func (s *RedisStore) decode(token string, v any) error {
	plaintext, err := s.ring.Decrypt(token)
	if err != nil {
		return err
	}
	var probe schemaProbe
	if err := json.Unmarshal(plaintext, &probe); err == nil {
		if probe.SchemaVersion > CurrentSchemaVersion {
			return errors.Wrapf(ErrUnsupportedSchema, "record schema version %d", probe.SchemaVersion)
		}
	}
	if err := json.Unmarshal(plaintext, v); err != nil {
		return errors.Wrap(err, "state: unmarshal record")
	}
	return nil
}

// getString reads a raw string value, mapping redis.Nil to ErrNotFound and any
// other error to ErrStoreUnavailable.
func (s *RedisStore) getString(ctx context.Context, key string) (string, error) {
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrNotFound
	}
	if err != nil {
		return "", errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return val, nil
}

// --- Response nodes ---------------------------------------------------------

// CreateResponse stores an immutable, encrypted response node.
func (s *RedisStore) CreateResponse(ctx context.Context, record *ResponseStateRecord, idempotencyKey string) (*ResponseStateRecord, error) {
	if record == nil {
		return nil, errors.New("state: nil response record")
	}
	if !record.Owner.Valid() {
		return nil, ErrInvalidOwner
	}
	count := len(record.InputItems) + len(record.OutputItems)
	if s.limits.itemCountExceeded(count) {
		return nil, errors.Wrapf(ErrLimitExceeded, "response item count %d", count)
	}

	if idempotencyKey != "" {
		// SetNX claims the idempotency key atomically; a loser reads the winner.
		ok, err := s.rdb.SetNX(ctx, s.respIdemKey(idempotencyKey), record.GatewayResponseID, s.nodeTTL(record.ExpiresAt)).Result()
		if err != nil {
			return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
		}
		if !ok {
			existingID, err := s.getString(ctx, s.respIdemKey(idempotencyKey))
			if err != nil {
				return nil, err
			}
			return s.GetResponse(ctx, record.Owner, existingID)
		}
	}

	stored, err := cloneResponseRecord(record)
	if err != nil {
		return nil, errors.Wrap(err, "clone response record")
	}
	if stored.SchemaVersion == 0 {
		stored.SchemaVersion = CurrentSchemaVersion
	}
	token, err := s.encode(stored)
	if err != nil {
		return nil, err
	}
	ttl := s.nodeTTL(stored.ExpiresAt)
	if err := s.rdb.Set(ctx, s.respKey(stored.GatewayResponseID), token, ttl).Err(); err != nil {
		return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	if err := s.indexItems(ctx, stored.Owner, append(append([]ItemEnvelope{}, stored.InputItems...), stored.OutputItems...), ttl); err != nil {
		return nil, err
	}
	return cloneResponseRecord(stored)
}

// nodeTTL derives a TTL from an absolute expiry, falling back to the configured
// default response TTL.
func (s *RedisStore) nodeTTL(expiresAt int64) time.Duration {
	if expiresAt > 0 {
		d := time.Until(time.Unix(expiresAt, 0))
		if d > 0 {
			return d
		}
		return time.Second
	}
	if s.ttl > 0 {
		return s.ttl
	}
	return 0
}

// GetResponse returns the owner's node, or ErrNotFound.
func (s *RedisStore) GetResponse(ctx context.Context, owner OwnerScope, id string) (*ResponseStateRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	token, err := s.getString(ctx, s.respKey(id))
	if err != nil {
		return nil, err
	}
	var rec ResponseStateRecord
	if err := s.decode(token, &rec); err != nil {
		return nil, err
	}
	if !rec.Owner.Matches(owner) {
		return nil, ErrNotFound
	}
	if rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt {
		return nil, ErrNotFound
	}
	return &rec, nil
}

// GetResponseBinding returns the provider binding only.
func (s *RedisStore) GetResponseBinding(ctx context.Context, owner OwnerScope, id string) (*ProviderBinding, error) {
	rec, err := s.GetResponse(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if rec.Binding == nil {
		return nil, nil
	}
	binding := *rec.Binding
	return &binding, nil
}

// DeleteResponse tombstones the node and removes its item index entries.
func (s *RedisStore) DeleteResponse(ctx context.Context, owner OwnerScope, id string) error {
	rec, err := s.GetResponse(ctx, owner, id)
	if err != nil {
		return err
	}
	if err := s.rdb.Del(ctx, s.respKey(id)).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	_ = s.rdb.Set(ctx, s.respTombKey(id), "1", s.nodeTTL(rec.ExpiresAt)).Err()
	for _, env := range append(append([]ItemEnvelope{}, rec.InputItems...), rec.OutputItems...) {
		_ = s.rdb.Del(ctx, s.itemKey(env.GatewayItemID)).Err()
		if env.UpstreamItemID != "" {
			_ = s.rdb.Del(ctx, s.itemKey(env.UpstreamItemID)).Err()
		}
	}
	return nil
}

// BatchGetResponses returns nodes in order with nil holes for missing/foreign
// nodes.
func (s *RedisStore) BatchGetResponses(ctx context.Context, owner OwnerScope, ids []string) ([]*ResponseStateRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	out := make([]*ResponseStateRecord, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = s.respKey(id)
	}
	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	for i, v := range vals {
		token, ok := v.(string)
		if !ok || token == "" {
			out[i] = nil
			continue
		}
		var rec ResponseStateRecord
		if err := s.decode(token, &rec); err != nil {
			out[i] = nil
			continue
		}
		if !rec.Owner.Matches(owner) || (rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt) {
			out[i] = nil
			continue
		}
		recCopy := rec
		out[i] = &recCopy
	}
	return out, nil
}

// --- Item index -------------------------------------------------------------

type itemIndexBlob struct {
	Owner OwnerScope   `json:"owner"`
	Env   ItemEnvelope `json:"env"`
}

func (s *RedisStore) indexItems(ctx context.Context, owner OwnerScope, items []ItemEnvelope, ttl time.Duration) error {
	for _, env := range items {
		if env.GatewayItemID == "" {
			continue
		}
		token, err := s.encode(itemIndexBlob{Owner: owner, Env: env})
		if err != nil {
			return err
		}
		if err := s.rdb.Set(ctx, s.itemKey(env.GatewayItemID), token, ttl).Err(); err != nil {
			return errors.Wrap(ErrStoreUnavailable, err.Error())
		}
		if env.UpstreamItemID != "" {
			_ = s.rdb.SetNX(ctx, s.itemKey(env.UpstreamItemID), token, ttl).Err()
		}
	}
	return nil
}

// GetItem resolves a stored item under owner scope.
func (s *RedisStore) GetItem(ctx context.Context, owner OwnerScope, itemID string) (*ItemEnvelope, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	token, err := s.getString(ctx, s.itemKey(itemID))
	if err != nil {
		return nil, err
	}
	var blob itemIndexBlob
	if err := s.decode(token, &blob); err != nil {
		return nil, err
	}
	if !blob.Owner.Matches(owner) {
		return nil, ErrNotFound
	}
	env := blob.Env
	return &env, nil
}

// --- Conversations ----------------------------------------------------------

// CreateConversation stores a new conversation record.
func (s *RedisStore) CreateConversation(ctx context.Context, record *ConversationStateRecord, idempotencyKey string) (*ConversationStateRecord, error) {
	if record == nil {
		return nil, errors.New("state: nil conversation record")
	}
	if !record.Owner.Valid() {
		return nil, ErrInvalidOwner
	}
	if s.limits.itemCountExceeded(len(record.Items)) {
		return nil, errors.Wrapf(ErrLimitExceeded, "conversation item count %d", len(record.Items))
	}

	if idempotencyKey != "" {
		ok, err := s.rdb.SetNX(ctx, s.convIdemKey(idempotencyKey), record.GatewayConversationID, s.convTTL(record.ExpiresAt)).Result()
		if err != nil {
			return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
		}
		if !ok {
			existingID, err := s.getString(ctx, s.convIdemKey(idempotencyKey))
			if err != nil {
				return nil, err
			}
			return s.GetConversation(ctx, record.Owner, existingID)
		}
	}

	stored, err := cloneConversationRecord(record)
	if err != nil {
		return nil, errors.Wrap(err, "clone conversation record")
	}
	if stored.SchemaVersion == 0 {
		stored.SchemaVersion = CurrentSchemaVersion
	}
	if err := s.writeConversation(ctx, stored); err != nil {
		return nil, err
	}
	if err := s.indexItems(ctx, stored.Owner, stored.Items, s.convTTL(stored.ExpiresAt)); err != nil {
		return nil, err
	}
	return cloneConversationRecord(stored)
}

func (s *RedisStore) convTTL(expiresAt int64) time.Duration {
	if expiresAt > 0 {
		d := time.Until(time.Unix(expiresAt, 0))
		if d > 0 {
			return d
		}
		return time.Second
	}
	return 0 // no automatic TTL (S03)
}

func (s *RedisStore) writeConversation(ctx context.Context, rec *ConversationStateRecord) error {
	token, err := s.encode(rec)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, s.convKey(rec.GatewayConversationID), token, s.convTTL(rec.ExpiresAt)).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return nil
}

// GetConversation returns the owner's conversation.
func (s *RedisStore) GetConversation(ctx context.Context, owner OwnerScope, id string) (*ConversationStateRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	token, err := s.getString(ctx, s.convKey(id))
	if err != nil {
		return nil, err
	}
	var rec ConversationStateRecord
	if err := s.decode(token, &rec); err != nil {
		return nil, err
	}
	if !rec.Owner.Matches(owner) {
		return nil, ErrNotFound
	}
	if rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt {
		return nil, ErrNotFound
	}
	return &rec, nil
}

// DeleteConversation tombstones the conversation and its items.
func (s *RedisStore) DeleteConversation(ctx context.Context, owner OwnerScope, id string) error {
	rec, err := s.GetConversation(ctx, owner, id)
	if err != nil {
		return err
	}
	if err := s.rdb.Del(ctx, s.convKey(id)).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	_ = s.rdb.Del(ctx, s.leaseKey(id)).Err()
	for _, env := range rec.Items {
		_ = s.rdb.Del(ctx, s.itemKey(env.GatewayItemID)).Err()
	}
	return nil
}

// AppendConversationItems atomically appends items and advances the version.
//
// Concurrency is serialized by the per-conversation lease the controller holds
// during a write (CON04); the version check here guards against a stale writer.
func (s *RedisStore) AppendConversationItems(ctx context.Context, owner OwnerScope, id string, expectedVersion int64, items []ItemEnvelope, idempotencyKey string) (*ConversationStateRecord, error) {
	if idempotencyKey != "" {
		ok, err := s.rdb.SetNX(ctx, s.convAppendIdemKey(id, idempotencyKey), "1", s.convAppendIdemTTL()).Result()
		if err != nil {
			return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
		}
		if !ok {
			// Already applied: return current state without a second append (S05).
			return s.GetConversation(ctx, owner, id)
		}
	}

	rec, err := s.GetConversation(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion != AnyVersion && expectedVersion != rec.Version {
		return nil, ErrVersionConflict
	}
	projected := len(rec.Items) + len(items)
	if s.limits.itemCountExceeded(projected) {
		return nil, errors.Wrapf(ErrLimitExceeded, "conversation item count %d", projected)
	}
	rec.Items = append(rec.Items, items...)
	rec.Version++
	if err := s.writeConversation(ctx, rec); err != nil {
		return nil, err
	}
	if err := s.indexItems(ctx, owner, items, s.convTTL(rec.ExpiresAt)); err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *RedisStore) convAppendIdemTTL() time.Duration {
	if s.ttl > 0 {
		return s.ttl
	}
	return 24 * time.Hour
}

// UpdateConversationMetadata updates metadata only and advances the version.
func (s *RedisStore) UpdateConversationMetadata(ctx context.Context, owner OwnerScope, id string, expectedVersion int64, metadata json.RawMessage) (*ConversationStateRecord, error) {
	rec, err := s.GetConversation(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion != AnyVersion && expectedVersion != rec.Version {
		return nil, ErrVersionConflict
	}
	rec.Metadata = cloneRaw(metadata)
	rec.Version++
	if err := s.writeConversation(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// DeleteConversationItem removes one item and advances the version.
func (s *RedisStore) DeleteConversationItem(ctx context.Context, owner OwnerScope, id, itemID string, expectedVersion int64) (*ConversationStateRecord, error) {
	rec, err := s.GetConversation(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if expectedVersion != AnyVersion && expectedVersion != rec.Version {
		return nil, ErrVersionConflict
	}
	filtered := make([]ItemEnvelope, 0, len(rec.Items))
	removed := false
	for _, env := range rec.Items {
		if env.GatewayItemID == itemID || (env.UpstreamItemID != "" && env.UpstreamItemID == itemID) {
			removed = true
			_ = s.rdb.Del(ctx, s.itemKey(env.GatewayItemID)).Err()
			continue
		}
		filtered = append(filtered, env)
	}
	if !removed {
		return nil, ErrNotFound
	}
	rec.Items = filtered
	rec.Version++
	if err := s.writeConversation(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// --- Conversation lease -----------------------------------------------------

// AcquireConversationLease grabs an exclusive lease via SET NX with a TTL, so an
// abandoned lease expires on its own (CON05).
func (s *RedisStore) AcquireConversationLease(ctx context.Context, owner OwnerScope, id string, ttl time.Duration) (string, error) {
	if _, err := s.GetConversation(ctx, owner, id); err != nil {
		return "", err
	}
	token, err := randomHex(16)
	if err != nil {
		return "", err
	}
	ok, err := s.rdb.SetNX(ctx, s.leaseKey(id), token, ttl).Result()
	if err != nil {
		return "", errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	if !ok {
		return "", ErrLeaseHeld
	}
	return token, nil
}

// RenewConversationLease extends a held lease.
func (s *RedisStore) RenewConversationLease(ctx context.Context, owner OwnerScope, id, leaseToken string, ttl time.Duration) error {
	if _, err := s.GetConversation(ctx, owner, id); err != nil {
		return err
	}
	current, err := s.getString(ctx, s.leaseKey(id))
	if err != nil {
		return ErrLeaseInvalid
	}
	if current != leaseToken {
		return ErrLeaseInvalid
	}
	if err := s.rdb.Expire(ctx, s.leaseKey(id), ttl).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return nil
}

// ReleaseConversationLease releases a held lease when the token matches.
func (s *RedisStore) ReleaseConversationLease(ctx context.Context, owner OwnerScope, id, leaseToken string) error {
	current, err := s.getString(ctx, s.leaseKey(id))
	if err == ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if current != leaseToken {
		return ErrLeaseInvalid
	}
	if err := s.rdb.Del(ctx, s.leaseKey(id)).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return nil
}

// --- Checkpoints ------------------------------------------------------------

// PutCheckpoint stores or overwrites a checkpoint.
func (s *RedisStore) PutCheckpoint(ctx context.Context, record *CheckpointRecord) error {
	if record == nil {
		return errors.New("state: nil checkpoint record")
	}
	if !record.Owner.Valid() {
		return ErrInvalidOwner
	}
	clone := cloneCheckpointRecord(record)
	if clone.SchemaVersion == 0 {
		clone.SchemaVersion = CurrentSchemaVersion
	}
	token, err := s.encode(clone)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, s.checkpointKey(record.Owner, record.Key), token, s.checkpointTTL(record.ExpiresAt)).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return nil
}

func (s *RedisStore) checkpointTTL(expiresAt int64) time.Duration {
	if expiresAt > 0 {
		d := time.Until(time.Unix(expiresAt, 0))
		if d > 0 {
			return d
		}
		return time.Second
	}
	if s.ttl > 0 {
		return s.ttl
	}
	return 0
}

// GetCheckpoint returns a checkpoint for the owner scope.
func (s *RedisStore) GetCheckpoint(ctx context.Context, owner OwnerScope, key string) (*CheckpointRecord, error) {
	if !owner.Valid() {
		return nil, ErrInvalidOwner
	}
	token, err := s.getString(ctx, s.checkpointKey(owner, key))
	if err != nil {
		return nil, err
	}
	var rec CheckpointRecord
	if err := s.decode(token, &rec); err != nil {
		return nil, err
	}
	if !rec.Owner.Matches(owner) {
		return nil, ErrNotFound
	}
	if rec.ExpiresAt > 0 && s.now().Unix() >= rec.ExpiresAt {
		return nil, ErrNotFound
	}
	return &rec, nil
}

// compile-time assertion that RedisStore satisfies the interface.
var _ ResponseStateStore = (*RedisStore)(nil)
