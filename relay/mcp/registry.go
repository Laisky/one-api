package mcp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/model"
)

// ToolPolicySnapshot captures policy and pricing decisions for a MCP tool.
type ToolPolicySnapshot struct {
	Allowed bool
	Pricing model.ToolPricingLocal
}

// ResolvedTool represents an MCP tool with policy applied.
type ResolvedTool struct {
	Tool        *model.MCPTool
	Policy      ToolPolicySnapshot
	ServerID    int
	ServerLabel string
	ServerURL   string
	DisplayName string
}

// ToolCandidate represents a resolved MCP tool with signature and priority metadata.
type ToolCandidate struct {
	ResolvedTool
	Signature      string
	ServerPriority int64
}

// ResolveTools applies layered policies (server/channel/user/allowed list) to MCP tools.
func ResolveTools(server *model.MCPServer, tools []*model.MCPTool, channelBlacklist []string, userBlacklist []string, allowedTools []string) ([]ResolvedTool, error) {
	if server == nil {
		return nil, errors.New("mcp server is nil")
	}

	allowList := normalizeToolSet(allowedTools)
	serverWhitelist := normalizeToolSet(server.ToolWhitelist)
	serverBlacklist := normalizeToolSet(server.ToolBlacklist)
	channelBlacklistSet := normalizeToolSet(channelBlacklist)
	userBlacklistSet := normalizeToolSet(userBlacklist)

	resolved := make([]ResolvedTool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := normalizeToolName(tool.Name)
		serverKey := normalizeToolName(server.Name + "." + tool.Name)
		if name == "" {
			continue
		}

		allowed := true
		if len(serverWhitelist) == 0 {
			allowed = false
		}
		if _, ok := serverWhitelist[name]; !ok {
			allowed = false
		}
		if _, ok := serverBlacklist[name]; ok {
			allowed = false
		}
		if toolInSet(channelBlacklistSet, name) || toolInSet(channelBlacklistSet, serverKey) {
			allowed = false
		}
		if toolInSet(userBlacklistSet, name) || toolInSet(userBlacklistSet, serverKey) {
			allowed = false
		}
		if len(allowList) > 0 {
			if !toolInSet(allowList, name) && !toolInSet(allowList, serverKey) {
				allowed = false
			}
		}

		pricing := server.ToolPricing[name]
		resolved = append(resolved, ResolvedTool{
			Tool:        tool,
			Policy:      ToolPolicySnapshot{Allowed: allowed, Pricing: pricing},
			ServerID:    server.Id,
			ServerLabel: server.Name,
			ServerURL:   server.BaseURL,
			DisplayName: tool.DisplayName,
		})
	}

	return resolved, nil
}

// BuildToolCandidates resolves MCP tools matching the name and signature across servers.
func BuildToolCandidates(servers []*model.MCPServer, toolsByServer map[int][]*model.MCPTool, channelBlacklist []string, userBlacklist []string, allowedTools []string, toolName string, signature string) ([]ToolCandidate, error) {
	normalizedName := normalizeToolName(toolName)
	if normalizedName == "" {
		return nil, errors.New("tool name is required")
	}

	normalizedSignature, err := normalizeSignature(signature)
	if err != nil {
		return nil, err
	}

	candidates := make([]ToolCandidate, 0)
	for _, server := range servers {
		if server == nil {
			continue
		}
		tools := toolsByServer[server.Id]
		if len(tools) == 0 {
			continue
		}
		resolved, err := ResolveTools(server, tools, channelBlacklist, userBlacklist, allowedTools)
		if err != nil {
			return nil, err
		}
		for _, entry := range resolved {
			if !entry.Policy.Allowed || entry.Tool == nil {
				continue
			}
			if normalizeToolName(entry.Tool.Name) != normalizedName {
				continue
			}
			toolSignature, err := SignatureFromJSON(entry.Tool.InputSchema)
			if err != nil {
				return nil, errors.Wrapf(err, "compute signature for %s", entry.Tool.Name)
			}
			if normalizedSignature != "" && toolSignature != normalizedSignature {
				continue
			}
			candidates = append(candidates, ToolCandidate{
				ResolvedTool:   entry,
				Signature:      toolSignature,
				ServerPriority: server.GetPriority(),
			})
		}
	}

	if signature == "" {
		hasSignature := false
		for _, candidate := range candidates {
			if candidate.Signature != "" {
				hasSignature = true
				break
			}
		}
		if hasSignature {
			filtered := make([]ToolCandidate, 0, len(candidates))
			for _, candidate := range candidates {
				if candidate.Signature != "" {
					filtered = append(filtered, candidate)
				}
			}
			candidates = filtered
		}
	}

	if err := enforceSignatureDisambiguation(candidates, normalizedSignature); err != nil {
		return nil, err
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ServerPriority == candidates[j].ServerPriority {
			return candidates[i].ServerID < candidates[j].ServerID
		}
		return candidates[i].ServerPriority > candidates[j].ServerPriority
	})

	return candidates, nil
}

// SignatureFromSchema canonicalizes the provided schema into a stable signature string.
func SignatureFromSchema(schema any) (string, error) {
	if schema == nil {
		return "", nil
	}
	return CanonicalizeJSON(schema)
}

// SignatureFromJSON canonicalizes a JSON schema string into a stable signature string.
func SignatureFromJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return "", errors.Wrap(err, "parse tool signature json")
	}
	if parsed == nil {
		return "", nil
	}
	return SignatureFromSchema(parsed)
}

// CanonicalizeJSON renders JSON values with stable key ordering for signature comparison.
func CanonicalizeJSON(value any) (string, error) {
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, value); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// writeCanonicalJSON writes a JSON value with deterministic key ordering.
func writeCanonicalJSON(buf *bytes.Buffer, value any) error {
	if buf == nil {
		return errors.New("json buffer is nil")
	}
	if value == nil {
		buf.WriteString("null")
		return nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return errors.Wrap(err, "parse raw json")
		}
		return writeCanonicalJSON(buf, parsed)
	}

	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Map:
		if val.Type().Key().Kind() != reflect.String {
			encoded, err := json.Marshal(value)
			if err != nil {
				return errors.Wrap(err, "marshal non-string map")
			}
			buf.Write(encoded)
			return nil
		}
		keys := val.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].String() < keys[j].String()
		})
		buf.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			encodedKey, err := json.Marshal(key.String())
			if err != nil {
				return errors.Wrap(err, "marshal json key")
			}
			buf.Write(encodedKey)
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, val.MapIndex(key).Interface()); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case reflect.Slice, reflect.Array:
		buf.WriteByte('[')
		for i := 0; i < val.Len(); i++ {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, val.Index(i).Interface()); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return errors.Wrap(err, "marshal json value")
		}
		buf.Write(encoded)
		return nil
	}
}

// normalizeSignature normalizes a signature string into a canonical form.
func normalizeSignature(signature string) (string, error) {
	trimmed := strings.TrimSpace(signature)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return SignatureFromJSON(trimmed)
	}
	return trimmed, nil
}

// enforceSignatureDisambiguation validates signature requirements when multiple tools share a name.
func enforceSignatureDisambiguation(candidates []ToolCandidate, signature string) error {
	if signature != "" {
		return nil
	}

	unique := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.Signature == "" {
			continue
		}
		unique[candidate.Signature] = struct{}{}
	}
	if len(unique) > 1 {
		return errors.New("multiple MCP tools share the same name; provide a server label or parameter signature")
	}
	return nil
}

// normalizeToolSet builds a canonical set of tool names.
func normalizeToolSet(list []string) map[string]struct{} {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(list))
	for _, raw := range list {
		name := normalizeToolName(raw)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	return set
}

// normalizeToolName standardizes a tool name for policy comparisons.
func normalizeToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// toolInSet reports whether a normalized tool name exists in a set.
func toolInSet(set map[string]struct{}, name string) bool {
	if len(set) == 0 {
		return false
	}
	if name == "" {
		return false
	}
	_, ok := set[name]
	return ok
}
