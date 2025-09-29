# MCP Resources Documentation

## Overview

MCP (Model Context Protocol) resources provide static reference documentation that complements the dynamic tools system. While tools generate parameter-specific documentation based on user input, resources offer comprehensive guides and reference materials that remain constant.

## Resources vs Tools

### Tools
- **Dynamic**: Generate documentation based on user-provided parameters
- **Interactive**: Accept arguments and produce customized output
- **Parameter-specific**: Show examples with actual values from tool calls
- **Use case**: When you need documentation for specific API calls with your parameters

### Resources
- **Static**: Provide comprehensive reference documentation
- **Informational**: Offer guides, overviews, and best practices
- **Comprehensive**: Cover broad topics and concepts
- **Use case**: When you need general information, guides, or reference materials

## Available Resources

### 1. API Endpoints Overview (`oneapi://docs/api-endpoints`)
**Purpose**: Comprehensive overview of all available One-API endpoints

**Content**:
- Complete list of supported API endpoints
- HTTP methods and URL patterns
- Brief descriptions of each endpoint's purpose
- Cross-references to related tools

**When to use**: 
- Getting an overview of available APIs
- Understanding the complete API surface
- Planning integration strategies

### 2. Tool Usage Guide (`oneapi://docs/tool-usage-guide`)
**Purpose**: Comprehensive guide for using MCP tools effectively

**Content**:
- Detailed explanations of each tool
- Parameter descriptions and examples
- Best practices for tool usage
- Common use cases and scenarios
- Troubleshooting tips

**When to use**:
- Learning how to use the MCP tools
- Understanding tool capabilities
- Finding examples and best practices

### 3. Authentication Guide (`oneapi://docs/authentication-guide`)
**Purpose**: Security and authentication best practices

**Content**:
- API key management
- Authentication methods
- Security considerations
- Rate limiting information
- Error handling for auth failures

**When to use**:
- Setting up authentication
- Implementing security best practices
- Troubleshooting authentication issues

### 4. Integration Patterns (`oneapi://docs/integration-patterns`)
**Purpose**: Real-world integration examples and patterns

**Content**:
- Common integration patterns
- Code examples in multiple languages
- Architecture recommendations
- Performance optimization tips
- Error handling strategies

**When to use**:
- Implementing integrations
- Learning best practices
- Finding code examples
- Architecture planning

## Usage Workflow

### Typical MCP Client Workflow:

1. **Start with Resources** for general understanding:
   ```
   Read resource: oneapi://docs/api-endpoints
   Read resource: oneapi://docs/tool-usage-guide
   ```

2. **Use Tools** for specific implementations:
   ```
   Call tool: chat_completions with your parameters
   Call tool: embeddings with your parameters
   ```

3. **Reference Resources** for troubleshooting:
   ```
   Read resource: oneapi://docs/authentication-guide
   Read resource: oneapi://docs/integration-patterns
   ```

## Template System

All resources use the Go template system with the following data structure:

```go
type TemplateData struct {
    BaseURL    string         // Your API base URL
    Parameters map[string]any // Additional context data
}
```

### Template Variables Available:
- `{{.BaseURL}}` - Your configured API base URL
- `{{.Parameters.ServerName}}` - MCP server name
- `{{.Parameters.ServerVersion}}` - MCP server version
- `{{.Parameters.AvailableTools}}` - List of available tools

## Implementation Details

### Resource URIs
Resources use the `oneapi://` URI scheme:
- `oneapi://docs/api-endpoints`
- `oneapi://docs/tool-usage-guide`
- `oneapi://docs/authentication-guide`
- `oneapi://docs/integration-patterns`

### MIME Type
All resources return `text/markdown` content.

### Template Location
Resource templates are stored in `mcp/docs/resources/` and embedded at compile time.

## Best Practices

### For MCP Client Developers:
1. **Cache resources** - They change infrequently
2. **Read resources first** - Get context before using tools
3. **Use tools for specifics** - Generate parameter-specific docs
4. **Reference resources for troubleshooting** - They contain comprehensive guides

### For API Users:
1. **Start with API Endpoints resource** - Understand what's available
2. **Read Tool Usage Guide** - Learn how to use tools effectively
3. **Use tools with your parameters** - Get customized documentation
4. **Keep Authentication Guide handy** - For security best practices

## Error Handling

If a resource template fails to load:
- A fallback message is returned explaining the issue
- The system remains functional
- Other resources continue to work normally

## Extending Resources

To add new resources:
1. Create a new template in `mcp/docs/resources/`
2. Add the resource registration in `addDocumentationResources()`
3. Update the embedded filesystem to include the new template
4. Update this documentation

## Integration Examples

### Python MCP Client
```python
# Read a resource
resource = await client.read_resource("oneapi://docs/api-endpoints")
print(resource.contents[0].text)

# Use a tool with parameters
result = await client.call_tool("chat_completions", {
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}]
})
```

### Node.js MCP Client
```javascript
// Read a resource
const resource = await client.readResource("oneapi://docs/tool-usage-guide");
console.log(resource.contents[0].text);

// Use a tool with parameters
const result = await client.callTool("embeddings", {
    model: "text-embedding-ada-002",
    input: "Sample text to embed"
});
```

This documentation system provides both static reference materials (resources) and dynamic, parameter-specific documentation (tools), giving users the flexibility to choose the right approach for their needs.
