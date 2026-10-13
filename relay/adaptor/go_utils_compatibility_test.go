package adaptor

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// replayContractAdaptor captures request replay behavior before dispatch.
type replayContractAdaptor struct {
	stubAdaptor
	inspect func(*http.Request)
}

// SetupRequestHeader inspects the request produced by the real relay helper.
func (a *replayContractAdaptor) SetupRequestHeader(_ *gin.Context, r *http.Request, _ *meta.Meta) error {
	a.inspect(r)
	return nil
}

// TestDoRequestHelperUtilityBodyCompatibility checks exact payload dispatch and replay.
// Its empty, nil and opaque-reader controls preserve the existing request contracts.
func TestDoRequestHelperUtilityBodyCompatibility(t *testing.T) {
	withTraceDB(t)
	for _, tc := range []struct {
		name            string
		payload         []byte
		opaque, nilBody bool
	}{
		{name: "binary", payload: []byte{0, 1, 255, 'x'}},
		{name: "empty", payload: []byte{}},
		{name: "nil", nilBody: true},
		{name: "opaque", payload: []byte("opaque-fixture"), opaque: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read synthetic request: %v", err)
				}
				received <- raw
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
			c.Set(ctxkey.ContentType, "application/octet-stream")
			var body io.Reader
			if !tc.nilBody {
				body = bytes.NewReader(tc.payload)
				if tc.opaque {
					body = struct{ io.Reader }{body}
				}
			}
			inspected := false
			a := &replayContractAdaptor{stubAdaptor: stubAdaptor{defaultURL: server.URL + "/v1/rerank"}, inspect: func(r *http.Request) {
				inspected = true
				if tc.opaque || tc.nilBody {
					require.Nil(t, r.GetBody)
					return
				}
				require.NotNil(t, r.GetBody)
				for range 2 {
					replay, err := r.GetBody()
					require.NoError(t, err)
					raw, err := io.ReadAll(replay)
					require.NoError(t, err)
					require.NoError(t, replay.Close())
					require.Equal(t, tc.payload, raw)
				}
			}}
			response, err := DoRequestHelper(a, c, &meta.Meta{Mode: relaymode.Rerank, Config: model.ChannelConfig{}}, body)
			require.NoError(t, err)
			require.True(t, inspected)
			require.Equal(t, http.StatusNoContent, response.StatusCode)
			require.NoError(t, response.Body.Close())
			require.Equal(t, string(tc.payload), string(<-received))
		})
	}
}
