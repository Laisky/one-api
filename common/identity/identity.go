// Package identity carries small, self-describing references to core entities
// (user, token, channel, ...) so that every log line and every error reports the
// external UUID and the human-readable name next to the internal integer id.
//
// Why: the management API no longer exposes integer ids to operators — the web UI
// lists users, tokens and channels by uuid+name — and common/idresolve.Resolve
// rejects any reference that does not look like a UUID. An operator holding
// "user_id=175" from a server log therefore has no product path back to that user.
// Logs must carry uuid and name too.
//
// IMPORT RULE: this package must stay near-leaf. It may import gin, zap, gmw and
// common/ctxkey only. It must NEVER import one-api/model: model imports this
// package, so the reverse edge is an import cycle.
package identity

import (
	"context"
	"fmt"
	"strings"

	"github.com/Laisky/zap"
)

// Ref is the behaviour shared by every entity reference, so that Tag and Set can
// hold heterogeneous references without reflection.
type Ref interface {
	// AppendZap appends this reference's non-empty fields to dst and returns dst.
	AppendZap(dst []zap.Field) []zap.Field
	// IsZero reports whether the reference carries no usable identity at all.
	IsZero() bool
}

// clean normalises a uuid/name read from the database.
//
// TrimSpace is load-bearing, not cosmetic: uuid columns are declared
// `gorm:"type:char(36)"` (model/user.go, model/token.go, model/channel.go) and
// PostgreSQL returns bpchar space-padded on the wire, so an empty uuid arrives as
// 36 spaces. Do not remove this as "redundant".
//
// Parameters:
//   - s: raw column value.
//
// Return values:
//   - string: value with surrounding whitespace removed.
func clean(s string) string { return strings.TrimSpace(s) }

// appendID appends an int field only when the id is meaningful (> 0).
func appendID(dst []zap.Field, key string, id int) []zap.Field {
	if id > 0 {
		dst = append(dst, zap.Int(key, id))
	}
	return dst
}

// appendStr appends a string field only when it is non-empty.
func appendStr(dst []zap.Field, key, value string) []zap.Field {
	if value != "" {
		dst = append(dst, zap.String(key, value))
	}
	return dst
}

// UserRef identifies a user by internal id, external UUID and username.
type UserRef struct {
	ID   int
	UUID string
	Name string // Username. Never DisplayName, never Email.
}

// NewUserRef builds a UserRef, normalising padded or blank database values.
//
// Parameters:
//   - id: internal primary key.
//   - uuid: external UUID as displayed by the web UI.
//   - username: login name; the only user filter accepted by /api/log/.
//
// Return values:
//   - UserRef: the normalised reference.
func NewUserRef(id int, uuid, username string) UserRef {
	return UserRef{ID: id, UUID: clean(uuid), Name: clean(username)}
}

// IsZero reports whether the reference carries no usable identity.
func (r UserRef) IsZero() bool { return r.ID == 0 && r.UUID == "" && r.Name == "" }

// AppendZap appends user_id plus, when known, user_uuid and username.
func (r UserRef) AppendZap(dst []zap.Field) []zap.Field {
	dst = appendID(dst, "user_id", r.ID)
	dst = appendStr(dst, "user_uuid", r.UUID)
	return appendStr(dst, "username", r.Name)
}

// Zap returns the reference as a standalone field slice.
func (r UserRef) Zap() []zap.Field { return r.AppendZap(nil) }

// String renders the reference for operator-facing text (notification bodies,
// admin audit notes). Never use it in an API-visible error message.
func (r UserRef) String() string { return render("user", r.ID, r.UUID, r.Name) }

// TokenRef identifies an API token by internal id, external UUID and name.
type TokenRef struct {
	ID   int
	UUID string
	Name string
}

// NewTokenRef builds a TokenRef, normalising padded or blank database values.
//
// Parameters:
//   - id: internal primary key.
//   - uuid: external UUID as displayed by the web UI.
//   - name: token label.
//
// Return values:
//   - TokenRef: the normalised reference.
func NewTokenRef(id int, uuid, name string) TokenRef {
	return TokenRef{ID: id, UUID: clean(uuid), Name: clean(name)}
}

// IsZero reports whether the reference carries no usable identity.
func (r TokenRef) IsZero() bool { return r.ID == 0 && r.UUID == "" && r.Name == "" }

// AppendZap appends token_id plus, when known, token_uuid and token_name.
func (r TokenRef) AppendZap(dst []zap.Field) []zap.Field {
	dst = appendID(dst, "token_id", r.ID)
	dst = appendStr(dst, "token_uuid", r.UUID)
	return appendStr(dst, "token_name", r.Name)
}

// Zap returns the reference as a standalone field slice.
func (r TokenRef) Zap() []zap.Field { return r.AppendZap(nil) }

// String renders the reference for operator-facing text.
func (r TokenRef) String() string { return render("token", r.ID, r.UUID, r.Name) }

// ChannelRef identifies an upstream channel by internal id, external UUID and name.
//
// Channel identity is OPERATOR-ONLY. Never render a ChannelRef into a message that
// is returned to an API client.
type ChannelRef struct {
	ID   int
	UUID string
	Name string
}

// NewChannelRef builds a ChannelRef, normalising padded or blank database values.
//
// Parameters:
//   - id: internal primary key.
//   - uuid: external UUID as displayed by the web UI.
//   - name: channel label.
//
// Return values:
//   - ChannelRef: the normalised reference.
func NewChannelRef(id int, uuid, name string) ChannelRef {
	return ChannelRef{ID: id, UUID: clean(uuid), Name: clean(name)}
}

// IsZero reports whether the reference carries no usable identity.
func (r ChannelRef) IsZero() bool { return r.ID == 0 && r.UUID == "" && r.Name == "" }

// AppendZap appends channel_id plus, when known, channel_uuid and channel_name.
func (r ChannelRef) AppendZap(dst []zap.Field) []zap.Field {
	dst = appendID(dst, "channel_id", r.ID)
	dst = appendStr(dst, "channel_uuid", r.UUID)
	return appendStr(dst, "channel_name", r.Name)
}

// Zap returns the reference as a standalone field slice.
func (r ChannelRef) Zap() []zap.Field { return r.AppendZap(nil) }

// String renders the reference for operator-facing text.
func (r ChannelRef) String() string { return render("channel", r.ID, r.UUID, r.Name) }

// LogRef identifies a billing/consume log row.
type LogRef struct {
	ID   int
	UUID string
}

// NewLogRef builds a LogRef, normalising padded or blank database values.
//
// Parameters:
//   - id: internal primary key.
//   - uuid: external UUID of the log row.
//
// Return values:
//   - LogRef: the normalised reference.
func NewLogRef(id int, uuid string) LogRef { return LogRef{ID: id, UUID: clean(uuid)} }

// IsZero reports whether the reference carries no usable identity.
func (r LogRef) IsZero() bool { return r.ID == 0 && r.UUID == "" }

// AppendZap appends log_id plus, when known, log_uuid.
func (r LogRef) AppendZap(dst []zap.Field) []zap.Field {
	dst = appendID(dst, "log_id", r.ID)
	return appendStr(dst, "log_uuid", r.UUID)
}

// Zap returns the reference as a standalone field slice.
func (r LogRef) Zap() []zap.Field { return r.AppendZap(nil) }

// String renders the reference for operator-facing text.
func (r LogRef) String() string { return render("log", r.ID, r.UUID, "") }

// MCPServerRef identifies an MCP server.
type MCPServerRef struct {
	ID   int
	UUID string
	Name string
}

// NewMCPServerRef builds an MCPServerRef, normalising padded or blank values.
//
// Parameters:
//   - id: internal primary key.
//   - uuid: external UUID of the MCP server.
//   - name: server label.
//
// Return values:
//   - MCPServerRef: the normalised reference.
func NewMCPServerRef(id int, uuid, name string) MCPServerRef {
	return MCPServerRef{ID: id, UUID: clean(uuid), Name: clean(name)}
}

// IsZero reports whether the reference carries no usable identity.
func (r MCPServerRef) IsZero() bool { return r.ID == 0 && r.UUID == "" && r.Name == "" }

// AppendZap appends mcp_server_id plus, when known, mcp_server_uuid and name.
func (r MCPServerRef) AppendZap(dst []zap.Field) []zap.Field {
	dst = appendID(dst, "mcp_server_id", r.ID)
	dst = appendStr(dst, "mcp_server_uuid", r.UUID)
	return appendStr(dst, "mcp_server_name", r.Name)
}

// Zap returns the reference as a standalone field slice.
func (r MCPServerRef) Zap() []zap.Field { return r.AppendZap(nil) }

// String renders the reference for operator-facing text.
func (r MCPServerRef) String() string { return render("mcp_server", r.ID, r.UUID, r.Name) }

// RedemptionRef identifies a redemption code row.
type RedemptionRef struct {
	ID   int
	UUID string
	Name string
}

// NewRedemptionRef builds a RedemptionRef, normalising padded or blank values.
//
// Parameters:
//   - id: internal primary key.
//   - uuid: external UUID of the redemption row.
//   - name: redemption label.
//
// Return values:
//   - RedemptionRef: the normalised reference.
func NewRedemptionRef(id int, uuid, name string) RedemptionRef {
	return RedemptionRef{ID: id, UUID: clean(uuid), Name: clean(name)}
}

// IsZero reports whether the reference carries no usable identity.
func (r RedemptionRef) IsZero() bool { return r.ID == 0 && r.UUID == "" && r.Name == "" }

// AppendZap appends redemption_id plus, when known, uuid and name.
func (r RedemptionRef) AppendZap(dst []zap.Field) []zap.Field {
	dst = appendID(dst, "redemption_id", r.ID)
	dst = appendStr(dst, "redemption_uuid", r.UUID)
	return appendStr(dst, "redemption_name", r.Name)
}

// Zap returns the reference as a standalone field slice.
func (r RedemptionRef) Zap() []zap.Field { return r.AppendZap(nil) }

// String renders the reference for operator-facing text.
func (r RedemptionRef) String() string { return render("redemption", r.ID, r.UUID, r.Name) }

// render formats "<kind> <name>(<uuid>)#<id>", omitting unknown parts.
func render(kind string, id int, uuid, name string) string {
	var b strings.Builder
	b.WriteString(kind)
	if name != "" {
		b.WriteString(" ")
		b.WriteString(name)
	}
	if uuid != "" {
		b.WriteString("(")
		b.WriteString(uuid)
		b.WriteString(")")
	}
	if id != 0 {
		fmt.Fprintf(&b, "#%d", id)
	}
	return b.String()
}

// Set is the full identity of a request: who called, with which token, served by
// which channel. It is a value type; copying it is cheap and safe to hand to a
// detached goroutine.
type Set struct {
	User    UserRef
	Token   TokenRef
	Channel ChannelRef
}

// AppendZap appends every known identity field of the set.
func (s Set) AppendZap(dst []zap.Field) []zap.Field {
	dst = s.User.AppendZap(dst)
	dst = s.Token.AppendZap(dst)
	return s.Channel.AppendZap(dst)
}

// Zap returns the set as a standalone field slice.
func (s Set) Zap() []zap.Field { return s.AppendZap(make([]zap.Field, 0, 9)) }

// IsZero reports whether nothing at all is known.
func (s Set) IsZero() bool { return s.User.IsZero() && s.Token.IsZero() && s.Channel.IsZero() }

// Merge returns s with every non-zero sub-reference of other applied ON TOP.
//
// Override (not fill-if-missing) is required so that a relay retry, which rebinds
// a different channel, replaces the previous channel rather than keeping the
// stale one.
//
// Parameters:
//   - other: newly learned identity.
//
// Return values:
//   - Set: the merged identity.
func (s Set) Merge(other Set) Set {
	if !other.User.IsZero() {
		s.User = other.User
	}
	if !other.Token.IsZero() {
		s.Token = other.Token
	}
	if !other.Channel.IsZero() {
		s.Channel = other.Channel
	}
	return s
}

type ctxKeySet struct{}

// NewContext returns a context carrying the identity set. Use it when detaching
// work from the gin context so the detached code keeps name+uuid with no lookup.
//
// Parameters:
//   - ctx: parent context; nil is treated as context.Background().
//   - s: identity to attach.
//
// Return values:
//   - context.Context: derived context carrying s.
func NewContext(ctx context.Context, s Set) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeySet{}, s)
}

// FromContext returns the identity set carried by ctx, or the zero Set. It never
// panics and never performs I/O. A *gin.Context is read straight off the gin
// keys, so it works even before Bind has run.
//
// Parameters:
//   - ctx: any context; nil yields the zero Set.
//
// Return values:
//   - Set: whatever identity ctx carries.
func FromContext(ctx context.Context) Set {
	if ctx == nil {
		return Set{}
	}
	if s, ok := currentFromGinContext(ctx); ok {
		return s
	}
	if s, ok := ctx.Value(ctxKeySet{}).(Set); ok {
		return s
	}
	return Set{}
}

// Of is the one-liner for a call site that already holds a context:
//
//	lg.Warn("upstream failed", append(identity.Of(ctx), zap.Error(err))...)
//
// Parameters:
//   - ctx: any context; nil yields no fields.
//
// Return values:
//   - []zap.Field: the identity fields carried by ctx.
func Of(ctx context.Context) []zap.Field { return FromContext(ctx).Zap() }
