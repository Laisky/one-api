package ratio

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/Laisky/zap"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/groupname"
	"github.com/Laisky/one-api/common/logger"
)

var groupRatioLock sync.RWMutex

// GroupRatio is the billing-group catalog: group name to price multiplier.
// Read it through GroupNames, HasGroup and GetGroupRatioWithContext, and replace
// it through UpdateGroupRatioByJSONString, so access stays under groupRatioLock.
var GroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

// GroupRatio2JSONString returns the current catalog encoded as a JSON object.
func GroupRatio2JSONString() string {
	groupRatioLock.RLock()
	defer groupRatioLock.RUnlock()
	jsonBytes, err := json.Marshal(GroupRatio)
	if err != nil {
		logger.Logger.Error("error marshalling group ratio", zap.Error(err))
	}
	return string(jsonBytes)
}

// parseGroupRatioJSON decodes a GroupRatio option value into a new map. It
// rejects anything that is not a JSON object of numbers, including null.
func parseGroupRatioJSON(jsonStr string) (map[string]float64, error) {
	var parsed map[string]float64
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, errors.Wrap(err, "invalid GroupRatio JSON")
	}
	if parsed == nil {
		return nil, errors.New("GroupRatio must be a JSON object")
	}
	return parsed, nil
}

// UpdateGroupRatioByJSONString replaces the catalog with the decoded jsonStr.
// The live catalog is swapped only after decoding succeeds, so a rejected value
// never empties it. It runs on startup and periodic option sync, so it accepts
// legacy names; write paths must call ValidateGroupRatioUpdate first.
// Returns an error when jsonStr is not a JSON object of numbers.
func UpdateGroupRatioByJSONString(jsonStr string) error {
	parsed, err := parseGroupRatioJSON(jsonStr)
	if err != nil {
		return errors.WithStack(err)
	}
	groupRatioLock.Lock()
	defer groupRatioLock.Unlock()
	GroupRatio = parsed
	return nil
}

// ValidateGroupRatioUpdate checks a GroupRatio value an administrator is about
// to save. It must be a JSON object without duplicate keys whose ratios are
// numbers >= 0. Keys that are new compared with the current catalog must pass
// groupname.Validate and must not differ only by case from any other key.
// Existing keys are grandfathered so legacy groups stay editable.
// Returns nil when the value may be saved, or an error safe to show to the administrator.
func ValidateGroupRatioUpdate(jsonStr string) error {
	parsed, err := parseGroupRatioJSON(jsonStr)
	if err != nil {
		return errors.WithStack(err)
	}

	keys, err := common.ScanJSONRootKeys([]byte(jsonStr))
	if err != nil {
		return errors.Wrap(err, "invalid GroupRatio JSON")
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, dup := seen[key]; dup {
			return errors.Errorf("group %q appears more than once in GroupRatio", key)
		}
		seen[key] = struct{}{}
	}

	foldOwners := make(map[string]string, len(keys))
	for _, key := range keys {
		isNew := !HasGroup(key)
		if isNew {
			if err := groupname.Validate(key); err != nil {
				return errors.WithStack(err)
			}
		}
		if ratio := parsed[key]; ratio < 0 {
			return errors.Errorf("ratio of group %q must be a number >= 0", key)
		}

		fold := groupname.FoldKey(key)
		other, clash := foldOwners[fold]
		if !clash {
			foldOwners[fold] = key
			continue
		}
		if isNew || !HasGroup(other) {
			return errors.Errorf("groups %q and %q differ only by case; group names must be unique ignoring case", other, key)
		}
	}
	return nil
}

// GroupNames returns the catalog's group names, sorted, as a new slice.
func GroupNames() []string {
	groupRatioLock.RLock()
	names := make([]string, 0, len(GroupRatio))
	for name := range GroupRatio {
		names = append(names, name)
	}
	groupRatioLock.RUnlock()
	slices.Sort(names)
	return names
}

// HasGroup reports whether name is an exact, case-sensitive catalog key.
func HasGroup(name string) bool {
	groupRatioLock.RLock()
	defer groupRatioLock.RUnlock()
	_, ok := GroupRatio[name]
	return ok
}

// GetGroupRatio returns a group ratio without request diagnostics context.
func GetGroupRatio(name string) float64 {
	return GetGroupRatioWithContext(context.Background(), name)
}

// GetGroupRatioWithContext returns a group ratio and includes request context in diagnostics.
// Parameters: ctx carries the request logger, and name identifies the group.
// Returns: the configured group ratio, or one when the group is unknown.
func GetGroupRatioWithContext(ctx context.Context, name string) float64 {
	groupRatioLock.RLock()
	defer groupRatioLock.RUnlock()
	ratio, ok := GroupRatio[name]
	if !ok {
		logger.FromContext(ctx).Error("group ratio not found", zap.String("group", name))
		return 1
	}
	return ratio
}
