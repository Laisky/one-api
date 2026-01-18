package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// buildClaudeToolsForMCP converts Claude tool definitions into OpenAI-style tools for MCP matching.
func buildClaudeToolsForMCP(request *ClaudeMessagesRequest) []relaymodel.Tool {
	if request == nil || len(request.Tools) == 0 {
		return nil
	}
	tools := make([]relaymodel.Tool, 0, len(request.Tools))
	for _, claudeTool := range request.Tools {
		toolType := strings.TrimSpace(claudeTool.Type)
		if toolType != "" && claudeTool.InputSchema == nil {
			tools = append(tools, relaymodel.Tool{Type: toolType})
			continue
		}
		parameters, ok := claudeTool.InputSchema.(map[string]any)
		if !ok {
			parameters = map[string]any{}
		}
		tools = append(tools, relaymodel.Tool{
			Type: "function",
			Function: &relaymodel.Function{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  parameters,
			},
		})
	}
	return tools
}

// detectClaudeMCPTools returns a converted request and MCP registry when Claude tools match MCP tools.
func detectClaudeMCPTools(c *gin.Context, meta *metalib.Meta, request *ClaudeMessagesRequest, adaptorInstance adaptor.Adaptor) (*mcpToolRegistry, map[string]struct{}, *relaymodel.GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, nil, nil, nil
	}
	channelRecord := func() *model.Channel {
		if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
			if channel, ok := channelModel.(*model.Channel); ok {
				return channel
			}
		}
		return nil
	}()

	probe := &relaymodel.GeneralOpenAIRequest{Model: request.Model, Tools: buildClaudeToolsForMCP(request)}
	registry, mcpToolNames, regErr := expandMCPBuiltinsInChatRequest(c, meta, channelRecord, adaptorInstance, probe)
	if regErr != nil || registry == nil {
		return nil, nil, nil, regErr
	}

	convertedAny, err := openai_compatible.ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, nil, nil, err
	}
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	if !ok {
		return nil, nil, nil, errors.New("converted Claude request is not OpenAI request")
	}
	registry, mcpToolNames, regErr = expandMCPBuiltinsInChatRequest(c, meta, channelRecord, adaptorInstance, converted)
	if regErr != nil || registry == nil {
		return nil, nil, nil, regErr
	}
	return registry, mcpToolNames, converted, nil
}

// renderClaudeMessagesFromChatResponse converts an OpenAI chat response into Claude Messages JSON output.
func renderClaudeMessagesFromChatResponse(c *gin.Context, response *openai.TextResponse) *relaymodel.ErrorWithStatusCode {
	if response == nil {
		return openai.ErrorWrapper(errors.New("chat response is nil"), "convert_response_failed", http.StatusInternalServerError)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_chat_response_failed", http.StatusInternalServerError)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}
	converted, errResp := openai_compatible.ConvertOpenAIResponseToClaudeResponse(c, resp)
	if errResp != nil {
		return errResp
	}
	defer converted.Body.Close()
	body, err := io.ReadAll(converted.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_converted_response_failed", http.StatusInternalServerError)
	}
	contentType := converted.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Writer.Header().Set("Content-Type", contentType)
	c.Writer.WriteHeader(converted.StatusCode)
	if _, err = c.Writer.Write(body); err != nil {
		return openai.ErrorWrapper(err, "write_converted_response_failed", http.StatusInternalServerError)
	}
	return nil
}
