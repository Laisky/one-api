package controller

import (
	"context"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// preparedClaudePromptTokens quotes the prepared provider representation without mutating the request or applying a native quote to converted text.
func preparedClaudePromptTokens(ctx context.Context, request *ClaudeMessagesRequest, converted any) (int, error) {
	switch outgoing := converted.(type) {
	case *relaymodel.GeneralOpenAIRequest:
		return preparedClaudeChatTokens(ctx, request, outgoing)
	case relaymodel.GeneralOpenAIRequest:
		return preparedClaudeChatTokens(ctx, request, &outgoing)
	case *openai.ResponseAPIRequest:
		if outgoing == nil {
			return 0, errors.New("nil prepared Claude Responses request")
		}
		quote := *outgoing
		quote.Model = claudeReservationModel(request)
		// The shared Responses estimator counts tools and text formats.
		return getResponseAPIPromptTokens(ctx, &quote), nil
	default:
		return getClaudeMessagesPromptTokens(ctx, request)
	}
}

// preparedClaudeChatTokens counts the converted Chat request with the shared
// Chat prompt estimator, priced with the Claude reservation model. Parameters:
// ctx carries the request logger, request is the client Messages request and
// outgoing the prepared Chat payload. It returns the estimate, or an error for a
// missing prepared request.
func preparedClaudeChatTokens(ctx context.Context, request *ClaudeMessagesRequest, outgoing *relaymodel.GeneralOpenAIRequest) (int, error) {
	if outgoing == nil {
		return 0, errors.New("nil prepared Claude chat request")
	}
	return chatPromptTokens(ctx, outgoing, claudeReservationModel(request)), nil
}
