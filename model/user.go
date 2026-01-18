package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/blacklist"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
)

const (
	RoleGuestUser  = 0
	RoleCommonUser = 1
	RoleAdminUser  = 10
	RoleRootUser   = 100
)

const (
	UserStatusEnabled  = 1 // don't use 0, 0 is the default value!
	UserStatusDisabled = 2 // also don't use 0
	UserStatusDeleted  = 3
)

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id               int             `json:"id"`
	Username         string          `json:"username" gorm:"unique;index" validate:"max=30"`
	Password         string          `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	DisplayName      string          `json:"display_name" gorm:"index" validate:"max=20"`
	Role             int             `json:"role" gorm:"type:int;default:1"`   // admin, util
	Status           int             `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email            string          `json:"email" gorm:"index" validate:"max=50"`
	GitHubId         string          `json:"github_id" gorm:"column:github_id;index"`
	WeChatId         string          `json:"wechat_id" gorm:"column:wechat_id;index"`
	LarkId           string          `json:"lark_id" gorm:"column:lark_id;index"`
	OidcId           string          `json:"oidc_id" gorm:"column:oidc_id;index"`
	VerificationCode string          `json:"verification_code" gorm:"-:all"`                                    // this field is only for Email verification, don't save it to database!
	AccessToken      string          `json:"access_token" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	TotpSecret       string          `json:"totp_secret,omitempty" gorm:"type:varchar(64);column:totp_secret"`  // TOTP secret for 2FA, omit from JSON when empty
	Quota            int64           `json:"quota" gorm:"bigint;default:0"`
	UsedQuota        int64           `json:"used_quota" gorm:"bigint;default:0;column:used_quota"` // used quota
	RequestCount     int             `json:"request_count" gorm:"type:int;default:0;"`             // request number
	Group            string          `json:"group" gorm:"type:varchar(32);default:'default'"`
	AffCode          string          `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	InviterId        int             `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	MCPToolBlacklist JSONStringSlice `json:"mcp_tool_blacklist" gorm:"type:text"`
	CreatedAt        int64           `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt        int64           `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

func GetMaxUserId() int {
	var user User
	DB.Last(&user)
	return user.Id
}

func GetAllUsers(startIdx int, num int, order string, sortBy string, sortOrder string) (users []*User, err error) {
	query := DB.Limit(num).Offset(startIdx).Omit("password").Where("status != ?", UserStatusDeleted)

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
		case "quota":
			query = query.Order("quota desc")
		case "used_quota":
			query = query.Order("used_quota desc")
		case "request_count":
			query = query.Order("request_count desc")
		default:
			query = query.Order("id desc")
		}
	}

	err = query.Find(&users).Error
	return users, err
}

func GetUserCount() (count int64, err error) {
	err = DB.Model(&User{}).Where("status != ?", UserStatusDeleted).Count(&count).Error
	return count, err
}

func SearchUsers(keyword string, sortBy string, sortOrder string) (users []*User, err error) {
	// Default sorting
	orderClause := "id desc"
	if sortBy != "" {
		if sortOrder == "asc" {
			orderClause = sortBy + " asc"
		} else {
			orderClause = sortBy + " desc"
		}
	}

	if !common.UsingPostgreSQL.Load() {
		err = DB.Omit("password").Where("id = ? or username LIKE ? or email LIKE ? or display_name LIKE ?", keyword, keyword+"%", keyword+"%", keyword+"%").Order(orderClause).Find(&users).Error
	} else {
		err = DB.Omit("password").Where("username LIKE ? or email LIKE ? or display_name LIKE ?", keyword+"%", keyword+"%", keyword+"%").Order(orderClause).Find(&users).Error
	}
	return users, err
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id is empty!")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password", "access_token").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode is empty!")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id is empty!")
	}
	user := User{Id: id}
	return user.Delete()
}

func (user *User) Insert(ctx context.Context, inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return errors.Wrapf(err, "failed to hash password for user: username=%s", user.Username)
		}
	}
	user.Quota = config.QuotaForNewUser
	user.AccessToken = random.GetUUID()
	user.AffCode = random.GetRandomString(4)
	result := DB.Create(user)
	if result.Error != nil {
		return errors.Wrapf(result.Error, "failed to create user: username=%s, inviterId=%d", user.Username, inviterId)
	}
	if config.QuotaForNewUser > 0 {
		RecordLog(ctx, user.Id, LogTypeSystem, fmt.Sprintf("New user registration gift %s", common.LogQuota(config.QuotaForNewUser)))
	}
	if inviterId != 0 {
		if config.QuotaForInvitee > 0 {
			_ = IncreaseUserQuota(ctx, user.Id, config.QuotaForInvitee)
			RecordLog(ctx, user.Id, LogTypeSystem, fmt.Sprintf("Gifted %s for using invitation code", common.LogQuota(config.QuotaForInvitee)))
		}
		if config.QuotaForInviter > 0 {
			_ = IncreaseUserQuota(ctx, inviterId, config.QuotaForInviter)
			RecordLog(ctx, inviterId, LogTypeSystem, fmt.Sprintf("Gifted %s for inviting user", common.LogQuota(config.QuotaForInviter)))
		}
	}
	// create default token
	cleanToken := Token{
		UserId:         user.Id,
		Name:           "default",
		Key:            random.GenerateKey(),
		CreatedTime:    helper.GetTimestamp(),
		AccessedTime:   helper.GetTimestamp(),
		ExpiredTime:    -1,
		RemainQuota:    -1,
		UnlimitedQuota: true,
	}
	result.Error = cleanToken.Insert(ctx)
	if result.Error != nil {
		// do not block
		logger.Logger.Error("create default token for user failed",
			zap.Int("user_id", user.Id),
			zap.Error(result.Error))
	}
	return nil
}

func (user *User) Update(updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return errors.Wrapf(err, "failed to hash password for user update: id=%d, username=%s", user.Id, user.Username)
		}
	}
	switch user.Status {
	case UserStatusDisabled:
		blacklist.BanUser(user.Id)
	case UserStatusEnabled:
		blacklist.UnbanUser(user.Id)
	}
	err = DB.Model(user).Updates(user).Error
	if err != nil {
		return errors.Wrapf(err, "failed to update user: id=%d, username=%s", user.Id, user.Username)
	}
	return nil
}

// ClearTotpSecret clears the TOTP secret for the user
func (user *User) ClearTotpSecret() error {
	err := DB.Model(user).Select("totp_secret").Updates(map[string]any{
		"totp_secret": "",
	}).Error
	if err != nil {
		return errors.Wrapf(err, "failed to clear TOTP secret for user: id=%d", user.Id)
	}
	return nil
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id is empty!")
	}
	blacklist.BanUser(user.Id)
	user.Username = fmt.Sprintf("deleted_%s", random.GetUUID())
	user.Status = UserStatusDeleted
	err := DB.Model(user).Updates(user).Error
	if err != nil {
		return errors.Wrapf(err, "failed to delete user: id=%d", user.Id)
	}
	return nil
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field’s value is 0, '', false or other zero values,
	// it won’t be used to build query conditions
	password := user.Password
	if user.Username == "" || password == "" {
		return errors.New("Username or password is empty")
	}
	err = DB.Where("username = ?", user.Username).First(user).Error
	if err != nil {
		// we must make sure check username firstly
		// consider this case: a malicious user set his username as other's email
		err := DB.Where("email = ?", user.Username).First(user).Error
		if err != nil {
			return errors.Errorf("username or password is wrong, or user has been banned: username=%s", user.Username)
		}
	}
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != UserStatusEnabled {
		return errors.New("Username or password is wrong, or user has been banned")
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id is empty!")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email is empty!")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id is empty!")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

func (user *User) FillUserByLarkId() error {
	if user.LarkId == "" {
		return errors.New("lark id is empty!")
	}
	DB.Where(User{LarkId: user.LarkId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id is empty!")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id is empty!")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByUsername() error {
	if user.Username == "" {
		return errors.New("username is empty!")
	}
	DB.Where(User{Username: user.Username}).First(user)
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	return DB.Where("email = ?", email).Find(&User{}).RowsAffected == 1
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsLarkIdAlreadyTaken(githubId string) bool {
	return DB.Where("lark_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsUsernameAlreadyTaken(username string) bool {
	return DB.Where("username = ?", username).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("Email address or password is empty!")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return errors.Wrap(err, "hash password for reset")
	}
	if err = DB.Model(&User{}).Where("email = ?", email).Update("password", hashedPassword).Error; err != nil {
		return errors.Wrapf(err, "update password for email %s", email)
	}
	return nil
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		logger.Logger.Error("no such user", zap.Error(err))
		return false
	}
	return user.Role >= RoleAdminUser
}

func IsUserEnabled(userId int) (bool, error) {
	if userId == 0 {
		return false, errors.New("user id is empty")
	}
	var user User
	err := DB.Where("id = ?", userId).Select("status").Find(&user).Error
	if err != nil {
		return false, errors.Wrapf(err, "query user %d status", userId)
	}
	return user.Status == UserStatusEnabled, nil
}

func ValidateAccessToken(token string) (user *User) {
	if token == "" {
		return nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user = &User{}
	if DB.Where("access_token = ?", token).First(user).RowsAffected == 1 {
		return user
	}
	return nil
}

func GetUserQuota(id int) (quota int64, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, errors.Wrapf(err, "get quota for user %d", id)
	}
	return quota, nil
}

func GetUserUsedQuota(id int) (quota int64, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	if err != nil {
		return 0, errors.Wrapf(err, "get used quota for user %d", id)
	}
	return quota, nil
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	if err != nil {
		return "", errors.Wrapf(err, "get email for user %d", id)
	}
	return email, nil
}

func GetUserGroup(id int) (group string, err error) {
	groupCol := "`group`"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
	}

	err = DB.Model(&User{}).Where("id = ?", id).Select(groupCol).Find(&group).Error
	if err != nil {
		return "", errors.Wrapf(err, "get group for user %d", id)
	}
	return group, nil
}

// IncreaseUserQuota increases the quota for a user by the given amount.
// ctx is the context for the operation; if nil, context.Background() is used.
func IncreaseUserQuota(ctx context.Context, id int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if quota < 0 {
		return errors.New("quota cannot be negative!")
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, quota)
		return nil
	}
	return increaseUserQuota(ctx, id, quota)
}

func increaseUserQuota(ctx context.Context, id int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	err = runWithSQLiteBusyRetry(ctx, func() error {
		return DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	})
	if err != nil {
		return errors.Wrapf(err, "increase quota for user %d", id)
	}
	return nil
}

// DecreaseUserQuota decreases the quota for a user by the given amount.
// ctx is the context for the operation; if nil, context.Background() is used.
func DecreaseUserQuota(ctx context.Context, id int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if quota < 0 {
		return errors.New("quota cannot be negative!")
	}
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUserQuota, id, -quota)
		return nil
	}
	return decreaseUserQuota(ctx, id, quota)
}

func decreaseUserQuota(ctx context.Context, id int, quota int64) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var result *gorm.DB
	err = runWithSQLiteBusyRetry(ctx, func() error {
		result = DB.Model(&User{}).
			Where("id = ? AND quota >= ?", id, quota).
			Update("quota", gorm.Expr("quota - ?", quota))
		return result.Error
	})
	if err != nil {
		return errors.Wrapf(err, "decrease quota for user %d", id)
	}
	if result.RowsAffected == 0 {
		return errors.Errorf("insufficient user quota for user %d", id)
	}
	return nil
}

func GetRootUserEmail() (email string) {
	DB.Model(&User{}).Where("role = ?", RoleRootUser).Select("email").Find(&email)
	return email
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int64) {
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

func updateUserUsedQuotaAndRequestCount(id int, quota int64, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		logger.Logger.Error("failed to update user used quota and request count - statistics may be inaccurate",
			zap.Error(err),
			zap.Int("userId", id),
			zap.Int64("quota", quota),
			zap.Int("count", count),
			zap.String("note", "billing completed successfully but usage statistics update failed"))
	}
}

func updateUserUsedQuota(id int, quota int64) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]any{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		logger.Logger.Error("failed to update user used quota", zap.Error(err))
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		logger.Logger.Error("failed to update user request count", zap.Error(err))
	}
}

func GetUsernameById(id int) (username string) {
	DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username)
	return username
}

func GetSiteWideQuotaStats() (totalQuota int64, usedQuota int64, status string, err error) {
	var result struct {
		TotalQuota  int64 `json:"total_quota"`
		UsedQuota   int64 `json:"used_quota"`
		ActiveUsers int64 `json:"active_users"`
		TotalUsers  int64 `json:"total_users"`
	}

	// Get aggregated quota statistics for all users
	err = DB.Model(&User{}).
		Select("SUM(quota) as total_quota, SUM(used_quota) as used_quota, COUNT(CASE WHEN status = ? THEN 1 END) as active_users, COUNT(*) as total_users", UserStatusEnabled).
		Where("status != ?", UserStatusDeleted).
		Scan(&result).Error

	if err != nil {
		return 0, 0, "", errors.Wrapf(err, "failed to get site-wide quota statistics")
	}

	totalQuota = result.TotalQuota
	usedQuota = result.UsedQuota

	// Determine overall status based on active vs total users
	switch result.ActiveUsers {
	case 0:
		status = "No Active Users"
	case result.TotalUsers:
		status = "All Active"
	default:
		status = fmt.Sprintf("%d/%d Active", result.ActiveUsers, result.TotalUsers)
	}

	return totalQuota, usedQuota, status, nil
}
