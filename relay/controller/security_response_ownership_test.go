package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

// unavailableResponseStore returns storage failures from authorization operations while retaining the store interface.
type unavailableResponseStore struct{ state.ResponseStateStore }

// GetResponse returns an unavailable-store error for the supplied owner and identifier.
func (s unavailableResponseStore) GetResponse(context.Context, state.OwnerScope, string) (*state.ResponseStateRecord, error) {
	return nil, errors.WithStack(state.ErrStoreUnavailable)
}

// GetResponseBinding returns an unavailable-store error for the supplied owner and identifier.
func (s unavailableResponseStore) GetResponseBinding(context.Context, state.OwnerScope, string) (*state.ProviderBinding, error) {
	return nil, errors.WithStack(state.ErrStoreUnavailable)
}

// DeleteResponse returns an unavailable-store error for the supplied owner and identifier.
func (s unavailableResponseStore) DeleteResponse(context.Context, state.OwnerScope, string) error {
	return errors.WithStack(state.ErrStoreUnavailable)
}

// TestSecurityResponseOwnership rejects missing ownership before native continuation or object dispatch and keeps owner controls usable.
func TestSecurityResponseOwnership(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, scenario := range []string{"owner", "foreign-user", "foreign-token", "expired", "deleted", "unknown", "disabled", "unavailable", "invalid-owner"} {
			for _, action := range []string{"continue", "get", "delete", "cancel"} {
				t.Run(scenario+"/"+action+map[bool]string{true: "/legacy", false: "/strict"}[legacy], func(t *testing.T) {
					store := enableStateForTest(t)
					state.SetForTest(store, state.WithLegacyPassthrough(legacy))
					owner := state.OwnerScope{UserID: 99, TokenID: 7}
					record := &state.ResponseStateRecord{GatewayResponseID: "resp_shared_account", Owner: owner, Status: state.StatusCompleted, StoreMode: true, Binding: &state.ProviderBinding{ChannelID: 42, APIType: 0, UpstreamResponseID: "resp_provider_handle"}}
					if scenario == "expired" {
						record.ExpiresAt = time.Now().Add(time.Hour).Unix()
					}
					seedResponse(t, store, record)
					if scenario == "expired" {
						store.SetClock(func() time.Time { return time.Now().Add(2 * time.Hour) })
					}
					if scenario == "deleted" {
						require.NoError(t, store.DeleteResponse(context.Background(), owner, record.GatewayResponseID))
					}
					if scenario == "disabled" {
						state.SetForTest(nil)
					}
					if scenario == "unavailable" {
						state.SetForTest(unavailableResponseStore{store})
					}
					id := record.GatewayResponseID
					if scenario == "unknown" {
						id = "resp_untracked"
					}
					url, hits := newCountingUpstream(t)
					c, _ := setupResponseAPIContext(t, http.MethodGet, "/v1/responses/"+id, url)
					c.Params = gin.Params{{Key: "response_id", Value: id}}
					meta := metalib.GetByContext(c)
					switch scenario {
					case "foreign-user":
						meta.UserId++
					case "foreign-token":
						meta.TokenId++
					case "invalid-owner":
						meta.UserId = 0
					}
					metalib.Set2Context(c, meta)
					switch action {
					case "continue":
						req := &openai.ResponseAPIRequest{PreviousResponseId: &id}
						divert, err := resolveNativePreviousResponse(c, meta, req)
						require.False(t, divert)
						if scenario == "owner" {
							require.Nil(t, err)
							require.Equal(t, "resp_provider_handle", *req.PreviousResponseId)
							return
						}
						require.NotNil(t, err, "unproven ownership must never forward a provider handle")
					default:
						handler := RelayResponseAPIGetHelper
						if action == "delete" {
							handler = RelayResponseAPIDeleteHelper
						}
						if action == "cancel" {
							handler = RelayResponseAPICancelHelper
						}
						err := handler(c)
						if scenario == "owner" && action != "cancel" {
							require.Nil(t, err)
						} else {
							require.NotNil(t, err)
							if scenario != "owner" && scenario != "disabled" && scenario != "unavailable" {
								require.Equal(t, http.StatusNotFound, err.StatusCode)
								require.Equal(t, "response_not_found", err.Code)
							}
						}
					}
					require.Zero(t, atomic.LoadInt64(hits), "object authorization must complete before provider dispatch")
				})
			}
		}
	}
}

// TestSecurityResponseStatelessNativeControl leaves an initial request usable without gateway storage.
func TestSecurityResponseStatelessNativeControl(t *testing.T) {
	state.SetForTest(nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	divert, err := resolveNativePreviousResponse(c, testMeta(), &openai.ResponseAPIRequest{Input: openai.ResponseAPIInput{"hello"}})
	require.Nil(t, err)
	require.False(t, divert)
}
