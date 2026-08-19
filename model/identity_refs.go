package model

import (
	"context"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/identity"
)

// Ref returns the user's log identity (id + uuid + username). No I/O.
func (u *User) Ref() identity.UserRef {
	if u == nil {
		return identity.UserRef{}
	}
	return identity.NewUserRef(u.Id, u.UUID, u.Username)
}

// Ref returns the token's log identity (id + uuid + name). No I/O.
func (t *Token) Ref() identity.TokenRef {
	if t == nil {
		return identity.TokenRef{}
	}
	return identity.NewTokenRef(t.Id, t.UUID, t.Name)
}

// OwnerRef returns the token owner's identity from the denormalised
// tokens.user_uuid column. The username is not stored on the token, so Name is
// left empty rather than triggering a lookup on a logging path. UserUUID is a
// *string and is legitimately NULL on rows predating the UUID backfill.
func (t *Token) OwnerRef() identity.UserRef {
	if t == nil {
		return identity.UserRef{}
	}
	return identity.NewUserRef(t.UserId, derefStr(t.UserUUID), "")
}

// Ref returns the channel's log identity (id + uuid + name). No I/O.
func (channel *Channel) Ref() identity.ChannelRef {
	if channel == nil {
		return identity.ChannelRef{}
	}
	return identity.NewChannelRef(channel.Id, channel.UUID, channel.Name)
}

// Ref returns the MCP server's log identity (id + uuid + name). No I/O.
func (s *MCPServer) Ref() identity.MCPServerRef {
	if s == nil {
		return identity.MCPServerRef{}
	}
	return identity.NewMCPServerRef(s.Id, s.UUID, s.Name)
}

// Ref returns the redemption's log identity (id + uuid + name). No I/O.
func (redemption *Redemption) Ref() identity.RedemptionRef {
	if redemption == nil {
		return identity.RedemptionRef{}
	}
	return identity.NewRedemptionRef(redemption.Id, redemption.UUID, redemption.Name)
}

// OwnerRef returns the redemption owner's identity from the denormalised
// redemptions.user_uuid column. No I/O.
func (redemption *Redemption) OwnerRef() identity.UserRef {
	if redemption == nil {
		return identity.UserRef{}
	}
	return identity.NewUserRef(redemption.UserId, derefStr(redemption.UserUUID), "")
}

// Refs returns the full identity denormalised on a consume-log row. No I/O.
//
// The Log row stores no token id, so the token reference carries uuid + name only.
func (log *Log) Refs() identity.Set {
	if log == nil {
		return identity.Set{}
	}
	return identity.Set{
		User:    identity.NewUserRef(log.UserId, derefStr(log.UserUUID), log.Username),
		Token:   identity.NewTokenRef(0, derefStr(log.TokenUUID), log.TokenName),
		Channel: identity.NewChannelRef(log.ChannelId, derefStr(log.ChannelUUID), log.ChannelName),
	}
}

// LogFields returns the log row's own reference plus the full denormalised
// identity of the user, token and channel it bills. No I/O.
//
// Parameters:
//   - extra: additional fields appended after the identity fields.
//
// Return values:
//   - []zap.Field: ready-to-log field slice.
func (log *Log) LogFields(extra ...zap.Field) []zap.Field {
	if log == nil {
		return extra
	}
	fields := log.Refs().AppendZap(identity.NewLogRef(log.Id, log.UUID).Zap())
	return append(fields, extra...)
}

// derefStr returns the pointed-to string, or "" for a nil pointer.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- fallback resolvers: LAST RESORT, background/cron/migration code only -----
//
// Contract, deliberately unusual and identical for all of them:
//   - never return an error; an unresolved reference degrades to id-only, using
//     the CANONICAL field key so an operator's `user_id=175` grep still matches;
//   - never log anything (a log inside an enrichment helper is a recursion trap);
//   - never panic: model.DB is nil before InitDB and after teardown, and a log
//     emitted then must not take the process down;
//   - consult the identity carried by ctx first, so a request-scoped call is free.
//
// BANNED on any per-request path. Request-scoped code already carries the identity
// on its logger and context; use identity.Of(ctx) there.

// LookupUserRef resolves a user reference by id, best effort. See the contract above.
//
// Parameters:
//   - ctx: request or background context; consulted before any query.
//   - id: user primary key.
//
// Return values:
//   - identity.UserRef: the fullest reference that could be resolved without failing.
func LookupUserRef(ctx context.Context, id int) identity.UserRef {
	if id <= 0 {
		return identity.UserRef{}
	}
	if s := identity.FromContext(ctx); s.User.ID == id && !s.User.IsZero() {
		return s.User
	}
	if DB == nil {
		return identity.NewUserRef(id, "", "")
	}

	var row struct {
		UUID     string
		Username string
	}
	if err := DB.Table("users").Select("uuid", "username").
		Where("id = ?", id).Limit(1).Scan(&row).Error; err != nil {
		// Deliberate: an enrichment helper must never fail a log statement.
		return identity.NewUserRef(id, "", "")
	}

	return identity.NewUserRef(id, row.UUID, row.Username)
}

// LookupTokenRef resolves a token reference by id, best effort. See the contract above.
//
// Parameters:
//   - ctx: request or background context; consulted before any query.
//   - id: token primary key.
//
// Return values:
//   - identity.TokenRef: the fullest reference that could be resolved without failing.
func LookupTokenRef(ctx context.Context, id int) identity.TokenRef {
	if id <= 0 {
		return identity.TokenRef{}
	}
	if s := identity.FromContext(ctx); s.Token.ID == id && !s.Token.IsZero() {
		return s.Token
	}
	if DB == nil {
		return identity.NewTokenRef(id, "", "")
	}

	var row struct {
		UUID string
		Name string
	}
	if err := DB.Table("tokens").Select("uuid", "name").
		Where("id = ?", id).Limit(1).Scan(&row).Error; err != nil {
		// Deliberate: an enrichment helper must never fail a log statement.
		return identity.NewTokenRef(id, "", "")
	}

	return identity.NewTokenRef(id, row.UUID, row.Name)
}

// LookupChannelRef resolves a channel reference by id, best effort. It is served
// from the in-memory channel snapshot when one exists, so it is free on any
// deployment with the channel cache built. See the contract above.
//
// Parameters:
//   - ctx: request or background context; consulted before any query.
//   - id: channel primary key.
//
// Return values:
//   - identity.ChannelRef: the fullest reference that could be resolved without failing.
func LookupChannelRef(ctx context.Context, id int) identity.ChannelRef {
	if id <= 0 {
		return identity.ChannelRef{}
	}
	if s := identity.FromContext(ctx); s.Channel.ID == id && !s.Channel.IsZero() {
		return s.Channel
	}
	if ch := cachedChannelById(id); ch != nil {
		return ch.Ref()
	}
	if DB == nil {
		return identity.NewChannelRef(id, "", "")
	}

	var row struct {
		UUID string
		Name string
	}
	if err := DB.Table("channels").Select("uuid", "name").
		Where("id = ?", id).Limit(1).Scan(&row).Error; err != nil {
		// Deliberate: an enrichment helper must never fail a log statement.
		return identity.NewChannelRef(id, "", "")
	}

	return identity.NewChannelRef(id, row.UUID, row.Name)
}

// ChannelRefsField renders a set of channel ids as ONE string array of
// "channel <name>(<uuid>)#<id>" elements, resolved from the in-memory snapshot
// only (never a query, because these sites log whole candidate lists).
//
// A single array is used rather than parallel id/uuid/name arrays so that an
// unresolvable entry cannot silently misalign the columns.
//
// Parameters:
//   - key: the zap field key (e.g. "excluded_channels").
//   - ids: channel ids, order preserved.
//
// Return values:
//   - zap.Field: zap.Strings(key, rendered); an empty ids slice yields zap.Skip().
func ChannelRefsField(key string, ids []int) zap.Field {
	if len(ids) == 0 {
		return zap.Skip()
	}

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if ch := cachedChannelById(id); ch != nil {
			out = append(out, ch.Ref().String())
			continue
		}
		out = append(out, identity.NewChannelRef(id, "", "").String())
	}

	return zap.Strings(key, out)
}
