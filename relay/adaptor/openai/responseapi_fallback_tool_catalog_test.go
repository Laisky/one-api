package openai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fallbackToolNamePattern is the tool-name charset and length accepted by both
// Chat Completions and Claude Messages upstreams.
var fallbackToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// fallbackHashedSuffixPattern matches the deterministic FNV-1a suffix that
// hashed fallback tool names carry.
var fallbackHashedSuffixPattern = regexp.MustCompile(`_[0-9a-f]{8}$`)

// fallbackToolRequest decodes a synthetic Responses request through the real
// wire decoder. Parameters: t owns assertions and raw is the request JSON.
// Returns the decoded request.
func fallbackToolRequest(t *testing.T, raw string) *ResponseAPIRequest {
	t.Helper()
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	return &request
}

// fallbackToolCatalog decodes raw and builds its fallback tool catalog.
// Parameters: t owns assertions and raw is the request JSON. Returns the
// catalog, failing the test when it cannot be built.
func fallbackToolCatalog(t *testing.T, raw string) *ResponseFallbackToolCatalog {
	t.Helper()
	catalog, err := BuildResponseFallbackToolCatalog(fallbackToolRequest(t, raw))
	require.NoError(t, err)
	require.NotNil(t, catalog)
	return catalog
}

// fallbackToolNames lists the flattened Chat tool names of catalog in order.
// Parameters: catalog is the built catalog. Returns the names.
func fallbackToolNames(catalog *ResponseFallbackToolCatalog) []string {
	names := make([]string, 0, len(catalog.Tools()))
	for _, tool := range catalog.Tools() {
		names = append(names, tool.Function.Name)
	}
	return names
}

// fallbackToolDescription returns the flattened description registered for
// flatName. Parameters: t owns assertions, catalog is the built catalog and
// flatName is the upstream name. Returns the description.
func fallbackToolDescription(t *testing.T, catalog *ResponseFallbackToolCatalog, flatName string) string {
	t.Helper()
	for _, tool := range catalog.Tools() {
		if tool.Function.Name == flatName {
			return tool.Function.Description
		}
	}
	require.Failf(t, "tool not found", "flattened tool %q is not in the catalog", flatName)
	return ""
}

// TestResponseFallbackToolCatalogFlattensNamespaceNames covers every joining
// rule: default namespace, "__" separator, "_"-prefixed connector children and
// namespaces that already end in "_". Each binding must reverse to the
// original namespace and name.
func TestResponseFallbackToolCatalogFlattensNamespaceNames(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[
		{"type":"function","name":"exec_command","strict":false,"parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}},
		{"type":"namespace","name":"functions","tools":[{"type":"function","name":"shell"}]},
		{"type":"namespace","name":"collaboration","description":"Multi-agent","tools":[{"type":"function","name":"spawn_agent"},{"type":"function","name":"wait_agent"}]},
		{"type":"namespace","name":"mcp__codex_apps__github","tools":[{"type":"function","name":"_create_issue"}]},
		{"type":"namespace","name":"mcp__docs_","tools":[{"type":"function","name":"read"}]}
	]}`)
	expected := []ResponseFallbackToolBinding{
		{FlatName: "exec_command", Namespace: "", Name: "exec_command", Kind: ResponseFallbackToolFunction},
		{FlatName: "shell", Namespace: "", Name: "shell", Kind: ResponseFallbackToolFunction},
		{FlatName: "collaboration__spawn_agent", Namespace: "collaboration", Name: "spawn_agent", Kind: ResponseFallbackToolFunction},
		{FlatName: "collaboration__wait_agent", Namespace: "collaboration", Name: "wait_agent", Kind: ResponseFallbackToolFunction},
		{FlatName: "mcp__codex_apps__github_create_issue", Namespace: "mcp__codex_apps__github", Name: "_create_issue", Kind: ResponseFallbackToolFunction},
		{FlatName: "mcp__docs_read", Namespace: "mcp__docs_", Name: "read", Kind: ResponseFallbackToolFunction},
	}
	names := make([]string, 0, len(expected))
	for _, want := range expected {
		names = append(names, want.FlatName)
		got, ok := catalog.Binding(want.FlatName)
		require.True(t, ok, "binding for %s", want.FlatName)
		require.Equal(t, want, got)
		// The default namespace "functions" is the absence of a namespace.
		require.Equal(t, want.FlatName, catalog.FlatName(want.Namespace, want.Name))
	}
	require.Equal(t, names, fallbackToolNames(catalog), "root order then namespace children in declaration order")
	require.Equal(t, "shell", catalog.FlatName("functions", "shell"))
	require.Equal(t, "shell", catalog.FlatName(" functions ", "shell"))
	for _, tool := range catalog.Tools() {
		require.Equal(t, "function", tool.Type)
		require.Regexp(t, fallbackToolNamePattern, tool.Function.Name)
		require.Nil(t, tool.Function.Strict, "fallback drops strict like the legacy converter")
	}
	_, ok := catalog.Binding("spawn_agent")
	require.False(t, ok, "unflattened leaf names are not declared")
}

// TestResponseFallbackToolCatalogHashesLongNames proves joined names beyond the
// 64-byte upstream limit become bounded, charset-safe, deterministic hashed
// names that remain distinct for identities sharing a long prefix.
func TestResponseFallbackToolCatalogHashesLongNames(t *testing.T) {
	t.Parallel()
	namespace := "mcp__" + strings.Repeat("n", 40)
	first := strings.Repeat("c", 50) + "_one"
	second := strings.Repeat("c", 50) + "_two"
	raw := fmt.Sprintf(`{"model":"fixture","input":"hi","tools":[{"type":"namespace","name":%q,"tools":[{"type":"function","name":%q},{"type":"custom","name":%q}]}]}`, namespace, first, second)
	catalog := fallbackToolCatalog(t, raw)
	again := fallbackToolCatalog(t, raw)

	names := fallbackToolNames(catalog)
	require.Len(t, names, 2)
	require.Equal(t, names, fallbackToolNames(again), "hashing must be deterministic across builds")
	require.NotEqual(t, names[0], names[1], "distinct identities keep distinct hashed names")
	for i, name := range names {
		require.LessOrEqual(t, len(name), 64)
		require.Regexp(t, fallbackToolNamePattern, name)
		require.Regexp(t, fallbackHashedSuffixPattern, name)
		require.True(t, strings.HasPrefix(name, namespace+"__"), "hashed names keep a readable prefix: %s", name)
		binding, ok := catalog.Binding(name)
		require.True(t, ok)
		require.Equal(t, namespace, binding.Namespace)
		require.Equal(t, []string{first, second}[i], binding.Name)
		require.Equal(t, name, catalog.FlatName(namespace, binding.Name))
	}
	custom, ok := catalog.Binding(names[1])
	require.True(t, ok)
	require.Equal(t, ResponseFallbackToolCustom, custom.Kind)

	// Undeclared long identities flatten with the same deterministic rule.
	require.Equal(t, names[0], (*ResponseFallbackToolCatalog)(nil).FlatName(namespace, first))
}

// TestHashedResponseToolNameSanitizesCharset proves the hashed fallback name
// replaces characters upstreams reject and never exceeds 64 bytes, even for
// multibyte input.
func TestHashedResponseToolNameSanitizesCharset(t *testing.T) {
	t.Parallel()
	cases := []responseToolIdentity{
		{namespace: "ns", name: "bad.name with space"},
		{name: strings.Repeat("工具", 80)},
		{namespace: strings.Repeat("x", 64), name: strings.Repeat("y", 64)},
		{search: true},
	}
	seen := map[string]bool{}
	for _, identity := range cases {
		name := hashedResponseToolName(identity)
		require.LessOrEqual(t, len(name), 64, name)
		require.Regexp(t, fallbackToolNamePattern, name)
		require.Regexp(t, fallbackHashedSuffixPattern, name)
		require.Equal(t, name, hashedResponseToolName(identity), "deterministic")
		require.False(t, seen[name], "distinct identities must not share %s", name)
		seen[name] = true
	}
	require.True(t, strings.HasPrefix(hashedResponseToolName(responseToolIdentity{search: true}), "tool_search_"))
	require.True(t, strings.HasPrefix(hashedResponseToolName(cases[0]), "ns__bad_name_with_space_"))
}

// TestResponseFallbackToolCatalogDisambiguatesCollisions proves that when a
// flattened name is already owned by another identity, the later definition
// gets a hashed name and both bindings stay reversible, in either order.
func TestResponseFallbackToolCatalogDisambiguatesCollisions(t *testing.T) {
	t.Parallel()
	root := `{"type":"function","name":"alpha__shared","description":"Root tool"}`
	group := `{"type":"namespace","name":"alpha","tools":[{"type":"function","name":"shared","description":"Grouped tool"}]}`
	for _, tc := range []struct {
		name           string
		tools          string
		hashedIdentity responseToolIdentity
	}{
		{name: "root_first", tools: root + "," + group, hashedIdentity: responseToolIdentity{namespace: "alpha", name: "shared"}},
		{name: "namespace_first", tools: group + "," + root, hashedIdentity: responseToolIdentity{name: "alpha__shared"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[`+tc.tools+`]}`)
			names := fallbackToolNames(catalog)
			require.Len(t, names, 2)
			require.Equal(t, "alpha__shared", names[0], "the first definition keeps the plain flattened name")
			hashed := hashedResponseToolName(tc.hashedIdentity)
			require.Equal(t, hashed, names[1])
			require.Regexp(t, fallbackToolNamePattern, hashed)

			plain, ok := catalog.Binding("alpha__shared")
			require.True(t, ok)
			other, ok := catalog.Binding(hashed)
			require.True(t, ok)
			bindings := map[responseToolIdentity]ResponseFallbackToolBinding{
				responseBindingIdentity(plain): plain, responseBindingIdentity(other): other,
			}
			require.Contains(t, bindings, responseToolIdentity{name: "alpha__shared"})
			require.Contains(t, bindings, responseToolIdentity{namespace: "alpha", name: "shared"})
			require.Equal(t, catalog.FlatName("", "alpha__shared"), bindings[responseToolIdentity{name: "alpha__shared"}].FlatName)
			require.Equal(t, catalog.FlatName("alpha", "shared"), bindings[responseToolIdentity{namespace: "alpha", name: "shared"}].FlatName)
			require.Contains(t, fallbackToolDescription(t, catalog, catalog.FlatName("alpha", "shared")), "Grouped tool")
			require.Contains(t, fallbackToolDescription(t, catalog, catalog.FlatName("", "alpha__shared")), "Root tool")
		})
	}
}

// TestResponseFallbackToolCatalogSearchNameCollision proves the client
// tool_search tool and a function literally named tool_search coexist with
// reversible, distinct upstream names.
func TestResponseFallbackToolCatalogSearchNameCollision(t *testing.T) {
	t.Parallel()
	search := `{"type":"tool_search","execution":"client","description":"Find tools"}`
	function := `{"type":"function","name":"tool_search","description":"A user tool"}`

	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[`+function+`,`+search+`]}`)
	searchName := catalog.SearchFlatName()
	require.Equal(t, hashedResponseToolName(responseToolIdentity{search: true}), searchName)
	require.Equal(t, []string{"tool_search", searchName}, fallbackToolNames(catalog))
	binding, ok := catalog.Binding(searchName)
	require.True(t, ok)
	require.Equal(t, ResponseFallbackToolSearch, binding.Kind)
	binding, ok = catalog.Binding("tool_search")
	require.True(t, ok)
	require.Equal(t, ResponseFallbackToolFunction, binding.Kind)

	catalog = fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[`+search+`,`+function+`]}`)
	require.Equal(t, "tool_search", catalog.SearchFlatName())
	userName := catalog.FlatName("", "tool_search")
	require.Equal(t, hashedResponseToolName(responseToolIdentity{name: "tool_search"}), userName)
	require.Equal(t, []string{"tool_search", userName}, fallbackToolNames(catalog))
	binding, ok = catalog.Binding(userName)
	require.True(t, ok)
	require.Equal(t, ResponseFallbackToolFunction, binding.Kind)
	require.Equal(t, "tool_search", binding.Name)
}

// TestResponseFallbackToolCatalogFlatNameUndeclared proves FlatName gives
// undeclared history identities the deterministic flattening, but never the
// name a different declared identity owns.
func TestResponseFallbackToolCatalogFlatNameUndeclared(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[{"type":"function","name":"beta__x"}]}`)
	require.Equal(t, "gamma__y", catalog.FlatName("gamma", "y"))
	require.Equal(t, "plain", catalog.FlatName("", "plain"))
	require.Equal(t, "plain", catalog.FlatName("functions", "plain"))
	require.Equal(t, "beta__x", catalog.FlatName("", "beta__x"))
	collided := catalog.FlatName("beta", "x")
	require.Equal(t, hashedResponseToolName(responseToolIdentity{namespace: "beta", name: "x"}), collided)
	require.NotEqual(t, "beta__x", collided, "an undeclared identity must not alias a declared tool")
	require.Equal(t, "tool_search", catalog.SearchFlatName(), "default search name without a declaration")

	var empty *ResponseFallbackToolCatalog
	require.Nil(t, empty.Tools())
	_, ok := empty.Binding("anything")
	require.False(t, ok)
	require.Equal(t, "ns__n", empty.FlatName("ns", "n"))
	require.Equal(t, "tool_search", empty.SearchFlatName())
}

// TestResponseFallbackToolCatalogOrderAndDedup proves root tools come first,
// then additional_tools and tool_search_output catalogs in input order, and
// that the first definition of an identity wins everywhere.
func TestResponseFallbackToolCatalogOrderAndDedup(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","tools":[
		{"type":"function","name":"a","description":"root a"},
		{"type":"function","name":"a","description":"duplicate root a"},
		{"type":"namespace","name":"ns","tools":[{"type":"function","name":"b","description":"root b"}]},
		{"type":"tool_search","execution":"client","description":"root search"}
	],"input":[
		{"type":"tool_search_output","call_id":"call_s1","execution":"client","status":"completed","tools":[
			{"type":"namespace","name":"ns","tools":[{"type":"function","name":"b","description":"discovered b"},{"type":"function","name":"d","description":"discovered d"}]}
		]},
		{"type":"additional_tools","role":"developer","tools":[
			{"type":"function","name":"c","description":"additional c"},
			{"type":"function","name":"a","description":"additional a"},
			{"type":"tool_search","execution":"client","description":"additional search"}
		]},
		{"type":"message","role":"user","content":"hi"},
		{"type":"tool_search_output","call_id":"call_s2","execution":"client","status":"completed","tools":[
			{"type":"function","name":"e","description":"discovered e"},
			{"type":"custom","name":"c","description":"custom c shadowed"}
		]}
	]}`)
	require.Equal(t, []string{"a", "ns__b", "tool_search", "ns__d", "c", "e"}, fallbackToolNames(catalog),
		"root first, then discovery catalogs in input order")
	require.Equal(t, "root a", fallbackToolDescription(t, catalog, "a"))
	require.Equal(t, "root b", fallbackToolDescription(t, catalog, "ns__b"))
	require.Equal(t, "root search", fallbackToolDescription(t, catalog, "tool_search"))
	require.Equal(t, "additional c", fallbackToolDescription(t, catalog, "c"))
	binding, ok := catalog.Binding("c")
	require.True(t, ok)
	require.Equal(t, ResponseFallbackToolFunction, binding.Kind, "function and custom tools share one identity per namespace")
}

// TestResponseFallbackToolCatalogAdditionalToolsSearch proves a client
// tool_search declared only inside additional_tools is admitted.
func TestResponseFallbackToolCatalogAdditionalToolsSearch(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":[
		{"type":"additional_tools","role":"developer","tools":[{"type":"tool_search","execution":"client"},{"type":"function","name":"lookup"}]},
		{"type":"message","role":"user","content":"hi"}
	]}`)
	require.Equal(t, []string{"tool_search", "lookup"}, fallbackToolNames(catalog))
	binding, ok := catalog.Binding("tool_search")
	require.True(t, ok)
	require.Equal(t, ResponseFallbackToolSearch, binding.Kind)
}

// TestResponseFallbackToolCatalogNamespaceDescriptionPrefix proves the
// namespace description prefixes each child description as "[desc] ".
func TestResponseFallbackToolCatalogNamespaceDescriptionPrefix(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[
		{"type":"namespace","name":"mcp__docs","description":"  Docs server  ","tools":[
			{"type":"function","name":"read_doc","description":"Read a document"},
			{"type":"function","name":"list_docs"},
			{"type":"custom","name":"edit_doc","description":"Edit freeform","format":{"type":"text"}}
		]},
		{"type":"namespace","name":"plain","tools":[{"type":"function","name":"tool","description":"Unprefixed"}]}
	]}`)
	require.Equal(t, "[Docs server] Read a document", fallbackToolDescription(t, catalog, "mcp__docs__read_doc"))
	require.Equal(t, "[Docs server]", fallbackToolDescription(t, catalog, "mcp__docs__list_docs"))
	require.True(t, strings.HasPrefix(fallbackToolDescription(t, catalog, "mcp__docs__edit_doc"), "[Docs server] Edit freeform\n\n"+responseCustomToolGatewayNote))
	require.Equal(t, "Unprefixed", fallbackToolDescription(t, catalog, "plain__tool"))
}

// TestResponseFallbackToolCatalogIgnoresHostedRootTools proves hosted and
// gateway-managed root tools keep their historical handling outside the
// catalog, unnamed functions are skipped, and empty requests build an empty
// catalog.
func TestResponseFallbackToolCatalogIgnoresHostedRootTools(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[
		{"type":"web_search","external_web_access":true},
		{"type":"mcp","server_label":"fixture"},
		{"type":"code_interpreter","container":{"type":"auto"}},
		{"type":"image_generation"},
		{"type":"function","parameters":{"type":"object"}},
		{"type":"function","name":"x"}
	]}`)
	require.Equal(t, []string{"x"}, fallbackToolNames(catalog))

	catalog = fallbackToolCatalog(t, `{"model":"fixture","input":"hi"}`)
	require.Nil(t, catalog.Tools())
	catalog, err := BuildResponseFallbackToolCatalog(nil)
	require.NoError(t, err)
	require.NotNil(t, catalog)
	require.Nil(t, catalog.Tools())
}

// fallbackFunctionChildren builds count synthetic namespace children without
// the wire decoder, which enforces its own bound first. Parameters: prefix
// names the children and count is the number to build. Returns the children.
func fallbackFunctionChildren(prefix string, count int) []ResponseAPITool {
	children := make([]ResponseAPITool, count)
	for i := range children {
		children[i] = ResponseAPITool{Type: "function", Name: fmt.Sprintf("%s_%04d", prefix, i)}
	}
	return children
}

// TestResponseFallbackToolCatalogRejectsUnrepresentable proves the catalog's
// own guards reject namespace and tool_search shapes fallback cannot emulate,
// including shapes the wire decoder would already refuse.
func TestResponseFallbackToolCatalogRejectsUnrepresentable(t *testing.T) {
	t.Parallel()
	cases := map[string][]ResponseAPITool{
		"namespace_over_1024_children": {{Type: "namespace", Name: "big", Tools: fallbackFunctionChildren("tool", MaxResponseNamespaceTools+1)}},
		"namespace_without_children":   {{Type: "namespace", Name: "empty"}},
		"namespace_invalid_name":       {{Type: "namespace", Name: "not valid", Tools: fallbackFunctionChildren("tool", 1)}},
		"namespace_missing_name":       {{Type: "namespace", Tools: fallbackFunctionChildren("tool", 1)}},
		"namespace_name_too_long":      {{Type: "namespace", Name: strings.Repeat("n", 65), Tools: fallbackFunctionChildren("tool", 1)}},
		"nested_namespace_child":       {{Type: "namespace", Name: "outer", Tools: []ResponseAPITool{{Type: "namespace", Name: "inner", Tools: fallbackFunctionChildren("tool", 1)}}}},
		"function_child_with_tools":    {{Type: "namespace", Name: "outer", Tools: []ResponseAPITool{{Type: "function", Name: "run", Tools: fallbackFunctionChildren("tool", 1)}}}},
		"hosted_web_search_child":      {{Type: "namespace", Name: "outer", Tools: []ResponseAPITool{{Type: "web_search"}}}},
		"mcp_child":                    {{Type: "namespace", Name: "outer", Tools: []ResponseAPITool{{Type: "mcp", ServerLabel: "fixture"}}}},
		"invalid_child_name":           {{Type: "namespace", Name: "outer", Tools: []ResponseAPITool{{Type: "function", Name: "bad name"}}}},
		"server_tool_search":           {{Type: "tool_search", Execution: "server"}},
		"implicit_tool_search":         {{Type: "tool_search"}},
	}
	for name, tools := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			catalog, err := BuildResponseFallbackToolCatalog(&ResponseAPIRequest{Model: "fixture", Tools: tools})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
			require.Nil(t, catalog)
		})
	}
}

// TestResponseFallbackToolCatalogRejectsDiscoveredSearch proves a tool_search
// definition loaded through tool_search_output, or a non-client definition in
// additional_tools, cannot be emulated.
func TestResponseFallbackToolCatalogRejectsDiscoveredSearch(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"client_search_in_output":    `[{"type":"tool_search_output","call_id":"call_s1","execution":"client","tools":[{"type":"tool_search","execution":"client"}]}]`,
		"server_search_in_output":    `[{"type":"tool_search_output","call_id":"call_s1","execution":"client","tools":[{"type":"tool_search","execution":"server"}]}]`,
		"server_search_additional":   `[{"type":"additional_tools","role":"developer","tools":[{"type":"tool_search","execution":"server"}]}]`,
		"nested_in_output_namespace": `[{"type":"tool_search_output","call_id":"call_s1","execution":"client","tools":[{"type":"namespace","name":"ns","tools":[{"type":"web_search"}]}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := fallbackToolRequest(t, `{"model":"fixture","input":`+input+`}`)
			catalog, err := BuildResponseFallbackToolCatalog(request)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
			require.Nil(t, catalog)
		})
	}
}

// TestResponseFallbackToolCatalogRejectsOversizedDiscoveredCatalog proves a
// tool_search_output catalog beyond the per-catalog bound fails before any
// tool DTO is built, even when it bypassed the request wire decoder.
func TestResponseFallbackToolCatalogRejectsOversizedDiscoveredCatalog(t *testing.T) {
	t.Parallel()
	tools := make([]any, MaxResponseNamespaceTools+1)
	for i := range tools {
		tools[i] = map[string]any{"type": "function", "name": fmt.Sprintf("tool_%04d", i)}
	}
	request := &ResponseAPIRequest{Model: "fixture", Input: ResponseAPIInput{
		map[string]any{"type": "tool_search_output", "call_id": "call_s1", "execution": "client", "tools": tools},
	}}
	catalog, err := BuildResponseFallbackToolCatalog(request)
	require.Error(t, err)
	require.ErrorContains(t, err, "count exceeds limit")
	require.Nil(t, catalog)

	wire, err := json.Marshal(map[string]any{"model": "fixture", "input": request.Input})
	require.NoError(t, err)
	var decoded ResponseAPIRequest
	require.Error(t, json.Unmarshal(wire, &decoded), "the wire decoder enforces the same bound")
}

// TestResponseFallbackToolCatalogBoundsTotalTools proves the flattened catalog
// accepts exactly MaxResponseFallbackTools distinct definitions, ignores
// duplicates for the bound, and rejects one more.
func TestResponseFallbackToolCatalogBoundsTotalTools(t *testing.T) {
	t.Parallel()
	full := make([]ResponseAPITool, 0, 4)
	for i := range 4 {
		full = append(full, ResponseAPITool{Type: "namespace", Name: fmt.Sprintf("group%d", i), Tools: fallbackFunctionChildren("tool", MaxResponseNamespaceTools)})
	}
	require.Equal(t, MaxResponseFallbackTools, 4*MaxResponseNamespaceTools)

	withDuplicate := append(append([]ResponseAPITool(nil), full...), ResponseAPITool{Type: "namespace", Name: "group0", Tools: fallbackFunctionChildren("tool", 3)})
	catalog, err := BuildResponseFallbackToolCatalog(&ResponseAPIRequest{Model: "fixture", Tools: withDuplicate})
	require.NoError(t, err)
	require.Len(t, catalog.Tools(), MaxResponseFallbackTools)

	overflow := append(append([]ResponseAPITool(nil), full...), ResponseAPITool{Type: "function", Name: "one_more"})
	catalog, err = BuildResponseFallbackToolCatalog(&ResponseAPIRequest{Model: "fixture", Tools: overflow})
	require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
	require.ErrorContains(t, err, "4096")
	require.Nil(t, catalog)

	five := make([]ResponseAPITool, 0, 5)
	for i := range 5 {
		five = append(five, ResponseAPITool{Type: "namespace", Name: fmt.Sprintf("bulk%d", i), Tools: fallbackFunctionChildren("tool", 1000)})
	}
	catalog, err = BuildResponseFallbackToolCatalog(&ResponseAPIRequest{Model: "fixture", Tools: five})
	require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
	require.Nil(t, catalog)
}
