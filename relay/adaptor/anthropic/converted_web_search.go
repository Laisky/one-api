package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

const (
	// WebSearchToolTypeBasic is Claude's basic server-side web search tool. It is
	// accepted by every platform that offers Claude web search, including
	// Google-managed Vertex AI and Azure-hosted Microsoft Foundry deployments.
	WebSearchToolTypeBasic = "web_search_20250305"
	// WebSearchToolTypeDynamic adds dynamic filtering (search results filtered by
	// provider-run code before they reach the context window). Anthropic documents
	// it for Claude 4.6 and later models on the first-party Claude API.
	WebSearchToolTypeDynamic = "web_search_20260209"
	// webSearchToolName is the fixed tool name every web search version requires.
	webSearchToolName = "web_search"
)

// maxConvertedWebSearchDomains bounds the domain filter list copied from an
// untrusted Responses declaration (the OpenAI web search limit is 100 domains).
const maxConvertedWebSearchDomains = 100

// maxConvertedWebSearchFieldBytes bounds each copied domain or location field.
const maxConvertedWebSearchFieldBytes = 256

// ErrConvertedWebSearchNotRepresentable identifies a Responses web search
// declaration whose options have no faithful Claude server-tool equivalent.
var ErrConvertedWebSearchNotRepresentable = errors.New("web search options cannot be represented on Claude")

// webSearchDynamicFilteringPrefixes lists the Claude model families Anthropic
// documents for web_search_20260209 dynamic filtering ("Claude 4.6 and later
// models and Claude Mythos Preview"). Earlier models need the basic version:
// without programmatic tool calling the dynamic version is rejected with 400.
// Source (verified 2026-11-05):
// https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool
var webSearchDynamicFilteringPrefixes = []string{
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-opus-4-7",
	"claude-opus-4-8",
	"claude-opus-5",
	"claude-sonnet-5",
	"claude-haiku-5-5",
	"claude-fable-5",
	"claude-mythos-5",
}

// MarshalJSON encodes caller-defined tools exactly as before and encodes
// server tools (non-empty Type) without the input_schema that Anthropic rejects
// on provider-executed tools. Parameters: t is the tool. Returns: its JSON or a
// wrapped encoding error.
func (t Tool) MarshalJSON() ([]byte, error) {
	if strings.TrimSpace(t.Type) == "" {
		type customTool Tool
		data, err := json.Marshal(customTool(t))
		return data, errors.Wrap(err, "encode Claude custom tool")
	}
	wire := struct {
		Type           string                 `json:"type"`
		Name           string                 `json:"name"`
		MaxUses        *int                   `json:"max_uses,omitempty"`
		AllowedDomains []string               `json:"allowed_domains,omitempty"`
		BlockedDomains []string               `json:"blocked_domains,omitempty"`
		UserLocation   *WebSearchUserLocation `json:"user_location,omitempty"`
	}{t.Type, t.Name, t.MaxUses, t.AllowedDomains, t.BlockedDomains, t.UserLocation}
	data, err := json.Marshal(wire)
	return data, errors.Wrap(err, "encode Claude server tool")
}

// ConvertedWebSearchToolType returns the Claude web search tool version a
// converted (Chat-shaped) request may declare on the platform described by
// info, or "" when that platform cannot run Claude server-side web search or
// the gateway cannot meter it from the converted response. Parameters: info is
// the request metadata with the mapped model. Returns: a WebSearchToolType*
// constant or "".
//
// First-party Anthropic channels use dynamic filtering where the model supports
// it. Azure Foundry and Vertex AI use the basic version, the only one available
// on Azure-hosted and Google-managed deployments. Bedrock (AwsClaude) offers no
// Claude web search and its receipt parser has no server-tool counters, and
// ClaudeCompatible upstreams have no documented server-tool contract, so both
// keep the historical drop.
func ConvertedWebSearchToolType(info *meta.Meta) string {
	if info == nil {
		return ""
	}
	switch info.ChannelType {
	case channeltype.Anthropic:
		if SupportsWebSearchDynamicFiltering(info.ActualModelName) {
			return WebSearchToolTypeDynamic
		}
		return WebSearchToolTypeBasic
	case channeltype.Azure:
		if info.AzureTargetsAnthropic() {
			return WebSearchToolTypeBasic
		}
	case channeltype.VertextAI:
		if meta.IsClaudeModelName(info.ActualModelName) {
			return WebSearchToolTypeBasic
		}
	}
	return ""
}

// SupportsWebSearchDynamicFiltering reports whether modelName belongs to a
// Claude family documented for web_search_20260209. Parameters: modelName is
// the upstream model identifier. Returns: true for Claude 4.6+ and Mythos
// Preview models, false otherwise (including unknown aliases).
func SupportsWebSearchDynamicFiltering(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "claude-mythos-preview" || strings.HasPrefix(name, "claude-mythos-preview-") {
		return true
	}
	for _, prefix := range webSearchDynamicFilteringPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ConvertedWebSearchMaxUses is the max_uses the gateway sets on a converted
// Claude web search; the fallback reserves exactly this many searches.
const ConvertedWebSearchMaxUses = 10

// ConvertedWebSearchToolFromResponses maps one Responses web_search declaration
// onto a Claude server-side web search tool. Parameters: tool is the admitted
// Responses declaration and toolType a WebSearchToolType* constant. Returns:
// the Claude tool, or an error wrapping ErrConvertedWebSearchNotRepresentable
// when the options are invalid, unbounded or contradictory. search_context_size,
// search_content_types and external_web_access have no Claude equivalent and
// are not forwarded.
func ConvertedWebSearchToolFromResponses(tool openai.ResponseAPITool, toolType string) (*Tool, error) {
	if toolType != WebSearchToolTypeBasic && toolType != WebSearchToolTypeDynamic {
		return nil, errors.Wrapf(ErrConvertedWebSearchNotRepresentable, "unsupported Claude web search version %q", toolType)
	}
	// Bound the searches to the allowance the gateway reserves for them, so the
	// pre-consumed hold is an upper bound on this tool's paid work.
	maxUses := ConvertedWebSearchMaxUses
	converted := &Tool{Type: toolType, Name: webSearchToolName, MaxUses: &maxUses}
	if tool.Filters != nil {
		allowed, err := convertedWebSearchDomains(tool.Filters.AllowedDomains)
		if err != nil {
			return nil, errors.Wrap(err, "allowed_domains")
		}
		blocked, err := convertedWebSearchDomains(tool.Filters.DisallowedDomains)
		if err != nil {
			return nil, errors.Wrap(err, "blocked_domains")
		}
		if len(allowed) > 0 && len(blocked) > 0 {
			return nil, errors.Wrap(ErrConvertedWebSearchNotRepresentable, "Claude web search accepts allowed or blocked domains, not both")
		}
		converted.AllowedDomains, converted.BlockedDomains = allowed, blocked
	}
	location, err := convertedWebSearchLocation(tool)
	if err != nil {
		return nil, errors.Wrap(err, "user_location")
	}
	converted.UserLocation = location
	return converted, nil
}

// convertedWebSearchDomains trims, bounds and copies one domain filter list.
// Parameters: domains is the untrusted list. Returns: the non-empty entries or
// an error when the list or an entry exceeds the gateway bounds.
func convertedWebSearchDomains(domains []string) ([]string, error) {
	if len(domains) == 0 {
		return nil, nil
	}
	if len(domains) > maxConvertedWebSearchDomains {
		return nil, errors.Wrapf(ErrConvertedWebSearchNotRepresentable, "at most %d domains are supported", maxConvertedWebSearchDomains)
	}
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if len(domain) > maxConvertedWebSearchFieldBytes {
			return nil, errors.Wrapf(ErrConvertedWebSearchNotRepresentable, "a domain exceeds %d bytes", maxConvertedWebSearchFieldBytes)
		}
		out = append(out, domain)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// convertedWebSearchLocation maps the flattened or nested Responses approximate
// location. Parameters: tool is the Responses declaration. Returns: nil when no
// location field is set, the Claude location, or a bound/type error.
func convertedWebSearchLocation(tool openai.ResponseAPITool) (*WebSearchUserLocation, error) {
	source := tool.UserLocation
	if source == nil {
		return nil, nil
	}
	if kind := strings.TrimSpace(source.Type); kind != "" && kind != "approximate" {
		return nil, errors.Wrapf(ErrConvertedWebSearchNotRepresentable, "location type %q is not approximate", kind)
	}
	pick := func(flat *string, nested func() *string) string {
		if flat != nil && strings.TrimSpace(*flat) != "" {
			return strings.TrimSpace(*flat)
		}
		if source.Approximate != nil {
			if value := nested(); value != nil {
				return strings.TrimSpace(*value)
			}
		}
		return ""
	}
	location := &WebSearchUserLocation{
		Type:     "approximate",
		City:     pick(source.City, func() *string { return source.Approximate.City }),
		Region:   pick(source.Region, func() *string { return source.Approximate.Region }),
		Country:  pick(source.Country, func() *string { return source.Approximate.Country }),
		Timezone: pick(source.Timezone, func() *string { return source.Approximate.Timezone }),
	}
	for _, value := range []string{location.City, location.Region, location.Country, location.Timezone} {
		if len(value) > maxConvertedWebSearchFieldBytes {
			return nil, errors.Wrapf(ErrConvertedWebSearchNotRepresentable, "a location field exceeds %d bytes", maxConvertedWebSearchFieldBytes)
		}
	}
	if location.City == "" && location.Region == "" && location.Country == "" && location.Timezone == "" {
		return nil, nil
	}
	return location, nil
}

// SetConvertedWebSearchTool records the web search declaration that the Claude
// request converter appends for the current attempt; nil clears it. Parameters:
// c is the request context and tool the declaration. Returns: nothing.
func SetConvertedWebSearchTool(c *gin.Context, tool *Tool) {
	if c == nil {
		return
	}
	c.Set(ctxkey.ClaudeConvertedWebSearchTool, tool)
}

// appendConvertedWebSearchTool appends the attempt's recorded web search
// declaration to a converted tool list. A caller tool already named
// web_search keeps the name (Claude rejects duplicate tool names), so the
// server tool is skipped and no search can run or be billed. Parameters: c is
// the request context and tools the converted caller tools. Returns: the list.
func appendConvertedWebSearchTool(c *gin.Context, tools []Tool) []Tool {
	search := convertedWebSearchTool(c)
	if search == nil {
		return tools
	}
	for _, tool := range tools {
		if tool.Name == search.Name {
			lg := gmw.GetLogger(c)
			lg.Debug("skip converted Claude web search: a caller tool already uses its name")
			return tools
		}
	}
	return append(tools, *search)
}

// convertedWebSearchTool returns a copy of the attempt's recorded web search
// declaration, or nil when none was recorded. Parameters: c is the request
// context. Returns: the declaration to append to a converted Claude request.
func convertedWebSearchTool(c *gin.Context) *Tool {
	if c == nil {
		return nil
	}
	raw, ok := c.Get(ctxkey.ClaudeConvertedWebSearchTool)
	if !ok {
		return nil
	}
	tool, ok := raw.(*Tool)
	if !ok || tool == nil {
		return nil
	}
	cloned := *tool
	cloned.AllowedDomains = append([]string(nil), tool.AllowedDomains...)
	cloned.BlockedDomains = append([]string(nil), tool.BlockedDomains...)
	if tool.UserLocation != nil {
		location := *tool.UserLocation
		cloned.UserLocation = &location
	}
	return &cloned
}
