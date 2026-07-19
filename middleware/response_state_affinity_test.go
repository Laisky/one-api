package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/state"
)

// setupAffinityTestDB installs an in-memory SQLite DB with a single enabled
// OpenAI channel and returns a cleanup-restoring handle.
func setupAffinityTestDB(t *testing.T, channel *dbmodel.Channel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&dbmodel.Channel{}))
	require.NoError(t, db.Create(channel).Error)

	originalDB := dbmodel.DB
	originalSQLite := common.UsingSQLite.Load()
	dbmodel.DB = db
	common.UsingSQLite.Store(true)
	t.Cleanup(func() {
		dbmodel.DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
	})
}

// enableAffinityState installs an in-memory state store, seeds a response node
// whose provider binding points at channelID, and returns the gateway id.
func enableAffinityState(t *testing.T, channelID int) string {
	t.Helper()
	store := state.NewMemoryStore(state.DefaultLimits())
	state.SetForTest(store)
	t.Cleanup(func() { state.SetForTest(nil) })

	gwID, err := state.NewResponseID()
	require.NoError(t, err)
	_, err = store.CreateResponse(context.Background(), &state.ResponseStateRecord{
		GatewayResponseID: gwID,
		Owner:             state.OwnerScope{UserID: 1, TokenID: 1},
		Status:            state.StatusCompleted,
		StoreMode:         true,
		Binding:           &state.ProviderBinding{ChannelID: channelID, ActualModel: "gpt-5"},
	}, "")
	require.NoError(t, err)
	return gwID
}

func affinityContext(t *testing.T, prevID string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := `{"model":"gpt-5","previous_response_id":"` + prevID + `","input":"hi"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	return c
}

// TestResponseStateAffinityPinsBoundChannel proves an eligible provider binding
// is preferred before normal selection (row R01, closing B12).
func TestResponseStateAffinityPinsBoundChannel(t *testing.T) {
	setupAffinityTestDB(t, &dbmodel.Channel{
		Id:     7,
		Type:   channeltype.OpenAI,
		Name:   "bound",
		Status: dbmodel.ChannelStatusEnabled,
		Models: "gpt-5",
		Group:  "default",
		Config: "{}",
	})
	gwID := enableAffinityState(t, 7)
	c := affinityContext(t, gwID)

	got := responseStateAffinityChannel(c, relaymode.ResponseAPI, "default", "gpt-5", false)
	require.NotNil(t, got)
	require.Equal(t, 7, got.Id)
}

// TestResponseStateAffinityDisabledIsNoOp proves the resolver is inert when the
// feature is off (row O01).
func TestResponseStateAffinityDisabledIsNoOp(t *testing.T) {
	setupAffinityTestDB(t, &dbmodel.Channel{
		Id: 7, Type: channeltype.OpenAI, Name: "bound",
		Status: dbmodel.ChannelStatusEnabled, Models: "gpt-5", Group: "default", Config: "{}",
	})
	require.False(t, state.Enabled())
	c := affinityContext(t, "resp_"+"00000000000000000000000000000000")
	require.Nil(t, responseStateAffinityChannel(c, relaymode.ResponseAPI, "default", "gpt-5", false))
}

// TestResponseStateAffinityUnknownIDFailsOpen proves an unresolvable selector
// falls back to normal selection rather than erroring (row R02/R05).
func TestResponseStateAffinityUnknownIDFailsOpen(t *testing.T) {
	setupAffinityTestDB(t, &dbmodel.Channel{
		Id: 7, Type: channeltype.OpenAI, Name: "bound",
		Status: dbmodel.ChannelStatusEnabled, Models: "gpt-5", Group: "default", Config: "{}",
	})
	// Enable state but reference an id that was never stored.
	store := state.NewMemoryStore(state.DefaultLimits())
	state.SetForTest(store)
	t.Cleanup(func() { state.SetForTest(nil) })
	unknown, err := state.NewResponseID()
	require.NoError(t, err)
	c := affinityContext(t, unknown)
	require.Nil(t, responseStateAffinityChannel(c, relaymode.ResponseAPI, "default", "gpt-5", false))
}

// TestResponseStateAffinityDisabledChannelFallsBack proves a binding to a disabled
// channel is not pinned so normal selection can replay canonically (row R02).
func TestResponseStateAffinityDisabledChannelFallsBack(t *testing.T) {
	setupAffinityTestDB(t, &dbmodel.Channel{
		Id: 7, Type: channeltype.OpenAI, Name: "bound",
		Status: dbmodel.ChannelStatusManuallyDisabled, Models: "gpt-5", Group: "default", Config: "{}",
	})
	gwID := enableAffinityState(t, 7)
	c := affinityContext(t, gwID)
	require.Nil(t, responseStateAffinityChannel(c, relaymode.ResponseAPI, "default", "gpt-5", false))
}

// TestResponseStateAffinityModelMismatchFallsBack proves a binding whose channel
// no longer serves the requested model is not pinned.
func TestResponseStateAffinityModelMismatchFallsBack(t *testing.T) {
	setupAffinityTestDB(t, &dbmodel.Channel{
		Id: 7, Type: channeltype.OpenAI, Name: "bound",
		Status: dbmodel.ChannelStatusEnabled, Models: "gpt-4o", Group: "default", Config: "{}",
	})
	gwID := enableAffinityState(t, 7)
	c := affinityContext(t, gwID)
	require.Nil(t, responseStateAffinityChannel(c, relaymode.ResponseAPI, "default", "gpt-5", false))
}
