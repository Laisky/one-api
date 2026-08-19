package aws

import (
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/Laisky/one-api/common/ctxkey"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// Ensure Adaptor implements the AwsAdapter interface at compile time.
var _ utils.AwsAdapter = new(Adaptor)

// Adaptor implements the AwsAdapter interface for Writer models.
// It handles request conversion and response processing for Writer's Palmyra series models
// running on AWS Bedrock using the Converse API.
type Adaptor struct {
}

// ConvertRequest converts a GeneralOpenAIRequest to Writer-specific format.
// It transforms the incoming OpenAI-compatible request into a Writer request structure
// and stores necessary context information for downstream processing.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	writerReq := ConvertRequest(*request)
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, writerReq)
	return writerReq, nil
}

// DoResponse handles the response processing for Writer models.
// It determines whether to use streaming or non-streaming response handling
// based on the request metadata and delegates to the appropriate handler.
func (a *Adaptor) DoResponse(c *gin.Context, awsCli *bedrockruntime.Client, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, awsCli)
	} else {
		err, usage = Handler(c, awsCli, meta.ActualModelName)
	}
	return
}
