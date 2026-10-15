package openai

import (
	"fmt"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestStreamParseFailureExplicitlyRedactsProviderPayload drives the real parser
// with malformed private content while preserving client forwarding and context.
func TestStreamParseFailureExplicitlyRedactsProviderPayload(t *testing.T) {
	c, recorder := newTestGinContext()
	core, entries := observer.New(zapcore.ErrorLevel)
	lg, err := glog.NewConsoleWithName("stream-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	gmw.SetLogger(c, lg.With(zap.String("trace_id", "fixture-trace"), zap.Int("channel_id", 17)))
	const privateLine = "data: {invalid-json fake-private-provider-content"
	stream := strings.Join([]string{privateLine, "", chatChunk("", shStrPtr("stop")), "", "data: [DONE]", ""}, "\n")
	relayErr, _, _ := StreamHandler(c, newSSEResponse(stream), relaymode.ChatCompletions)
	require.Nil(t, relayErr)
	require.Contains(t, recorder.Body.String(), privateLine)
	failures := entries.FilterMessage("unmarshalling stream data").All()
	require.Len(t, failures, 1)
	fields := failures[0].ContextMap()
	require.Equal(t, logger.Redact(privateLine), fields["data"])
	require.EqualValues(t, len(privateLine), fields["data_bytes"])
	require.NotEmpty(t, fields["error"])
	require.Equal(t, "fixture-trace", fields["trace_id"])
	require.EqualValues(t, 17, fields["channel_id"])
}

// TestStreamNumericJSONErrorRedaction takes t and verifies normal and oversized parser failures preserve their existing forwarding behavior without logging numeric payload values.
func TestStreamNumericJSONErrorRedaction(t *testing.T) {
	const literal = "918273645546372819e9999"
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprintf("oversized_%t", oversized), func(t *testing.T) {
			c, recorder := newTestGinContext()
			core, entries := observer.New(zapcore.ErrorLevel)
			lg, err := glog.NewConsoleWithName("numeric-stream-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			gmw.SetLogger(c, lg)
			padding := ""
			if oversized {
				padding = strings.Repeat("x", 128*1024)
			}
			line := `data: {"padding":"` + padding + `","choices":[{"delta":{"content":` + literal + `}}]}`
			stream := strings.Join([]string{line, "", chatChunk("", shStrPtr("stop")), "", "data: [DONE]", ""}, "\n")
			relayErr, _, _ := StreamHandler(c, newSSEResponse(stream), relaymode.ChatCompletions)
			require.Nil(t, relayErr)
			message := "unmarshalling stream data"
			if oversized {
				message = "unmarshalling oversized stream data"
				require.NotContains(t, recorder.Body.String(), literal)
			} else {
				require.Contains(t, recorder.Body.String(), line)
			}
			failures := entries.FilterMessage(message).All()
			require.Len(t, failures, 1)
			require.Contains(t, fmt.Sprint(failures[0].ContextMap()["error"]), "number [REDACTED]")
			for _, entry := range entries.All() {
				buffer, err := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()).EncodeEntry(entry.Entry, entry.Context)
				require.NoError(t, err)
				rendered := buffer.String()
				buffer.Free()
				require.NotContains(t, rendered, literal)
			}
			require.Contains(t, recorder.Body.String(), "[DONE]", "existing stream completion forwarding remains intact")
		})
	}
}
