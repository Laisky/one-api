package state

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

var (
	mu                sync.RWMutex
	active            ResponseStateStore
	enabled           bool
	shadow            bool
	legacyPassthrough bool
	allowlist         allowlistSet
)

// Enabled reports whether the gateway state layer is active. When false, one-api
// keeps its current behavior exactly (row O01).
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// ShadowMode reports whether the feature runs in shadow mode: hydration and
// portability are computed but routing and payloads are unchanged (row O02).
func ShadowMode() bool {
	mu.RLock()
	defer mu.RUnlock()
	return shadow
}

// LegacyPassthroughEnabled reports whether unknown incoming response IDs may be
// forwarded upstream on GET/DELETE/cancel (rows R08, SEC04). It is only ever
// consulted when the feature is enabled.
func LegacyPassthroughEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return legacyPassthrough
}

// Store returns the active store, or nil when the feature is disabled.
func Store() ResponseStateStore {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

// AllowedFor reports whether a given (userID, tokenID, channelID) is in scope for
// gateway state behavior under the configured allowlist (row O03). An empty
// allowlist means all identities are in scope when the feature is enabled.
func AllowedFor(userID, tokenID, channelID int) bool {
	mu.RLock()
	defer mu.RUnlock()
	if !enabled {
		return false
	}
	return allowlist.allows(userID, tokenID, channelID)
}

// LimitsFromConfig builds the state Limits from the configured knobs.
func LimitsFromConfig() Limits {
	return Limits{
		MaxChainDepth:           config.ResponseStateMaxChainDepth,
		MaxItemCount:            config.ResponseStateMaxItemCount,
		MaxRecordBytes:          config.ResponseStateMaxRecordBytes,
		MaxHydratedBytes:        config.ResponseStateMaxHydratedBytes,
		MaxHydratedTokens:       config.ResponseStateMaxHydratedTokens,
		MaxResponsesPerUser:     config.ResponseStateMaxResponsesPerUser,
		MaxConversationsPerUser: config.ResponseStateMaxConversationsPerUser,
	}
}

// ConversationIdleTTLFromConfig returns the configured sliding idle TTL for
// conversations (row L08). Zero retains conversations until explicit deletion.
func ConversationIdleTTLFromConfig() time.Duration {
	days := config.ResponseStateConversationIdleTTLDays
	if days <= 0 {
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// ResponseTTLFromConfig returns the configured default response node TTL.
func ResponseTTLFromConfig() time.Duration {
	days := config.ResponseStateResponseTTLDays
	if days <= 0 {
		return DefaultResponseTTL
	}
	return time.Duration(days) * 24 * time.Hour
}

// responseStateEnabledEnv is the environment variable that turns the feature on
// or off explicitly. When unset, Init auto-enables the feature if a stable
// encryption key and a healthy Redis are both present.
const responseStateEnabledEnv = "RESPONSE_STATE_ENABLED"

// autoEnable reports whether the feature should be turned on by default. An
// operator who did not set RESPONSE_STATE_ENABLED explicitly gets the feature
// automatically once both prerequisites — a stable encryption key and a healthy
// Redis — are present. An explicit setting always wins over this default.
func autoEnable(explicitlySet, keyPresent, redisReady bool) bool {
	return !explicitlySet && keyPresent && redisReady
}

// encryptionKeyMaterialPresent reports whether a stable encryption key is
// available: either RESPONSE_STATE_ENCRYPTION_KEYS is configured, or SESSION_SECRET
// was set explicitly by the operator (config.SessionSecretEnvValue is the raw env
// value, empty when auto-generated per boot). An auto-generated secret is NOT
// stable and never counts (Section 5.4).
func encryptionKeyMaterialPresent() bool {
	return strings.TrimSpace(config.ResponseStateEncryptionKeys) != "" ||
		strings.TrimSpace(config.SessionSecretEnvValue) != ""
}

// resolveKeyRing builds the key ring from the strongest available source:
// RESPONSE_STATE_ENCRYPTION_KEYS if set, otherwise an explicitly configured
// SESSION_SECRET. It returns an error when neither is present so the feature can
// never silently enable without encryption.
func resolveKeyRing() (*KeyRing, error) {
	if spec := strings.TrimSpace(config.ResponseStateEncryptionKeys); spec != "" {
		return ParseKeyRing(spec)
	}
	if secret := strings.TrimSpace(config.SessionSecretEnvValue); secret != "" {
		return DeriveKeyRingFromSecret(secret)
	}
	return nil, errors.New("state: no encryption key material (set RESPONSE_STATE_ENCRYPTION_KEYS or an explicit SESSION_SECRET)")
}

// disabledReason returns a short, content-free explanation of why the feature is
// off, for the startup log.
func disabledReason(explicitlySet, keyPresent, redisReady bool) string {
	if explicitlySet {
		return "RESPONSE_STATE_ENABLED explicitly set to false"
	}
	switch {
	case !keyPresent && !redisReady:
		return "auto-enable prerequisites missing: an encryption key (RESPONSE_STATE_ENCRYPTION_KEYS or explicit SESSION_SECRET) and Redis (REDIS_CONN_STRING)"
	case !keyPresent:
		return "auto-enable prerequisite missing: an encryption key (RESPONSE_STATE_ENCRYPTION_KEYS or explicit SESSION_SECRET)"
	case !redisReady:
		return "auto-enable prerequisite missing: Redis (REDIS_CONN_STRING)"
	default:
		return "disabled by configuration"
	}
}

// Init wires the production store from configuration. It is a startup step and
// enforces the non-negotiable gate from Section 5.4: the feature can only enable
// when Redis is configured and healthy AND a stable encryption key is present. It
// never degrades to an in-process store in production; enabling without those
// prerequisites is a startup error.
//
// When RESPONSE_STATE_ENABLED is not set explicitly, the feature auto-enables as
// soon as both prerequisites are present (a deliberately configured encryption
// key plus Redis). Init always emits one INFO line describing the resolved
// enabled/disabled state and the reason, whether or not the feature turns on.
func Init() error {
	keyPresent := encryptionKeyMaterialPresent()
	redisReady := common.IsRedisEnabled() && common.RDB != nil
	_, explicitlySet := os.LookupEnv(responseStateEnabledEnv)

	if autoEnable(explicitlySet, keyPresent, redisReady) {
		config.ResponseStateEnabled = true
	}

	err := initWithStore(func(ring *KeyRing) (ResponseStateStore, error) {
		if !common.IsRedisEnabled() || common.RDB == nil {
			return nil, errors.New("RESPONSE_STATE_ENABLED requires Redis (set REDIS_CONN_STRING); refusing to enable with an in-process store")
		}
		store, err := NewRedisStore(common.RDB, ring, LimitsFromConfig(), ResponseTTLFromConfig())
		if err != nil {
			return nil, errors.Wrap(err, "build redis state store")
		}
		store.SetConversationIdleTTL(ConversationIdleTTLFromConfig())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			return nil, errors.Wrap(err, "state store health check failed")
		}
		return store, nil
	})

	logInitStatus(err, explicitlySet, keyPresent, redisReady)
	if err != nil {
		return errors.Wrap(err, "initialize gateway response state")
	}
	return nil
}

// logInitStatus emits exactly one INFO line describing whether the gateway
// response-state feature is enabled and why, so operators can confirm the
// resolved state at boot. It logs only bounded, non-secret fields.
func logInitStatus(initErr error, explicitlySet, keyPresent, redisReady bool) {
	if logger.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.Bool("encryption_key_present", keyPresent),
		zap.Bool("redis_ready", redisReady),
		zap.Bool("explicitly_set", explicitlySet),
	}
	switch {
	case initErr != nil:
		logger.Logger.Info("gateway response state DISABLED: initialization error",
			append(fields, zap.Error(initErr))...)
	case Enabled():
		logger.Logger.Info("gateway response state ENABLED",
			append(fields,
				zap.Bool("shadow_mode", ShadowMode()),
				zap.Bool("legacy_passthrough", LegacyPassthroughEnabled()))...)
	default:
		logger.Logger.Info("gateway response state DISABLED",
			append(fields, zap.String("reason", disabledReason(explicitlySet, keyPresent, redisReady)))...)
	}
}

// initWithStore performs the common gating and installs a store built by build.
// It is separated so tests can substitute a store builder without a live Redis.
func initWithStore(build func(ring *KeyRing) (ResponseStateStore, error)) error {
	mu.Lock()
	defer mu.Unlock()

	active = nil
	enabled = false
	shadow = false
	legacyPassthrough = false
	allowlist = allowlistSet{}

	if !config.ResponseStateEnabled {
		return nil
	}

	ring, err := resolveKeyRing()
	if err != nil {
		return errors.Wrap(err, "RESPONSE_STATE_ENABLED requires a stable encryption key (RESPONSE_STATE_ENCRYPTION_KEYS or an explicit SESSION_SECRET)")
	}

	store, err := build(ring)
	if err != nil {
		return errors.Wrap(err, "build state store")
	}

	active = store
	enabled = true
	shadow = config.ResponseStateShadow
	legacyPassthrough = config.ResponseStateLegacyPassthrough
	allowlist = parseAllowlist(config.ResponseStateAllowlist)
	return nil
}

// SetForTest installs a store and toggles for tests. It is only meant for tests
// in other packages that exercise the state-aware paths. Passing nil disables the
// feature and clears the store. When a store is installed the allowlist defaults
// to allow-all unless an option overrides it.
func SetForTest(store ResponseStateStore, opts ...Option) {
	mu.Lock()
	defer mu.Unlock()
	active = store
	enabled = store != nil
	shadow = false
	legacyPassthrough = false
	allowlist = allowlistSet{empty: true}
	for _, opt := range opts {
		opt()
	}
}

// Option configures test-only toggles via SetForTest.
type Option func()

// WithShadow toggles shadow mode for tests.
func WithShadow(on bool) Option { return func() { shadow = on } }

// WithLegacyPassthrough toggles legacy passthrough for tests.
func WithLegacyPassthrough(on bool) Option { return func() { legacyPassthrough = on } }

// allowlistSet holds the parsed allowlist scopes.
type allowlistSet struct {
	users    map[int]struct{}
	tokens   map[int]struct{}
	channels map[int]struct{}
	empty    bool
}

func (a allowlistSet) allows(userID, tokenID, channelID int) bool {
	if a.empty {
		return true
	}
	if _, ok := a.users[userID]; ok {
		return true
	}
	if _, ok := a.tokens[tokenID]; ok {
		return true
	}
	if _, ok := a.channels[channelID]; ok {
		return true
	}
	return false
}

// parseAllowlist parses "user:1,token:2,channel:3" style specs. A bare number is
// treated as a user ID for convenience.
func parseAllowlist(spec string) allowlistSet {
	set := allowlistSet{
		users:    map[int]struct{}{},
		tokens:   map[int]struct{}{},
		channels: map[int]struct{}{},
	}
	fields := strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		set.empty = true
		return set
	}
	for _, field := range fields {
		kind, value := "user", field
		if sep := strings.IndexByte(field, ':'); sep > 0 {
			kind = strings.ToLower(field[:sep])
			value = field[sep+1:]
		}
		id, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		switch kind {
		case "token":
			set.tokens[id] = struct{}{}
		case "channel":
			set.channels[id] = struct{}{}
		default:
			set.users[id] = struct{}{}
		}
	}
	return set
}
