package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
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
	rdb         redis.Cmdable
	ring        *KeyRing
	limits      Limits
	ttl         time.Duration
	convIdleTTL time.Duration
	ns          string
	clock       func() time.Time
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

// SetConversationIdleTTL configures the sliding idle time-to-live applied to
// conversations (row L08). Zero retains conversations until explicit deletion
// (today's S03 default).
func (s *RedisStore) SetConversationIdleTTL(ttl time.Duration) { s.convIdleTTL = ttl }

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
func (s *RedisStore) userRespZKey(userID int) string {
	return s.ns + ":ucap:resp:" + strconv.Itoa(userID)
}
func (s *RedisStore) userConvZKey(userID int) string {
	return s.ns + ":ucap:conv:" + strconv.Itoa(userID)
}
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
	if s.limits.RecordBytesExceeded(len(data)) {
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
	if errors.Is(err, redis.Nil) {
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
	if s.limits.ItemCountExceeded(count) {
		return nil, errors.Wrapf(ErrLimitExceeded, "response item count %d", count)
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
	// Write the record and its item index BEFORE claiming the idempotency marker.
	// If the process crashes between the two, a retry never finds a marker pointing
	// at a missing record, so it cannot be stranded on ErrNotFound (ST-018). Since a
	// retried commit re-uses the same content, a re-write is a harmless no-op.
	if err := s.rdb.Set(ctx, s.respKey(stored.GatewayResponseID), token, ttl).Err(); err != nil {
		return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	if err := s.indexItems(ctx, stored.Owner, append(append([]ItemEnvelope{}, stored.InputItems...), stored.OutputItems...), ttl); err != nil {
		return nil, err
	}
	if idempotencyKey != "" {
		// Claim the marker; a loser reads the winner (whose record is already
		// present because it was written before its own marker).
		ok, err := s.rdb.SetNX(ctx, s.respIdemKey(idempotencyKey), stored.GatewayResponseID, ttl).Result()
		if err != nil {
			return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
		}
		if !ok {
			existingID, err := s.getString(ctx, s.respIdemKey(idempotencyKey))
			if err != nil {
				return nil, err
			}
			if existingID != stored.GatewayResponseID {
				return s.GetResponse(ctx, record.Owner, existingID)
			}
		}
	}
	// Track against the owner's per-user budget and prune the oldest records on
	// overflow (TTL+LRU, row L06).
	s.trackAndEvictUserResponse(ctx, stored, ttl)
	return cloneResponseRecord(stored)
}

// trackAndEvictUserResponse records a stored response in its owner's per-user
// ZSET (scored by creation time) and evicts the oldest records once the cap is
// exceeded. A non-positive cap disables the accounting so behavior is unchanged
// when the feature is off (row L05). Failures are best-effort: the record is
// already durably committed, so a governance hiccup must not fail the request.
func (s *RedisStore) trackAndEvictUserResponse(ctx context.Context, rec *ResponseStateRecord, ttl time.Duration) {
	limit := s.limits.MaxResponsesPerUser
	if limit <= 0 {
		return
	}
	zkey := s.userRespZKey(rec.Owner.UserID)
	_ = s.rdb.ZAdd(ctx, zkey, &redis.Z{Score: float64(rec.CreatedAt), Member: rec.GatewayResponseID}).Err()
	// Keep the index alive at least as long as the newest record it tracks.
	if ttl > 0 {
		_ = s.rdb.Expire(ctx, zkey, ttl+time.Hour).Err()
	}
	count, err := s.rdb.ZCard(ctx, zkey).Result()
	if err != nil || int(count) <= limit {
		return
	}
	overflow := int(count) - limit
	oldest, err := s.rdb.ZRange(ctx, zkey, 0, int64(overflow-1)).Result()
	if err != nil {
		return
	}
	for _, oid := range oldest {
		s.evictResponse(ctx, oid, zkey)
	}
}

// evictResponse removes a response node during LRU pruning: it purges the record,
// its item indexes (gateway and upstream), tombstones the id, and drops it from
// the per-user ZSET (rows L06, S06).
func (s *RedisStore) evictResponse(ctx context.Context, id, zkey string) {
	if token, err := s.getString(ctx, s.respKey(id)); err == nil {
		var rec ResponseStateRecord
		if s.decode(token, &rec) == nil {
			s.purgeResponse(ctx, id, &rec)
		} else {
			_ = s.rdb.Del(ctx, s.respKey(id)).Err()
		}
	}
	// Tombstone even if the record already TTL'd out, so the id stays deleted.
	tombTTL := s.ttl
	if tombTTL <= 0 {
		tombTTL = DefaultResponseTTL
	}
	_ = s.rdb.Set(ctx, s.respTombKey(id), "1", tombTTL).Err()
	_ = s.rdb.ZRem(ctx, zkey, id).Err()
}

// purgeResponse deletes a response record key, tombstones it, and removes its
// item index entries (both gateway and upstream ids). Shared by DeleteResponse
// and eviction so the two paths behave identically (ST-018 backend parity).
func (s *RedisStore) purgeResponse(ctx context.Context, id string, rec *ResponseStateRecord) {
	_ = s.rdb.Del(ctx, s.respKey(id)).Err()
	_ = s.rdb.Set(ctx, s.respTombKey(id), "1", s.nodeTTL(rec.ExpiresAt)).Err()
	for _, env := range append(append([]ItemEnvelope{}, rec.InputItems...), rec.OutputItems...) {
		_ = s.rdb.Del(ctx, s.itemKey(env.GatewayItemID)).Err()
		if env.UpstreamItemID != "" {
			_ = s.rdb.Del(ctx, s.itemKey(env.UpstreamItemID)).Err()
		}
	}
}

// ResponseTombstoned reports whether a response id was explicitly deleted or
// LRU-evicted (row S06, ST-018).
func (s *RedisStore) ResponseTombstoned(ctx context.Context, id string) (bool, error) {
	n, err := s.rdb.Exists(ctx, s.respTombKey(id)).Result()
	if err != nil {
		return false, errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return n > 0, nil
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
	s.purgeResponse(ctx, id, rec)
	if s.limits.MaxResponsesPerUser > 0 {
		_ = s.rdb.ZRem(ctx, s.userRespZKey(owner.UserID), id).Err()
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
	if s.limits.ItemCountExceeded(len(record.Items)) {
		return nil, errors.Wrapf(ErrLimitExceeded, "conversation item count %d", len(record.Items))
	}

	if idempotencyKey != "" {
		ok, err := s.rdb.SetNX(ctx, s.convIdemKey(idempotencyKey), record.GatewayConversationID, s.convKeyTTL(record)).Result()
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

	// Enforce the per-user active-conversation cap before writing. Idle-expired
	// members are pruned first so the count reflects only live conversations; on
	// overflow the create fails explicitly (row L07) — never silently evicted.
	if s.limits.MaxConversationsPerUser > 0 {
		zkey := s.userConvZKey(record.Owner.UserID)
		if s.convIdleTTL > 0 {
			cutoff := s.now().Add(-s.convIdleTTL).Unix()
			_ = s.rdb.ZRemRangeByScore(ctx, zkey, "0", strconv.FormatInt(cutoff, 10)).Err()
		}
		count, err := s.rdb.ZCard(ctx, zkey).Result()
		if err != nil {
			return nil, errors.Wrap(ErrStoreUnavailable, err.Error())
		}
		if int(count) >= s.limits.MaxConversationsPerUser {
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
	if err := s.writeConversation(ctx, stored); err != nil {
		return nil, err
	}
	if err := s.indexItems(ctx, stored.Owner, stored.Items, s.convKeyTTL(stored)); err != nil {
		return nil, err
	}
	s.trackConversationActivity(ctx, stored.Owner.UserID, stored.GatewayConversationID)
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

// convKeyTTL is the effective TTL for a conversation's keys. A configured idle
// TTL takes precedence and is what makes an abandoned conversation expire (row
// L08); otherwise an explicit ExpiresAt is honored, and a zero means no automatic
// TTL (S03 default).
func (s *RedisStore) convKeyTTL(rec *ConversationStateRecord) time.Duration {
	if s.convIdleTTL > 0 {
		return s.convIdleTTL
	}
	return s.convTTL(rec.ExpiresAt)
}

func (s *RedisStore) writeConversation(ctx context.Context, rec *ConversationStateRecord) error {
	token, err := s.encode(rec)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, s.convKey(rec.GatewayConversationID), token, s.convKeyTTL(rec)).Err(); err != nil {
		return errors.Wrap(ErrStoreUnavailable, err.Error())
	}
	return nil
}

// trackConversationActivity records a conversation's last-activity time in its
// owner's per-user ZSET, so the L07 cap can prune idle entries. When an idle TTL
// is configured the index is bounded to it; otherwise it persists (conversations
// are retained until explicit deletion, S03). No-op when the cap is disabled.
func (s *RedisStore) trackConversationActivity(ctx context.Context, userID int, id string) {
	if s.limits.MaxConversationsPerUser <= 0 {
		return
	}
	zkey := s.userConvZKey(userID)
	_ = s.rdb.ZAdd(ctx, zkey, &redis.Z{Score: float64(s.now().Unix()), Member: id}).Err()
	if s.convIdleTTL > 0 {
		_ = s.rdb.Expire(ctx, zkey, s.convIdleTTL+time.Hour).Err()
	}
}

// touchConversation slides a conversation's idle TTL forward on read/write and
// refreshes its activity timestamp (row L08). No-op on the key TTL when idle TTL
// is disabled.
func (s *RedisStore) touchConversation(ctx context.Context, userID int, id string) {
	if s.convIdleTTL > 0 {
		_ = s.rdb.Expire(ctx, s.convKey(id), s.convIdleTTL).Err()
	}
	s.trackConversationActivity(ctx, userID, id)
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
	// Reading is activity: slide the idle TTL forward (row L08).
	s.touchConversation(ctx, owner.UserID, id)
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
	// Purge both gateway-id and upstream-id item index entries (ST-018 parity).
	for _, env := range rec.Items {
		_ = s.rdb.Del(ctx, s.itemKey(env.GatewayItemID)).Err()
		if env.UpstreamItemID != "" {
			_ = s.rdb.Del(ctx, s.itemKey(env.UpstreamItemID)).Err()
		}
	}
	if s.limits.MaxConversationsPerUser > 0 {
		_ = s.rdb.ZRem(ctx, s.userConvZKey(owner.UserID), id).Err()
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
	if s.limits.ItemCountExceeded(projected) {
		return nil, errors.Wrapf(ErrLimitExceeded, "conversation item count %d", projected)
	}
	rec.Items = append(rec.Items, items...)
	rec.Version++
	if err := s.writeConversation(ctx, rec); err != nil {
		return nil, err
	}
	if err := s.indexItems(ctx, owner, items, s.convKeyTTL(rec)); err != nil {
		return nil, err
	}
	// Appending is activity: slide the idle TTL forward (row L08).
	s.touchConversation(ctx, owner.UserID, id)
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
			// Purge both gateway-id and upstream-id index entries (ST-018 parity).
			_ = s.rdb.Del(ctx, s.itemKey(env.GatewayItemID)).Err()
			if env.UpstreamItemID != "" {
				_ = s.rdb.Del(ctx, s.itemKey(env.UpstreamItemID)).Err()
			}
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
	s.touchConversation(ctx, owner.UserID, id)
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
	if errors.Is(err, ErrNotFound) {
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
