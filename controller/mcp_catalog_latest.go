package controller

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

const modernMCPToolsPageSize = 128

type modernMCPToolsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

type modernMCPToolsCursor struct {
	Offset      int    `json:"offset"`
	Fingerprint string `json:"fingerprint"`
}

// listModernMCPToolsPage returns one deterministic page from the authenticated aggregate tool catalog.
//
// Parameters:
//   - ctx: the request context controlling database and policy work.
//   - c: the Gin context containing the authenticated user.
//   - rawParams: the encoded tools/list parameters containing an optional opaque cursor.
//
// Return values:
//   - gin.H: a current MCP tools/list result with caching and optional nextCursor fields.
//   - error: a wrapped authentication, database, descriptor, cursor, or encoding error.
func listModernMCPToolsPage(ctx context.Context, c *gin.Context, rawParams json.RawMessage) (gin.H, error) {
	var params modernMCPToolsListParams
	if len(rawParams) != 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, &modernMCPValidationError{Status: 400, Code: mcpErrInvalidParams, Err: errors.Wrap(err, "decode mcp tools/list params")}
		}
	}
	if len(params.Cursor) > 2048 {
		return nil, &modernMCPValidationError{Status: 400, Code: mcpErrInvalidParams, Err: errors.New("mcp tools/list cursor is too large")}
	}

	tools, err := listMCPToolDescriptorsForUser(ctx, c)
	if err != nil {
		return nil, err
	}
	fingerprint, err := fingerprintMCPToolDescriptors(tools)
	if err != nil {
		return nil, errors.Wrap(err, "fingerprint mcp tool catalog")
	}
	offset := 0
	if params.Cursor != "" {
		cursor, err := decodeModernMCPToolsCursor(params.Cursor)
		if err != nil {
			return nil, &modernMCPValidationError{Status: 400, Code: mcpErrInvalidParams, Err: err}
		}
		if cursor.Fingerprint != fingerprint {
			return nil, &modernMCPValidationError{Status: 400, Code: mcpErrInvalidParams, Err: errors.New("mcp tools/list cursor is stale")}
		}
		if cursor.Offset < 0 || cursor.Offset > len(tools) {
			return nil, &modernMCPValidationError{Status: 400, Code: mcpErrInvalidParams, Err: errors.New("mcp tools/list cursor offset is invalid")}
		}
		offset = cursor.Offset
	}

	end := offset + modernMCPToolsPageSize
	if end > len(tools) {
		end = len(tools)
	}
	page := append([]mcp.ToolDescriptor(nil), tools[offset:end]...)
	result := gin.H{
		"resultType": mcp.ResultTypeComplete,
		"tools":      page,
		"ttlMs":      int64(60000),
		"cacheScope": mcp.CacheScopePrivate,
	}
	if end < len(tools) {
		nextCursor, err := encodeModernMCPToolsCursor(modernMCPToolsCursor{Offset: end, Fingerprint: fingerprint})
		if err != nil {
			return nil, errors.Wrap(err, "encode next mcp tools/list cursor")
		}
		result["nextCursor"] = nextCursor
	}
	return result, nil
}

// listMCPToolDescriptorsForUser builds the lossless policy-filtered aggregate catalog for one user.
//
// Parameters:
//   - ctx: the request context controlling database and policy work.
//   - c: the Gin context containing the authenticated user.
//
// Return values:
//   - []mcp.ToolDescriptor: qualified descriptors sorted deterministically by exact wire name.
//   - error: a wrapped authentication, database, policy, or stored-descriptor error.
func listMCPToolDescriptorsForUser(ctx context.Context, c *gin.Context) ([]mcp.ToolDescriptor, error) {
	_ = ctx
	user, err := getUserFromContext(c)
	if err != nil {
		return nil, errors.Wrap(err, "get user from context")
	}
	servers, err := model.ListEnabledMCPServers()
	if err != nil {
		return nil, errors.Wrap(err, "list enabled mcp servers")
	}
	sort.SliceStable(servers, func(left, right int) bool {
		if servers[left].GetPriority() == servers[right].GetPriority() {
			return servers[left].Id < servers[right].Id
		}
		return servers[left].GetPriority() > servers[right].GetPriority()
	})

	descriptors := make([]mcp.ToolDescriptor, 0)
	for _, server := range servers {
		if server == nil {
			continue
		}
		tools, err := model.GetMCPToolsByServerID(server.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "get mcp tools for server %d", server.Id)
		}
		resolved, err := mcp.ResolveTools(server, tools, nil, user.MCPToolBlacklist, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "resolve mcp tools for server %d", server.Id)
		}
		for _, entry := range resolved {
			if !entry.Policy.Allowed || entry.Tool == nil {
				continue
			}
			descriptor, err := descriptorForMCPTool(entry.Tool)
			if err != nil {
				return nil, errors.Wrapf(err, "restore mcp descriptor for server %d tool %q", server.Id, entry.Tool.Name)
			}
			descriptor.Name = server.Name + "." + descriptor.Name
			descriptors = append(descriptors, descriptor)
		}
	}
	sort.SliceStable(descriptors, func(left, right int) bool {
		return descriptors[left].Name < descriptors[right].Name
	})
	return descriptors, nil
}

// descriptorForMCPTool restores the complete wire descriptor with compatibility fallbacks for old rows.
//
// Parameters:
//   - tool: the synchronized database row.
//
// Return values:
//   - mcp.ToolDescriptor: the lossless descriptor with exact wire name and a non-nil input schema.
//   - error: a wrapped stored JSON or legacy input-schema decoding error.
func descriptorForMCPTool(tool *model.MCPTool) (mcp.ToolDescriptor, error) {
	if tool == nil {
		return mcp.ToolDescriptor{}, errors.New("mcp tool is nil")
	}
	descriptor := mcp.ToolDescriptor{}
	if strings.TrimSpace(tool.DescriptorJSON) != "" {
		if err := json.Unmarshal([]byte(tool.DescriptorJSON), &descriptor); err != nil {
			return mcp.ToolDescriptor{}, errors.Wrap(err, "decode stored mcp descriptor")
		}
	}
	descriptor.Name = tool.Name
	if descriptor.Title == "" {
		descriptor.Title = tool.DisplayName
	}
	if descriptor.Description == "" {
		descriptor.Description = tool.Description
	}
	if descriptor.InputSchema == nil && strings.TrimSpace(tool.InputSchema) != "" {
		if err := json.Unmarshal([]byte(tool.InputSchema), &descriptor.InputSchema); err != nil {
			return mcp.ToolDescriptor{}, errors.Wrap(err, "decode stored mcp input schema")
		}
	}
	if descriptor.InputSchema == nil {
		descriptor.InputSchema = map[string]any{"type": "object"}
	}
	return descriptor, nil
}

// findModernMCPToolDescriptor resolves one exact qualified tool name for header validation.
//
// Parameters:
//   - ctx: the request context controlling database and policy work.
//   - c: the Gin context containing the authenticated user.
//   - name: the exact qualified wire name returned by tools/list.
//
// Return values:
//   - mcp.ToolDescriptor: the matching descriptor.
//   - error: a wrapped catalog error or an unknown-tool error.
func findModernMCPToolDescriptor(ctx context.Context, c *gin.Context, name string) (mcp.ToolDescriptor, error) {
	tools, err := listMCPToolDescriptorsForUser(ctx, c)
	if err != nil {
		return mcp.ToolDescriptor{}, errors.Wrap(err, "list mcp tools for header validation")
	}
	for _, tool := range tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return mcp.ToolDescriptor{}, errors.Errorf("no eligible MCP tool found for %q", name)
}

// fingerprintMCPToolDescriptors returns a stable digest of one deterministic catalog.
//
// Parameters:
//   - tools: the sorted aggregate descriptors.
//
// Return values:
//   - string: a lowercase hexadecimal SHA-256 digest.
//   - error: a wrapped encoding error.
func fingerprintMCPToolDescriptors(tools []mcp.ToolDescriptor) (string, error) {
	encoded, err := json.Marshal(tools)
	if err != nil {
		return "", errors.Wrap(err, "marshal mcp tool catalog")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// encodeModernMCPToolsCursor encodes one cursor as opaque URL-safe JSON.
//
// Parameters:
//   - cursor: the validated offset and catalog fingerprint.
//
// Return values:
//   - string: the URL-safe opaque cursor.
//   - error: a wrapped JSON encoding error.
func encodeModernMCPToolsCursor(cursor modernMCPToolsCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", errors.Wrap(err, "marshal mcp tools/list cursor")
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

// decodeModernMCPToolsCursor decodes one opaque cursor and validates required fields.
//
// Parameters:
//   - value: the URL-safe cursor supplied by a client.
//
// Return values:
//   - modernMCPToolsCursor: the decoded cursor.
//   - error: a wrapped Base64, JSON, or required-field error.
func decodeModernMCPToolsCursor(value string) (modernMCPToolsCursor, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return modernMCPToolsCursor{}, errors.Wrap(err, "decode mcp tools/list cursor")
	}
	var cursor modernMCPToolsCursor
	if err := json.Unmarshal(encoded, &cursor); err != nil {
		return modernMCPToolsCursor{}, errors.Wrap(err, "unmarshal mcp tools/list cursor")
	}
	if cursor.Fingerprint == "" {
		return modernMCPToolsCursor{}, errors.New("mcp tools/list cursor fingerprint is required")
	}
	return cursor, nil
}
