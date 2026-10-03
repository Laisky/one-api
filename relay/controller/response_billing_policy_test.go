package controller

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestNativeResponseRootManifestCoversTypedProtocol requires protocol additions
// to receive an explicit billing/compatibility review before being forwarded.
func TestNativeResponseRootManifestCoversTypedProtocol(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(openai.ResponseAPIRequest{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		require.True(t, nativeResponseRootAllowed(key), "review admission policy for typed field %s", key)
	}
}

// TestNativeResponseWireRequiresTypedPolicy prevents raw-only callers from
// accidentally obtaining an unvalidated upstream request body.
func TestNativeResponseWireRequiresTypedPolicy(t *testing.T) {
	t.Parallel()
	wire, _, _, err := normalizeResponseAPIRawBody([]byte(`{"tools":[{"type":"code_interpreter"}]}`), nil, channeltype.OpenAI)
	require.Error(t, err)
	require.Empty(t, wire)
}
