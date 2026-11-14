package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// MaxResponseFallbackTools bounds the distinct callable definitions one fallback
// request may expose after namespaces and discovered catalogs are flattened.
const MaxResponseFallbackTools = 4096

// responseFallbackToolNameLimit is the strictest tool-name length shared by the
// Chat Completions and Claude Messages upstreams that fallback targets.
const responseFallbackToolNameLimit = 64

// responseDefaultNamespace is the Responses namespace that clients treat as the
// absence of a namespace (Codex folds top-level tools into it in Lite mode).
const responseDefaultNamespace = "functions"

// ResponseFallbackToolKind identifies the Responses item family a flattened
// fallback tool call must be rendered back into.
type ResponseFallbackToolKind string

const (
	// ResponseFallbackToolFunction renders calls as function_call items.
	ResponseFallbackToolFunction ResponseFallbackToolKind = "function"
	// ResponseFallbackToolCustom renders calls as custom_tool_call items.
	ResponseFallbackToolCustom ResponseFallbackToolKind = "custom"
	// ResponseFallbackToolSearch renders calls as client tool_search_call items.
	ResponseFallbackToolSearch ResponseFallbackToolKind = "tool_search"
)

// ResponseFallbackToolBinding records the Responses identity behind one
// flattened Chat tool name so a fallback tool call can be rendered faithfully.
type ResponseFallbackToolBinding struct {
	FlatName  string
	Namespace string
	Name      string
	Kind      ResponseFallbackToolKind
}

// responseToolIdentity is the client-visible identity of a callable definition.
// Function and custom tools share one name space per namespace, as in Codex.
type responseToolIdentity struct {
	namespace string
	name      string
	search    bool
}

// ResponseFallbackToolCatalog maps Responses tool definitions (top-level tools,
// namespaces, client tool search, and catalogs loaded through history) onto the
// flat tool list that Chat Completions and Claude Messages upstreams accept.
// The mapping is a pure function of the request, so the request converter and
// the response renderers rebuild identical bindings without shared state.
type ResponseFallbackToolCatalog struct {
	preserveJSONSchema bool
	byFlat             map[string]ResponseFallbackToolBinding
	byIdentity         map[responseToolIdentity]string
	tools              []model.Tool
	// discovery caches the decoded discovery items of the request input the
	// catalog was built from, so history lowering need not decode them again.
	discovery         []ResponseToolSearchInputItem
	discoveryDecoded  bool
	discoveryInputLen int
}

// BuildResponseFallbackToolCatalog builds the flattened tool catalog for request.
// Root tools come first in request order, followed by developer additional_tools
// catalogs and tool_search_output catalogs in input order, so the upstream tool
// prefix stays byte-stable while discovery appends definitions. It returns an
// error when a definition cannot be represented faithfully on fallback.
// Optional preserveJSONSchema selects lossless function schemas for a trusted capable provider; omission retains legacy sanitation.
func BuildResponseFallbackToolCatalog(request *ResponseAPIRequest, preserveJSONSchema ...bool) (*ResponseFallbackToolCatalog, error) {
	catalog := &ResponseFallbackToolCatalog{
		byFlat:     make(map[string]ResponseFallbackToolBinding),
		byIdentity: make(map[responseToolIdentity]string),
	}
	if len(preserveJSONSchema) > 0 {
		catalog.preserveJSONSchema = preserveJSONSchema[0]
	}
	if request == nil {
		return catalog, nil
	}
	if err := catalog.addTools(request.Tools, true); err != nil {
		return nil, errors.Wrap(err, "flatten response tools")
	}
	items, err := responseFallbackToolSearchItems(request.Input)
	if err != nil {
		return nil, errors.Wrap(err, "inspect response tool catalogs")
	}
	catalog.discovery, catalog.discoveryDecoded, catalog.discoveryInputLen = items, true, len(request.Input)
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "additional_tools":
			err = catalog.addTools(item.Tools, true)
		case "tool_search_output":
			err = catalog.addTools(item.Tools, false)
		default:
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "flatten %s catalog", item.Type)
		}
	}
	return catalog, nil
}

// responseFallbackToolSearchItems decodes discovery history items from input.
// Parameters: input is the request input. Returns the typed discovery items in
// input order, or a wrapped decode error.
func responseFallbackToolSearchItems(input ResponseAPIInput) ([]ResponseToolSearchInputItem, error) {
	if len(input) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, errors.Wrap(err, "marshal response input")
	}
	items, err := InspectResponseToolSearchInput(data)
	if err != nil {
		return nil, errors.Wrap(err, "decode response discovery items")
	}
	return items, nil
}

// Tools returns the flattened Chat tool definitions in catalog order, or nil
// when the catalog is empty.
func (c *ResponseFallbackToolCatalog) Tools() []model.Tool {
	if c == nil || len(c.tools) == 0 {
		return nil
	}
	return c.tools
}

// Binding resolves a flattened upstream tool name back to its Responses
// identity. It returns false for names the request never declared.
func (c *ResponseFallbackToolCatalog) Binding(flatName string) (ResponseFallbackToolBinding, bool) {
	if c == nil {
		return ResponseFallbackToolBinding{}, false
	}
	binding, ok := c.byFlat[flatName]
	return binding, ok
}

// FlatName returns the upstream tool name for a namespaced Responses call
// identity. Declared identities return their registered name; undeclared ones
// get the deterministic flattening, disambiguated if a declared tool owns it.
func (c *ResponseFallbackToolCatalog) FlatName(namespace, name string) string {
	identity := responseToolIdentity{namespace: canonicalResponseNamespace(namespace), name: name}
	if c != nil {
		if flat, ok := c.byIdentity[identity]; ok {
			return flat
		}
	}
	candidate := flattenResponseToolName(identity.namespace, name)
	if c != nil {
		if owner, taken := c.byFlat[candidate]; taken && responseBindingIdentity(owner) != identity {
			return hashedResponseToolName(identity)
		}
	}
	return candidate
}

// SearchFlatName returns the upstream name used for the client tool_search tool.
func (c *ResponseFallbackToolCatalog) SearchFlatName() string {
	identity := responseToolIdentity{search: true}
	if c != nil {
		if flat, ok := c.byIdentity[identity]; ok {
			return flat
		}
	}
	return "tool_search"
}

// addTools flattens one catalog. allowSearch admits a client tool_search
// definition at this catalog's root. Hosted or gateway-managed root tools keep
// their historical fallback behavior and are not added here.
func (c *ResponseFallbackToolCatalog) addTools(tools []ResponseAPITool, allowSearch bool) error {
	for _, tool := range tools {
		switch strings.ToLower(strings.TrimSpace(tool.Type)) {
		case "function", "custom":
			if err := c.addCallable("", "", tool); err != nil {
				return err
			}
		case "namespace":
			if err := c.addNamespace(tool); err != nil {
				return err
			}
		case "tool_search":
			if !allowSearch || !strings.EqualFold(strings.TrimSpace(tool.Execution), "client") {
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "only a root client-executed tool_search can be emulated")
			}
			if err := c.register(responseToolIdentity{search: true}, ResponseFallbackToolSearch, responseSearchToolToChat(tool, c.preserveJSONSchema)); err != nil {
				return err
			}
		default:
			// web_search, mcp and other hosted tools retain their existing
			// fallback handling outside the local catalog.
		}
	}
	return nil
}

// addNamespace flattens every function/custom child of one namespace group.
func (c *ResponseFallbackToolCatalog) addNamespace(group ResponseAPITool) error {
	if !validResponseToolIdentifier(group.Name) || len(group.Tools) == 0 || len(group.Tools) > MaxResponseNamespaceTools {
		return errors.Wrap(ErrResponseFallbackNotRepresentable, "namespace requires a valid name and 1 to 1024 tools")
	}
	for _, child := range group.Tools {
		switch strings.ToLower(strings.TrimSpace(child.Type)) {
		case "function", "custom":
			if len(child.Tools) > 0 {
				return errors.Wrap(ErrResponseFallbackNotRepresentable, "nested namespaces are not supported")
			}
			if err := c.addCallable(group.Name, group.Description, child); err != nil {
				return err
			}
		default:
			return errors.Wrapf(ErrResponseFallbackNotRepresentable, "namespace child type %q is not a local tool", child.Type)
		}
	}
	return nil
}

// addCallable registers one function or custom definition under namespace.
// namespaceDescription, when present, prefixes the child description so the
// upstream model keeps the group context that the namespace carried.
func (c *ResponseFallbackToolCatalog) addCallable(namespace, namespaceDescription string, tool ResponseAPITool) error {
	namespace = canonicalResponseNamespace(namespace)
	name := tool.Name
	if tool.Function != nil && tool.Function.Name != "" {
		name = tool.Function.Name
	}
	if strings.TrimSpace(name) == "" {
		// Legacy fallback skipped unnamed function definitions.
		return nil
	}
	if namespace != "" && !validResponseToolIdentifier(name) {
		return errors.Wrap(ErrResponseFallbackNotRepresentable, "namespaced tool requires a valid name")
	}
	identity := responseToolIdentity{namespace: namespace, name: name}
	if _, exists := c.byIdentity[identity]; exists {
		// The first definition keeps its position so the upstream prefix is stable.
		return nil
	}
	kind := ResponseFallbackToolFunction
	var chatTool *model.Tool
	if strings.EqualFold(strings.TrimSpace(tool.Type), "custom") {
		kind = ResponseFallbackToolCustom
		chatTool = responseCustomToolToChat(tool)
	} else {
		chatTool = responseFunctionToolToChat(tool, c.preserveJSONSchema)
	}
	if chatTool == nil {
		return nil
	}
	if namespaceDescription = strings.TrimSpace(namespaceDescription); namespaceDescription != "" {
		chatTool.Function.Description = strings.TrimSpace("[" + namespaceDescription + "] " + chatTool.Function.Description)
	}
	return c.register(identity, kind, chatTool)
}

// register assigns a collision-free flat name to identity and records the tool.
func (c *ResponseFallbackToolCatalog) register(identity responseToolIdentity, kind ResponseFallbackToolKind, chatTool *model.Tool) error {
	if chatTool == nil || chatTool.Function == nil {
		return nil
	}
	if _, exists := c.byIdentity[identity]; exists {
		return nil
	}
	if len(c.tools) >= MaxResponseFallbackTools {
		return errors.Wrapf(ErrResponseFallbackNotRepresentable, "fallback tool catalog exceeds %d definitions", MaxResponseFallbackTools)
	}
	flat := "tool_search"
	if !identity.search {
		flat = flattenResponseToolName(identity.namespace, identity.name)
	}
	if _, taken := c.byFlat[flat]; taken {
		flat = hashedResponseToolName(identity)
		if _, stillTaken := c.byFlat[flat]; stillTaken {
			return errors.Wrapf(ErrResponseFallbackNotRepresentable, "tool name %q collides after flattening", identity.name)
		}
	}
	chatTool.Function.Name = flat
	c.byFlat[flat] = ResponseFallbackToolBinding{FlatName: flat, Namespace: identity.namespace, Name: identity.name, Kind: kind}
	c.byIdentity[identity] = flat
	c.tools = append(c.tools, *chatTool)
	return nil
}

// responseBindingIdentity recovers the identity key from a binding.
func responseBindingIdentity(binding ResponseFallbackToolBinding) responseToolIdentity {
	if binding.Kind == ResponseFallbackToolSearch {
		return responseToolIdentity{search: true}
	}
	return responseToolIdentity{namespace: binding.Namespace, name: binding.Name}
}

// canonicalResponseNamespace maps the client default namespace to "".
func canonicalResponseNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == responseDefaultNamespace {
		return ""
	}
	return namespace
}

// flattenResponseToolName joins a namespace and a tool name into one upstream
// tool name. Default-namespace tools keep their own name. A child whose name
// starts with "_" (Codex app connectors) or a namespace ending in "_" is joined
// directly; other pairs use "__", matching Codex's historical MCP names. Names
// that are too long or carry characters upstreams reject are hashed.
func flattenResponseToolName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	joined := namespace + "__" + name
	if strings.HasPrefix(name, "_") || strings.HasSuffix(namespace, "_") {
		joined = namespace + name
	}
	if len(joined) <= responseFallbackToolNameLimit && validResponseToolIdentifier(joined) {
		return joined
	}
	return hashedResponseToolName(responseToolIdentity{namespace: namespace, name: name})
}

// hashedResponseToolName returns a bounded, charset-safe, deterministic name
// for identity: a readable sanitized prefix plus an FNV-1a suffix of the
// complete identity, so distinct identities keep distinct names.
func hashedResponseToolName(identity responseToolIdentity) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(identity.namespace + "\x00" + identity.name))
	suffix := fmt.Sprintf("_%08x", hasher.Sum32())
	base := identity.name
	if identity.namespace != "" {
		base = identity.namespace + "__" + identity.name
	}
	if identity.search {
		base = "tool_search"
	}
	var b strings.Builder
	for _, r := range base {
		if b.Len() >= responseFallbackToolNameLimit-len(suffix) {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String() + suffix
}

// validResponseToolIdentifier reports whether name is a nonempty ASCII tool
// identifier of letters, digits, underscores or hyphens within 64 bytes.
func validResponseToolIdentifier(name string) bool {
	if len(name) == 0 || len(name) > responseFallbackToolNameLimit {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// cachedDiscoveryFor returns the cached discovery items when they were decoded
// from input: same length, and each cached item still sits at its index with
// identical raw bytes (or, for a decoded map, the same item type). It returns
// false when the cache cannot be proven to describe input.
func (c *ResponseFallbackToolCatalog) cachedDiscoveryFor(input ResponseAPIInput) ([]ResponseToolSearchInputItem, bool) {
	if c == nil || !c.discoveryDecoded || c.discoveryInputLen != len(input) {
		return nil, false
	}
	for _, item := range c.discovery {
		if item.Index < 0 || item.Index >= len(input) {
			return nil, false
		}
		switch raw := input[item.Index].(type) {
		case json.RawMessage:
			if !bytes.Equal(raw, item.Raw) {
				return nil, false
			}
		case map[string]any:
			if kind, _ := raw["type"].(string); !strings.EqualFold(strings.TrimSpace(kind), strings.TrimSpace(item.Type)) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return c.discovery, true
}
