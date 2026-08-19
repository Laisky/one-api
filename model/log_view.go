package model

import "github.com/Laisky/one-api/dto"

// ToResponse builds the external boundary DTO for a log entry. It replaces the
// retired Log.MarshalJSON whitelist: no internal integer id or FK crosses the
// API; channel_name/metadata keep their omitempty semantics.
//
// Parameters: none (pointer receiver).
//
// Return values:
//   - dto.LogResponse: the UUID-only external shape. A nil receiver yields the
//     zero shape.
func (log *Log) ToResponse() dto.LogResponse {
	if log == nil {
		return dto.LogResponse{}
	}
	return dto.LogResponse{
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
		Metadata:           map[string]any(log.Metadata),
	}
}

// LogsToResponses maps a slice of logs to their external DTOs, pre-allocating
// the result (log lists are the hot path — up to a page of rows each call).
//
// Parameters:
//   - logs: rows to convert; nil elements map to the zero shape.
//
// Return values:
//   - []dto.LogResponse: one entry per input row.
func LogsToResponses(logs []*Log) []dto.LogResponse {
	out := make([]dto.LogResponse, 0, len(logs))
	for _, l := range logs {
		out = append(out, l.ToResponse())
	}
	return out
}
