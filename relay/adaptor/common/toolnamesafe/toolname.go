// Package toolnamesafe sanitizes tool/function identifiers so chat completion
// payloads pass strict provider validators that require `^[a-zA-Z0-9_-]+$`
// (OpenAI, DeepSeek, Anthropic). The package keeps a per-request reverse map
// on the gin context so response handlers can restore the original
// client-facing names before they reach the caller, preserving round-trip
// transparency for MCP-namespaced tools (e.g. `server.tool`, `a/b`).
package toolnamesafe

import (
	"encoding/hex"
	"hash/fnv"
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// MaxToolNameLen is the upper bound enforced by DeepSeek (and OpenAI-compatible
// validators) on `tools[].function.name` and related identifier fields.
const MaxToolNameLen = 64

// disallowedToolNameChar matches any character outside DeepSeek's accepted
// `^[a-zA-Z0-9_-]+$` pattern.
var disallowedToolNameChar = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// SanitizeToolName rewrites name into a DeepSeek-compatible identifier by
// replacing every disallowed character with `_` and clamping the result to
// MaxToolNameLen bytes. It returns the sanitized value and whether it differs
// from the input. An empty input is returned unchanged.
func SanitizeToolName(name string) (sanitized string, changed bool) {
	if name == "" {
		return name, false
	}
	cleaned := disallowedToolNameChar.ReplaceAllString(name, "_")
	if len(cleaned) > MaxToolNameLen {
		cleaned = cleaned[:MaxToolNameLen]
	}
	return cleaned, cleaned != name
}

// collisionSuffix derives a stable 8-character hexadecimal suffix from the
// original name, used to break ties when two different originals collapse to
// the same sanitized base.
func collisionSuffix(original string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(original))
	var buf [4]byte
	sum := h.Sum32()
	buf[0] = byte(sum >> 24)
	buf[1] = byte(sum >> 16)
	buf[2] = byte(sum >> 8)
	buf[3] = byte(sum)
	return hex.EncodeToString(buf[:])
}

// applyCollisionSuffix appends a deterministic suffix derived from original so
// the final identifier remains unique even after multiple collisions, while
// staying within MaxToolNameLen bytes.
func applyCollisionSuffix(base, original string) string {
	suffix := "_" + collisionSuffix(original)
	maxBase := MaxToolNameLen - len(suffix)
	if maxBase < 0 {
		maxBase = 0
	}
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

// SanitizeRequestToolNames rewrites tool/function identifiers across a chat
// completion request so the payload passes DeepSeek's regex validator. The
// reverse map (sanitized → original) is stashed on c under
// ctxkey.ToolNameSanitizeMap whenever at least one name is rewritten, so
// downstream response handlers can restore originals before forwarding to the
// client. It returns the number of rewritten identifiers.
//
// The function mutates the following locations in-place when needed:
//   - request.Tools[i].Function.Name
//   - request.ToolChoice (object form: {"type":"function","function":{"name":...}})
//   - request.Messages[i].ToolCalls[j].Function.Name
//   - request.Messages[i].Name (legacy `function`-role name field)
//
// SanitizeRequestToolNames is safe to call multiple times; it merges into any
// pre-existing map stored on c.
func SanitizeRequestToolNames(c *gin.Context, request *model.GeneralOpenAIRequest) int {
	if request == nil {
		return 0
	}

	revMap, _ := getOrInitToolNameMap(c)
	rewrites := 0

	register := func(original string) string {
		if original == "" {
			return original
		}
		sanitized, changed := SanitizeToolName(original)
		if !changed {
			return original
		}
		if existing, ok := revMap[sanitized]; ok && existing != original {
			sanitized = applyCollisionSuffix(sanitized, original)
		}
		revMap[sanitized] = original
		rewrites++
		return sanitized
	}

	// tools[].function.name
	for i := range request.Tools {
		fn := request.Tools[i].Function
		if fn == nil {
			continue
		}
		fn.Name = register(fn.Name)
	}

	// tool_choice when expressed as an object with a target function name.
	if mp, ok := request.ToolChoice.(map[string]any); ok {
		if fnAny, ok := mp["function"]; ok {
			if fnMap, ok := fnAny.(map[string]any); ok {
				if name, ok := fnMap["name"].(string); ok && name != "" {
					fnMap["name"] = register(name)
				}
			}
		}
	}

	// messages[].tool_calls[].function.name and legacy messages[].name on role=function/tool.
	for i := range request.Messages {
		msg := &request.Messages[i]
		for j := range msg.ToolCalls {
			fn := msg.ToolCalls[j].Function
			if fn == nil {
				continue
			}
			fn.Name = register(fn.Name)
		}
		if msg.Name != nil && *msg.Name != "" {
			renamed := register(*msg.Name)
			msg.Name = &renamed
		}
	}

	if rewrites > 0 {
		c.Set(ctxkey.ToolNameSanitizeMap, revMap)
	}
	return rewrites
}

// RestoreToolCallNames walks a slice of tool-call entries (as found on
// `choices[].message.tool_calls` or streamed deltas) and restores any
// previously-sanitized function names from the per-request map. It returns
// true when at least one identifier was rewritten back to its original form,
// allowing callers that emit raw upstream JSON to detect when they must
// re-marshal a chunk before forwarding.
func RestoreToolCallNames(c *gin.Context, toolCalls []model.Tool) bool {
	if len(toolCalls) == 0 {
		return false
	}
	mp, ok := lookupToolNameMap(c)
	if !ok {
		return false
	}
	changed := false
	for i := range toolCalls {
		fn := toolCalls[i].Function
		if fn == nil || fn.Name == "" {
			continue
		}
		if orig, found := mp[fn.Name]; found && orig != fn.Name {
			fn.Name = orig
			changed = true
		}
	}
	return changed
}

// RestoreToolName looks up the original client-provided name for a sanitized
// identifier. It returns the input unchanged when no map exists or no entry
// matches.
func RestoreToolName(c *gin.Context, sanitized string) string {
	if sanitized == "" {
		return sanitized
	}
	mp, ok := lookupToolNameMap(c)
	if !ok {
		return sanitized
	}
	if orig, found := mp[sanitized]; found {
		return orig
	}
	return sanitized
}

// getOrInitToolNameMap fetches the per-request rename table from c, creating
// it on demand. The second return value reports whether a pre-existing map was
// found.
func getOrInitToolNameMap(c *gin.Context) (map[string]string, bool) {
	if c == nil {
		return map[string]string{}, false
	}
	if raw, exists := c.Get(ctxkey.ToolNameSanitizeMap); exists {
		if mp, ok := raw.(map[string]string); ok {
			return mp, true
		}
	}
	return map[string]string{}, false
}

// lookupToolNameMap returns the rename table when one has been published on c.
func lookupToolNameMap(c *gin.Context) (map[string]string, bool) {
	if c == nil {
		return nil, false
	}
	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	if !exists {
		return nil, false
	}
	mp, ok := raw.(map[string]string)
	if !ok || len(mp) == 0 {
		return nil, false
	}
	return mp, true
}
