# One API Official MCP Documentation Template System

## Overview

The One API Official MCP documentation system by [H0llyW00dzZ](https://github.com/H0llyW00dzZ) has been refactored to use Go's magic embed file system with template-based documentation generation. This approach provides better maintainability, scalability, and separation of concerns.

**Status: ✅ COMPLETED** - The template system is now fully implemented and operational.

## Architecture

### Template Structure
```
mcp/docs/templates/
├── chat_completions.tmpl   # Chat completions API
├── completions.tmpl        # Text completions API
├── embeddings.tmpl         # Embeddings API
├── images.tmpl             # Image generation API
├── audio_transcriptions.tmpl
├── audio_translations.tmpl
├── audio_speech.tmpl
├── moderations.tmpl
├── models_list.tmpl
└── claude_messages.tmpl
```

### Key Components

1. **Embedded File System**: Templates are embedded at compile time using `//go:embed`
2. **Template Renderer**: `DocumentationRenderer` manages template loading and rendering
3. **Template Data**: `TemplateData` struct provides dynamic content injection
4. **Backward Compatibility**: Fallback to original functions if templates fail

## Adding New API Documentation

### Step 1: Create Template File
Create a new `.tmpl` file in `docs/templates/` following this structure:

```markdown
# {{API Name}} API

## Endpoint
{{METHOD}} {{.BaseURL}}/{{endpoint}}

## Description
{{Description of the API}}

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
```bash
curl {{.BaseURL}}/{{endpoint}} \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{{example_json}}'
```

## Parameters
- **param1** (required): Description
- **param2**: Optional parameter description
```

### Step 2: Template Auto-Discovery
Templates are automatically discovered using the embed file system. No manual registration required.

### Step 3: Add Template Function
Create a corresponding function in `magic_documentation.go`:

```go
func generateNewAPIDocumentationFromTemplate(baseURL string) string {
    if globalRenderer == nil {
        return generateNewAPIDocumentation(baseURL)
    }
    
    doc, err := globalRenderer.RenderDocumentation("new_api", TemplateData{BaseURL: baseURL})
    if err != nil {
        return generateNewAPIDocumentation(baseURL)
    }
    return doc
}

// Fallback function (hardcoded)
func generateNewAPIDocumentation(baseURL string) string {
    return fmt.Sprintf(`# New API Documentation
    
## Endpoint
POST %s/v1/new-endpoint

## Description
Description of the new API endpoint.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
` + "```" + `bash
curl %s/v1/new-endpoint \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "param": "value"
  }'
` + "```" + `

## Parameters
- **param** (required): Parameter description`, baseURL, baseURL)
}
```

### Step 4: Update Handlers
Add the new tool in `handlers.go`:

```go
// Define argument structure
type NewAPIArgs struct {
    Param string `json:"param" jsonschema_description:"Parameter description" jsonschema_required:"true"`
}

// Add tool to server
mcp.AddTool(s.server, &mcp.Tool{
    Name:        "new_api",
    Description: "Description of the new API endpoint",
}, func(ctx context.Context, req *mcp.CallToolRequest, args NewAPIArgs) (*mcp.CallToolResult, any, error) {
    baseURL := getBaseURL()
    doc := generateNewAPIDocumentationFromTemplate(baseURL)
    
    return &mcp.CallToolResult{
        Content: []mcp.Content{
            &mcp.TextContent{Text: doc},
        },
    }, nil, nil
})
```

## Template Variables

### Available Variables
- `{{.BaseURL}}`: Dynamic base URL from configuration

### Future Enhancements
The `TemplateData` struct can be extended with additional fields:

```go
type TemplateData struct {
    BaseURL     string
    APIVersion  string
    Examples    map[string]interface{}
    Models      []string
    Features    []string
}
```

## Best Practices

### Template Design
1. **Consistent Structure**: Follow the standard sections (Endpoint → Description → Authentication → Example → Parameters)
2. **Clear Examples**: Provide realistic, working examples
3. **Parameter Documentation**: Clearly mark required vs optional parameters
4. **Error Handling**: Templates should gracefully handle missing data

### Code Organization
1. **Naming Convention**: Template files should match their function names
2. **Error Handling**: Always provide fallback mechanisms
3. **Testing**: Add tests for each new template
4. **Documentation**: Update this guide when adding new features

## Testing Templates

### Template Validation Test
```go
func TestNewAPITemplate(t *testing.T) {
    renderer, err := NewDocumentationRenderer()
    assert.NoError(t, err)
    
    doc, err := renderer.RenderDocumentation("new_api", TemplateData{
        BaseURL: "https://test.example.com",
    })
    
    assert.NoError(t, err)
    assert.Contains(t, doc, "# New API")
    assert.Contains(t, doc, "https://test.example.com")
    assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
}
```

## Migration from Hardcoded Documentation

### ✅ COMPLETED: Migration Status
The migration has been successfully completed:

- ✅ **Phase 1**: Template system implemented with backward compatibility
- ✅ **Phase 2**: Handlers updated to use template-based functions
- ✅ **Phase 3**: All tests passing, benchmarks fixed, documentation updated

### Current Implementation
- All handlers now use `generateXXXDocumentationFromTemplate()` functions
- Automatic fallback to hardcoded functions if templates fail
- Full backward compatibility maintained
- Template system validated and tested

## Troubleshooting

### Common Issues

1. **Template Not Found**: Ensure template file is in `docs/templates/` with `.tmpl` extension
2. **Rendering Errors**: Check template syntax and variable names using Go template syntax
3. **Fallback Behavior**: System automatically falls back to hardcoded functions if template loading fails
4. **Initialization Errors**: Check that `globalRenderer` is properly initialized in `init()` function

### Debug Tips

1. **Check Initialization**: Verify `globalRenderer` is not nil
2. **Template Validation**: Templates are validated during `NewDocumentationRenderer()`
3. **Test Template Rendering**: Use `magic_documentation_test.go` as reference
4. **Fallback Testing**: Test both template and fallback code paths

## Performance Considerations

- Templates are loaded once at initialization
- Embedded files provide zero-cost runtime access
- Template compilation happens at startup
- Consider template caching for high-frequency usage

## Security Notes

- Templates use `text/template` package (safe for markdown)
- No user input directly passed to templates
- BaseURL is the only dynamic content, properly escaped
- Embedded files prevent runtime template injection
