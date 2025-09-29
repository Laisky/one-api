package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/message"
)

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

type Token struct {
	Id             int     `json:"id"`
	UserId         int     `json:"user_id"`
	Key            string  `json:"key" gorm:"type:char(48);uniqueIndex"`
	Status         int     `json:"status" gorm:"default:1"`
	Name           string  `json:"name" gorm:"index" `
	CreatedTime    int64   `json:"created_time" gorm:"bigint"`
	AccessedTime   int64   `json:"accessed_time" gorm:"bigint"`
	ExpiredTime    int64   `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota    int64   `json:"remain_quota" gorm:"bigint;default:0"`
	UnlimitedQuota bool    `json:"unlimited_quota" gorm:"default:false"`
	UsedQuota      int64   `json:"used_quota" gorm:"bigint;default:0"` // used quota
	CreatedAt      int64   `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt      int64   `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
	Models         *string `json:"models" gorm:"type:text"`  // allowed models
	Subnet         *string `json:"subnet" gorm:"default:''"` // allowed subnet
}

// MarshalJSON ensures that any token serialized to JSON will include the configured key prefix.
// This does not modify the stored key; it's applied only at response time.
func (t Token) MarshalJSON() ([]byte, error) {
	// Normalize: strip any known legacy prefixes from stored value, then apply configured prefix
	raw := t.Key
	raw = strings.TrimPrefix(raw, "sk-")
	raw = strings.TrimPrefix(raw, "laisky-")
	prefix := config.TokenKeyPrefix
	if prefix == "" {
		prefix = "sk-"
	}

	type tokenDTO struct {
		Id             int     `json:"id"`
		UserId         int     `json:"user_id"`
		Key            string  `json:"key"`
		Status         int     `json:"status"`
		Name           string  `json:"name"`
		CreatedTime    int64   `json:"created_time"`
		AccessedTime   int64   `json:"accessed_time"`
		ExpiredTime    int64   `json:"expired_time"`
		RemainQuota    int64   `json:"remain_quota"`
		UnlimitedQuota bool    `json:"unlimited_quota"`
		UsedQuota      int64   `json:"used_quota"`
		CreatedAt      int64   `json:"created_at"`
		UpdatedAt      int64   `json:"updated_at"`
		Models         *string `json:"models"`
		Subnet         *string `json:"subnet"`
	}
	dto := tokenDTO{
		Id:             t.Id,
		UserId:         t.UserId,
		Key:            prefix + raw,
		Status:         t.Status,
		Name:           t.Name,
		CreatedTime:    t.CreatedTime,
		AccessedTime:   t.AccessedTime,
		ExpiredTime:    t.ExpiredTime,
		RemainQuota:    t.RemainQuota,
		UnlimitedQuota: t.UnlimitedQuota,
		UsedQuota:      t.UsedQuota,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		Models:         t.Models,
		Subnet:         t.Subnet,
	}
	return json.Marshal(dto)
}

func clearTokenCache(key string) {
	if common.RedisEnabled {
		err := common.RedisDel(fmt.Sprintf("token:%s", key))
		if err != nil {
			logger.Logger.Warn("failed to clear token cache, continuing", zap.String("key", key), zap.Error(err))
		}
	}
}

func GetAllUserTokens(userId int, startIdx int, num int, order string, sortBy string, sortOrder string) ([]*Token, error) {
	var tokens []*Token
	var err error
	query := DB.Where("user_id = ?", userId)

	// Handle new sorting parameters first
	if sortBy != "" {
		orderClause := sortBy
		if sortOrder == "asc" {
			orderClause += " asc"
		} else {
			orderClause += " desc"
		}
		query = query.Order(orderClause)
	} else {
		// Fallback to legacy order parameter for backward compatibility
		switch order {
		case "remain_quota":
			query = query.Order("unlimited_quota desc, remain_quota desc")
		case "used_quota":
			query = query.Order("used_quota desc")
		default:
			query = query.Order("id desc")
		}
	}

	err = query.Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

func GetUserTokenCount(userId int) (count int64, err error) {
	err = DB.Model(&Token{}).Where("user_id = ?", userId).Count(&count).Error
	return count, err
}

func SearchUserTokens(userId int, keyword string, startIdx int, num int, sortBy string, sortOrder string) (tokens []*Token, total int64, err error) {
	db := DB.Model(&Token{}).Where("user_id = ?", userId)
	if keyword != "" {
		db = db.Where("name LIKE ?", keyword+"%")
	}
	orderClause := "id desc"
	if sortBy != "" {
		if sortOrder == "asc" {
			orderClause = sortBy + " asc"
		} else {
			orderClause = sortBy + " desc"
		}
	}
	db = db.Order(orderClause)
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, total, err
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, errors.New("No token provided")
	}
	token, err = CacheGetTokenByKey(key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(err, "token not found for key: %s", key)
		}

		return nil, errors.Wrapf(err, "failed to get token by key: %s", key)
	}

	switch token.Status {
	case TokenStatusExhausted:
		return nil, errors.Errorf("API Key %s (#%d) quota has been exhausted", token.Name, token.Id)
	case TokenStatusExpired:
		return nil, errors.Errorf("token %s (#%d) has expired", token.Name, token.Id)
	}

	if token.Status != TokenStatusEnabled {
		return nil, errors.Errorf("token %s (#%d) status is not available (status: %d)", token.Name, token.Id, token.Status)
	}
	if token.ExpiredTime != -1 && token.ExpiredTime < helper.GetTimestamp() {
		if !common.RedisEnabled {
			token.Status = TokenStatusExpired
			err := token.SelectUpdate()
			if err != nil {
				logger.Logger.Error("failed to update token status", zap.Int("token_id", token.Id), zap.Error(err))
			}
		} else {
			// If Redis is enabled, the cache will be updated by the next fetch
			// or we can proactively delete it here.
			// For consistency with other operations, let SelectUpdate handle it if it's called.
			// However, SelectUpdate is only called if Redis is NOT enabled in this block.
			// So, if Redis IS enabled, and token is expired, we should clear it.
			clearTokenCache(token.Key)
		}
		return nil, errors.Errorf("token %s (#%d) has expired at timestamp %d", token.Name, token.Id, token.ExpiredTime)
	}
	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		if !common.RedisEnabled {
			// in this case, we can make sure the token is exhausted
			token.Status = TokenStatusExhausted
			err := token.SelectUpdate()
			if err != nil {
				logger.Logger.Error("failed to update token status", zap.Int("token_id", token.Id), zap.Error(err))
			}
		} else {
			// If Redis IS enabled, and token is exhausted, we should clear it.
			clearTokenCache(token.Key)
		}
		return nil, errors.Errorf("token %s (#%d) quota has been used up (remaining: %d)", token.Name, token.Id, token.RemainQuota)
	}

	return token, nil
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.Errorf("invalid parameters: id=%d, userId=%d", id, userId)
	}
	token := Token{Id: id, UserId: userId}
	err := DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get token by id=%d and userId=%d", id, userId)
	}
	return &token, nil
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.Errorf("invalid token id: %d", id)
	}
	token := Token{Id: id}
	err := DB.First(&token, "id = ?", id).Error
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get token by id=%d", id)
	}
	return &token, nil
}

func (t *Token) Insert() error {
	var err error
	err = DB.Create(t).Error
	if err == nil {
		clearTokenCache(t.Key)
		return nil
	}
	return errors.Wrapf(err, "failed to insert token: id=%d, user_id=%d", t.Id, t.UserId)
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (t *Token) Update() error {
	var err error
	err = DB.Model(t).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota", "models", "subnet").Updates(t).Error
	if err == nil {
		clearTokenCache(t.Key)
		return nil
	}
	return errors.Wrapf(err, "failed to update token: id=%d, user_id=%d", t.Id, t.UserId)
}

func (t *Token) SelectUpdate() error {
	// This can update zero values
	err := DB.Model(t).Select("accessed_time", "status").Updates(t).Error
	if err == nil {
		clearTokenCache(t.Key)
		return nil
	}
	return errors.Wrapf(err, "failed to select update token: id=%d, user_id=%d", t.Id, t.UserId)
}

func (t *Token) Delete() error {
	var err error
	err = DB.Delete(t).Error
	if err == nil {
		clearTokenCache(t.Key)
		return nil
	}
	return errors.Wrapf(err, "failed to delete token: id=%d, user_id=%d", t.Id, t.UserId)
}

func (t *Token) GetModels() string {
	if t == nil {
		return ""
	}
	if t.Models == nil {
		return ""
	}
	return *t.Models
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.Errorf("invalid parameters: id=%d, userId=%d", id, userId)
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return errors.Wrapf(err, "failed to find token for deletion: id=%d, userId=%d", id, userId)
	}
	// The key is now populated in token object
	// token.Delete() will handle clearing the cache
	err = token.Delete()
	if err != nil {
		return errors.Wrapf(err, "failed to delete token: id=%d, userId=%d", id, userId)
	}
	return nil
}

func IncreaseTokenQuota(id int, quota int64) (err error) {
	if quota < 0 {
		return errors.Errorf("quota cannot be negative: %d", quota)
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, quota)
		return nil
	}
	return increaseTokenQuota(id, quota)
}

func increaseTokenQuota(id int, quota int64) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": helper.GetTimestamp(),
		},
	).Error
	if err == nil {
		token, fetchErr := GetTokenById(id)
		if fetchErr == nil && token != nil {
			clearTokenCache(token.Key)
		} else if fetchErr != nil {
			logger.Logger.Error("failed to fetch token for cache clearing after quota increase", zap.Int("token_id", id), zap.Error(fetchErr))
		}
		return nil
	}
	return errors.Wrapf(err, "failed to increase token quota: id=%d", id)
}

func DecreaseTokenQuota(id int, quota int64) (err error) {
	if quota < 0 {
		return errors.Errorf("quota cannot be negative: %d", quota)
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(id, quota)
}

func decreaseTokenQuota(id int, quota int64) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": helper.GetTimestamp(),
		},
	).Error
	if err == nil {
		token, fetchErr := GetTokenById(id)
		if fetchErr == nil && token != nil {
			clearTokenCache(token.Key)
		} else if fetchErr != nil {
			logger.Logger.Error("failed to fetch token for cache clearing after quota decrease", zap.Int("token_id", id), zap.Error(fetchErr))
		}
		return nil
	}
	return errors.Wrapf(err, "failed to decrease token quota: id=%d", id)
}

func PreConsumeTokenQuota(tokenId int, quota int64) (err error) {
	if quota < 0 {
		return errors.Errorf("quota cannot be negative: %d", quota)
	}
	token, err := GetTokenById(tokenId)
	if err != nil {
		return errors.Wrapf(err, "failed to get token for pre-consume: tokenId=%d", tokenId)
	}
	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return errors.Errorf("insufficient token quota: required=%d, available=%d, tokenId=%d", quota, token.RemainQuota, tokenId)
	}
	userQuota, err := GetUserQuota(token.UserId)
	if err != nil {
		return errors.Wrapf(err, "failed to get user quota for pre-consume: userId=%d, tokenId=%d", token.UserId, tokenId)
	}
	if userQuota < quota {
		return errors.Errorf("insufficient user quota: required=%d, available=%d, userId=%d, tokenId=%d", quota, userQuota, token.UserId, tokenId)
	}
	quotaTooLow := userQuota >= config.QuotaRemindThreshold && userQuota-quota < config.QuotaRemindThreshold
	noMoreQuota := userQuota-quota <= 0
	if quotaTooLow || noMoreQuota {
		go func() {
			email, err := GetUserEmail(token.UserId)
			if err != nil {
				logger.Logger.Error("failed to fetch user email", zap.Int("user_id", token.UserId), zap.Error(err))
			}
			prompt := "Quota Reminder"
			var contentText string
			if noMoreQuota {
				contentText = "Your quota has been exhausted"
			} else {
				contentText = "Your quota is about to be exhausted"
			}
			if email != "" {
				topUpLink := fmt.Sprintf("%s/topup", config.ServerAddress)
				content := message.EmailTemplate(
					prompt,
					fmt.Sprintf(`
								<p>Hello!</p>
								<p>%s, your current remaining quota is <strong>%d</strong>.</p>
								<p>To avoid any disruption to your service, please top up in a timely manner.</p>
								<p style="text-align: center; margin: 30px 0;">
									<a href="%s" style="background-color: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">Top Up Now</a>
								</p>
								<p style="color: #666;">If the button does not work, please copy the following link and paste it into your browser:</p>
								<p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px; word-break: break-all;">%s</p>
							`, contentText, userQuota, topUpLink, topUpLink),
				)
				err = message.SendEmail(prompt, email, content)
				if err != nil {
					logger.Logger.Error("failed to send email", zap.String("email", email), zap.Error(err))
				}
			}
		}()
	}
	if !token.UnlimitedQuota {
		if err = DecreaseTokenQuota(tokenId, quota); err != nil {
			return errors.Wrapf(err, "decrease quota for token %d", tokenId)
		}
	}
	if err = DecreaseUserQuota(token.UserId, quota); err != nil {
		return errors.Wrapf(err, "decrease quota for user %d in pre-consume", token.UserId)
	}
	return nil
}

func PostConsumeTokenQuota(tokenId int, quota int64) (err error) {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return errors.Wrapf(err, "get token %d for post-consume", tokenId)
	}
	if quota > 0 {
		err = DecreaseUserQuota(token.UserId, quota)
	} else {
		err = IncreaseUserQuota(token.UserId, -quota)
	}
	if !token.UnlimitedQuota {
		if quota > 0 {
			err = DecreaseTokenQuota(tokenId, quota)
		} else {
			err = IncreaseTokenQuota(tokenId, -quota)
		}
		if err != nil {
			return errors.Wrapf(err, "adjust token %d quota in post-consume", tokenId)
		}
	}
	return nil
}
