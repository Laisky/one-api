package controller

import (
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// RelayResponseAPIGetHelper handles GET /v1/responses/:response_id requests
func RelayResponseAPIGetHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)

	if meta.ChannelType != channeltype.OpenAI {
		return openai.ErrorWrapper(errors.New("Response API is only supported for OpenAI channels"), "unsupported_channel", http.StatusBadRequest)
	}

	if err := applyResponseAPIStreamParams(c, meta); err != nil {
		return openai.ErrorWrapper(err, "invalid_query_parameter", http.StatusBadRequest)
	}
	metalib.Set2Context(c, meta)

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandlerWithContext(c, resp)
	}

	_, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		return respErr
	}

	return nil
}

// RelayResponseAPIDeleteHelper handles DELETE /v1/responses/:response_id requests
func RelayResponseAPIDeleteHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)
	meta.IsStream = false
	metalib.Set2Context(c, meta)

	if meta.ChannelType != channeltype.OpenAI {
		return openai.ErrorWrapper(errors.New("Response API is only supported for OpenAI channels"), "unsupported_channel", http.StatusBadRequest)
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandlerWithContext(c, resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	if err = resp.Body.Close(); err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	if resp.Header.Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(body); err != nil {
		return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError)
	}

	return nil
}

// RelayResponseAPICancelHelper handles POST /v1/responses/:response_id/cancel requests
func RelayResponseAPICancelHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)
	meta.IsStream = false
	metalib.Set2Context(c, meta)

	if meta.ChannelType != channeltype.OpenAI {
		return openai.ErrorWrapper(errors.New("Response API is only supported for OpenAI channels"), "unsupported_channel", http.StatusBadRequest)
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandlerWithContext(c, resp)
	}

	_, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		return respErr
	}

	return nil
}
