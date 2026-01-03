package aws

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/tracing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/internal/streamfinalizer"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// AwsModelIDMap maps Writer model names to their AWS Bedrock model IDs.
// Support for Writer models (Palmyra series)
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
var AwsModelIDMap = map[string]string{
	// Writer Palmyra models
	"palmyra-x4": "writer.palmyra-x4-v1:0",
	"palmyra-x5": "writer.palmyra-x5-v1:0",
}

// awsModelID retrieves the AWS Bedrock model ID for a given Writer model name.
// It returns an error if the requested model is not supported.
func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}

	return "", errors.Errorf("model %s not found", requestModel)
}

// ConvertRequest converts a GeneralOpenAIRequest to a Writer-specific request format.
// It handles parameter mapping including max tokens, temperature, top_p, and stop sequences.
func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	writerRequest := &Request{
		Messages:    textRequest.Messages,
		Temperature: textRequest.Temperature,
		TopP:        textRequest.TopP,
	}

	// Handle max tokens
	if textRequest.MaxTokens == 0 {
		writerRequest.MaxTokens = config.DefaultMaxToken
	} else {
		writerRequest.MaxTokens = textRequest.MaxTokens
	}

	// Handle stop sequences
	if textRequest.Stop != nil {
		if stopSlice, ok := textRequest.Stop.([]any); ok {
			stopSequences := make([]string, 0, len(stopSlice))
			for _, stop := range stopSlice {
				if stopStr, ok := stop.(string); ok && stopStr != "" {
					stopSequences = append(stopSequences, stopStr)
				}
			}
			if len(stopSequences) > 0 {
				writerRequest.Stop = stopSequences
			}
		} else if stopStr, ok := textRequest.Stop.(string); ok {
			if stopStr != "" {
				writerRequest.Stop = []string{stopStr}
			}
		} else if stopSlice, ok := textRequest.Stop.([]string); ok {
			filt := stopSlice[:0]
			for _, s := range stopSlice {
				if s != "" {
					filt = append(filt, s)
				}
			}
			if len(filt) > 0 {
				writerRequest.Stop = filt
			}
		}
	}

	return writerRequest
}

// convertWriterToConverseRequest converts Writer request to Converse API format.
// It transforms Writer-specific parameters into AWS Bedrock Converse API format,
// handling message role conversion and inference configuration setup.
func convertWriterToConverseRequest(writerReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range writerReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.StringContent(),
			})
		case "user":
			// User messages use standard Converse API format
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.StringContent(),
				},
			}

			converseMessages = append(converseMessages, types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			})
		case "assistant":
			// Assistant messages use standard Converse API format
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.StringContent(),
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
	if writerReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(writerReq.MaxTokens))
	}

	if writerReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*writerReq.Temperature))
	}
	if writerReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*writerReq.TopP))
	}
	if len(writerReq.Stop) > 0 {
		stopSequences := make([]string, len(writerReq.Stop))
		copy(stopSequences, writerReq.Stop)
		inferenceConfig.StopSequences = stopSequences
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

// convertWriterToConverseStreamRequest converts Writer request to Converse Stream API format.
// It reuses the standard Converse conversion and adapts it for streaming.
func convertWriterToConverseStreamRequest(writerReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	converseReq, err := convertWriterToConverseRequest(writerReq, modelID)
	if err != nil {
		return nil, errors.Wrap(err, "convert writer request for stream")
	}

	return &bedrockruntime.ConverseStreamInput{
		ModelId:         converseReq.ModelId,
		Messages:        converseReq.Messages,
		System:          converseReq.System,
		InferenceConfig: converseReq.InferenceConfig,
	}, nil
}

// convertStopReason converts AWS Bedrock stop reasons to OpenAI-compatible finish reasons.
// It maps AWS-specific stop reasons to standardized OpenAI format for consistency.
func convertStopReason(awsReason string) *string {
	switch awsReason {
	case "end_turn":
		return aws.String("stop")
	case "tool_use":
		return aws.String("tool_calls")
	case "max_tokens":
		return aws.String("length")
	case "stop_sequence":
		return aws.String("stop")
	case "content_filtered":
		return aws.String("content_filter")
	default:
		// Return the original reason if unknown
		return aws.String(awsReason)
	}
}

// convertConverseResponseToOpenAI converts AWS Bedrock Converse response to OpenAI format.
// It extracts text content and usage information, transforming them into OpenAI-compatible response structure.
func convertConverseResponseToOpenAI(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *openai.TextResponse {
	var responseText string
	var finishReason string

	// Extract response content from Converse API response
	if converseResp.Output != nil {
		switch outputValue := converseResp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			if len(outputValue.Value.Content) > 0 {
				// Process content blocks
				for _, contentBlock := range outputValue.Value.Content {
					switch contentValue := contentBlock.(type) {
					case *types.ContentBlockMemberText:
						// Add text content
						if contentValue.Value != "" {
							responseText = contentValue.Value
							break
						}
					}
				}
			}
			// Convert stop reason
			if stopReason := convertStopReason(string(converseResp.StopReason)); stopReason != nil {
				finishReason = *stopReason
			}
		}
	}

	// Create OpenAI-compatible choice
	choice := openai.TextResponseChoice{
		Index: 0,
		Message: relaymodel.Message{
			Role:    "assistant",
			Content: responseText,
			Name:    nil,
		},
		FinishReason: finishReason,
	}

	// Create OpenAI-compatible response
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionIDFromContext(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []openai.TextResponseChoice{choice},
	}

	return &fullTextResponse
}

// Handler handles non-streaming Writer model requests using AWS Bedrock Converse API.
// It processes the request, calls the Writer model, and returns the response in OpenAI format
// along with token usage information for billing purposes.
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	writerReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Writer request to Converse API format
	converseReq, err := convertWriterToConverseRequest(writerReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to OpenAI format
	openaiResp := convertConverseResponseToOpenAI(c, awsResp, modelName)

	// Convert usage to relaymodel.Usage for billing
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

	c.JSON(http.StatusOK, openaiResp)
	return nil, &usage
}

// StreamHandler handles streaming Writer model requests using AWS Bedrock Converse Stream API.
// It processes streaming requests, handles real-time response chunks, and converts them to
// OpenAI-compatible streaming format while tracking token usage for billing.
func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	lg := gmw.GetLogger(c)
	createdTime := helper.GetTimestamp()
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	writerReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Writer request to Converse API format
	converseReq, err := convertWriterToConverseStreamRequest(writerReq.(*Request), awsModelName)
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
			// Handle message start
			id = tracing.GenerateChatCompletionIDFromContext(c)
			finalizer.SetID(id)
			return true

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			// Handle content delta - this is where the actual text content comes from ConverseStream
			if v.Value.Delta != nil {
				// Check if this is a text delta (AWS SDK union type pattern)
				switch deltaValue := v.Value.Delta.(type) {
				case *types.ContentBlockDeltaMemberText:
					if textDelta := deltaValue.Value; textDelta != "" {
						// Create OpenAI-compatible streaming response with simple string content
						response := &openai.ChatCompletionsStreamResponse{
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
