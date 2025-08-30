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

// Adaptor implements the AWS Bedrock adapter for Cohere Command R language models.
//
// This struct provides the core functionality for integrating with AWS Bedrock's
// Cohere Command R family models, implementing the AwsAdapter interface to ensure consistent
// behavior across all AWS Bedrock integrations in the One API system.
//
// The adapter handles the complete request-response lifecycle:
//   - Converting OpenAI-compatible requests to AWS Bedrock Cohere Command R format
//   - Processing responses from AWS Bedrock back to OpenAI-compatible format
//   - Managing both streaming and non-streaming response modes
//   - Handling Cohere's enterprise-grade features including multi-lingual support
//   - Managing error conditions and usage tracking
//   - Supporting Cohere's advanced safety and content filtering mechanisms
type Adaptor struct {
	// No additional fields required - stateless adapter design
}

// ConvertRequest transforms an OpenAI-compatible request into AWS Bedrock Cohere Command R format.
//
// This method performs the critical translation between the One API's unified request format
// and the specific format expected by AWS Bedrock's Cohere Command R models. It handles:
//
//   - Message format conversion from OpenAI to Cohere Command R structure
//   - Parameter mapping and validation for Cohere's enterprise features
//   - Stop sequence processing for proper generation control
//   - Context storage for downstream processing including multi-lingual support
//   - Integration with Cohere's safety and content filtering mechanisms
//
// Parameters:
//   - c: Gin context for the HTTP request, used for storing converted data
//   - relayMode: Processing mode indicator (currently unused but maintained for interface compliance)
//   - request: OpenAI-compatible request to be converted to Cohere Command R format
//
// Returns:
//   - any: Converted request in AWS Bedrock Cohere Command R format, ready for API submission
//   - error: Error if the request is invalid or conversion fails
//
// The method stores both the original model name and converted request in the context
// for use by downstream handlers and response processors, enabling proper enterprise-grade
// conversation handling with Cohere's multi-lingual and safety features.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	cohereReq := ConvertRequest(*request)
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, cohereReq)
	return cohereReq, nil
}

// DoResponse processes the response from AWS Bedrock Cohere Command R and converts it back to OpenAI format.
//
// This method handles the complete response processing pipeline, supporting both streaming
// and non-streaming modes. It coordinates with specialized handlers to:
//
//   - Process AWS Bedrock Cohere Command R responses in their native format
//   - Convert responses back to OpenAI-compatible structure
//   - Handle Cohere's enterprise-grade conversation features and multi-lingual responses
//   - Track token usage for billing and quota management
//   - Manage Cohere's advanced safety filtering and content moderation results
//   - Handle errors and edge cases appropriately with enterprise-grade reliability
//
// The method automatically detects the response mode (streaming vs non-streaming) based
// on metadata and delegates to the appropriate specialized handler that understands
// Cohere Command R's enterprise conversation format and safety mechanisms.
//
// Parameters:
//   - c: Gin context containing request data and used for response writing
//   - awsCli: AWS Bedrock Runtime client for making API calls to AWS services
//   - meta: Request metadata including streaming flags and model information
//
// Returns:
//   - usage: Token usage statistics for billing and monitoring purposes with enterprise tracking
//   - err: Error with HTTP status code if processing fails, nil on success
//
// Error conditions include network failures, AWS API errors, response parsing issues,
// Cohere safety filtering triggers, content moderation blocks, and context cancellation scenarios.
func (a *Adaptor) DoResponse(c *gin.Context, awsCli *bedrockruntime.Client, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, awsCli)
	} else {
		err, usage = Handler(c, awsCli, meta.ActualModelName)
	}
	return
}
