package anthropic

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// maxWebSearchQueryInputBytes bounds the streamed server_tool_use input that is
// buffered to recover a search query from an untrusted upstream.
const maxWebSearchQueryInputBytes = 64 << 10

// claudeServerTextSeparator joins assistant text written before a server-tool
// round with the text Claude writes after reading its results, so the
// converted message does not glue two sentences together.
const claudeServerTextSeparator = "\n\n"

// isClaudeServerSideBlock reports whether a content block kind is a
// provider-executed tool invocation or result, which converted Chat output
// must never expose as a caller tool call. Parameters: kind is the block type.
// Returns: true for server_tool_use, mcp_tool_use and every *_tool_result kind.
func isClaudeServerSideBlock(kind string) bool {
	switch kind {
	case "server_tool_use", "mcp_tool_use":
		return true
	}
	return strings.HasSuffix(kind, "_tool_result")
}

// webSearchQueryFromInput extracts the query from a server_tool_use input.
// Parameters: raw is the input JSON. Returns: the query, or "" when absent.
func webSearchQueryFromInput(raw []byte) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return ""
	}
	return input.Query
}

// webSearchResultStatus classifies a web_search_tool_result content value: a
// result list is completed, an error object (or anything else) is failed.
// Parameters: raw is the content JSON. Returns: a model.WebSearchCall* status.
func webSearchResultStatus(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return model.WebSearchCallCompleted
	}
	return model.WebSearchCallFailed
}

// webSearchCallCollector pairs complete server_tool_use web_search blocks with
// their web_search_tool_result blocks in one non-streaming Claude message.
type webSearchCallCollector struct {
	calls []model.WebSearchCall
	index map[string]int
}

// observe records one content block. Parameters: block is the decoded block.
// Returns: true when the block is a server-side block of any kind.
func (w *webSearchCallCollector) observe(block Content) bool {
	if !isClaudeServerSideBlock(block.Type) {
		return false
	}
	switch {
	case block.Type == "server_tool_use" && block.Name == webSearchToolName:
		query := ""
		if input, err := json.Marshal(block.Input); err == nil {
			query = webSearchQueryFromInput(input)
		}
		if w.index == nil {
			w.index = make(map[string]int)
		}
		w.index[block.Id] = len(w.calls)
		// A search with no result block before the message ended never
		// returned results to the model; the result block upgrades it.
		w.calls = append(w.calls, model.WebSearchCall{ID: block.Id, Query: query, Status: model.WebSearchCallFailed})
	case block.Type == "web_search_tool_result":
		if position, ok := w.index[block.ToolUseId]; ok {
			w.calls[position].Status = webSearchResultStatus(block.rawFields["content"])
		}
	}
	return true
}

// streamServerBlock carries the streamed state of one provider-side block.
type streamServerBlock struct {
	searchID    string
	searchInput []byte
	overflow    bool
}

// consumeServerBlock handles a converted stream event for a provider-side
// block: web searches become Chat chunks carrying model.WebSearchCall entries,
// and no server block ever reaches the caller as text or a tool call.
// Parameters: c is the request context, eventType the Claude event, raw its
// JSON and block the registered block. Returns: a decode or delivery error.
func (s *httpStreamState) consumeServerBlock(c *gin.Context, eventType string, raw []byte, block *httpContentBlock) error {
	if block.server == nil {
		block.server = &streamServerBlock{}
	}
	server := block.server
	switch eventType {
	case "content_block_start":
		var start struct {
			Block struct {
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Input     json.RawMessage `json:"input"`
				ToolUseID string          `json:"tool_use_id"`
				Content   json.RawMessage `json:"content"`
			} `json:"content_block"`
		}
		if err := decodeClaudeJSON(raw, &start); err != nil {
			return errors.Wrap(err, "decode Claude server block")
		}
		switch {
		case block.kind == "server_tool_use" && start.Block.Name == webSearchToolName:
			server.searchID = start.Block.ID
			if trimmed := bytes.TrimSpace(start.Block.Input); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("{}")) {
				server.appendInput(trimmed)
			}
		case block.kind == "web_search_tool_result":
			call := model.WebSearchCall{ID: start.Block.ToolUseID, Status: webSearchResultStatus(start.Block.Content)}
			call.Query = s.searchQueries[call.ID]
			delete(s.searchQueries, call.ID)
			return s.writeWebSearchCall(c, call)
		}
	case "content_block_delta":
		if server.searchID == "" {
			return nil
		}
		var delta struct {
			Delta struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := decodeClaudeJSON(raw, &delta); err != nil {
			return errors.Wrap(err, "decode Claude server block delta")
		}
		if delta.Delta.Type == "input_json_delta" {
			server.appendInput([]byte(delta.Delta.PartialJSON))
		}
	case "content_block_stop":
		s.separatorPending = s.textEmitted
		if server.searchID == "" {
			return nil
		}
		query := ""
		if !server.overflow {
			query = webSearchQueryFromInput(server.searchInput)
		}
		if s.searchQueries == nil {
			s.searchQueries = make(map[string]string)
		}
		s.searchQueries[server.searchID] = query
		return s.writeWebSearchCall(c, model.WebSearchCall{ID: server.searchID, Query: query, Status: model.WebSearchCallInProgress})
	}
	return nil
}

// appendInput buffers streamed tool input up to maxWebSearchQueryInputBytes;
// larger input is discarded and the query is reported as unknown.
func (b *streamServerBlock) appendInput(part []byte) {
	if b.overflow {
		return
	}
	if len(b.searchInput)+len(part) > maxWebSearchQueryInputBytes {
		b.overflow, b.searchInput = true, nil
		return
	}
	b.searchInput = append(b.searchInput, part...)
}

// writeWebSearchCall emits one converted chunk describing a web search.
// Parameters: c is the request context and call the search. Returns: a
// delivery error from the Chat-to-Responses bridge.
func (s *httpStreamState) writeWebSearchCall(c *gin.Context, call model.WebSearchCall) error {
	chunk := &openai.ChatCompletionsStreamResponse{Choices: []openai.ChatCompletionsStreamResponseChoice{{}}}
	chunk.Choices[0].Delta.Role = "assistant"
	chunk.Choices[0].Delta.WebSearchCalls = []model.WebSearchCall{call}
	return s.writeChat(c, chunk)
}

// separateServerText prefixes the first text written after a server-tool
// round with claudeServerTextSeparator. Parameters: chunk is a converted text
// chunk. Returns: nothing; the chunk is updated in place.
func (s *httpStreamState) separateServerText(chunk *openai.ChatCompletionsStreamResponse) {
	if chunk == nil || len(chunk.Choices) == 0 {
		return
	}
	text, ok := chunk.Choices[0].Delta.Content.(string)
	if !ok || text == "" {
		return
	}
	if s.separatorPending {
		chunk.Choices[0].Delta.Content = claudeServerTextSeparator + text
		s.separatorPending = false
	}
	s.textEmitted = true
}
