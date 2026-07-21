package model

import (
	"context"
	"fmt"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
)

const (
	RedemptionCodeStatusEnabled  = 1 // don't use 0, 0 is the default value!
	RedemptionCodeStatusDisabled = 2 // also don't use 0
	RedemptionCodeStatusUsed     = 3 // also don't use 0
)

type Redemption struct {
	Id           int     `json:"id"`
	UUID         string  `json:"uuid" gorm:"type:char(36);column:uuid"`
	UserId       int     `json:"user_id"`
	UserUUID     *string `json:"user_uuid" gorm:"type:char(36);column:user_uuid;index"`
	Key          string  `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int     `json:"status" gorm:"default:1"`
	Name         string  `json:"name" gorm:"index"`
	Quota        int64   `json:"quota" gorm:"bigint;default:100"`
	CreatedTime  int64   `json:"created_time" gorm:"bigint"`
	RedeemedTime int64   `json:"redeemed_time" gorm:"bigint"`
	Count        int     `json:"count" gorm:"-:all"` // only for api request
	CreatedAt    int64   `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt    int64   `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

var redemptionSortFields = map[string]string{
	"id":            "id",
	"name":          "name",
	"status":        "status",
	"quota":         "quota",
	"created_time":  "created_time",
	"redeemed_time": "redeemed_time",
	"created_at":    "created_at",
	"updated_at":    "updated_at",
}

func GetAllRedemptions(startIdx int, num int) ([]*Redemption, error) {
	var redemptions []*Redemption
	var err error
	err = DB.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		return nil, errors.Wrap(err, "get all redemptions")
	}
	return redemptions, nil
}

func GetRedemptionCount() (count int64, err error) {
	err = DB.Model(&Redemption{}).Count(&count).Error
	if err != nil {
		return 0, errors.Wrap(err, "count redemptions")
	}
	return count, nil
}

func SearchRedemptions(keyword string, startIdx int, num int, sortBy string, sortOrder string) (redemptions []*Redemption, total int64, err error) {
	db := DB.Model(&Redemption{})
	if keyword != "" {
		// user_uuid lets an operator paste a user UUID to list that user's redemptions.
		if scoped, matched := applyUUIDKeyword(db, keyword, "uuid", "user_uuid"); matched {
			db = scoped
		} else {
			// The internal incremental id is deliberately not searchable; UUID is the
			// only external identifier for a redemption.
			db = db.Where("name LIKE ?", keyword+"%")
		}
	}
	db = db.Order(ValidateOrderClause(sortBy, sortOrder, redemptionSortFields, "id desc"))
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "search redemptions")
	}
	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id is empty!")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	if err != nil {
		tagged := identity.Tag(
			errors.Wrapf(err, "get redemption by id %d", id),
			identity.NewRedemptionRef(id, "", ""))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The id comes straight from the caller, so an absent row means the
			// caller named a redemption that does not exist.
			return nil, errkind.NotFoundErr(tagged)
		}
		return nil, tagged
	}
	return &redemption, nil
}

func Redeem(ctx context.Context, key string, userId int) (quota int64, err error) {
	if key == "" {
		return 0, errkind.InvalidRequestErr(errors.New("No redemption code provided"))
	}
	if userId == 0 {
		return 0, errkind.InvalidRequestErr(errors.New("Invalid user id"))
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingPostgreSQL.Load() {
		keyCol = `"key"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		// 1. Read the row with a row-level lock. clause.Locking is the
		//    GORM-v2 API for `SELECT ... FOR UPDATE`; the previous
		//    `tx.Set("gorm:query_option", "FOR UPDATE")` was a GORM-v1
		//    hook key that v2 silently ignores, so without this line two
		//    concurrent callers could both pass the status check below.
		//    On SQLite the lock is a no-op but the CAS step (2) still
		//    guarantees correctness.
		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where(keyCol+" = ?", key).
			First(redemption).Error
		if err != nil {
			// The message is deliberately identical for both causes (it is what the
			// client sees), but the fault attribution is not: only a genuinely
			// absent row is the caller's fault. A driver failure swallowed here must
			// stay unclassified so it can still surface as a server-side error.
			notFound := errors.New("Invalid redemption code")
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errkind.NotFoundErr(notFound)
			}
			return notFound
		}
		if redemption.Status != RedemptionCodeStatusEnabled {
			// A code the caller already spent (or an operator disabled).
			return errkind.InvalidRequestErr(errors.New("The redemption code has been used"))
		}

		// 2. Compare-and-swap claim of the redemption row. The WHERE on
		//    status = Enabled is the critical safety net: even if a peer
		//    transaction snuck past the FOR UPDATE on a backend where it
		//    is unsupported, only one UPDATE can flip the row from
		//    Enabled to Used. RowsAffected == 0 means we lost the race.
		now := helper.GetTimestamp()
		claim := tx.Model(&Redemption{}).
			Where("id = ? AND status = ?", redemption.Id, RedemptionCodeStatusEnabled).
			Updates(map[string]any{
				"status":        RedemptionCodeStatusUsed,
				"redeemed_time": now,
			})
		if claim.Error != nil {
			return errors.Wrap(claim.Error, "claim redemption")
		}
		if claim.RowsAffected == 0 {
			// Lost the compare-and-swap race against a concurrent redemption of the
			// same code: a conflict, not a server fault.
			return errkind.ConflictErr(errors.New("The redemption code has been used"))
		}

		// 3. Only after we own the row do we credit the user. Doing this
		//    last means a failed CAS leaves the user untouched.
		err = tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
		if err != nil {
			return identity.Tag(
				errors.Wrapf(err, "increase user %d quota with redemption", userId),
				redemption.Ref(), identity.NewUserRef(userId, "", ""))
		}

		// Reflect the persisted state for the audit log emitted below.
		redemption.RedeemedTime = now
		redemption.Status = RedemptionCodeStatusUsed
		return nil
	})
	if err != nil {
		// redemption is zero when the code was never found, in which case Tag drops
		// the reference. Error path only, so the user lookup is off the hot path.
		return 0, identity.Tag(
			errors.Wrap(err, "Redeem failed"),
			redemption.Ref(), LookupUserRef(ctx, userId))
	}
	RecordLog(ctx, userId, LogTypeTopup, fmt.Sprintf("Recharged %s using redemption code", common.LogQuota(redemption.Quota)))
	return redemption.Quota, nil
}

func (redemption *Redemption) Insert() error {
	if err := DB.Create(redemption).Error; err != nil {
		return identity.Tag(
			errors.Wrap(err, "insert redemption"),
			redemption.Ref(), redemption.OwnerRef())
	}
	return nil
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values.
	// The driver error is returned as-is (no errors.Wrap) to keep the message
	// byte-identical for callers; Tag only attaches identity beside it.
	return identity.Tag(
		DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error,
		redemption.Ref(), redemption.OwnerRef())
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	if err := DB.Model(redemption).Select("name", "status", "quota", "redeemed_time").Updates(redemption).Error; err != nil {
		return identity.Tag(
			errors.Wrapf(err, "update redemption %d", redemption.Id),
			redemption.Ref(), redemption.OwnerRef())
	}
	return nil
}

func (redemption *Redemption) Delete() error {
	if err := DB.Delete(redemption).Error; err != nil {
		return identity.Tag(
			errors.Wrapf(err, "delete redemption %d", redemption.Id),
			redemption.Ref(), redemption.OwnerRef())
	}
	return nil
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id is empty!")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		tagged := identity.Tag(
			errors.Wrapf(err, "find redemption %d", id),
			identity.NewRedemptionRef(id, "", ""))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The caller asked to delete a redemption that is not there.
			return errkind.NotFoundErr(tagged)
		}
		return tagged
	}
	return redemption.Delete()
}
