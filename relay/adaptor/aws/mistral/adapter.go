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

// Adaptor implements the AWS Bedrock adapter for Mistral Large language model.
//
// This struct provides the core functionality for integrating with AWS Bedrock's
// Mistral Large model, implementing the AwsAdapter interface to ensure consistent
// behavior across all AWS Bedrock integrations in the One API system.
//
// The adapter handles the complete request-response lifecycle:
//   - Converting OpenAI-compatible requests to AWS Bedrock Mistral format
//   - Processing responses from AWS Bedrock back to OpenAI-compatible format
//   - Managing both streaming and non-streaming response modes
//   - Handling error conditions and usage tracking
type Adaptor struct {
	// No additional fields required - stateless adapter design
}

// ConvertRequest transforms an OpenAI-compatible request into AWS Bedrock Mistral format.
//
// This method performs the critical translation between the One API's unified request format
// and the specific format expected by AWS Bedrock's Mistral Large model. It handles:
//
//   - Message format conversion from OpenAI to Mistral structure
//   - Parameter mapping and validation
//   - Tool/function calling setup when applicable
//   - Context storage for downstream processing
//
// Parameters:
//   - c: Gin context for the HTTP request, used for storing converted data
//   - relayMode: Processing mode indicator (currently unused but maintained for interface compliance)
//   - request: OpenAI-compatible request to be converted to Mistral format
//
// Returns:
//   - any: Converted request in AWS Bedrock Mistral format, ready for API submission
//   - error: Error if the request is invalid or conversion fails
//
// The method stores both the original model name and converted request in the context
// for use by downstream handlers and response processors.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	mistralReq := ConvertRequest(*request)
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, mistralReq)
	return mistralReq, nil
}

// DoResponse processes the response from AWS Bedrock Mistral and converts it back to OpenAI format.
//
// This method handles the complete response processing pipeline, supporting both streaming
// and non-streaming modes. It coordinates with specialized handlers to:
//
//   - Process AWS Bedrock responses in their native format
//   - Convert responses back to OpenAI-compatible structure
//   - Track token usage for billing and quota management
//   - Handle errors and edge cases appropriately
//
// The method automatically detects the response mode (streaming vs non-streaming) based
// on metadata and delegates to the appropriate specialized handler.
//
// Parameters:
//   - c: Gin context containing request data and used for response writing
//   - awsCli: AWS Bedrock Runtime client for making API calls to AWS services
//   - meta: Request metadata including streaming flags and model information
//
// Returns:
//   - usage: Token usage statistics for billing and monitoring purposes
//   - err: Error with HTTP status code if processing fails, nil on success
//
// Error conditions include network failures, AWS API errors, response parsing issues,
// and context cancellation scenarios.
func (a *Adaptor) DoResponse(c *gin.Context, awsCli *bedrockruntime.Client, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, awsCli)
	} else {
		err, usage = Handler(c, awsCli, meta.ActualModelName)
	}
	return
}
