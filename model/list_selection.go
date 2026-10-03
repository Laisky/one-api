package model

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

// MaxListSelection bounds selection snapshots and SQL parameters; oversized sets fail rather than being truncated.
const MaxListSelection = 10000

// ListSelection is an explicit UUID set or the current filtered universe minus excluded UUIDs.
// Empty or ambiguous payloads are never interpreted as permission to act on every row.
type ListSelection struct {
	Mode        string   `json:"mode"`
	IDs         []string `json:"ids,omitempty"`
	ExcludedIDs []string `json:"excluded_ids,omitempty"`
}

// Normalize validates the selection mode and canonicalizes and deduplicates UUIDs without database access.
func (selection ListSelection) Normalize() (ListSelection, error) {
	if selection.Mode != "ids" && selection.Mode != "all_matching" {
		return ListSelection{}, errkind.InvalidRequestErr(errors.New("Select rows before performing a batch action."))
	}
	if selection.Mode == "ids" && (len(selection.IDs) == 0 || len(selection.ExcludedIDs) != 0) ||
		selection.Mode == "all_matching" && len(selection.IDs) != 0 {
		return ListSelection{}, errkind.InvalidRequestErr(errors.New("Invalid or empty row selection."))
	}
	values := selection.IDs
	if selection.Mode == "all_matching" {
		values = selection.ExcludedIDs
	}
	if len(values) > MaxListSelection {
		return ListSelection{}, errkind.InvalidRequestErr(errors.New("Selection is too large; narrow the filters."))
	}
	seen := make(map[string]bool, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		id, ok := canonicalUUIDKeyword(value)
		if !ok || !strings.Contains(value, "-") {
			return ListSelection{}, errkind.InvalidRequestErr(errors.New("Selection must contain public UUIDs, not row numbers."))
		}
		if !seen[id] {
			seen[id] = true
			normalized = append(normalized, id)
		}
	}
	if selection.Mode == "ids" {
		selection.IDs = normalized
	} else {
		selection.ExcludedIDs = normalized
	}
	return selection, nil
}

// applyListSelection adds only validated, bound UUID predicates to an already authorized query.
func applyListSelection(db *gorm.DB, selection ListSelection) *gorm.DB {
	if selection.Mode == "ids" {
		return db.Where("uuid IN ?", selection.IDs)
	}
	if len(selection.ExcludedIDs) > 0 {
		return db.Where("uuid NOT IN ?", selection.ExcludedIDs)
	}
	return db
}
