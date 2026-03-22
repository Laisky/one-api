package model

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/logger"
)

// PasskeyCredential stores a single WebAuthn credential (passkey) for a user.
// Each user may have multiple credentials (e.g. multiple devices).
type PasskeyCredential struct {
	Id              int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId          int    `json:"user_id" gorm:"index;not null"`
	CredentialName  string `json:"credential_name" gorm:"type:varchar(128);not null"`  // human-friendly label
	CredentialID    []byte `json:"-" gorm:"type:varbinary(1024);uniqueIndex;not null"` // raw credential ID
	PublicKey       []byte `json:"-" gorm:"type:varbinary(1024);not null"`             // COSE public key
	AttestationType string `json:"-" gorm:"type:varchar(64)"`                          // attestation type
	AAGUID          []byte `json:"-" gorm:"type:varbinary(64)"`                        // authenticator AAGUID
	SignCount       uint32 `json:"sign_count" gorm:"type:int unsigned;default:0"`      // signature counter
	Transport       string `json:"-" gorm:"type:varchar(256)"`                         // comma-separated transports
	CreatedAt       int64  `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

func (PasskeyCredential) TableName() string {
	return "passkey_credentials"
}

// GetPasskeyCredentialsByUserId returns all passkey credentials for a user.
func GetPasskeyCredentialsByUserId(userId int) ([]*PasskeyCredential, error) {
	var creds []*PasskeyCredential
	err := DB.Where("user_id = ?", userId).Find(&creds).Error
	if err != nil {
		return nil, errors.Wrapf(err, "get passkey credentials for user %d", userId)
	}
	return creds, nil
}

// GetPasskeyCredentialByID returns a single credential by primary key.
func GetPasskeyCredentialByID(id int) (*PasskeyCredential, error) {
	var cred PasskeyCredential
	err := DB.First(&cred, "id = ?", id).Error
	if err != nil {
		return nil, errors.Wrapf(err, "get passkey credential %d", id)
	}
	return &cred, nil
}

// GetPasskeyCredentialByCredentialID looks up a credential by its raw credential ID.
func GetPasskeyCredentialByCredentialID(credID []byte) (*PasskeyCredential, error) {
	var cred PasskeyCredential
	err := DB.First(&cred, "credential_id = ?", credID).Error
	if err != nil {
		return nil, errors.Wrapf(err, "get passkey credential by credential_id")
	}
	return &cred, nil
}

// CreatePasskeyCredential inserts a new passkey credential.
func CreatePasskeyCredential(cred *PasskeyCredential) error {
	err := DB.Create(cred).Error
	if err != nil {
		return errors.Wrapf(err, "create passkey credential for user %d", cred.UserId)
	}
	return nil
}

// DeletePasskeyCredential removes a passkey credential by id and user_id.
func DeletePasskeyCredential(id, userId int) error {
	result := DB.Where("id = ? AND user_id = ?", id, userId).Delete(&PasskeyCredential{})
	if result.Error != nil {
		return errors.Wrapf(result.Error, "delete passkey credential %d for user %d", id, userId)
	}
	if result.RowsAffected == 0 {
		return errors.Errorf("passkey credential %d not found for user %d", id, userId)
	}
	return nil
}

// UpdatePasskeySignCount updates the sign count after a successful authentication.
func UpdatePasskeySignCount(id int, signCount uint32) {
	err := DB.Model(&PasskeyCredential{}).Where("id = ?", id).
		Update("sign_count", signCount).Error
	if err != nil {
		logger.Logger.Error("failed to update passkey sign count",
			zap.Int("id", id), zap.Error(err))
	}
}

// HasPasskeyCredentials returns true if the user has at least one passkey registered.
func HasPasskeyCredentials(userId int) bool {
	var count int64
	DB.Model(&PasskeyCredential{}).Where("user_id = ?", userId).Count(&count)
	return count > 0
}

// DeletePasskeyCredentialsByUserId removes all passkeys for a user (admin use).
func DeletePasskeyCredentialsByUserId(userId int) error {
	err := DB.Where("user_id = ?", userId).Delete(&PasskeyCredential{}).Error
	if err != nil {
		return errors.Wrapf(err, "delete all passkey credentials for user %d", userId)
	}
	return nil
}
