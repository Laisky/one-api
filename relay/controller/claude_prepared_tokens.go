package controller

import (
	"context"
	"encoding/json"

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
		tokens := getResponseAPIPromptTokens(ctx, &quote)
		for _, value := range []any{quote.Tools, quote.Text} {
			additional, err := claudePreparedJSONTokens(value, quote.Model)
			if err != nil {
				return 0, err
			}
			tokens += additional
		}
		return tokens, nil
	default:
		return getClaudeMessagesPromptTokens(ctx, request)
	}
}

// preparedClaudeChatTokens counts actual converted message content plus prompt fields omitted by the shared message counter.
func preparedClaudeChatTokens(ctx context.Context, request *ClaudeMessagesRequest, outgoing *relaymodel.GeneralOpenAIRequest) (int, error) {
	if outgoing == nil {
		return 0, errors.New("nil prepared Claude chat request")
	}
	model := claudeReservationModel(request)
	tokens := openai.CountTokenMessages(ctx, outgoing.Messages, model)
	for _, message := range outgoing.Messages {
		// Count only the remaining provider-visible fields after the shared counter handles content, role, and name.
		quote := message
		quote.Content = nil
		quote.Role = ""
		quote.Name = nil
		additional, err := claudePreparedJSONTokens(quote, model)
		if err != nil {
			return 0, err
		}
		tokens += additional
	}
	for _, value := range []any{outgoing.Tools, outgoing.Functions, outgoing.ResponseFormat} {
		additional, err := claudePreparedJSONTokens(value, model)
		if err != nil {
			return 0, err
		}
		tokens += additional
	}
	return tokens, nil
}

// claudePreparedJSONTokens counts nonempty provider-visible JSON prompt fields and returns serialization errors to admission.
func claudePreparedJSONTokens(value any, model string) (int, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, errors.Wrap(err, "marshal prepared Claude prompt fields")
	}
	switch string(encoded) {
	case "null", "[]", "{}":
		return 0, nil
	}
	return openai.CountTokenText(string(encoded), model), nil
}
