package aws

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/internal/streamfinalizer"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// AwsModelIDMap maps friendly Qwen model names to AWS Bedrock model IDs.
//
// This mapping translates user-friendly model names (like "qwen3-coder-480b") into
// the full AWS Bedrock model identifiers required by the AWS SDK. The map enables
// consistent model naming across the One API system while maintaining compatibility
// with AWS Bedrock's naming conventions.
//
// Supported models:
//   - qwen3-235b: Qwen3 235B parameter model for general-purpose tasks
//   - qwen3-32b: Qwen3 32B parameter model for efficient inference
//   - qwen3-coder-30b: Qwen3 Coder 30B parameter model for code generation
//   - qwen3-coder-480b: Qwen3 Coder 480B parameter model optimized for advanced code generation
var AwsModelIDMap = map[string]string{
	"qwen3-235b":       "qwen.qwen3-235b-a22b-2507-v1:0",
	"qwen3-32b":        "qwen.qwen3-32b-v1:0",
	"qwen3-coder-30b":  "qwen.qwen3-coder-30b-a3b-v1:0",
	"qwen3-coder-480b": "qwen.qwen3-coder-480b-a35b-v1:0",
}

// awsModelID retrieves the AWS Bedrock model ID for a given model name.
//
// This function looks up the full AWS Bedrock model identifier from the friendly
// model name provided in the request. It ensures that only supported Qwen models
// can be used with this adapter.
//
// Parameters:
//   - requestModel: The friendly model name from the API request
//
// Returns:
//   - string: The full AWS Bedrock model ID
//   - error: Error if the model name is not found in the mapping
func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}
	return "", errors.Errorf("model %s not found", requestModel)
}

// ConvertRequest converts an OpenAI-compatible request to Qwen request format.
//
// This function transforms the unified OpenAI request format into the specific
// structure required by Qwen3 Coder models via AWS Bedrock. It handles:
//
//   - Message conversion with tool calling support
//   - Parameter mapping (temperature, top_p, max_tokens)
//   - Stop sequence conversion from various input formats
//   - Tool definition conversion for function calling
//   - Tool choice configuration for controlling tool invocation
//
// The conversion preserves all code-focused features and ensures compatibility
// with Qwen's programming capabilities and technical accuracy.
//
// Parameters:
//   - textRequest: OpenAI-compatible chat completion request
//
// Returns:
//   - *Request: Qwen-formatted request ready for AWS Bedrock submission
func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	qwenReq := &Request{
		Messages: ConvertMessages(textRequest.Messages),
	}

	if textRequest.MaxTokens == 0 {
		qwenReq.MaxTokens = config.DefaultMaxToken
	} else {
		qwenReq.MaxTokens = textRequest.MaxTokens
	}

	if textRequest.Temperature != nil {
		qwenReq.Temperature = textRequest.Temperature
	}

	if textRequest.TopP != nil {
		qwenReq.TopP = textRequest.TopP
	}

	if textRequest.Stop != nil {
		if stopSlice, ok := textRequest.Stop.([]any); ok {
			stopSequences := make([]string, len(stopSlice))
			for i, stop := range stopSlice {
				if stopStr, ok := stop.(string); ok {
					stopSequences[i] = stopStr
				}
			}
			qwenReq.Stop = stopSequences
		} else if stopStr, ok := textRequest.Stop.(string); ok {
			qwenReq.Stop = []string{stopStr}
		} else if stopSlice, ok := textRequest.Stop.([]string); ok {
			qwenReq.Stop = stopSlice
		}
	}

	if textRequest.ReasoningEffort != nil {
		qwenReq.ReasoningEffort = textRequest.ReasoningEffort
	}

	if len(textRequest.Tools) > 0 {
		qwenTools := make([]QwenTool, 0, len(textRequest.Tools))
		for _, tool := range textRequest.Tools {
			if tool.Function == nil {
				continue
			}

			qwenTools = append(qwenTools, QwenTool{
				Type: "function",
				Function: QwenToolSpec{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			})
		}
		qwenReq.Tools = qwenTools

		if textRequest.ToolChoice != nil {
			qwenReq.ToolChoice = textRequest.ToolChoice
		}
	}

	return qwenReq
}

// Handler processes non-streaming chat completion requests for Qwen models.
//
// This function handles the complete lifecycle of a non-streaming request to
// Qwen3 Coder models through AWS Bedrock's Converse API. It:
//
//   - Retrieves and validates the AWS model ID
//   - Converts the request to AWS Converse API format
//   - Calls the AWS Bedrock Converse API
//   - Converts the response back to OpenAI-compatible format
//   - Extracts usage statistics for billing
//
// The function uses AWS Bedrock's Converse API which provides accurate token
// counting and consistent behavior across different model providers.
//
// Parameters:
//   - c: Gin context containing the request data
//   - awsCli: AWS Bedrock Runtime client for API calls
//   - modelName: The friendly model name for response metadata
//
// Returns:
//   - *relaymodel.ErrorWithStatusCode: Error with HTTP status code if the request fails
//   - *relaymodel.Usage: Token usage statistics for billing and monitoring
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	qwenReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	converseReq, err := convertQwenToConverseRequest(qwenReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	qwenResp := convertConverseResponseToQwen(c, awsResp, modelName)

	usage := qwenResp.Usage

	c.JSON(http.StatusOK, qwenResp)
	return nil, &usage
}

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

	awsResp, err := awsCli.ConverseStream(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "ConverseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	common.SetEventStreamHeaders(c)

	var usage relaymodel.Usage
	var id string
	toolCallsMap := make(map[int32]*QwenToolCallResponse)
	finalizer := streamfinalizer.NewFinalizer(
		c.GetString(ctxkey.RequestModel),
		createdTime,
		&usage,
		lg,
		func(payload []byte) bool {
			c.Render(-1, common.CustomEvent{Data: "data: " + string(payload)})
			return true
		},
	)

	c.Stream(func(w io.Writer) bool {
		event, ok := <-stream.Events()
		if !ok {
			if !finalizer.FinalizeOnClose() {
				return false
			}
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
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
					jsonStr, err := json.Marshal(response)
					if err != nil {
						lg.Error("error marshalling stream response", zap.Error(err))
						return true
					}
					c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
				}
			}
			return true

		case *types.ConverseStreamOutputMemberContentBlockStop:
			return true

		case *types.ConverseStreamOutputMemberMessageStop:
			return finalizer.RecordStop(convertStopReason(string(v.Value.StopReason)))

		case *types.ConverseStreamOutputMemberMetadata:
			return finalizer.RecordMetadata(v.Value.Usage)

		default:
			return true
		}
	})

	return nil, &usage
}

// convertStopReason converts AWS Bedrock stop reasons to OpenAI-compatible format.
//
// This function maps AWS Bedrock's stop reason strings to the corresponding
// OpenAI API finish_reason values, ensuring API compatibility. It handles:
//
//   - max_tokens -> "length"
//   - end_turn/stop_sequence -> "stop"
//   - content_filtered -> "content_filter"
//   - Unknown reasons are passed through as-is
//
// Parameters:
//   - awsReason: The stop reason string from AWS Bedrock
//
// Returns:
//   - *string: Pointer to the converted stop reason, nil if input is empty
func convertStopReason(awsReason string) *string {
	if awsReason == "" {
		return nil
	}

	var result string
	switch awsReason {
	case "max_tokens":
		result = "length"
	case "end_turn", "stop_sequence":
		result = "stop"
	case "content_filtered":
		result = "content_filter"
	default:
		result = awsReason
	}

	return &result
}

// convertMessages converts Qwen messages to AWS Bedrock Converse API format.
//
// This function transforms Qwen message structures into the format required by
// AWS Bedrock's Converse API. It handles:
//
//   - System messages: Converted to separate system content blocks
//   - User messages: Support both regular text and tool result messages
//   - Assistant messages: Support text content and tool call invocations
//   - Tool results: Encoded with tool_use_id and success status
//   - Tool calls: Converted to ToolUseBlock with JSON document input
//
// The function properly separates system messages from conversation messages
// as required by the Converse API, and ensures tool calling workflows are
// correctly formatted for AWS Bedrock processing.
//
// Parameters:
//   - messages: Slice of Qwen message structures
//
// Returns:
//   - []types.Message: Conversation messages in AWS Converse API format
//   - []types.SystemContentBlock: System messages as separate content blocks
//   - error: Error if message conversion fails (e.g., invalid tool call JSON)
func convertMessages(messages []Message) ([]types.Message, []types.SystemContentBlock, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user":
			var contentBlocks []types.ContentBlock

			if msg.ToolCallID != "" {
				toolResult := &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: &msg.ToolCallID,
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{
								Value: msg.Content,
							},
						},
						Status: types.ToolResultStatusSuccess,
					},
				}
				contentBlocks = append(contentBlocks, toolResult)
			} else if msg.Content != "" {
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
					Value: msg.Content,
				})
			}

			if len(contentBlocks) > 0 {
				converseMessages = append(converseMessages, types.Message{
					Role:    types.ConversationRole("user"),
					Content: contentBlocks,
				})
			}
		case "assistant":
			var contentBlocks []types.ContentBlock

			if msg.Content != "" {
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
					Value: msg.Content,
				})
			}

			for _, toolCall := range msg.ToolCalls {
				var inputData map[string]any
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputData); err != nil {
					return nil, nil, errors.Wrapf(err, "unmarshal tool call arguments for tool %s", toolCall.Function.Name)
				}

				docInput := document.NewLazyDocument(inputData)

				toolUse := &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: &toolCall.ID,
						Name:      &toolCall.Function.Name,
						Input:     docInput,
					},
				}
				contentBlocks = append(contentBlocks, toolUse)
			}

			if len(contentBlocks) > 0 {
				converseMessages = append(converseMessages, types.Message{
					Role:    types.ConversationRole("assistant"),
					Content: contentBlocks,
				})
			}
		}
	}

	return converseMessages, systemMessages, nil
}

// createInferenceConfig creates AWS Bedrock inference configuration from Qwen request.
//
// This function maps Qwen request parameters to AWS Bedrock's InferenceConfiguration
// format, which controls how the model generates responses. It handles:
//
//   - MaxTokens: Generation length limit (uses default if not specified)
//   - Temperature: Response randomness (0.0 = deterministic, 1.0 = creative)
//   - TopP: Nucleus sampling for token selection
//   - StopSequences: Custom strings that terminate generation
//
// All parameters are optional except MaxTokens, which defaults to the system
// default if not provided in the request.
//
// Parameters:
//   - qwenReq: Qwen request containing inference parameters
//
// Returns:
//   - *types.InferenceConfiguration: AWS Bedrock inference configuration
func createInferenceConfig(qwenReq *Request) *types.InferenceConfiguration {
	inferenceConfig := &types.InferenceConfiguration{}

	if qwenReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(qwenReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if qwenReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*qwenReq.Temperature))
	}
	if qwenReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*qwenReq.TopP))
	}
	if len(qwenReq.Stop) > 0 {
		stopSequences := make([]string, len(qwenReq.Stop))
		copy(stopSequences, qwenReq.Stop)
		inferenceConfig.StopSequences = stopSequences
	}

	return inferenceConfig
}

// createToolConfig creates AWS Bedrock tool configuration for function calling.
//
// This function converts Qwen tool definitions into AWS Bedrock's ToolConfiguration
// format, enabling the model to intelligently invoke external functions. It handles:
//
//   - Tool specifications: Name, description, and JSON schema parameters
//   - Tool choice modes:
//   - "auto": Model decides whether to use tools
//   - "any": Model must use at least one tool
//   - Specific tool: Force invocation of a named function
//   - Default to "auto" if not specified
//
// Returns nil if no tools are provided, optimizing for normal conversation mode
// without tool calling overhead.
//
// Parameters:
//   - qwenTools: Slice of tool definitions with functions and parameters
//   - toolChoice: Tool invocation strategy (auto/any/specific)
//
// Returns:
//   - *types.ToolConfiguration: AWS Bedrock tool configuration, nil if no tools
func createToolConfig(qwenTools []QwenTool, toolChoice any) *types.ToolConfiguration {
	if len(qwenTools) == 0 {
		return nil
	}

	var awsTools []types.Tool
	for _, tool := range qwenTools {
		var inputSchemaDoc document.Interface
		if tool.Function.Parameters != nil {
			inputSchemaDoc = document.NewLazyDocument(tool.Function.Parameters)
		}

		toolSpec := &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(tool.Function.Name),
				Description: aws.String(tool.Function.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: inputSchemaDoc,
				},
			},
		}
		awsTools = append(awsTools, toolSpec)
	}

	toolConfig := &types.ToolConfiguration{
		Tools: awsTools,
	}

	if toolChoice != nil {
		if toolChoiceMap, ok := toolChoice.(map[string]any); ok {
			if funcMap, ok := toolChoiceMap["function"].(map[string]any); ok {
				if funcName, ok := funcMap["name"].(string); ok {
					toolConfig.ToolChoice = &types.ToolChoiceMemberTool{
						Value: types.SpecificToolChoice{
							Name: aws.String(funcName),
						},
					}
				}
			}
		} else if toolChoiceStr, ok := toolChoice.(string); ok {
			switch toolChoiceStr {
			case "auto":
				toolConfig.ToolChoice = &types.ToolChoiceMemberAuto{}
			case "any":
				toolConfig.ToolChoice = &types.ToolChoiceMemberAny{}
			}
		}
	} else {
		toolConfig.ToolChoice = &types.ToolChoiceMemberAuto{}
	}

	return toolConfig
}

// convertQwenToConverseRequest converts Qwen request to AWS Converse API format for non-streaming.
//
// This function performs the complete conversion from Qwen request format to
// AWS Bedrock's Converse API input format. It orchestrates the conversion of:
//
//   - Messages (including tool calls and results)
//   - System messages (separated from conversation)
//   - Inference configuration (temperature, top_p, etc.)
//   - Tool configuration (function definitions and tool choice)
//
// The function ensures that all Qwen features are properly mapped to their
// AWS Bedrock equivalents, maintaining full compatibility with tool calling
// and code generation capabilities.
//
// Parameters:
//   - qwenReq: Qwen request structure with messages and parameters
//   - modelID: AWS Bedrock model ID for the Converse API call
//
// Returns:
//   - *bedrockruntime.ConverseInput: Complete Converse API request
//   - error: Error if message conversion fails
func convertQwenToConverseRequest(qwenReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	converseMessages, systemMessages, err := convertMessages(qwenReq.Messages)
	if err != nil {
		return nil, errors.Wrap(err, "convert messages for Qwen request")
	}

	inferenceConfig := createInferenceConfig(qwenReq)

	converseReq := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	if toolConfig := createToolConfig(qwenReq.Tools, qwenReq.ToolChoice); toolConfig != nil {
		converseReq.ToolConfig = toolConfig
	}

	if qwenReq.ReasoningEffort != nil {
		reasoningConfig := map[string]string{
			"reasoning_config": *qwenReq.ReasoningEffort,
		}
		docInput := document.NewLazyDocument(reasoningConfig)
		converseReq.AdditionalModelRequestFields = docInput
	}

	return converseReq, nil
}

// convertConverseResponseToQwen converts AWS Converse API response to OpenAI-compatible Qwen format.
//
// This function transforms AWS Bedrock's Converse API response into the OpenAI-compatible
// format expected by clients. It handles:
//
//   - Text content extraction from content blocks
//   - Tool call extraction with function names and arguments
//   - Stop reason conversion to OpenAI finish_reason format
//   - Usage statistics mapping (prompt_tokens, completion_tokens, total_tokens)
//   - Response metadata (ID, timestamps, model name)
//
// The function properly handles mixed responses containing both text and tool calls,
// which can occur when the model provides explanation along with function invocations.
//
// Parameters:
//   - c: Gin context for trace ID generation
//   - converseResp: AWS Bedrock Converse API response
//   - modelName: Friendly model name for response metadata
//
// Returns:
//   - *QwenResponse: OpenAI-compatible response with Qwen content and tool calls
func convertConverseResponseToQwen(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *QwenResponse {
	var content string
	var toolCalls []QwenToolCallResponse
	var reasoningContent *string
	var finishReason string

	if converseResp.Output != nil {
		switch outputValue := converseResp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			if len(outputValue.Value.Content) > 0 {
				for _, contentBlock := range outputValue.Value.Content {
					switch contentValue := contentBlock.(type) {
					case *types.ContentBlockMemberText:
						if contentValue.Value != "" {
							content += contentValue.Value
						}
					case *types.ContentBlockMemberReasoningContent:
						if contentValue.Value != nil {
							switch reasoningBlock := contentValue.Value.(type) {
							case *types.ReasoningContentBlockMemberReasoningText:
								if reasoningBlock.Value.Text != nil && *reasoningBlock.Value.Text != "" {
									reasoningContent = reasoningBlock.Value.Text
								}
							}
						}
					case *types.ContentBlockMemberToolUse:
						toolUse := contentValue.Value
						if toolUse.ToolUseId != nil && toolUse.Name != nil {
							var inputJSON string
							if toolUse.Input != nil {
								if inputBytes, err := json.Marshal(toolUse.Input); err == nil {
									inputJSON = string(inputBytes)
								}
							}

							toolCall := QwenToolCallResponse{
								ID:   *toolUse.ToolUseId,
								Type: "function",
								Function: QwenToolFunction{
									Name:      *toolUse.Name,
									Arguments: inputJSON,
								},
							}
							toolCalls = append(toolCalls, toolCall)
						}
					}
				}
			}
			if stopReason := convertStopReason(string(converseResp.StopReason)); stopReason != nil {
				finishReason = *stopReason
			}
		}
	}

	message := QwenResponseMessage{
		Role:             "assistant",
		Content:          content,
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningContent,
	}

	choice := QwenResponseChoice{
		Index:        0,
		Message:      message,
		FinishReason: finishReason,
	}

	var usage relaymodel.Usage
	if converseResp.Usage != nil {
		if converseResp.Usage.InputTokens != nil {
			usage.PromptTokens = int(*converseResp.Usage.InputTokens)
		}
		if converseResp.Usage.OutputTokens != nil {
			usage.CompletionTokens = int(*converseResp.Usage.OutputTokens)
		}
		if converseResp.Usage.TotalTokens != nil {
			usage.TotalTokens = int(*converseResp.Usage.TotalTokens)
		} else {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
	}

	return &QwenResponse{
		ID:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []QwenResponseChoice{choice},
		Usage:   usage,
	}
}

// getTraceIDSafe retrieves the trace ID from gin context with panic recovery.
//
// This function safely extracts the trace ID from the gin context, recovering
// from any panics that might occur if the tracing middleware is not initialized.
// This defensive approach ensures that trace ID failures don't crash the request.
//
// Parameters:
//   - c: Gin context containing tracing information
//
// Returns:
//   - string: Trace ID if available, empty string if unavailable or panics occur
func getTraceIDSafe(c *gin.Context) (traceID string) {
	defer func() {
		if r := recover(); r != nil {
			traceID = ""
		}
	}()
	return tracing.GetTraceID(c)
}

// convertQwenToConverseStreamRequest converts Qwen request to AWS Converse API format for streaming.
//
// This function performs the same conversion as convertQwenToConverseRequest but
// produces a ConverseStreamInput for streaming responses. It handles:
//
//   - Messages (including tool calls and results)
//   - System messages (separated from conversation)
//   - Inference configuration (temperature, top_p, etc.)
//   - Tool configuration (function definitions and tool choice)
//
// The streaming version supports the same features as non-streaming, including
// full tool calling with incremental argument delivery.
//
// Parameters:
//   - qwenReq: Qwen request structure with messages and parameters
//   - modelID: AWS Bedrock model ID for the Converse Stream API call
//
// Returns:
//   - *bedrockruntime.ConverseStreamInput: Complete Converse Stream API request
//   - error: Error if message conversion fails
func convertQwenToConverseStreamRequest(qwenReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	converseMessages, systemMessages, err := convertMessages(qwenReq.Messages)
	if err != nil {
		return nil, errors.Wrap(err, "convert messages for Qwen stream request")
	}

	inferenceConfig := createInferenceConfig(qwenReq)

	converseReq := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	if toolConfig := createToolConfig(qwenReq.Tools, qwenReq.ToolChoice); toolConfig != nil {
		converseReq.ToolConfig = toolConfig
	}

	if qwenReq.ReasoningEffort != nil {
		reasoningConfig := map[string]string{
			"reasoning_config": *qwenReq.ReasoningEffort,
		}
		docInput := document.NewLazyDocument(reasoningConfig)
		converseReq.AdditionalModelRequestFields = docInput
	}

	return converseReq, nil
}

// ConvertMessages converts OpenAI relay model messages to Qwen message format.
//
// This function transforms the unified relay model message format into Qwen-specific
// message structures. It handles:
//
//   - Role preservation (system, user, assistant, tool)
//   - Content extraction from various content formats
//   - Tool calls: Converts from relay format to QwenToolCall structures
//   - Tool results: Maps "tool" role to "user" role with tool_call_id
//   - Arguments: Handles both string and JSON-encoded tool call arguments
//
// The function ensures that tool calling workflows are properly mapped, with
// tool results being sent as user messages (as required by AWS Bedrock) and
// maintaining the link between tool calls and their results via tool_call_id.
//
// Parameters:
//   - messages: Slice of relay model messages from the unified API
//
// Returns:
//   - []Message: Slice of Qwen message structures ready for AWS Bedrock
func ConvertMessages(messages []relaymodel.Message) []Message {
	qwenMessages := make([]Message, 0, len(messages))

	for _, message := range messages {
		qwenMessage := Message{
			Role:    message.Role,
			Content: message.StringContent(),
		}

		if len(message.ToolCalls) > 0 {
			toolCalls := make([]QwenToolCall, 0, len(message.ToolCalls))
			for _, toolCall := range message.ToolCalls {
				arguments := ""
				if toolCall.Function.Arguments != nil {
					switch arg := toolCall.Function.Arguments.(type) {
					case string:
						arguments = arg
					default:
						if marshaled, err := json.Marshal(arg); err == nil {
							arguments = string(marshaled)
						}
					}
				}

				toolCalls = append(toolCalls, QwenToolCall{
					ID:   toolCall.Id,
					Type: "function",
					Function: QwenToolFunction{
						Name:      toolCall.Function.Name,
						Arguments: arguments,
					},
				})
			}
			qwenMessage.ToolCalls = toolCalls
		}

		if message.Role == "tool" {
			qwenMessage.Role = "user"
			qwenMessage.ToolCallID = message.ToolCallId
		}

		qwenMessages = append(qwenMessages, qwenMessage)
	}

	return qwenMessages
}
