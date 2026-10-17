package controller

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// maxClaudeMCPLoopBody bounds native reconstruction before retaining raw snapshots.
const maxClaudeMCPLoopBody = 32 << 20

// claudeNativeLoopWire retains one request's original wire and immutable typed view.
type claudeNativeLoopWire struct {
	root     map[string]json.RawMessage
	typed    map[string]json.RawMessage
	original ClaudeMessagesRequest
}

// newClaudeNativeLoopWire snapshots the existing request without reading private configuration.
// Parameters: c carries the caller body. Returns: a request-local bounded snapshot or an error.
func newClaudeNativeLoopWire(c *gin.Context) (*claudeNativeLoopWire, error) {
	raw, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "read native Claude MCP body")
	}
	if len(raw) > maxClaudeMCPLoopBody {
		return nil, errors.New("native Claude MCP body exceeds 32 MiB limit")
	}
	wire := &claudeNativeLoopWire{}
	if err := json.Unmarshal(raw, &wire.root); err != nil {
		return nil, errors.Wrap(err, "decode native Claude MCP body")
	}
	if wire.root == nil {
		return nil, errors.New("native Claude MCP body must be an object")
	}
	if err := json.Unmarshal(raw, &wire.original); err != nil {
		return nil, errors.Wrap(err, "decode native Claude MCP snapshot")
	}
	typed, err := json.Marshal(&wire.original)
	if err != nil {
		return nil, errors.Wrap(err, "encode native Claude MCP snapshot")
	}
	if err := json.Unmarshal(typed, &wire.typed); err != nil {
		return nil, errors.Wrap(err, "retain native Claude MCP snapshot")
	}
	return wire, nil
}

// prepare retains unchanged root/tool/message fields while applying current typed changes.
// Parameters: request is the current loop turn. Returns: the exact bounded native bytes to quote/send.
func (w *claudeNativeLoopWire) prepare(request *ClaudeMessagesRequest) ([]byte, error) {
	currentBytes, err := json.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "encode native Claude MCP turn")
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal(currentBytes, &current); err != nil {
		return nil, errors.Wrap(err, "decode native Claude MCP turn")
	}
	result := mergeClaudeLoopFields(w.root, w.typed, current)
	if !bytes.Equal(w.typed["messages"], current["messages"]) {
		messages, err := mergeClaudeLoopMessages(w.root["messages"], w.original.Messages, request.Messages)
		if err != nil {
			return nil, err
		}
		result["messages"] = messages
	}
	if !bytes.Equal(w.typed["tools"], current["tools"]) && len(request.Tools) > 0 {
		tools, err := mergeClaudeLoopTools(w.root["tools"], w.original.Tools, request.Tools)
		if err != nil {
			return nil, err
		}
		result["tools"] = tools
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, errors.Wrap(err, "encode lossless native Claude MCP turn")
	}
	// Keep the existing mapped-model, sampling, thinking and extra_body policy.
	raw, _, err = rewriteAndSanitizeClaudeRequestBody(raw, request)
	if err != nil {
		return nil, errors.Wrap(err, "sanitize native Claude MCP turn")
	}
	if len(raw) > maxClaudeMCPLoopBody {
		return nil, errors.New("native Claude MCP turn exceeds 32 MiB limit")
	}
	return raw, nil
}

// mergeClaudeLoopFields overlays only changed typed fields and retains unrelated raw metadata.
func mergeClaudeLoopFields(raw, previous, current map[string]json.RawMessage) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(raw)+len(current))
	for key, value := range raw {
		result[key] = value
	}
	keys := make(map[string]struct{}, len(previous)+len(current))
	for key := range previous {
		keys[key] = struct{}{}
	}
	for key := range current {
		keys[key] = struct{}{}
	}
	for key := range keys {
		before, beforeOK := previous[key]
		after, afterOK := current[key]
		if beforeOK == afterOK && bytes.Equal(before, after) {
			continue
		}
		if afterOK {
			result[key] = after
		} else {
			delete(result, key)
		}
	}
	return result
}

// mergeClaudeLoopMessages preserves unchanged original messages and appends lossless native blocks.
func mergeClaudeLoopMessages(raw json.RawMessage, previous, current []relaymodel.ClaudeMessage) ([]byte, error) {
	var original []json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &original); err != nil {
			return nil, errors.Wrap(err, "decode original Claude MCP messages")
		}
	}
	result := make([]json.RawMessage, 0, len(current))
	for index, message := range current {
		encoded, err := json.Marshal(message)
		if err != nil {
			return nil, errors.Wrap(err, "encode Claude MCP message")
		}
		if index < len(previous) && index < len(original) {
			before, err := json.Marshal(previous[index])
			if err != nil {
				return nil, errors.Wrap(err, "encode previous Claude MCP message")
			}
			if bytes.Equal(before, encoded) {
				encoded = original[index]
			} else {
				encoded, err = mergeClaudeLoopTypedObject(original[index], before, encoded)
				if err != nil {
					return nil, err
				}
			}
		}
		result = append(result, encoded)
	}
	encoded, err := json.Marshal(result)
	return encoded, errors.Wrap(err, "encode native Claude MCP messages")
}

// mergeClaudeLoopTools retains tool extensions/cache markers while honoring typed edits and injection.
func mergeClaudeLoopTools(raw json.RawMessage, previous, current []relaymodel.ClaudeTool) ([]byte, error) {
	var original []json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &original); err != nil {
			return nil, errors.Wrap(err, "decode original Claude MCP tools")
		}
	}
	used := make([]bool, len(previous))
	result := make([]json.RawMessage, 0, len(current))
	for _, tool := range current {
		encoded, err := json.Marshal(tool)
		if err != nil {
			return nil, errors.Wrap(err, "encode Claude MCP tool")
		}
		for index, beforeTool := range previous {
			if used[index] || beforeTool.Name != tool.Name || beforeTool.Type != tool.Type {
				continue
			}
			used[index] = true
			if index < len(original) {
				before, err := json.Marshal(beforeTool)
				if err != nil {
					return nil, errors.Wrap(err, "encode previous Claude MCP tool")
				}
				if bytes.Equal(before, encoded) {
					encoded = original[index]
				} else {
					encoded, err = mergeClaudeLoopTypedObject(original[index], before, encoded)
					if err != nil {
						return nil, err
					}
				}
			}
			break
		}
		result = append(result, encoded)
	}
	encoded, err := json.Marshal(result)
	return encoded, errors.Wrap(err, "encode native Claude MCP tools")
}

// mergeClaudeLoopTypedObject preserves unknown fields of one changed message/tool.
func mergeClaudeLoopTypedObject(raw, before, after []byte) ([]byte, error) {
	var original, previous, current map[string]json.RawMessage
	for _, part := range []struct {
		data   []byte
		target *map[string]json.RawMessage
	}{
		{raw, &original}, {before, &previous}, {after, &current},
	} {
		if err := json.Unmarshal(part.data, part.target); err != nil {
			return nil, errors.Wrap(err, "decode Claude MCP field snapshot")
		}
	}
	encoded, err := json.Marshal(mergeClaudeLoopFields(original, previous, current))
	return encoded, errors.Wrap(err, "encode changed Claude MCP object")
}
