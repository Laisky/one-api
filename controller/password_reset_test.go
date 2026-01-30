package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
)

func TestSendPasswordResetEmailSecurity(t *testing.T) {
	setupUserControllerTest(t)

	// Create a registered user
	registeredEmail := "registered@example.com"
	user := &model.User{
		Username: "testuser",
		Email:    registeredEmail,
		Password: "password123",
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	router := gin.New()
	router.GET("/api/reset_password", SendPasswordResetEmail)

	tests := []struct {
		name  string
		email string
	}{
		{
			name:  "Registered Email",
			email: registeredEmail,
		},
		{
			name:  "Unregistered Email",
			email: "unregistered@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/reset_password?email="+tt.email, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var resp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

			// Both should return success: true and the same message (empty)
			require.True(t, resp.Success)
			require.Equal(t, "", resp.Message)
		})
	}

	// Wait a bit for background goroutines to finish to avoid "database is closed" logs
	time.Sleep(100 * time.Millisecond)
}

func TestSendEmailVerificationSecurity(t *testing.T) {
	setupUserControllerTest(t)

	// Create a registered user
	registeredEmail := "taken@example.com"
	user := &model.User{
		Username: "takenuser",
		Email:    registeredEmail,
		Password: "password123",
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	router := gin.New()
	router.GET("/api/verification", SendEmailVerification)

	tests := []struct {
		name  string
		email string
	}{
		{
			name:  "Email Already Taken",
			email: registeredEmail,
		},
		{
			name:  "Email Available",
			email: "available@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/verification?email="+tt.email, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var resp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

			// Both should return success: true and the same message (empty)
			require.True(t, resp.Success)
			require.Equal(t, "", resp.Message)
		})
	}

	// Wait a bit for background goroutines to finish
	time.Sleep(100 * time.Millisecond)
}
