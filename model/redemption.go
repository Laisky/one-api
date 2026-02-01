package model

import (
	"context"
	"fmt"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/helper"
)

const (
	RedemptionCodeStatusEnabled  = 1 // don't use 0, 0 is the default value!
	RedemptionCodeStatusDisabled = 2 // also don't use 0
	RedemptionCodeStatusUsed     = 3 // also don't use 0
)

type Redemption struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id"`
	Key          string `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int    `json:"status" gorm:"default:1"`
	Name         string `json:"name" gorm:"index"`
	Quota        int64  `json:"quota" gorm:"bigint;default:100"`
	CreatedTime  int64  `json:"created_time" gorm:"bigint"`
	RedeemedTime int64  `json:"redeemed_time" gorm:"bigint"`
	Count        int    `json:"count" gorm:"-:all"` // only for api request
	CreatedAt    int64  `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
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
	return redemptions, err
}

func GetRedemptionCount() (count int64, err error) {
	err = DB.Model(&Redemption{}).Count(&count).Error
	return count, err
}

func SearchRedemptions(keyword string, startIdx int, num int, sortBy string, sortOrder string) (redemptions []*Redemption, total int64, err error) {
	db := DB.Model(&Redemption{})
	if keyword != "" {
		db = db.Where("id = ? or name LIKE ?", keyword, keyword+"%")
	}
	db = db.Order(ValidateOrderClause(sortBy, sortOrder, redemptionSortFields, "id desc"))
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&redemptions).Error
	return redemptions, total, err
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id is empty!")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(ctx context.Context, key string, userId int) (quota int64, err error) {
	if key == "" {
		return 0, errors.New("No redemption code provided")
	}
	if userId == 0 {
		return 0, errors.New("Invalid user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingPostgreSQL.Load() {
		keyCol = `"key"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("Invalid redemption code")
		}
		if redemption.Status != RedemptionCodeStatusEnabled {
			return errors.New("The redemption code has been used")
		}
		err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
		if err != nil {
			return errors.Wrapf(err, "increase user %d quota with redemption", userId)
		}
		redemption.RedeemedTime = helper.GetTimestamp()
		redemption.Status = RedemptionCodeStatusUsed
		if err = tx.Save(redemption).Error; err != nil {
			return errors.Wrap(err, "update redemption status")
		}
		return nil
	})
	if err != nil {
		return 0, errors.Wrap(err, "Redeem failed")
	}
	RecordLog(ctx, userId, LogTypeTopup, fmt.Sprintf("Recharged %s using redemption code", common.LogQuota(redemption.Quota)))
	return redemption.Quota, nil
}

func (redemption *Redemption) Insert() error {
	if err := DB.Create(redemption).Error; err != nil {
		return errors.Wrap(err, "insert redemption")
	}
	return nil
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	if err := DB.Model(redemption).Select("name", "status", "quota", "redeemed_time").Updates(redemption).Error; err != nil {
		return errors.Wrapf(err, "update redemption %d", redemption.Id)
	}
	return nil
}

func (redemption *Redemption) Delete() error {
	if err := DB.Delete(redemption).Error; err != nil {
		return errors.Wrapf(err, "delete redemption %d", redemption.Id)
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
		return errors.Wrapf(err, "find redemption %d", id)
	}
	return redemption.Delete()
}
