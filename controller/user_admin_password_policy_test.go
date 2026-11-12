package controller

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
)

// TestAdminUserPasswordPolicy verifies the shared frontend fixtures against actual
// create/update handlers and the stored password hashes without changing policy.
// Parameter t runs the endpoint cases; the test returns no value.
func TestAdminUserPasswordPolicy(t *testing.T) {
	data, err := os.ReadFile("../web/modern/src/pages/users/__tests__/testdata/admin-user-passwords.json")
	require.NoError(t, err)
	var cases []struct {
		Name        string `json:"name"`
		Password    string `json:"password"`
		CreateValid bool   `json:"createValid"`
		EditValid   bool   `json:"editValid"`
	}
	require.NoError(t, json.Unmarshal(data, &cases))
	router := setupGroupAssignmentTest(t)
	target := createLockTargetUser(t, "policy-update", false)

	for index, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			username := "policy-create-" + strconv.Itoa(index)
			created := sendGroupAssignmentRequest(t, router, http.MethodPost, "/api/user/",
				map[string]any{"username": username, "password": tc.Password})
			require.Equal(t, tc.CreateValid, created.Success, "create: %s", created.Message)
			var stored model.User
			if tc.CreateValid {
				require.NoError(t, model.DB.Where("username = ?", username).First(&stored).Error)
				require.True(t, common.ValidatePasswordAndHash(tc.Password, stored.Password), "creation must preserve whitespace")
			} else {
				var count int64
				require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error)
				require.Zero(t, count)
			}

			stored = model.User{}
			require.NoError(t, model.DB.First(&stored, target.Id).Error)
			previousHash := stored.Password
			updated := sendGroupAssignmentRequest(t, router, http.MethodPut, "/api/user/",
				map[string]any{"uuid": target.UUID, "password": tc.Password})
			require.Equal(t, tc.EditValid, updated.Success, "edit: %s", updated.Message)
			stored = model.User{}
			require.NoError(t, model.DB.First(&stored, target.Id).Error)
			if tc.EditValid {
				require.True(t, common.ValidatePasswordAndHash(strings.TrimSpace(tc.Password), stored.Password), "updates must hash the Go-trimmed password")
			} else {
				require.Equal(t, previousHash, stored.Password, "invalid edits must preserve the password")
			}
		})
	}
}
