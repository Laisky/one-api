// Package aws provides the AWS adaptor for the relay service.
package aws

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// AwsModelIDMap provides mapping for Mistral Large models via InvokeModel API
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
var AwsModelIDMap = map[string]string{
	"mistral-small-2402":         "mistral.mistral-small-2402-v1:0",
	"mistral-large-2402":         "mistral.mistral-large-2402-v1:0",
	"mistral-pixtral-large-2502": "mistral.pixtral-large-2502-v1:0",
}

func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}
	return "", errors.Errorf("model %s not found", requestModel)
}

// ConvertMessages converts OpenAI messages to Mistral messages
func ConvertMessages(messages []relaymodel.Message) []Message {
	var mistralMessages []Message

	for _, msg := range messages {
		mistralMsg := Message{
			Role: msg.Role,
		}

		// Handle different message types
		switch msg.Role {
		case "tool":
			// Tool result message
			if msg.ToolCallId != "" {
				mistralMsg.ToolCallID = msg.ToolCallId
			}
			mistralMsg.Content = msg.StringContent()
		case "assistant":
			// Assistant message with potential tool calls
			mistralMsg.Content = msg.StringContent()
			if len(msg.ToolCalls) > 0 {
				mistralMsg.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					// Convert arguments from any to string
					var argsStr string
					if tc.Function != nil && tc.Function.Arguments != nil {
						if str, ok := tc.Function.Arguments.(string); ok {
							argsStr = str
						}
					}
					mistralMsg.ToolCalls[i] = ToolCall{
						ID: tc.Id,
						Function: Function{
							Name:      tc.Function.Name,
							Arguments: argsStr,
						},
					}
				}
			}
		default:
			// System, user, or other message types
			mistralMsg.Content = msg.StringContent()
		}

		mistralMessages = append(mistralMessages, mistralMsg)
	}

	return mistralMessages
}

// ConvertTools converts OpenAI tools to Mistral tools
func ConvertTools(tools []relaymodel.Tool) []Tool {
	if len(tools) == 0 {
		return nil
	}

	mistralTools := make([]Tool, len(tools))
	for i, tool := range tools {
		mistralTools[i] = Tool{
			Type: tool.Type,
			Function: ToolSpec{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}

	return mistralTools
}

// ConvertRequest converts OpenAI request to Mistral request
func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	mistralReq := &Request{
		Messages: ConvertMessages(textRequest.Messages),
	}

	// Handle tools
	if len(textRequest.Tools) > 0 {
		mistralReq.Tools = ConvertTools(textRequest.Tools)
	}

	// Handle tool choice
	if textRequest.ToolChoice != nil {
		mistralReq.ToolChoice = textRequest.ToolChoice
	}

	// Handle inference parameters
	if textRequest.MaxTokens != 0 {
		mistralReq.MaxTokens = textRequest.MaxTokens
	}

	if textRequest.Temperature != nil {
		mistralReq.Temperature = textRequest.Temperature
	}

	if textRequest.TopP != nil {
		mistralReq.TopP = textRequest.TopP
	}

	return mistralReq
}

// Handler handles non-streaming requests using Converse API
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(awsModelName, awsCli.Options().Region)

	mistralReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Mistral request to Converse API format
	converseReq, err := convertMistralToConverseRequest(mistralReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API instead of InvokeModel to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to OpenAI format
	openaiResp := convertConverseResponseToOpenAI(awsResp, modelName)

	// Extract actual usage from Converse API response
	var usage relaymodel.Usage
	if awsResp.Usage != nil {
		if awsResp.Usage.InputTokens != nil {
			usage.PromptTokens = int(*awsResp.Usage.InputTokens)
		}
		if awsResp.Usage.OutputTokens != nil {
			usage.CompletionTokens = int(*awsResp.Usage.OutputTokens)
		}
		if awsResp.Usage.TotalTokens != nil {
			usage.TotalTokens = int(*awsResp.Usage.TotalTokens)
		} else {
			// Calculate total if not provided
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
	}

	openaiResp.Usage = usage

	c.JSON(http.StatusOK, openaiResp)
	return nil, &usage
}

// ResponseMistral2OpenAI converts Mistral response to OpenAI format
func ResponseMistral2OpenAI(mistralResponse *Response) *openai.TextResponse {
	var responseText string
	var finishReason string

	if len(mistralResponse.Choices) > 0 {
		responseText = mistralResponse.Choices[0].Message.Content
		if stopReason := convertStopReason(mistralResponse.Choices[0].StopReason); stopReason != nil {
			finishReason = *stopReason
		}
	}

	choice := openai.TextResponseChoice{
		Index: 0,
		Message: relaymodel.Message{
			Role:    "assistant",
			Content: responseText,
		},
		FinishReason: finishReason,
	}

	return &openai.TextResponse{
		Id:      fmt.Sprintf("chatcmpl-oneapi-%s", random.GetUUID()),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   "mistral-large-2407",
		Choices: []openai.TextResponseChoice{choice},
	}
}

// StreamHandler handles streaming requests using Converse API
func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	lg := gmw.GetLogger(c)
	createdTime := helper.GetTimestamp()
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(awsModelName, awsCli.Options().Region)

	mistralReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Mistral request to Converse API format
	converseReq, err := convertMistralToConverseStreamRequest(mistralReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	awsResp, err := awsCli.ConverseStream(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "ConverseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	var usage relaymodel.Usage
	var id string
	var totalContent strings.Builder // Track accumulated content for fallback usage calculation

	c.Stream(func(w io.Writer) bool {
		event, ok := <-stream.Events()
		if !ok {
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}

		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// Handle message start
			id = fmt.Sprintf("chatcmpl-oneapi-%s", random.GetUUID())
			return true

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			// Handle content delta - this is where the actual text content comes from ConverseStream
			if v.Value.Delta != nil {
				// Check if this is a text delta (AWS SDK union type pattern)
				switch deltaValue := v.Value.Delta.(type) {
				case *types.ContentBlockDeltaMemberText:
					if textDelta := deltaValue.Value; textDelta != "" {
						// Accumulate content for fallback usage calculation
						totalContent.WriteString(textDelta)

						// Create streaming response with the text delta
						response := &openai.ChatCompletionsStreamResponse{
							Id:      id,
							Object:  "chat.completion.chunk",
							Created: createdTime,
							Model:   c.GetString(ctxkey.OriginalModel),
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

						jsonStr, err := json.Marshal(response)
						if err != nil {
							lg.Error("error marshalling stream response", zap.Error(err))
							return true
						}
						c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
					}
				}
			}
			return true

		case *types.ConverseStreamOutputMemberMessageStop:
			// Handle message stop
			finishReason := convertStopReason(string(v.Value.StopReason))
			response := &openai.ChatCompletionsStreamResponse{
				Id:      id,
				Object:  "chat.completion.chunk",
				Created: createdTime,
				Model:   c.GetString(ctxkey.OriginalModel),
				Choices: []openai.ChatCompletionsStreamResponseChoice{
					{
						Index:        0,
						Delta:        relaymodel.Message{},
						FinishReason: finishReason,
					},
				},
			}

			jsonStr, err := json.Marshal(response)
			if err != nil {
				lg.Error("error marshalling final stream response", zap.Error(err))
				return false
			}
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
			return true

		case *types.ConverseStreamOutputMemberMetadata:
			// Handle metadata (usage)
			if streamUsage := v.Value.Usage; streamUsage != nil {
				if streamUsage.InputTokens != nil {
					usage.PromptTokens = int(*streamUsage.InputTokens)
				}
				if streamUsage.OutputTokens != nil {
					usage.CompletionTokens = int(*streamUsage.OutputTokens)
				}
				if streamUsage.TotalTokens != nil {
					usage.TotalTokens = int(*streamUsage.TotalTokens)
				}
			}
			return true

		default:
			// Handle other event types
			return true
		}
	})

	return nil, &usage
}

// StreamResponseMistral2OpenAI converts Mistral streaming responses to the OpenAI format.
//
// Note: This function is currently unused. It was previously used in the invoke method, but is now replaced by converse,
// as the invoke method does not provide token usage information.
func StreamResponseMistral2OpenAI(mistralResponse *StreamResponse, modelName string) *openai.ChatCompletionsStreamResponse {
	if len(mistralResponse.Choices) == 0 {
		return nil
	}

	choice := mistralResponse.Choices[0]

	return &openai.ChatCompletionsStreamResponse{
		Id:      "chatcmpl-oneapi-" + random.GetRandomString(29),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []openai.ChatCompletionsStreamResponseChoice{
			{
				Index: choice.Index,
				Delta: relaymodel.Message{
					Role:      choice.Delta.Role,
					Content:   choice.Delta.Content,
					ToolCalls: convertToolCalls(choice.Delta.ToolCalls),
				},
				FinishReason: convertStopReason(choice.StopReason),
			},
		},
	}
}

// convertToolCalls converts Mistral tool calls to OpenAI format
func convertToolCalls(mistralToolCalls []ToolCall) []relaymodel.Tool {
	if len(mistralToolCalls) == 0 {
		return nil
	}

	toolCalls := make([]relaymodel.Tool, len(mistralToolCalls))
	for i, tc := range mistralToolCalls {
		toolCalls[i] = relaymodel.Tool{
			Id:   tc.ID,
			Type: "function",
			Function: &relaymodel.Function{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}

	return toolCalls
}

// convertStopReason converts Mistral stop reason to OpenAI format
func convertStopReason(mistralReason string) *string {
	if mistralReason == "" {
		return nil
	}

	var result string
	switch mistralReason {
	case "stop":
		result = "stop"
	case "length":
		result = "length"
	case "tool_calls":
		result = "tool_calls"
	default:
		result = "stop"
	}
	return &result
}

// convertMistralToConverseRequest converts Mistral request to Converse API format for non-streaming
func convertMistralToConverseRequest(mistralReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages
	for _, msg := range mistralReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user", "assistant":
			// Convert to Converse API message format
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.Content,
				},
			}

			converseMessages = append(converseMessages, types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			})
		}
	}

	// Create inference configuration
	inferenceConfig := &types.InferenceConfiguration{}
	if mistralReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(mistralReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if mistralReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*mistralReq.Temperature))
	}
	if mistralReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*mistralReq.TopP))
	}

	converseReq := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	// Add system messages if any
	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	return converseReq, nil
}

// convertConverseResponseToOpenAI converts AWS Converse response to OpenAI format
func convertConverseResponseToOpenAI(converseResp *bedrockruntime.ConverseOutput, modelName string) *openai.TextResponse {
	var responseText string
	var finishReason string

	// Extract response content from Converse API response
	if converseResp.Output != nil {
		switch outputValue := converseResp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			if len(outputValue.Value.Content) > 0 {
				// Get the first content block (assuming text)
				switch contentValue := outputValue.Value.Content[0].(type) {
				case *types.ContentBlockMemberText:
					responseText = contentValue.Value
				}
			}
			// Convert stop reason
			if stopReason := convertStopReason(string(converseResp.StopReason)); stopReason != nil {
				finishReason = *stopReason
			}
		}
	}

	choice := openai.TextResponseChoice{
		Index: 0,
		Message: relaymodel.Message{
			Role:    "assistant",
			Content: responseText,
		},
		FinishReason: finishReason,
	}

	return &openai.TextResponse{
		Id:      fmt.Sprintf("chatcmpl-oneapi-%s", random.GetUUID()),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []openai.TextResponseChoice{choice},
	}
}

// convertMistralToConverseStreamRequest converts Mistral request to Converse API format for streaming
func convertMistralToConverseStreamRequest(mistralReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages
	for _, msg := range mistralReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user", "assistant":
			// Convert to Converse API message format
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.Content,
				},
			}

			converseMessages = append(converseMessages, types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			})
		}
	}

	// Create inference configuration
	inferenceConfig := &types.InferenceConfiguration{}
	if mistralReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(mistralReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if mistralReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*mistralReq.Temperature))
	}
	if mistralReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*mistralReq.TopP))
	}

	converseReq := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	// Add system messages if any
	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	return converseReq, nil
}
