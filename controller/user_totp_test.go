package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate the tables
	err = db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Token{}, &model.Option{}, &model.Redemption{}, &model.Ability{}, &model.Log{}, &model.UserRequestCost{})
	require.NoError(t, err)

	return db
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Setup session middleware
	store := cookie.NewStore([]byte("test-secret"))
	router.Use(sessions.Sessions("test-session", store))

	return router
}

func setupTestEnvironment(t *testing.T) (*gorm.DB, func()) {
	// Setup test database
	testDB := setupTestDB(t)

	// Store original DB and replace with test DB
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	model.DB = testDB
	model.LOG_DB = testDB // Use same DB for logging in tests

	// Set SQLite flag for proper query handling
	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)

	// Disable Redis for testing to use memory-based rate limiting
	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)

	// Create a test user for TOTP tests
	testUser := &model.User{
		Id:          1,
		Username:    "testuser",
		Password:    "hashedpassword",
		Role:        model.RoleCommonUser,
		Status:      model.UserStatusEnabled,
		DisplayName: "Test User",
		Email:       "test@example.com",
		AccessToken: "test-access-token-1",
		AffCode:     "TEST1",
		TotpSecret:  "",
	}
	err := testDB.Create(testUser).Error
	require.NoError(t, err)

	// Return cleanup function
	cleanup := func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
		common.SetRedisEnabled(originalRedisEnabled)
	}

	return testDB, cleanup
}

func TestTotpBasicFunctionality(t *testing.T) {
	// Use a known valid base32 secret
	secret := "JBSWY3DPEHPK3PXP" // Base32 encoded "Hello!"

	// Create TOTP instance
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
	})
	assert.NoError(t, err)

	// Generate current code
	currentCode := totp.Key()
	assert.Len(t, currentCode, 6) // TOTP codes should be 6 digits

	// Create another TOTP instance with same secret to verify
	totp2, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
	})
	assert.NoError(t, err)

	// The codes should match
	assert.Equal(t, currentCode, totp2.Key())
}

func TestSetupTotp(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	router := setupTestRouter()
	router.GET("/totp/setup", func(c *gin.Context) {
		// Mock user ID in context
		c.Set(ctxkey.Id, 1)
		SetupTotp(c)
	})

	req, _ := http.NewRequest("GET", "/totp/setup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response to verify TOTP setup data
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))

	data := response["data"].(map[string]any)
	secret := data["secret"].(string)
	qrCode := data["qr_code"].(string)

	// Verify secret is not empty and is valid base32
	assert.NotEmpty(t, secret)
	assert.Regexp(t, "^[A-Z2-7]+=*$", secret, "Secret should be valid base32")

	// Verify QR code URI format and proper encoding
	assert.NotEmpty(t, qrCode)
	assert.Contains(t, qrCode, "otpauth://totp/", "QR code should start with otpauth://totp/")
	assert.Contains(t, qrCode, secret, "QR code should contain the secret")
	assert.Contains(t, qrCode, "testuser", "QR code should contain the username")

	// Verify proper URI encoding - spaces should be %20, not double-encoded
	// The issuer name should be properly encoded
	assert.Contains(t, qrCode, "issuer=", "QR code should contain issuer parameter")

	// Test that the URI can be parsed correctly
	assert.NotContains(t, qrCode, "%%", "QR code should not contain double-encoded characters")
	assert.NotContains(t, qrCode, "%2520", "QR code should not contain double-encoded spaces")

	// Verify the secret can be used to create a valid TOTP instance
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
	})
	assert.NoError(t, err, "Secret from setup should be valid for TOTP creation")
	assert.NotNil(t, totp)

	// Verify that a code can be generated
	code := totp.Key()
	assert.Len(t, code, 6, "Generated TOTP code should be 6 digits")
	assert.Regexp(t, "^[0-9]{6}$", code, "TOTP code should be 6 digits")
}

func TestTotpSetupRequest(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	router := setupTestRouter()

	// First, set up TOTP to get a secret in session
	router.GET("/totp/setup", func(c *gin.Context) {
		c.Set(ctxkey.Id, 1)
		SetupTotp(c)
	})

	router.POST("/totp/confirm", func(c *gin.Context) {
		c.Set(ctxkey.Id, 1)
		ConfirmTotp(c)
	})

	// Step 1: Call setup to get secret and store it in session
	setupReq, _ := http.NewRequest("GET", "/totp/setup", nil)
	setupW := httptest.NewRecorder()
	router.ServeHTTP(setupW, setupReq)

	assert.Equal(t, http.StatusOK, setupW.Code)

	// Parse setup response to get the secret
	var setupResponse map[string]any
	err := json.Unmarshal(setupW.Body.Bytes(), &setupResponse)
	assert.NoError(t, err)
	assert.True(t, setupResponse["success"].(bool))

	data := setupResponse["data"].(map[string]any)
	secret := data["secret"].(string)

	// Step 2: Generate a valid TOTP code using the secret from setup
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
	})
	require.NoError(t, err)
	validCode := totp.Key()

	// Step 3: Confirm TOTP with the valid code
	// We need to use the same session, so we'll extract cookies from setup response
	reqBody := TotpSetupRequest{
		TotpCode: validCode,
	}
	jsonBody, _ := json.Marshal(reqBody)

	confirmReq, _ := http.NewRequest("POST", "/totp/confirm", bytes.NewBuffer(jsonBody))
	confirmReq.Header.Set("Content-Type", "application/json")

	// Copy cookies from setup request to maintain session
	for _, cookie := range setupW.Result().Cookies() {
		confirmReq.AddCookie(cookie)
	}

	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)

	assert.Equal(t, http.StatusOK, confirmW.Code)

	// Parse response to verify success
	var confirmResponse map[string]any
	err = json.Unmarshal(confirmW.Body.Bytes(), &confirmResponse)
	assert.NoError(t, err)
	assert.True(t, confirmResponse["success"].(bool))
}

func TestLoginRequest(t *testing.T) {
	// Test LoginRequest struct
	loginReq := LoginRequest{
		Username: "testuser",
		Password: "testpass",
		TotpCode: "123456",
	}

	assert.Equal(t, "testuser", loginReq.Username)
	assert.Equal(t, "testpass", loginReq.Password)
	assert.Equal(t, "123456", loginReq.TotpCode)
}

func TestTotpSetupResponse(t *testing.T) {
	// Test TotpSetupResponse struct
	response := TotpSetupResponse{
		Secret: "ABCDEFGHIJKLMNOP",
		QRCode: "otpauth://totp/test",
	}

	assert.Equal(t, "ABCDEFGHIJKLMNOP", response.Secret)
	assert.Equal(t, "otpauth://totp/test", response.QRCode)
}

func TestTotpCodeGeneration(t *testing.T) {
	// Test TOTP code generation with known parameters
	secret := "JBSWY3DPEHPK3PXP" // Base32 encoded "Hello!"

	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
		AccountName:  "test@example.com",
		IssuerName:   "Test App",
	})
	assert.NoError(t, err)

	// Generate code
	code := totp.Key()
	assert.Len(t, code, 6) // TOTP codes should be 6 digits

	// Create another TOTP instance to verify
	totp2, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
		AccountName:  "test@example.com",
		IssuerName:   "Test App",
	})
	assert.NoError(t, err)

	// The codes should match
	assert.Equal(t, code, totp2.Key())

	// Test URI generation
	uri := totp.URI()
	assert.Contains(t, uri, "otpauth://totp/")
	assert.Contains(t, uri, "test@example.com")
	assert.Contains(t, uri, "Test")
	assert.Contains(t, uri, secret)
}

func TestTotpReplayProtection(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Test TOTP replay protection
	secret := "JBSWY3DPEHPK3PXP"
	userId := 123

	ctx := context.Background()

	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
	})
	assert.NoError(t, err)

	code := totp.Key()

	// First verification should succeed
	assert.True(t, verifyTotpCode(ctx, userId, secret, code))

	// Second verification with same code should fail (replay protection)
	assert.False(t, verifyTotpCode(ctx, userId, secret, code))

	// Different user should still be able to use the same code
	assert.True(t, verifyTotpCode(ctx, userId+1, secret, code))
}

func TestTotpSecurityFunctions(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	userId := 456
	code := "123456"

	ctx := context.Background()

	// Initially, code should not be marked as used
	assert.False(t, common.IsTotpCodeUsed(ctx, userId, code))

	// Mark code as used
	err := common.MarkTotpCodeAsUsed(ctx, userId, code)
	assert.NoError(t, err)

	// Now code should be marked as used
	assert.True(t, common.IsTotpCodeUsed(ctx, userId, code))

	// Different user should not be affected
	assert.False(t, common.IsTotpCodeUsed(ctx, userId+1, code))

	// Different code should not be affected
	assert.False(t, common.IsTotpCodeUsed(ctx, userId, "654321"))
}

func TestAdminDisableUserTotp(t *testing.T) {
	testDB, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create an admin user
	adminUser := &model.User{
		Id:          2,
		Username:    "admin",
		Password:    "hashedpassword",
		Role:        model.RoleAdminUser,
		Status:      model.UserStatusEnabled,
		DisplayName: "Admin User",
		Email:       "admin@example.com",
		AccessToken: "test-access-token-2",
		AffCode:     "ADMIN",
	}
	err := testDB.Create(adminUser).Error
	require.NoError(t, err)

	// Create a target user with TOTP enabled
	targetUser := &model.User{
		Id:          3,
		Username:    "target",
		Password:    "hashedpassword",
		Role:        model.RoleCommonUser,
		Status:      model.UserStatusEnabled,
		DisplayName: "Target User",
		Email:       "target@example.com",
		AccessToken: "test-access-token-3",
		AffCode:     "TARG3",
		TotpSecret:  "JBSWY3DPEHPK3PXP",
	}
	err = testDB.Create(targetUser).Error
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/admin/totp/disable/:id", func(c *gin.Context) {
		// Mock admin user context
		c.Set(ctxkey.Id, 2)                     // Admin user ID
		c.Set(ctxkey.Role, model.RoleAdminUser) // Admin role
		AdminDisableUserTotp(c)
	})

	// Test with valid user ID
	req, _ := http.NewRequest("POST", "/admin/totp/disable/3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response to verify success
	var response map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))

	// Verify that TOTP secret was cleared from database
	var updatedUser model.User
	err = testDB.First(&updatedUser, 3).Error
	assert.NoError(t, err)
	assert.Empty(t, updatedUser.TotpSecret)

	// Test with invalid user ID
	req, _ = http.NewRequest("POST", "/admin/totp/disable/invalid", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response["success"].(bool))
	assert.Equal(t, "Invalid user ID", response["message"])
}
