package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/aws/internal/streamfinalizer"
	"github.com/Laisky/one-api/relay/adaptor/aws/internal/streamusage"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
	"io"
)

// StreamHandler processes streaming chat completion requests for Qwen models.
//
// This function handles real-time streaming responses from Qwen3 Coder models
// through AWS Bedrock's Converse API. It provides progressive content delivery
// with support for:
//
//   - Text streaming with incremental token delivery
//   - Tool calling with streaming argument generation
//   - Multiple concurrent tool calls tracked by block index
//   - Usage statistics collected at stream completion
//
// The function processes AWS Bedrock stream events and converts them to
// OpenAI-compatible Server-Sent Events (SSE) format for client consumption.
// Tool calls are fully supported in streaming mode, with tool names announced
// first followed by incremental argument delivery.
//
// Parameters:
//   - c: Gin context for streaming response writing
//   - awsCli: AWS Bedrock Runtime client for streaming API calls
//
// Returns:
//   - *relaymodel.ErrorWithStatusCode: Error with HTTP status code if streaming fails
//   - *relaymodel.Usage: Token usage statistics collected at stream completion
func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	lg := gmw.GetLogger(c)
	createdTime := helper.GetTimestamp()
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	qwenReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	converseReq, err := convertQwenToConverseStreamRequest(qwenReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	utils.MarkInvocation(c)
	awsResp, err := awsCli.ConverseStream(gmw.Ctx(c), converseReq, utils.NoInferenceRetry)
	if err != nil {
		return utils.InvocationError(c, errors.Wrap(err, "ConverseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	common.SetEventStreamHeaders(c)

	var usage relaymodel.Usage
	observer := streamusage.New(c, &usage)
	var id string
	toolCallsMap := make(map[int32]*QwenToolCallResponse)
	finalizer := streamfinalizer.NewFinalizer(
		c.GetString(ctxkey.RequestModel),
		createdTime,
		&usage,
		lg,
		func(payload []byte) bool {
			// The finalizer marshals an openai.ChatCompletionsStreamResponse to
			// bytes; decode it back into the chunk object so it can be routed
			// through the Response API rewrite bridge (the /v1/responses chat
			// fallback) when one is installed, instead of emitting raw
			// chat-completion SSE.
			var chunk openai.ChatCompletionsStreamResponse
			if err := json.Unmarshal(payload, &chunk); err != nil {
				lg.Error("error unmarshalling final stream response", zap.Error(err))
				return false
			}
			if err := observer.Render(c, &chunk); err != nil {
				lg.Error("error rendering final stream response", zap.Error(err))
				return false
			}
			return true
		},
	)

	// Gin checks CloseNotify only between callbacks. A callback waiting for
	// an SDK event must also select cancellation so idle providers are closed.
	streamContext := gmw.Ctx(c)
	clientClosed := c.Writer.CloseNotify()
	disconnected := c.Stream(func(w io.Writer) bool {
		var event types.ConverseStreamOutput
		var ok bool
		select {
		case <-streamContext.Done():
			observer.Fail(streamContext.Err())
			return false
		case <-clientClosed:
			observer.Fail(context.Canceled)
			return false
		case event, ok = <-stream.Events():
		}
		if !ok {
			observer.Fail(stream.Err())
			if !observer.Complete() {
				return false
			}
			if !finalizer.FinalizeOnClose() {
				return false
			}
			openai_compatible.FinalizeStreamWithBridge(c, &usage)
			return false
		}

		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			id = fmt.Sprintf("chatcmpl-oneapi-%s", tracing.GetTraceIDFromContext(c))
			finalizer.SetID(id)
			return true

		case *types.ConverseStreamOutputMemberContentBlockStart:
			if v.Value.Start != nil {
				switch startValue := v.Value.Start.(type) {
				case *types.ContentBlockStartMemberToolUse:
					if v.Value.ContentBlockIndex == nil {
						return true
					}
					blockIndex := *v.Value.ContentBlockIndex
					toolUseStart := startValue.Value
					if toolUseStart.ToolUseId != nil && toolUseStart.Name != nil {
						toolCallsMap[blockIndex] = &QwenToolCallResponse{
							ID:   *toolUseStart.ToolUseId,
							Type: "function",
							Function: QwenToolFunction{
								Name:      *toolUseStart.Name,
								Arguments: "",
							},
						}

						response := &openai.ChatCompletionsStreamResponse{
							Id:      id,
							Object:  "chat.completion.chunk",
							Created: createdTime,
							Model:   c.GetString(ctxkey.RequestModel),
							Choices: []openai.ChatCompletionsStreamResponseChoice{
								{
									Index: 0,
									Delta: relaymodel.Message{
										Role: "assistant",
										ToolCalls: []relaymodel.Tool{
											{
												Id:   toolCallsMap[blockIndex].ID,
												Type: toolCallsMap[blockIndex].Type,
												Function: &relaymodel.Function{
													Name: toolCallsMap[blockIndex].Function.Name,
												},
												Index: aws.Int(int(blockIndex)),
											},
										},
									},
								},
							},
						}

						if err := observer.Render(c, response); err != nil {
							lg.Error("error rendering stream response", zap.Error(err))
							return false
						}
					}
				}
			}
			return true

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			if v.Value.Delta != nil {
				var response *openai.ChatCompletionsStreamResponse

				switch deltaValue := v.Value.Delta.(type) {
				case *types.ContentBlockDeltaMemberText:
					if textDelta := deltaValue.Value; textDelta != "" {
						response = &openai.ChatCompletionsStreamResponse{
							Id:      id,
							Object:  "chat.completion.chunk",
							Created: createdTime,
							Model:   c.GetString(ctxkey.RequestModel),
							Choices: []openai.ChatCompletionsStreamResponseChoice{
								{
									Index: 0,
									Delta: relaymodel.Message{
										Role:    "assistant",
										Content: textDelta,
									},
								},
							},
						}
					}
				case *types.ContentBlockDeltaMemberReasoningContent:
					if deltaValue.Value != nil {
						switch reasoningDelta := deltaValue.Value.(type) {
						case *types.ReasoningContentBlockDeltaMemberText:
							if reasoningText := reasoningDelta.Value; reasoningText != "" {
								response = &openai.ChatCompletionsStreamResponse{
									Id:      id,
									Object:  "chat.completion.chunk",
									Created: createdTime,
									Model:   c.GetString(ctxkey.RequestModel),
									Choices: []openai.ChatCompletionsStreamResponseChoice{
										{
											Index: 0,
											Delta: relaymodel.Message{
												Role:             "assistant",
												ReasoningContent: &reasoningText,
											},
										},
									},
								}
							}
						}
					}
				case *types.ContentBlockDeltaMemberToolUse:
					if v.Value.ContentBlockIndex == nil || deltaValue.Value.Input == nil {
						break
					}
					blockIndex := *v.Value.ContentBlockIndex
					if tool, exists := toolCallsMap[blockIndex]; exists {
						tool.Function.Arguments += *deltaValue.Value.Input

						response = &openai.ChatCompletionsStreamResponse{
							Id:      id,
							Object:  "chat.completion.chunk",
							Created: createdTime,
							Model:   c.GetString(ctxkey.RequestModel),
							Choices: []openai.ChatCompletionsStreamResponseChoice{
								{
									Index: 0,
									Delta: relaymodel.Message{
										Role: "assistant",
										ToolCalls: []relaymodel.Tool{
											{
												Index: aws.Int(int(blockIndex)),
												Function: &relaymodel.Function{
													Arguments: *deltaValue.Value.Input,
												},
											},
										},
									},
								},
							},
						}
					}
				}

				if response != nil {
					if err := observer.Render(c, response); err != nil {
						lg.Error("error rendering stream response", zap.Error(err))
						return false
					}
				}
			}
			return true

		case *types.ConverseStreamOutputMemberContentBlockStop:
			return true

		case *types.ConverseStreamOutputMemberMessageStop:
			observer.RecordStop()
			return finalizer.RecordStop(convertStopReason(string(v.Value.StopReason)))

		case *types.ConverseStreamOutputMemberMetadata:
			if !observer.RecordMetadata(v.Value.Usage) {
				return false
			}
			return finalizer.RecordMetadata(v.Value.Usage)

		default:
			return true
		}
	})

	return observer.Finish(disconnected, stream.Err())
}
