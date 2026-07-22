package model

import (
	"context"
	"fmt"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/message"
)

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

type Token struct {
	Id             int     `json:"id"`
	UUID           string  `json:"uuid" gorm:"type:char(36);column:uuid"`
	UserId         int     `json:"user_id"`
	UserUUID       *string `json:"user_uuid" gorm:"type:char(36);column:user_uuid;index"`
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

var tokenSortFields = map[string]string{
	"id":           "id",
	"uuid":         "uuid",
	"name":         "name",
	"status":       "status",
	"expired_time": "expired_time",
	"remain_quota": "remain_quota",
	"used_quota":   "used_quota",
	"created_at":   "created_at",
	"updated_at":   "updated_at",
}

func clearTokenCache(ctx context.Context, key string) {
	if common.IsRedisEnabled() {
		if ctx == nil {
			ctx = context.Background()
		}
		err := common.RedisDel(ctx, fmt.Sprintf("token:%s", key))
		if err != nil {
			// The raw API key must never appear verbatim in a log (same invariant
			// enforced in ValidateUserToken below). No identity reference is
			// resolved here: this runs on every token write and a lookup by key
			// would add a query to a hot path.
			logger.Logger.Warn("failed to clear token cache, continuing",
				zap.String("key", helper.MaskAPIKey(key)), zap.Error(err))
		}
	}
}

func GetAllUserTokens(userId int, startIdx int, num int, order string, sortBy string, sortOrder string) ([]*Token, error) {
	var tokens []*Token
	var err error
	query := DB.Where("user_id = ?", userId)

	// Handle new sorting parameters first
	if sortBy != "" {
		query = query.Order(ValidateOrderClause(sortBy, sortOrder, tokenSortFields, "id desc"))
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
	if err != nil {
		return nil, identity.Tag(
			errors.Wrapf(err, "get user %d tokens", userId),
			LookupUserRef(context.Background(), userId))
	}
	return tokens, nil
}

func GetUserTokenCount(userId int) (count int64, err error) {
	err = DB.Model(&Token{}).Where("user_id = ?", userId).Count(&count).Error
	if err != nil {
		return 0, identity.Tag(
			errors.Wrapf(err, "count user %d tokens", userId),
			LookupUserRef(context.Background(), userId))
	}
	return count, nil
}

func SearchUserTokens(userId int, keyword string, startIdx int, num int, sortBy string, sortOrder string) (tokens []*Token, total int64, err error) {
	db := DB.Model(&Token{}).Where("user_id = ?", userId)
	if keyword != "" {
		// user_uuid lets an operator paste a user UUID to list that user's tokens; the
		// user_id scope above still ANDs, so it can never cross into another owner.
		if scoped, matched := applyUUIDKeyword(db, keyword, "uuid", "user_uuid"); matched {
			db = scoped
		} else {
			db = db.Where("(name LIKE ?)", keyword+"%")
		}
	}
	orderClause := ValidateOrderClause(sortBy, sortOrder, tokenSortFields, "id desc")
	db = db.Order(orderClause)
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&tokens).Error
	if err != nil {
		return nil, 0, identity.Tag(
			errors.Wrapf(err, "search user %d tokens", userId),
			LookupUserRef(context.Background(), userId))
	}
	return tokens, total, nil
}

// GetAllTokensForAdmin lists tokens across any user. Pass userId > 0 to filter to a single owner,
// or 0 to see every user's tokens. Admin-scoped and read-only — callers must enforce auth.
func GetAllTokensForAdmin(userId int, startIdx int, num int, sortBy string, sortOrder string) (tokens []*Token, total int64, err error) {
	db := DB.Model(&Token{})
	if userId > 0 {
		db = db.Where("user_id = ?", userId)
	}
	orderClause := ValidateOrderClause(sortBy, sortOrder, tokenSortFields, "id desc")
	db = db.Order(orderClause)
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&tokens).Error
	if err != nil {
		return nil, 0, identity.Tag(
			errors.Wrapf(err, "admin list tokens for user_id=%d", userId),
			LookupUserRef(context.Background(), userId))
	}
	return tokens, total, nil
}

// SearchAllTokensForAdmin searches tokens across any user by keyword (token name prefix match).
// Admin-scoped and read-only — callers must enforce auth.
func SearchAllTokensForAdmin(keyword string, startIdx int, num int, sortBy string, sortOrder string) (tokens []*Token, total int64, err error) {
	db := DB.Model(&Token{})
	if keyword != "" {
		// user_uuid lets an admin paste a user UUID to list every token that user owns.
		if scoped, matched := applyUUIDKeyword(db, keyword, "uuid", "user_uuid"); matched {
			db = scoped
		} else {
			db = db.Where("(name LIKE ?)", keyword+"%")
		}
	}
	orderClause := ValidateOrderClause(sortBy, sortOrder, tokenSortFields, "id desc")
	db = db.Order(orderClause)
	err = db.Count(&total).Limit(num).Offset(startIdx).Find(&tokens).Error
	if err != nil {
		return nil, 0, errors.Wrapf(err, "admin search tokens by keyword=%q", keyword)
	}
	return tokens, total, nil
}

func ValidateUserToken(ctx context.Context, key string) (token *Token, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if key == "" {
		// No credential was presented at all: the caller's fault, never the server's.
		return nil, errkind.UnauthorizedErr(errors.New("No token provided"))
	}
	token, err = CacheGetTokenByKey(ctx, key)
	if err != nil {
		// Mask the key: it must never appear verbatim in logs/errors, but keep
		// the "token not found for key:" prefix that shouldLogAsWarning matches.
		maskedKey := helper.MaskAPIKey(key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The key simply does not exist: an unrecognised credential.
			return nil, errkind.UnauthorizedErr(
				errors.Wrapf(err, "token not found for key: %s", maskedKey))
		}

		// Anything else here is a database/Redis failure. Middleware answers every
		// token validation failure with HTTP 401, so without this mark a real
		// outage would be logged at WARN and page nobody.
		return nil, errkind.ServerErr(
			errors.Wrapf(err, "failed to get token by key: %s", maskedKey))
	}

	switch token.Status {
	case TokenStatusExhausted:
		// Specifically about funds.
		return nil, errkind.Quota(identity.Tag(
			errors.Errorf("API Key %s (#%d) quota has been exhausted", token.Name, token.Id),
			token.Ref(), token.OwnerRef()))
	case TokenStatusExpired:
		return nil, errkind.ForbiddenErr(identity.Tag(
			errors.Errorf("token %s (#%d) has expired", token.Name, token.Id),
			token.Ref(), token.OwnerRef()))
	}

	if token.Status != TokenStatusEnabled {
		// A valid credential that the operator or the owner has disabled.
		return nil, errkind.ForbiddenErr(identity.Tag(
			errors.Errorf("token %s (#%d) status is not available (status: %d)", token.Name, token.Id, token.Status),
			token.Ref(), token.OwnerRef()))
	}
	if token.ExpiredTime != -1 && token.ExpiredTime < helper.GetTimestamp() {
		if !common.IsRedisEnabled() {
			token.Status = TokenStatusExpired
			err := token.SelectUpdate(ctx)
			if err != nil {
				logger.Logger.Error("failed to update token status",
					append(token.OwnerRef().AppendZap(token.Ref().Zap()), zap.Error(err))...)
			}
		} else {
			// If Redis is enabled, the cache will be updated by the next fetch
			// or we can proactively delete it here.
			// For consistency with other operations, let SelectUpdate handle it if it's called.
			// However, SelectUpdate is only called if Redis is NOT enabled in this block.
			// So, if Redis IS enabled, and token is expired, we should clear it.
			clearTokenCache(ctx, token.Key)
		}
		return nil, errkind.ForbiddenErr(identity.Tag(
			errors.Errorf("token %s (#%d) has expired at timestamp %d", token.Name, token.Id, token.ExpiredTime),
			token.Ref(), token.OwnerRef()))
	}
	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		if !common.IsRedisEnabled() {
			// in this case, we can make sure the token is exhausted
			token.Status = TokenStatusExhausted
			err := token.SelectUpdate(ctx)
			if err != nil {
				logger.Logger.Error("failed to update token status",
					append(token.OwnerRef().AppendZap(token.Ref().Zap()), zap.Error(err))...)
			}
		} else {
			// If Redis IS enabled, and token is exhausted, we should clear it.
			clearTokenCache(ctx, token.Key)
		}
		// Out of funds: the caller's condition, not a server fault.
		return nil, errkind.Quota(identity.Tag(
			errors.Errorf("token %s (#%d) quota has been used up (remaining: %d)", token.Name, token.Id, token.RemainQuota),
			token.Ref(), token.OwnerRef()))
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
		tagged := identity.Tag(
			errors.Wrapf(err, "failed to get token by id=%d and userId=%d", id, userId),
			identity.NewTokenRef(id, "", ""), identity.NewUserRef(userId, "", ""))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Both ids come from the request (the id from the caller, the owner
			// from its session), so an absent row means the caller named a token
			// that does not exist — not a broken internal invariant.
			return nil, errkind.NotFoundErr(tagged)
		}
		return nil, tagged
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
		return nil, identity.Tag(
			errors.Wrapf(err, "failed to get token by id=%d", id),
			identity.NewTokenRef(id, "", ""))
	}
	return &token, nil
}

func (t *Token) Insert(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if t.UserUUID == nil && t.UserId > 0 {
		userUUID, err := GetUserUUIDByID(t.UserId)
		if err != nil {
			return identity.Tag(
				errors.Wrapf(err, "get token user uuid: user_id=%d", t.UserId),
				identity.NewUserRef(t.UserId, "", ""))
		}
		if userUUID != "" {
			t.UserUUID = &userUUID
		}
	}
	var err error
	err = DB.Create(t).Error
	if err == nil {
		clearTokenCache(ctx, t.Key)
		return nil
	}
	return identity.Tag(
		errors.Wrapf(err, "failed to insert token: id=%d, user_id=%d", t.Id, t.UserId),
		t.Ref(), t.OwnerRef())
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (t *Token) Update(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	err = DB.Model(t).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota", "models", "subnet").Updates(t).Error
	if err == nil {
		clearTokenCache(ctx, t.Key)
		return nil
	}
	return identity.Tag(
		errors.Wrapf(err, "failed to update token: id=%d, user_id=%d", t.Id, t.UserId),
		t.Ref(), t.OwnerRef())
}

func (t *Token) SelectUpdate(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// This can update zero values
	err := DB.Model(t).Select("accessed_time", "status").Updates(t).Error
	if err == nil {
		clearTokenCache(ctx, t.Key)
		return nil
	}
	return identity.Tag(
		errors.Wrapf(err, "failed to select update token: id=%d, user_id=%d", t.Id, t.UserId),
		t.Ref(), t.OwnerRef())
}

func (t *Token) Delete(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	err = DB.Delete(t).Error
	if err == nil {
		clearTokenCache(ctx, t.Key)
		return nil
	}
	return identity.Tag(
		errors.Wrapf(err, "failed to delete token: id=%d, user_id=%d", t.Id, t.UserId),
		t.Ref(), t.OwnerRef())
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

func DeleteTokenById(ctx context.Context, id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.Errorf("invalid parameters: id=%d, userId=%d", id, userId)
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		tagged := identity.Tag(
			errors.Wrapf(err, "failed to find token for deletion: id=%d, userId=%d", id, userId),
			identity.NewTokenRef(id, "", ""), identity.NewUserRef(userId, "", ""))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The caller asked to delete a token that is not theirs or not there.
			return errkind.NotFoundErr(tagged)
		}
		return tagged
	}
	// The key is now populated in token object
	// token.Delete() will handle clearing the cache
	err = token.Delete(ctx)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to delete token: id=%d, userId=%d", id, userId),
			token.Ref(), token.OwnerRef())
	}
	return nil
}

func IncreaseTokenQuota(ctx context.Context, id int, quota int64) (err error) {
	if quota < 0 {
		return errors.Errorf("quota cannot be negative: %d", quota)
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, quota)
		return nil
	}
	return increaseTokenQuota(ctx, id, quota)
}

func increaseTokenQuota(ctx context.Context, id int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result *gorm.DB
	err = runWithSQLiteBusyRetry(ctx, func() error {
		result = DB.Model(&Token{}).Where("id = ?", id).Updates(
			map[string]any{
				"remain_quota":  gorm.Expr("remain_quota + ?", quota),
				"used_quota":    gorm.Expr("used_quota - ?", quota),
				"accessed_time": helper.GetTimestamp(),
			},
		)
		return result.Error
	})
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to increase token quota: id=%d", id),
			LookupTokenRef(ctx, id))
	}

	token, fetchErr := GetTokenById(id)
	if fetchErr == nil && token != nil {
		clearTokenCache(ctx, token.Key)
	} else if fetchErr != nil {
		// Error path only: LookupTokenRef consults the context identity first and
		// only falls back to a narrow SELECT.
		logger.Logger.Error("failed to fetch token for cache clearing after quota increase",
			append(LookupTokenRef(ctx, id).Zap(), zap.Error(fetchErr))...)
	}
	return nil
}

func DecreaseTokenQuota(ctx context.Context, id int, quota int64) (err error) {
	if quota < 0 {
		return errors.Errorf("quota cannot be negative: %d", quota)
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(ctx, id, quota)
}

func decreaseTokenQuota(ctx context.Context, id int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result *gorm.DB
	err = runWithSQLiteBusyRetry(ctx, func() error {
		result = DB.Model(&Token{}).
			Where("id = ? AND remain_quota >= ?", id, quota).
			Updates(map[string]any{
				"remain_quota":  gorm.Expr("remain_quota - ?", quota),
				"used_quota":    gorm.Expr("used_quota + ?", quota),
				"accessed_time": helper.GetTimestamp(),
			})
		return result.Error
	})
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to decrease token quota: id=%d", id),
			LookupTokenRef(ctx, id))
	}
	if result.RowsAffected == 0 {
		// Deliberately left unclassified (Unknown => today's ERROR treatment).
		// This helper serves both the pre-consume admission check and the
		// post-consume debit (PostConsumeTokenQuota). After the response has
		// already been delivered, a zero-rows conditional UPDATE means the debit
		// silently failed or the token row vanished: unbilled usage and a
		// data-integrity break that must keep paging. The genuine "caller is out
		// of funds" case is marked upfront in PreConsumeTokenQuota.
		return identity.Tag(
			errors.Errorf("insufficient token quota for token %d", id),
			LookupTokenRef(ctx, id))
	}

	token, fetchErr := GetTokenById(id)
	if fetchErr == nil && token != nil {
		clearTokenCache(ctx, token.Key)
	} else if fetchErr != nil {
		// Error path only: LookupTokenRef consults the context identity first and
		// only falls back to a narrow SELECT.
		logger.Logger.Error("failed to fetch token for cache clearing after quota decrease",
			append(LookupTokenRef(ctx, id).Zap(), zap.Error(fetchErr))...)
	}
	return nil
}

func PreConsumeTokenQuota(ctx context.Context, tokenId int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if quota < 0 {
		return errors.Errorf("quota cannot be negative: %d", quota)
	}
	token, err := GetTokenById(tokenId)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to get token for pre-consume: tokenId=%d", tokenId),
			identity.NewTokenRef(tokenId, "", ""))
	}
	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return errkind.Quota(identity.Tag(
			errors.Errorf("insufficient token quota: required=%d, available=%d, tokenId=%d", quota, token.RemainQuota, tokenId),
			token.Ref(), token.OwnerRef()))
	}
	userQuota, err := GetUserQuota(token.UserId)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to get user quota for pre-consume: userId=%d, tokenId=%d", token.UserId, tokenId),
			token.Ref(), token.OwnerRef())
	}
	if userQuota < quota {
		// Running out of funds is the caller's condition, not a server fault: it
		// must log at WARN without a stack, whatever HTTP status the transport uses.
		return errkind.Quota(identity.Tag(
			errors.Errorf("insufficient user quota: required=%d, available=%d, userId=%d, tokenId=%d", quota, userQuota, token.UserId, tokenId),
			token.Ref(), token.OwnerRef()))
	}
	quotaTooLow := userQuota >= config.QuotaRemindThreshold && userQuota-quota < config.QuotaRemindThreshold
	noMoreQuota := userQuota-quota <= 0
	var reminderEmail string
	if quotaTooLow || noMoreQuota {
		// Value copies: safe to capture in the reminder goroutine below.
		tokenRef, ownerRef := token.Ref(), token.OwnerRef()
		var emailErr error
		reminderEmail, emailErr = GetUserEmail(token.UserId)
		if emailErr != nil {
			logger.Logger.Error("failed to fetch user email",
				append(ownerRef.AppendZap(tokenRef.Zap()), zap.Error(emailErr))...)
		}
		go func(email string, exhausted bool, quotaRemaining int64) {
			prompt := "Quota Reminder"
			var contentText string
			if exhausted {
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
					`, contentText, quotaRemaining, topUpLink, topUpLink),
				)
				// Local variable: assigning to the enclosing function's named
				// return value from this goroutine would race with the caller.
				// The address is never logged (see the no-email rule).
				if sendErr := message.SendEmail(prompt, email, content); sendErr != nil {
					logger.Logger.Error("failed to send email",
						append(ownerRef.AppendZap(tokenRef.Zap()), zap.Error(sendErr))...)
				}
			}
		}(reminderEmail, noMoreQuota, userQuota)
	}
	if !token.UnlimitedQuota {
		if err = DecreaseTokenQuota(ctx, tokenId, quota); err != nil {
			return identity.Tag(
				errors.Wrapf(err, "decrease quota for token %d", tokenId),
				token.Ref(), token.OwnerRef())
		}
	}
	if err = DecreaseUserQuota(ctx, token.UserId, quota); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "decrease quota for user %d in pre-consume", token.UserId),
			token.Ref(), token.OwnerRef())
	}
	return nil
}

func PostConsumeTokenQuota(ctx context.Context, tokenId int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	token, err := GetTokenById(tokenId)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "get token %d for post-consume", tokenId),
			identity.NewTokenRef(tokenId, "", ""))
	}
	if quota > 0 {
		err = DecreaseUserQuota(ctx, token.UserId, quota)
	} else {
		err = IncreaseUserQuota(ctx, token.UserId, -quota)
	}
	if !token.UnlimitedQuota {
		if quota > 0 {
			err = DecreaseTokenQuota(ctx, tokenId, quota)
		} else {
			err = IncreaseTokenQuota(ctx, tokenId, -quota)
		}
		if err != nil {
			return identity.Tag(
				errors.Wrapf(err, "adjust token %d quota in post-consume", tokenId),
				token.Ref(), token.OwnerRef())
		}
	}
	return nil
}
