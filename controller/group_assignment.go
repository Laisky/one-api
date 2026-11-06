package controller

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/groupname"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// validateGroupAssignment enforces the rule that groups are created only in the
// GroupRatio settings and every other write path may only assign existing ones.
// requested holds the normalized group names a request wants the entity to
// carry and stored holds the names currently persisted on it. A requested name
// that is already stored (exact, case-sensitive match) is grandfathered, even
// when it predates the naming rule or is missing from the catalog, so editing
// an old entity for an unrelated field never fails. Every other name must pass
// groupname.Validate and be groupname.Default or an exact GroupRatio catalog key.
// Returns nil when every requested name may be assigned, or an
// errkind.InvalidRequest error naming the first rejected group.
func validateGroupAssignment(requested, stored []string) error {
	storedSet := make(map[string]struct{}, len(stored))
	for _, name := range stored {
		storedSet[name] = struct{}{}
	}
	for _, name := range requested {
		if _, ok := storedSet[name]; ok {
			continue
		}
		if err := groupname.Validate(name); err != nil {
			return errkind.InvalidRequestErr(errors.WithStack(err))
		}
		if name != groupname.Default && !ratio.HasGroup(name) {
			return errkind.InvalidRequestErr(errors.Errorf(
				"group %q is not configured; create it in Settings → Operation → Groups & Ratios first", name))
		}
	}
	return nil
}

// resolveChannelGroups normalizes a submitted channels.group list and checks it
// against the list currently stored on the channel (empty for a new channel).
// requestedCSV is the comma-separated list from the request and storedCSV the
// persisted one. Returns the normalized list in channels.group format, which is
// empty when the submission names no group so callers keep their existing
// default or no-change semantics, or a client error when a name may not be assigned.
func resolveChannelGroups(requestedCSV, storedCSV string) (string, error) {
	requested := groupname.ParseList(requestedCSV)
	if err := validateGroupAssignment(requested, groupname.ParseList(storedCSV)); err != nil {
		return "", errors.WithStack(err)
	}
	return groupname.JoinList(requested), nil
}

// normalizeChannelUpdateGroups rewrites the group list of a channel update
// payload in place after checking it against the groups stored on that channel.
// channel is the decoded update whose Id is already resolved. A submission that
// names no group is cleared to empty, so the struct-based update keeps the
// stored value exactly as an omitted field always has. Returns a client error
// when a newly assigned group may not be assigned, or a wrapped lookup error
// when the stored channel cannot be read.
func normalizeChannelUpdateGroups(channel *model.Channel) error {
	if groupname.ParseList(channel.Group) == nil {
		channel.Group = ""
		return nil
	}
	stored, err := model.GetChannelById(channel.Id, false)
	if err != nil {
		return errors.Wrap(err, "load stored channel groups")
	}
	group, err := resolveChannelGroups(channel.Group, stored.Group)
	if err != nil {
		return errors.WithStack(err)
	}
	channel.Group = group
	return nil
}

// resolveUserGroup normalizes a submitted users.group value and checks it
// against the group currently stored on the user (empty for a new user).
// requested is the group from the request and stored the persisted one.
// Returns the normalized group, which is empty when the submission is blank so
// callers keep their existing default or rejection semantics, or a client error
// when the group may not be assigned.
func resolveUserGroup(requested, stored string) (string, error) {
	name := groupname.Normalize(requested)
	if name == "" {
		return "", nil
	}
	var storedNames []string
	if storedName := groupname.Normalize(stored); storedName != "" {
		storedNames = []string{storedName}
	}
	if err := validateGroupAssignment([]string{name}, storedNames); err != nil {
		return "", errors.WithStack(err)
	}
	return name, nil
}
