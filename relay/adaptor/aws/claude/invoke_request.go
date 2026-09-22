package aws

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
)

const (
	invokeBodyKey = "aws_claude_prepared_invoke_body"
	// Bedrock InvokeModel's body limit is 25,000,000 bytes, not 25 MiB.
	// https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_InvokeModel.html
	maxInvokeBodyBytes = 25000000
)

type preparedInvokeBody struct {
	model string
	body  []byte
}

// PrepareRequestBody retains the controller-prepared payload in the request's
// context. It accepts the authoritative sanitized reader and returns validation
// errors without invoking AWS. It never rereads the unsanitized inbound body.
func (a *Adaptor) PrepareRequestBody(c *gin.Context, reader io.Reader) error {
	if c == nil {
		return errors.New("missing Claude request context")
	}
	c.Set(invokeBodyKey, (*preparedInvokeBody)(nil))
	if reader == nil {
		return errors.New("missing Claude request body")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxInvokeBodyBytes+1))
	if err != nil {
		return errors.Wrap(err, "read Claude Invoke body")
	}
	if len(raw) > maxInvokeBodyBytes {
		return errors.New("Claude Invoke body exceeds Bedrock size limit")
	}
	body, err := normalizeInvokeBody(c, raw)
	if err != nil {
		return err
	}
	c.Set(invokeBodyKey, &preparedInvokeBody{model: c.GetString(ctxkey.RequestModel), body: body})
	return nil
}

// normalizeInvokeBody translates only transport-owned fields in a JSON object.
// Its raw-message values preserve unknown fields, nested schemas, opaque
// signatures, structured content, and integers larger than float64 can hold.
func normalizeInvokeBody(c *gin.Context, raw []byte) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.Wrap(err, "decode Claude Invoke body")
	}
	if payload == nil {
		return nil, errors.New("Claude Invoke body must be an object")
	}
	delete(payload, "model")
	delete(payload, "stream")
	payload["anthropic_version"] = json.RawMessage(`"bedrock-2023-05-31"`)
	// Anthropic's HTTP beta header becomes the Invoke JSON beta array. Do not
	// invent opt-ins; retain the administrator/controller's explicit choices.
	if c.Request != nil && strings.TrimSpace(c.Request.Header.Get("anthropic-beta")) != "" {
		var betas []string
		if value, ok := payload["anthropic_beta"]; ok {
			if err := json.Unmarshal(value, &betas); err != nil {
				return nil, errors.Wrap(err, "decode Claude beta array")
			}
		}
		seen := make(map[string]bool, len(betas))
		merged := make([]string, 0, len(betas))
		for _, beta := range append(betas, strings.Split(c.Request.Header.Get("anthropic-beta"), ",")...) {
			beta = strings.TrimSpace(beta)
			if beta != "" && !seen[beta] {
				merged = append(merged, beta)
				seen[beta] = true
			}
		}
		value, err := json.Marshal(merged)
		if err != nil {
			return nil, errors.Wrap(err, "encode Claude beta array")
		}
		payload["anthropic_beta"] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "encode Claude Invoke body")
	}
	if len(body) > maxInvokeBodyBytes {
		return nil, errors.New("Claude Invoke body exceeds Bedrock size limit")
	}
	return body, nil
}

// claudeInvokeBody returns the prepared request payload. Typed chat callers
// retain their historical fallback; native requests must supply a prepared body
// rather than silently losing fields through the billing-only typed view.
func claudeInvokeBody(c *gin.Context) ([]byte, error) {
	if value, exists := c.Get(invokeBodyKey); exists {
		prepared, ok := value.(*preparedInvokeBody)
		if !ok || prepared == nil || prepared.model != c.GetString(ctxkey.RequestModel) {
			return nil, errors.New("Claude Invoke body is not prepared for the selected model")
		}
		return bytes.Clone(prepared.body), nil
	}
	if c.GetBool(ctxkey.ClaudeDirectPassthrough) {
		return nil, errors.New("native Claude Invoke body was not prepared")
	}
	request, exists := c.Get(ctxkey.ConvertedRequest)
	if !exists || request == nil {
		return nil, errors.New("missing converted Claude request")
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "encode converted Claude request")
	}
	return normalizeInvokeBody(c, raw)
}

// resolveClaudeInvokeModel selects the explicit channel ARN before any catalog
// mapping. Otherwise it applies the existing deterministic profile mapping.
// It never sends inference probes or changes routing on an authentication,
// throttling, timeout, or malformed-payload response.
func resolveClaudeInvokeModel(c *gin.Context, client *bedrockruntime.Client) (string, error) {
	if client == nil {
		return "", errors.New("AWS Bedrock client is not initialized")
	}
	if target := strings.TrimSpace(AwsClaudeModelTransArn(c, client)); target != "" {
		return target, nil
	}
	modelID, err := AwsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return "", errors.Wrap(err, "resolve Claude model")
	}
	return utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), modelID, client.Options().Region), nil
}
