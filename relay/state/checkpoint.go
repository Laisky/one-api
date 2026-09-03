package state

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"time"

	"github.com/Laisky/errors/v2"
)

// ============================================================================
// ST-012: exact transcript-checkpoint hashing and matching for stateless clients
// ============================================================================
//
// Chat Completions and Claude Messages clients always resend their full history,
// so their stateless semantics are self-sufficient: this checkpoint layer is a
// pure OPTIMIZATION. It lets a repeated stateless request reuse a provider-bound
// upstream Responses handle (avoiding a full re-render/replay of prior
// provider-bound items) when — and only when — the downstream-visible transcript
// prefix is a byte-exact, unambiguous match of a previously rendered turn.
//
// The layer ALWAYS fails open. A cache miss, an expired or deleted target, an
// ambiguous key, a store error, or a disabled feature all resolve to the same
// safe outcome: the caller performs an ordinary explicit replay of the
// client-provided transcript. Nothing here is ever required for correctness
// (Section 5.7, acceptance rows CP01-CP10). Only an explicit Responses selector
// fails closed; a stateless checkpoint never does.
//
// The key is a deterministic SHA-256 over the owner scope, client family, public
// model, provider-binding identity, and every field of the first N canonical
// messages. Encoding is length-prefixed so no delimiter collision can make two
// distinct transcripts hash equal: a one-byte content edit, a changed tool-call
// id, a different thinking signature, a different owner, or a different
// route/model each produce a different key (CP02-CP06). There is no time and no
// randomness in the key (it must be reproducible across processes; Date.now and
// rand are unavailable here regardless), so the same inputs always hash the same.
//
// SEAM (wiring is intentionally out of scope for this helper):
// Turning this optimization on in the live Chat and Claude controllers
// ADDITIONALLY requires capturing a native-Responses upstream handle
// (Binding.UpstreamResponseID) at commit time and threading it into
// RecordCheckpoint. The current Chat-mediated fallback path does not produce a
// native Responses response id, so there is nothing to bind yet. Until a
// native-Responses commit populates Binding.UpstreamResponseID, a matched
// checkpoint has no provider handle to continue from and the caller must still
// replay. This file implements the algorithm only; controller integration is a
// separate task (see ST-012 wiring note in the proposal appendix).

// checkpointHashDomain is a fixed domain-separation prefix mixed into every key
// so checkpoint digests can never collide with any other SHA-256 use in the
// codebase, and so a future encoding change can be versioned deterministically.
const checkpointHashDomain = "one-api/relay/state/checkpoint/v1"

// CheckpointMessage is an abstract, format-agnostic view of one downstream-visible
// turn. Chat Completions and Claude Messages controllers map their own message
// shapes into this canonical struct before hashing; the algorithm here is
// deliberately ignorant of any wire format. Every field participates in the hash,
// so two transcripts that differ in any of them (including a signed-thinking
// Signature or a ToolCallID) produce different checkpoint keys.
type CheckpointMessage struct {
	// Role is the canonical role of the turn (e.g. "user", "assistant", "tool").
	Role string
	// Content is the canonical, order-stable textual/structured content of the turn
	// rendered to a stable string by the caller.
	Content string
	// ToolCallID links a tool result to its originating tool call, when present.
	ToolCallID string
	// Name is the tool/function name for a tool call or tool result, when present.
	Name string
	// Signature is a provider thinking/reasoning signature (e.g. Claude signed
	// thinking) that must be preserved exactly; it is part of the hash so a
	// different signature never matches (CP02).
	Signature string
}

// CheckpointKeyAt returns the deterministic hex SHA-256 checkpoint key computed
// over the owner scope, client family, public model, provider-binding identity
// (channel id + api type + actual model), and the first prefixLen messages of
// msgs — including every field of each message. prefixLen is clamped to
// [0, len(msgs)]. The encoding is length-prefixed for every string and
// fixed-width for every integer, so no delimiter collision can make two distinct
// inputs share a key (CP02-CP06). The computation is linear in the supplied
// transcript bytes with bounded per-field allocation (PERF05).
func CheckpointKeyAt(owner OwnerScope, clientFamily, publicModel string, binding *ProviderBinding, msgs []CheckpointMessage, prefixLen int) string {
	if prefixLen < 0 {
		prefixLen = 0
	}
	if prefixLen > len(msgs) {
		prefixLen = len(msgs)
	}

	h := sha256.New()

	// Domain and schema separation.
	hashWriteString(h, checkpointHashDomain)
	hashWriteInt(h, CurrentSchemaVersion)

	// Owner scope (CP05): both user and token id bind the key to one tenant.
	hashWriteInt(h, owner.UserID)
	hashWriteInt(h, owner.TokenID)

	// Client family and public model (CP06).
	hashWriteString(h, clientFamily)
	hashWriteString(h, publicModel)

	// Provider-binding identity (CP06). A nil binding is encoded distinctly from a
	// present binding so "no binding" never collides with a zero-value binding.
	if binding == nil {
		hashWriteInt(h, 0)
	} else {
		hashWriteInt(h, 1)
		hashWriteInt(h, binding.ChannelID)
		hashWriteInt(h, binding.APIType)
		hashWriteString(h, binding.ActualModel)
	}

	// Transcript prefix. The count is hashed first so a shorter prefix can never
	// alias a longer one, then every field of every message is length-prefixed.
	hashWriteInt(h, prefixLen)
	for i := 0; i < prefixLen; i++ {
		m := msgs[i]
		hashWriteString(h, m.Role)
		hashWriteString(h, m.Content)
		hashWriteString(h, m.ToolCallID)
		hashWriteString(h, m.Name)
		hashWriteString(h, m.Signature)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// maxCheckpointProbes bounds how many message boundaries LongestCheckpointMatch
// probes. Each probe is one store round-trip plus a full-prefix hash, so this is
// the difference between a constant cost and one the caller chooses by sending a
// longer message list.
const maxCheckpointProbes = 64

// LongestCheckpointMatch computes checkpoint keys at every message boundary from
// the longest prefix down to a single message and returns the longest prefix
// whose checkpoint exists, is live, and is unambiguous (CP01). A match is only
// ever a full-prefix hash: it never keys on the last assistant text or tool-call
// id alone. It fails open in every failure mode — a missing, expired, deleted
// (ErrNotFound), or otherwise unreadable target, and any ambiguous checkpoint
// (CP07), are all skipped — so ok=false simply tells the caller to perform an
// ordinary explicit replay (CP03, CP08). It never returns an error: the
// optimization must never break correctness.
func LongestCheckpointMatch(ctx context.Context, store ResponseStateStore, owner OwnerScope, clientFamily, publicModel string, binding *ProviderBinding, msgs []CheckpointMessage) (matched *CheckpointRecord, prefixLen int, ok bool) {
	if store == nil || !owner.Valid() || len(msgs) == 0 {
		return nil, 0, false
	}

	// Longest prefix first so the first live, unambiguous hit is the longest match.
	//
	// The probe count is bounded: the loop performs one store round-trip per
	// boundary and CheckpointKeyAt re-hashes the whole prefix each time, so an
	// unbounded walk turned a single request's message count — taken verbatim from
	// the request body — into O(N) Redis round-trips and O(N^2) hashing on the
	// request thread. A continuation appends one or two messages to what the
	// gateway already stored, so a real match is always within the newest few
	// boundaries; anything older simply misses and the caller does an ordinary
	// explicit replay, which this function is documented to fail open into.
	lowest := 1
	if len(msgs) > maxCheckpointProbes {
		lowest = len(msgs) - maxCheckpointProbes + 1
	}
	for n := len(msgs); n >= lowest; n-- {
		key := CheckpointKeyAt(owner, clientFamily, publicModel, binding, msgs, n)
		rec, err := store.GetCheckpoint(ctx, owner, key)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// Miss / expired / deleted: keep trying shorter prefixes.
				continue
			}
			// A genuine store fault (unavailable, invalid owner) yields the same
			// keyed error at every boundary; stop and fail open rather than hammer it.
			return nil, 0, false
		}
		if rec == nil {
			continue
		}
		if rec.Ambiguous {
			// CP07: an ambiguous key maps to more than one continuation; the
			// optimization is disabled for it. Fall through to a shorter, unambiguous
			// prefix if one exists.
			continue
		}
		return rec, n, true
	}

	return nil, 0, false
}

// RecordCheckpoint computes the full-transcript key for msgs and stores a
// checkpoint mapping it to responseID. If a checkpoint already exists under that
// key with a DIFFERENT responseID, the stored record is marked Ambiguous and
// written back instead: an identical downstream-visible transcript that resolved
// to two distinct continuations must never be optimized (CP07 ambiguity
// detection). A repeat with the same responseID refreshes the record (and its
// TTL). A non-positive ttl stores the checkpoint without an expiry; callers that
// bind to a response node should pass a ttl no longer than that node's lifetime
// (proposal rows S04, S02) so a checkpoint never outlives its target.
func RecordCheckpoint(ctx context.Context, store ResponseStateStore, owner OwnerScope, clientFamily, publicModel string, binding *ProviderBinding, msgs []CheckpointMessage, responseID string, ttl time.Duration) error {
	if store == nil {
		return errors.New("state: nil checkpoint store")
	}
	if !owner.Valid() {
		return errors.Wrap(ErrInvalidOwner, "state: record checkpoint")
	}

	key := CheckpointKeyAt(owner, clientFamily, publicModel, binding, msgs, len(msgs))

	existing, err := store.GetCheckpoint(ctx, owner, key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return errors.Wrap(err, "state: read existing checkpoint")
	}

	if existing != nil {
		if existing.Ambiguous {
			// Already poisoned; keep it disabled and do not resurrect a single mapping.
			return nil
		}
		if existing.ResponseID != responseID {
			// CP07: same key, two distinct continuations -> mark ambiguous.
			existing.Ambiguous = true
			if err := store.PutCheckpoint(ctx, existing); err != nil {
				return errors.Wrap(err, "state: mark checkpoint ambiguous")
			}
			return nil
		}
		// Same continuation: fall through to refresh the record and its TTL.
	}

	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).Unix()
	}

	rec := &CheckpointRecord{
		SchemaVersion: CurrentSchemaVersion,
		Key:           key,
		Owner:         owner,
		ClientFamily:  clientFamily,
		PublicModel:   publicModel,
		Binding:       binding,
		ResponseID:    responseID,
		ExpiresAt:     expiresAt,
	}
	if err := store.PutCheckpoint(ctx, rec); err != nil {
		return errors.Wrap(err, "state: put checkpoint")
	}
	return nil
}

// hashWriteString mixes s into h as an 8-byte big-endian length prefix followed by
// the raw bytes, so concatenated fields are unambiguously delimited regardless of
// their content. The hash.Hash contract guarantees Write never returns an error.
func hashWriteString(h hash.Hash, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(s))
}

// hashWriteInt mixes v into h as a fixed 8-byte big-endian value so integer fields
// are self-delimiting and never collide with adjacent length-prefixed strings.
func hashWriteInt(h hash.Hash, v int) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(int64(v)))
	_, _ = h.Write(n[:])
}
