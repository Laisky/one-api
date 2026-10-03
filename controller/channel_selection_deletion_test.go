package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestSelectedDeletionReportsAlreadyMissing verifies HTTP outcomes when a
// confirmed target disappears between snapshot resolution and its transaction.
// A deterministic trigger removes the second target while the first commits.
func TestSelectedDeletionReportsAlreadyMissing(t *testing.T) {
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	channels := []model.Channel{
		{Name: "deleted", Models: "Foo", Group: "default", Status: model.ChannelStatusManuallyDisabled},
		{Name: "already-gone", Models: "Foo", Group: "default", Status: model.ChannelStatusManuallyDisabled},
		{Name: "enabled", Models: "Foo", Group: "default", Status: model.ChannelStatusEnabled},
		{Name: "write-failure", Models: "Foo", Group: "default", Status: model.ChannelStatusManuallyDisabled},
	}
	ids := make([]string, 0, len(channels))
	for i := range channels {
		require.NoError(t, channels[i].Insert())
		ids = append(ids, channels[i].UUID)
	}
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`
		CREATE TRIGGER remove_later_selected_channel AFTER DELETE ON channels WHEN OLD.id = %d
		BEGIN
			DELETE FROM abilities WHERE channel_id = %d;
			DELETE FROM channels WHERE id = %d;
		END`, channels[0].Id, channels[1].Id, channels[1].Id)).Error)
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`
		CREATE TRIGGER reject_selected_delete BEFORE DELETE ON channels WHEN OLD.id = %d
		BEGIN SELECT RAISE(FAIL, 'private-delete-failure'); END`, channels[3].Id)).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { gmwSetLoggerForUUIDContract(c) })
	engine.POST("/delete", DeleteSelectedDisabledChannels)
	body, err := json.Marshal(channelSelectionRequest{Selection: model.ListSelection{Mode: "ids", IDs: ids}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/delete", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, recorder.Code)
	var result struct {
		Success bool `json:"success"`
		Data    []struct {
			UUID    string `json:"uuid"`
			Success bool   `json:"success"`
			Skipped bool   `json:"skipped"`
			Message string `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Len(t, result.Data, 4)
	require.True(t, result.Data[0].Success)
	require.Equal(t, channels[1].UUID, result.Data[1].UUID)
	require.False(t, result.Data[1].Success)
	require.True(t, result.Data[1].Skipped)
	require.Equal(t, "This channel no longer exists.", result.Data[1].Message)
	require.False(t, result.Data[2].Success)
	require.True(t, result.Data[2].Skipped)
	require.Empty(t, result.Data[2].Message, "the existing enabled-skip fallback remains applicable")
	require.False(t, result.Data[3].Success)
	require.False(t, result.Data[3].Skipped)
	require.Equal(t, "Failed to delete this channel.", result.Data[3].Message)
	require.NotContains(t, recorder.Body.String(), "private-delete-failure")
	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id IN ?", []int{channels[0].Id, channels[1].Id}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", channels[3].Id).Count(&count).Error)
	require.EqualValues(t, 1, count, "the operational failure still rolls back its ability deletion")
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.channel.Id).Count(&count).Error)
	require.EqualValues(t, 1, count, "the unselected channel is untouched")
}
