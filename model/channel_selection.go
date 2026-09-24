package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

// ErrSelectedChannelMissing identifies a confirmed channel removed before its deletion transaction.
var ErrSelectedChannelMissing = errors.New("This channel no longer exists.")

// applyChannelSearch applies the list's existing prefix-name or exact-UUID search predicate.
func applyChannelSearch(db *gorm.DB, keyword string) *gorm.DB {
	if scoped, matched := applyUUIDKeyword(db, keyword, "uuid"); matched {
		return scoped
	}
	return db.Where("name LIKE ?", keyword+"%")
}

// SelectChannelTargets resolves only the requested channel identities using the same filter as the list.
func SelectChannelTargets(ctx context.Context, selection ListSelection, keyword string) ([]ChannelModelResetTarget, error) {
	normalized, err := selection.Normalize()
	if err != nil {
		return nil, errors.Wrap(err, "validate channel selection")
	}
	if len(keyword) > 255 {
		return nil, errkind.InvalidRequestErr(errors.New("Channel search is too long."))
	}
	query := DB.WithContext(ctx).Model(&Channel{})
	if keyword != "" {
		query = applyChannelSearch(query, keyword)
	}
	var targets []ChannelModelResetTarget
	if err := applyListSelection(query, normalized).Select("id", "uuid", "name").Order("id").Limit(MaxListSelection + 1).Scan(&targets).Error; err != nil {
		return nil, errors.Wrap(err, "resolve selected channels")
	}
	if len(targets) > MaxListSelection {
		return nil, errkind.InvalidRequestErr(errors.New("More than 10000 channels match; narrow the filters before selecting all pages."))
	}
	return targets, nil
}

// DeleteSelectedDisabledChannel atomically checks the current status and removes one disabled channel and its abilities.
// The id identifies the selected channel. The result reports deletion; enabled channels
// return false without error, missing channels return ErrSelectedChannelMissing, and
// database failures return wrapped errors. No failed transaction invalidates caches.
func DeleteSelectedDisabledChannel(ctx context.Context, id int) (bool, error) {
	var current Channel
	deleted := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Channel{}).Where("id = ?", id).UpdateColumn("status", gorm.Expr("status")).Error; err != nil {
			return errors.Wrap(err, "lock selected channel before deletion")
		}
		if err := tx.Omit("key").First(&current, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.WithStack(ErrSelectedChannelMissing)
			}
			return errors.Wrap(err, "read selected channel before deletion")
		}
		if current.Status != ChannelStatusAutoDisabled && current.Status != ChannelStatusManuallyDisabled {
			return nil
		}
		if err := deleteAbilitiesWithDB(tx, id); err != nil {
			return errors.Wrap(err, "delete selected channel abilities")
		}
		if err := tx.Delete(&Channel{}, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "delete selected disabled channel")
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, errors.Wrap(err, "delete selected channel transaction")
	}
	if deleted {
		InvalidateChannelModelCachesWithContext(ctx, current.Group)
	}
	return deleted, nil
}
