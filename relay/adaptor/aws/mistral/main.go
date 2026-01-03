package aws

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
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
	//
	// Set max tokens for legacy compatibility: handles clients (chatbots, git commit message generators) that don't specify max_tokens
	if textRequest.MaxTokens == 0 {
		mistralReq.MaxTokens = config.DefaultMaxToken
	} else {
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

// Handler handles non-streaming requests using appropriate API
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	mistralReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	req := mistralReq.(*Request)

	// Use InvokeModel API when tools are present, Converse API otherwise
	if len(req.Tools) > 0 {
		return handleWithInvokeModel(c, awsCli, req, awsModelName, modelName)
	}

	// Use Converse API for regular requests
	return handleWithConverseAPI(c, awsCli, req, awsModelName, modelName)
}

// handleWithInvokeModel handles requests with tools using InvokeModel API
func handleWithInvokeModel(c *gin.Context, awsCli *bedrockruntime.Client, mistralReq *Request, awsModelName, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	// Convert request to JSON for InvokeModel
	requestJSON, err := json.Marshal(mistralReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "marshal request")), nil
	}

	// Make the API call to Bedrock using InvokeModel
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(awsModelName),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        requestJSON,
	}

	awsResp, err := awsCli.InvokeModel(gmw.Ctx(c), input)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "InvokeModel")), nil
	}

	// Parse the response
	var mistralResp Response
	err = json.Unmarshal(awsResp.Body, &mistralResp)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "unmarshal response")), nil
	}

	// Convert to OpenAI format
	openaiResp := ResponseMistral2OpenAI(c, &mistralResp, modelName)
	openaiResp.Model = modelName

	// Calculate token usage using accurate AWS Bedrock CountTokens API
	// Convert Mistral messages to relaymodel.Message for accurate token counting
	var relayMessages []relaymodel.Message
	for _, msg := range mistralReq.Messages {
		relayMessages = append(relayMessages, relaymodel.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Use accurate token counting from AWS Bedrock
	promptTokens, err := utils.GetAccurateTokenCount(gmw.Ctx(c), awsCli, relayMessages, modelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "prompt token")), nil
	}

	usage := relaymodel.Usage{
		PromptTokens: promptTokens,
	}

	// Use accurate completion token counting from response content
	if len(openaiResp.Choices) > 0 {
		content := openaiResp.Choices[0].Message.StringContent()
		completionTokens, err := utils.CountTokenText(gmw.Ctx(c), awsCli, content, modelName)
		if err != nil {
			return utils.WrapErr(errors.Wrap(err, "completion token")), nil
		}

		usage.CompletionTokens = completionTokens
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	openaiResp.Usage = usage

	c.JSON(http.StatusOK, openaiResp)
	return nil, &usage
}

// handleWithConverseAPI handles regular requests using Converse API
func handleWithConverseAPI(c *gin.Context, awsCli *bedrockruntime.Client, mistralReq *Request, awsModelName, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	// Convert Mistral request to Converse API format
	converseReq, err := convertMistralToConverseRequest(mistralReq, awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to OpenAI format
	openaiResp := convertConverseResponseToOpenAI(c, awsResp, c.GetString(ctxkey.RequestModel))

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
func ResponseMistral2OpenAI(c *gin.Context, mistralResponse *Response, modelName string) *openai.TextResponse {
	var responseText string
	var finishReason string
	var toolCalls []relaymodel.Tool

	if len(mistralResponse.Choices) > 0 {
		responseText = mistralResponse.Choices[0].Message.Content
		if stopReason := convertStopReason(mistralResponse.Choices[0].StopReason); stopReason != nil {
			finishReason = *stopReason
		}

		// Convert tool calls from Mistral to OpenAI format
		if len(mistralResponse.Choices[0].Message.ToolCalls) > 0 {
			toolCalls = make([]relaymodel.Tool, len(mistralResponse.Choices[0].Message.ToolCalls))
			for i, tc := range mistralResponse.Choices[0].Message.ToolCalls {
				toolCalls[i] = relaymodel.Tool{
					Id:   tc.ID,
					Type: "function",
					Function: &relaymodel.Function{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}

	choice := openai.TextResponseChoice{
		Index: 0,
		Message: relaymodel.Message{
			Role:      "assistant",
			Content:   responseText,
			ToolCalls: toolCalls,
		},
		FinishReason: finishReason,
	}

	return &openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []openai.TextResponseChoice{choice},
	}
}

// StreamHandler handles streaming requests using appropriate API
func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	lg := gmw.GetLogger(c)
	createdTime := helper.GetTimestamp()
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	mistralReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	req := mistralReq.(*Request)

	// Use InvokeModelWithResponseStream when tools are present, ConverseStream otherwise
	if len(req.Tools) > 0 {
		return handleStreamWithInvokeModel(c, awsCli, req, awsModelName, createdTime, lg)
	}

	// Use ConverseStream API for regular requests
	return handleStreamWithConverseAPI(c, awsCli, req, awsModelName, createdTime, lg)
}

// handleStreamWithInvokeModel handles streaming requests with tools using InvokeModelWithResponseStream
func handleStreamWithInvokeModel(c *gin.Context, awsCli *bedrockruntime.Client, mistralReq *Request, awsModelName string, createdTime int64, lg log.Logger) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	// Convert request to JSON for InvokeModelWithResponseStream
	requestJSON, err := json.Marshal(mistralReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "marshal request")), nil
	}

	awsReq := &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(awsModelName),
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
		Body:        requestJSON,
	}

	awsResp, err := awsCli.InvokeModelWithResponseStream(gmw.Ctx(c), awsReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "InvokeModelWithResponseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Pragma", "no-cache")

	var usage relaymodel.Usage
	var id string

	c.Stream(func(w io.Writer) bool {
		event, ok := <-stream.Events()
		if !ok {
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}

		switch v := event.(type) {
		case *types.ResponseStreamMemberChunk:
			var mistralResp StreamResponse
			err := json.Unmarshal(v.Value.Bytes, &mistralResp)
			if err != nil {
				lg.Error("error unmarshalling stream response", zap.Error(err))
				return false
			}

			// Convert Mistral streaming response to OpenAI format
			openaiResp := StreamResponseMistral2OpenAI(c, &mistralResp, c.GetString(ctxkey.RequestModel))
			if openaiResp == nil {
				return true
			}

			if id == "" {
				id = openaiResp.Id
			}
			openaiResp.Id = id
			openaiResp.Model = c.GetString(ctxkey.RequestModel)
			openaiResp.Created = createdTime

			jsonStr, err := json.Marshal(openaiResp)
			if err != nil {
				lg.Error("error marshalling stream response", zap.Error(err))
				return true
			}
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
			return true

		case *types.UnknownUnionMember:
			lg.Warn("unknown event type in stream", zap.String("tag", v.Tag))
			return false
		default:
			lg.Warn("unexpected event type in stream")
			return false
		}
	})

	return nil, &usage
}

// handleStreamWithConverseAPI handles streaming requests using ConverseStream API
func handleStreamWithConverseAPI(c *gin.Context, awsCli *bedrockruntime.Client, mistralReq *Request, awsModelName string, createdTime int64, lg log.Logger) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	// Convert Mistral request to Converse API format
	converseReq, err := convertMistralToConverseStreamRequest(mistralReq, awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	awsResp, err := awsCli.ConverseStream(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "ConverseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	var usage relaymodel.Usage
	var id string
	finalizer := streamfinalizer.NewFinalizer(
		c.GetString(ctxkey.RequestModel),
		createdTime,
		&usage,
		gmw.GetLogger(c),
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
			// Handle message start
			id = tracing.GenerateChatCompletionIDFromContext(c)
			finalizer.SetID(id)
			return true

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			// Handle content delta - this is where the actual text content comes from ConverseStream
			if v.Value.Delta != nil {
				var response *openai.ChatCompletionsStreamResponse

				// Check if this is a text delta (AWS SDK union type pattern)
				switch deltaValue := v.Value.Delta.(type) {
				case *types.ContentBlockDeltaMemberText:
					if textDelta := deltaValue.Value; textDelta != "" {
						// Create OpenAI-compatible streaming response with simple string content
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
				}

				// Send the response if we have one
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

		case *types.ConverseStreamOutputMemberMessageStop:
			return finalizer.RecordStop(convertStopReason(string(v.Value.StopReason)))

		case *types.ConverseStreamOutputMemberMetadata:
			return finalizer.RecordMetadata(v.Value.Usage)

		default:
			// Handle other event types
			return true
		}
	})

	return nil, &usage
}

// StreamResponseMistral2OpenAI converts Mistral streaming response to OpenAI format
func StreamResponseMistral2OpenAI(c *gin.Context, mistralResponse *StreamResponse, modelName string) *openai.ChatCompletionsStreamResponse {
	if len(mistralResponse.Choices) == 0 {
		return nil
	}

	choice := mistralResponse.Choices[0]

	return &openai.ChatCompletionsStreamResponse{
		Id:      tracing.GenerateChatCompletionID(c),
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
		// If-else statement removed as it is already handled in the caller during request conversion
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
func convertConverseResponseToOpenAI(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *openai.TextResponse {
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
		Id:      tracing.GenerateChatCompletionID(c),
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
