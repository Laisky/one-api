package streamfinalizer

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

type capturedRender struct {
	payloads [][]byte
	allow    bool
}

func (c *capturedRender) render(b []byte) bool {
	if !c.allow {
		return false
	}
	cp := append([]byte(nil), b...)
	c.payloads = append(c.payloads, cp)
	return true
}

func TestFinalizerEmitsAfterStopAndMetadata(t *testing.T) {
	usage := relaymodel.Usage{}
	cap := &capturedRender{allow: true}
	f := NewFinalizer("test-model", 123, &usage, zap.NewNop(), cap.render)
	f.SetID("chatcmpl-1")

	stop := "stop"
	require.True(t, f.RecordStop(&stop), "record stop returned false")
	require.Empty(t, cap.payloads, "expected no emission before metadata")

	meta := &types.TokenUsage{
		InputTokens:  aws.Int32(10),
		OutputTokens: aws.Int32(20),
		TotalTokens:  aws.Int32(30),
	}
	require.True(t, f.RecordMetadata(meta), "record metadata returned false")
	require.Len(t, cap.payloads, 1, "expected one final chunk")

	var payload struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(cap.payloads[0], &payload), "unmarshal")
	require.Equal(t, 10, payload.Usage.PromptTokens, "unexpected prompt_tokens")
	require.Equal(t, 20, payload.Usage.CompletionTokens, "unexpected completion_tokens")
	require.Equal(t, 30, payload.Usage.TotalTokens, "unexpected total_tokens")
	require.Len(t, payload.Choices, 1, "expected one choice")
	require.NotNil(t, payload.Choices[0].FinishReason, "expected finish_reason")
	require.Equal(t, "stop", *payload.Choices[0].FinishReason, "unexpected finish reason")
}

func TestFinalizerMetadataBeforeStop(t *testing.T) {
	usage := relaymodel.Usage{}
	cap := &capturedRender{allow: true}
	f := NewFinalizer("test-model", 123, &usage, zap.NewNop(), cap.render)
	f.SetID("chatcmpl-2")

	meta := &types.TokenUsage{}
	require.True(t, f.RecordMetadata(meta), "record metadata returned false")
	require.Empty(t, cap.payloads, "expected no chunk before stop")

	reason := "length"
	require.True(t, f.RecordStop(&reason), "record stop returned false")
	require.Len(t, cap.payloads, 1, "expected final chunk after stop")
}

func TestFinalizerFinalizeOnCloseWithoutMetadata(t *testing.T) {
	usage := relaymodel.Usage{}
	cap := &capturedRender{allow: true}
	f := NewFinalizer("test-model", 123, &usage, zap.NewNop(), cap.render)
	f.SetID("chatcmpl-3")

	reason := "stop"
	require.True(t, f.RecordStop(&reason), "record stop returned false")
	require.Empty(t, cap.payloads, "expected no chunk until close")

	require.True(t, f.FinalizeOnClose(), "finalize on close returned false")
	require.Len(t, cap.payloads, 1, "expected one chunk on close")

	var payload struct {
		Usage *struct{} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(cap.payloads[0], &payload), "unmarshal")
	require.Nil(t, payload.Usage, "expected no usage when metadata missing")
}

func TestFinalizerFinalizeWithoutStop(t *testing.T) {
	usage := relaymodel.Usage{}
	cap := &capturedRender{allow: true}
	f := NewFinalizer("test-model", 123, &usage, zap.NewNop(), cap.render)
	f.SetID("chatcmpl-4")

	require.True(t, f.FinalizeOnClose(), "finalize on close returned false")
	require.Len(t, cap.payloads, 1, "expected chunk even without stop")

	var payload struct {
		Choices []struct {
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(cap.payloads[0], &payload), "unmarshal")
	require.Len(t, payload.Choices, 1, "expected one choice")
	require.Nil(t, payload.Choices[0].FinishReason, "expected nil finish reason")
}

func TestFinalizerNotDuplicate(t *testing.T) {
	usage := relaymodel.Usage{}
	cap := &capturedRender{allow: true}
	f := NewFinalizer("test-model", 123, &usage, zap.NewNop(), cap.render)
	f.SetID("chatcmpl-5")

	reason := "stop"
	f.RecordStop(&reason)
	meta := &types.TokenUsage{}
	f.RecordMetadata(meta)
	require.Len(t, cap.payloads, 1, "expected one chunk after first emit")

	require.True(t, f.FinalizeOnClose(), "finalize on close returned false")
	require.Len(t, cap.payloads, 1, "expected no additional chunks")
}
