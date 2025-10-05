package aws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
)

func TestConvertConverseResponseToQwenUsageMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	in := int32(11)
	out := int32(22)
	total := int32(33)

	converse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{Value: types.Message{Role: types.ConversationRole("assistant")}},
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(in),
			OutputTokens: aws.Int32(out),
			TotalTokens:  aws.Int32(total),
		},
	}

	resp := convertConverseResponseToQwen(c, converse, "qwen.qwen3-coder-480b-a35b-v1:0")

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal qwen response: %v", err)
	}

	js := string(b)
	if want := "\"prompt_tokens\":11"; !strings.Contains(js, want) {
		t.Fatalf("expected %s in json, got %s", want, js)
	}
	if want := "\"completion_tokens\":22"; !strings.Contains(js, want) {
		t.Fatalf("expected %s in json, got %s", want, js)
	}
	if want := "\"total_tokens\":33"; !strings.Contains(js, want) {
		t.Fatalf("expected %s in json, got %s", want, js)
	}
}
