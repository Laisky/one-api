package aws

import (
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/songquanpeng/one-api/common/ctxkey"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// Compile-time verification that Adaptor implements the AwsAdapter interface.
var _ utils.AwsAdapter = new(Adaptor)

// Adaptor implements the AWS Bedrock adapter for OpenAI OSS language models.
//
// This struct provides the core functionality for integrating with AWS Bedrock's
// OpenAI OSS models ([gpt-oss-20b], [gpt-oss-120b]), implementing the AwsAdapter interface
// to ensure consistent behavior across all AWS Bedrock integrations in the One API system.
//
// The adapter handles the complete request-response lifecycle:
//   - Converting OpenAI-compatible requests to AWS Bedrock OpenAI OSS format
//   - Processing responses from AWS Bedrock back to OpenAI-compatible format
//   - Managing both streaming and non-streaming response modes
//   - Handling OpenAI OSS models' reasoning content capabilities (similar to DeepSeek-R1)
//   - Managing error conditions and usage tracking
//
// [gpt-oss-20b]: https://openai.com/index/introducing-gpt-oss/
// [gpt-oss-120b]: https://openai.com/index/introducing-gpt-oss/
type Adaptor struct {
	// No additional fields required - stateless adapter design
}

// ConvertRequest transforms an OpenAI-compatible request into AWS Bedrock OpenAI OSS format.
//
// This method performs the critical translation between the One API's unified request format
// and the specific format expected by AWS Bedrock's OpenAI OSS models. It handles:
//
//   - Message format conversion from OpenAI to OpenAI OSS structure
//   - Parameter mapping and validation for OpenAI OSS specific features
//   - Stop sequence processing for proper generation control
//   - Context storage for downstream processing including reasoning content support
//
// Parameters:
//   - c: Gin context for the HTTP request, used for storing converted data
//   - relayMode: Processing mode indicator (currently unused but maintained for interface compliance)
//   - request: OpenAI-compatible request to be converted to OpenAI OSS format
//
// Returns:
//   - any: Converted request in AWS Bedrock OpenAI OSS format, ready for API submission
//   - error: Error if the request is invalid or conversion fails
//
// The method stores both the original model name and converted request in the context
// for use by downstream handlers and response processors, enabling proper reasoning
// content handling in OpenAI OSS responses.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	openaiReq := ConvertRequest(*request)
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, openaiReq)
	return openaiReq, nil
}

// DoResponse processes the response from AWS Bedrock OpenAI OSS models and converts it back to OpenAI format.
//
// This method handles the complete response processing pipeline, supporting both streaming
// and non-streaming modes. It coordinates with specialized handlers to:
//
//   - Process AWS Bedrock OpenAI OSS responses in their native format
//   - Convert responses back to OpenAI-compatible structure
//   - Handle OpenAI OSS models' reasoning content blocks and structured responses
//   - Track token usage for billing and quota management
//   - Manage reasoning-specific metadata and usage statistics
//   - Handle errors and edge cases appropriately
//
// The method automatically detects the response mode (streaming vs non-streaming) based
// on metadata and delegates to the appropriate specialized handler that understands
// OpenAI OSS models' reasoning content format.
//
// Parameters:
//   - c: Gin context containing request data and used for response writing
//   - awsCli: AWS Bedrock Runtime client for making API calls to AWS services
//   - meta: Request metadata including streaming flags and model information
//
// Returns:
//   - usage: Token usage statistics for billing and monitoring purposes, including reasoning token counts
//   - err: Error with HTTP status code if processing fails, nil on success
//
// Error conditions include network failures, AWS API errors, response parsing issues,
// reasoning content processing errors, and context cancellation scenarios.
func (a *Adaptor) DoResponse(c *gin.Context, awsCli *bedrockruntime.Client, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, awsCli)
	} else {
		err, usage = Handler(c, awsCli, meta.ActualModelName)
	}
	return
}
