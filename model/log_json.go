package model

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// MarshalJSON emits the external log contract without internal integer identifiers.
// Parameters: none.
//
// Return values:
//   - []byte: encoded JSON containing UUID identifiers and log attributes.
//   - error: wrapped marshal failure, if encoding fails.
func (log Log) MarshalJSON() ([]byte, error) {
	type logJSON struct {
		UUID               string      `json:"uuid"`
		UserUUID           *string     `json:"user_uuid"`
		CreatedAt          int64       `json:"created_at"`
		Type               int         `json:"type"`
		Content            string      `json:"content"`
		Username           string      `json:"username"`
		TokenName          string      `json:"token_name"`
		TokenUUID          *string     `json:"token_uuid"`
		ModelName          string      `json:"model_name"`
		OriginModelName    string      `json:"origin_model_name"`
		Quota              int         `json:"quota"`
		PromptTokens       int         `json:"prompt_tokens"`
		CompletionTokens   int         `json:"completion_tokens"`
		ChannelUUID        *string     `json:"channel_uuid"`
		ChannelName        string      `json:"channel_name,omitempty"`
		RequestId          string      `json:"request_id"`
		TraceId            string      `json:"trace_id"`
		UpdatedAt          int64       `json:"updated_at"`
		ElapsedTime        int64       `json:"elapsed_time"`
		IsStream           bool        `json:"is_stream"`
		SystemPromptReset  bool        `json:"system_prompt_reset"`
		CachedPromptTokens int         `json:"cached_prompt_tokens"`
		Metadata           LogMetadata `json:"metadata,omitempty"`
	}

	payload, err := json.Marshal(logJSON{
		UUID:               log.UUID,
		UserUUID:           log.UserUUID,
		CreatedAt:          log.CreatedAt,
		Type:               log.Type,
		Content:            log.Content,
		Username:           log.Username,
		TokenName:          log.TokenName,
		TokenUUID:          log.TokenUUID,
		ModelName:          log.ModelName,
		OriginModelName:    log.OriginModelName,
		Quota:              log.Quota,
		PromptTokens:       log.PromptTokens,
		CompletionTokens:   log.CompletionTokens,
		ChannelUUID:        log.ChannelUUID,
		ChannelName:        log.ChannelName,
		RequestId:          log.RequestId,
		TraceId:            log.TraceId,
		UpdatedAt:          log.UpdatedAt,
		ElapsedTime:        log.ElapsedTime,
		IsStream:           log.IsStream,
		SystemPromptReset:  log.SystemPromptReset,
		CachedPromptTokens: log.CachedPromptTokens,
		Metadata:           log.Metadata,
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal log")
	}

	return payload, nil
}
