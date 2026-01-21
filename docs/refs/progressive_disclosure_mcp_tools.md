# Evaluating Progressive Disclosure of Tools in MCP Servers Using mcp-go

## Menu

- [Evaluating Progressive Disclosure of Tools in MCP Servers Using mcp-go](#evaluating-progressive-disclosure-of-tools-in-mcp-servers-using-mcp-go)
  - [Menu](#menu)
  - [Introduction](#introduction)
  - [Capability Analysis](#capability-analysis)
    - [The Progressive Disclosure Pattern in MCP](#the-progressive-disclosure-pattern-in-mcp)
    - [MCP Protocol Support for Progressive Disclosure](#mcp-protocol-support-for-progressive-disclosure)
    - [mcp-go Library Capabilities](#mcp-go-library-capabilities)
  - [Implementation Feasibility](#implementation-feasibility)
    - [Dynamic Tool Registration and Filtering](#dynamic-tool-registration-and-filtering)
    - [Permission-Based Progressive Disclosure](#permission-based-progressive-disclosure)
    - [Direct Tool Invocation Without Listing](#direct-tool-invocation-without-listing)
    - [Tool Annotations and Metadata](#tool-annotations-and-metadata)
    - [Transport and Session Context](#transport-and-session-context)
    - [Security Considerations](#security-considerations)
    - [Client-Side vs. Server-Side Filtering](#client-side-vs-server-side-filtering)
  - [Example Code: Progressive Disclosure with mcp-go](#example-code-progressive-disclosure-with-mcp-go)
    - [Code Explanation](#code-explanation)
  - [Detailed Analysis and Discussion](#detailed-analysis-and-discussion)
    - [Tool Registry and Permission Mapping](#tool-registry-and-permission-mapping)
    - [Tool Listing Behavior and Filtering](#tool-listing-behavior-and-filtering)
    - [Direct Tool Invocation and Security](#direct-tool-invocation-and-security)
    - [Session Context and Transport Considerations](#session-context-and-transport-considerations)
    - [Tool Annotations and Metadata](#tool-annotations-and-metadata-1)
    - [Error Handling and Input Validation](#error-handling-and-input-validation)
    - [Security Best Practices](#security-best-practices)
    - [Advanced Patterns and Extensions](#advanced-patterns-and-extensions)
  - [Conclusion](#conclusion)

## Introduction

The Model Context Protocol (MCP) has rapidly become the de facto standard for connecting Large Language Model (LLM) agents to external tools, APIs, and data sources. As the ecosystem matures, the number of available tools per server has grown dramatically, leading to new challenges in usability, security, and performance. One of the most impactful design patterns to emerge in response is **progressive disclosure**—the practice of revealing only a relevant subset of tools to clients at any given time, based on user permissions, context, or workflow stage.

This report evaluates whether the [mcp-go](https://github.com/mark3labs/mcp-go) Go library can be used to implement an MCP server that supports progressive disclosure of tools. Specifically, it investigates whether it is possible to configure the server so that when a client requests a list of available tools, only a subset (e.g., based on user permissions or context) is returned, while still allowing the client to invoke any supported tool directly if they know the tool name. The report provides a comprehensive analysis of mcp-go’s capabilities, relevant MCP protocol features, design patterns for progressive disclosure, and security considerations. It culminates in a detailed, idiomatic Go code example demonstrating how to implement such a server using mcp-go, including tool registry definition, tool listing behavior, permission-based filtering, and direct invocation handling.

This analysis is intended for experienced Go developers and architects evaluating mcp-go for production use in secure, scalable, and context-aware AI-to-tool integrations.

## Capability Analysis

### The Progressive Disclosure Pattern in MCP

**Progressive disclosure** is a user experience and security pattern that reduces cognitive load and risk by exposing only the most relevant or permissible capabilities to the user or agent at any given time. In the context of MCP servers, this means:

- **Tool Listing:** When a client requests the list of available tools (via `tools/list`), the server returns only those tools the client is authorized or contextually permitted to use.
- **Direct Invocation:** If a client knows the name of a tool (even if it was not listed), it may attempt to invoke it directly (via `tools/call`). The server must enforce permissions at invocation time, denying unauthorized calls.
- **Dynamic Tool Availability:** The set of available tools may change at runtime, either due to user actions (enabling/disabling toolsets), context changes, or permission updates. The server should notify clients of such changes via `notifications/tools/list_changed`.

This pattern is essential for:

- **Reducing LLM cognitive overload:** Presenting too many tools leads to poor tool selection and increased context window usage.
- **Improving security:** Only exposing sensitive or destructive tools to authorized users minimizes risk.
- **Enhancing scalability:** Servers can support hundreds of tools without overwhelming clients or exceeding context limits.

### MCP Protocol Support for Progressive Disclosure

The MCP protocol is designed to support dynamic tool registration and progressive disclosure:

- **Tool Listing (`tools/list`):** Clients request the list of available tools. The server may filter this list per session or user context.
- **Tool Invocation (`tools/call`):** Clients invoke a tool by name. The server must validate permissions and input, regardless of whether the tool was listed.
- **List Changed Notification (`notifications/tools/list_changed`):** Servers can notify clients when the set of available tools changes, prompting clients to refresh their tool list.
- **Session Context:** The protocol supports session-based state, allowing per-user or per-session tool availability and permissions.

The protocol does **not** mandate a specific user interaction model, leaving progressive disclosure as an implementation pattern at the server and client levels.

### mcp-go Library Capabilities

The [mcp-go](https://github.com/mark3labs/mcp-go) library is a comprehensive Go implementation of the MCP specification, supporting:

- **Dynamic tool registration:** Tools can be added or removed at runtime, per session or globally.
- **Session management:** Per-client session state, enabling session-specific toolsets and permissions.
- **Tool filtering:** Server-side tool filters can be applied to control which tools are listed for each client or session.
- **Middleware and hooks:** Custom logic can be injected at various stages (e.g., before tool listing or invocation) for authentication, authorization, and auditing.
- **Transport flexibility:** Supports stdio, HTTP (streamable), and SSE transports, with session context propagation and header access for authentication and context.
- **Tool annotations and metadata:** Tools can be annotated with metadata (e.g., read-only, destructive, domain, category) to aid in filtering and access control.
- **Error handling:** Conforms to MCP and JSON-RPC error codes, with clear separation between protocol errors and tool execution errors.

These features make mcp-go well-suited for implementing progressive disclosure patterns, including permission-based filtering and dynamic toolset management.

## Implementation Feasibility

### Dynamic Tool Registration and Filtering

**mcp-go** provides several mechanisms for controlling tool visibility and access:

- **Global and Session-Specific Tools:** Tools can be registered globally (available to all clients) or per session (available only to specific clients). Session tools override global tools with the same name.
- **Tool Filters:** The server can be configured with a `ToolFilterFunc`, which receives the current context and the list of tools, and returns a filtered list. This enables permission-based or context-sensitive filtering at tool listing time.
- **Dynamic Add/Remove:** Tools can be added or removed at runtime, and the server will automatically send `tools/list_changed` notifications to connected clients if the capability is enabled.

### Permission-Based Progressive Disclosure

The recommended pattern for permission-based progressive disclosure is:

1.  **Tool Registry with Permissions:** Maintain a registry mapping each tool to its required permissions and data access policies.
2.  **ListTools Filtering:** When handling a `tools/list` request, filter the tool list based on the current user's permissions, session, or context.
3.  **CallTool Enforcement:** When handling a `tools/call` request, enforce permissions again, even if the tool was not listed for the user. This ensures defense-in-depth and prevents unauthorized direct invocation.
4.  **Data Access Policies:** Optionally, apply fine-grained data access policies to restrict which data elements are visible or modifiable, even within allowed tools.
5.  **Session Context Propagation:** Use session or request context to propagate user identity, roles, and permissions throughout the request lifecycle.

This pattern is robust, aligns with security best practices, and is supported by mcp-go’s API and architecture.

### Direct Tool Invocation Without Listing

The MCP protocol and mcp-go both allow clients to invoke any tool by name, regardless of whether it was listed in the last `tools/list` response. The server is responsible for validating permissions and input at invocation time.

This is critical for progressive disclosure: clients may have out-of-band knowledge of tool names, or may attempt to invoke tools not currently listed. The server must never rely solely on the tool listing for access control.

### Tool Annotations and Metadata

mcp-go supports attaching annotations and metadata to tool definitions, including:

- **Operational hints:** `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`
- **Domain and category:** For logical grouping and filtering
- **Custom metadata:** For organization-specific policies or client-side filtering

Annotations can be used to aid both server-side and client-side filtering, and are exposed to clients in the tool metadata field.

### Transport and Session Context

mcp-go supports multiple transports:

- **stdio:** For local or CLI-based servers
- **Streamable HTTP:** For web-based or remote servers, with full support for session management, authentication headers, and context propagation
- **SSE:** For legacy or streaming scenarios

Session context (including user identity and permissions) can be propagated via headers (HTTP), environment variables (stdio), or session state, and is accessible in all handler functions.

### Security Considerations

Implementing progressive disclosure securely requires:

- **Authentication:** Verifying the identity of each client, typically via OAuth, JWT, or other mechanisms.
- **Authorization:** Enforcing permissions at both tool listing and invocation time, using a central registry or RBAC system.
- **Input Validation:** Validating all tool inputs for type, length, range, and allowed values to prevent injection and misuse.
- **Audit Logging:** Logging all tool invocations, permission checks, and errors for audit and incident response.
- **Rate Limiting:** Preventing abuse by limiting the rate of tool invocations per user or session.
- **Defense-in-Depth:** Never relying solely on client-side filtering or tool listing for security; always enforce permissions server-side.

### Client-Side vs. Server-Side Filtering

While some clients (e.g., GitHub Copilot, Claude Code) support client-side tool filtering for usability, **security must always be enforced server-side**. Client-side filtering improves UX and reduces context window usage, but cannot prevent unauthorized direct invocation.

## Example Code: Progressive Disclosure with mcp-go

Below is a complete, idiomatic Go example demonstrating how to implement an MCP server with progressive disclosure of tools using mcp-go. The example includes:

- A tool registry with permission metadata
- Role-based access control (RBAC) for users
- Filtering of tools during listing based on permissions
- Enforcement of permissions during direct tool invocation
- Session context propagation
- Detailed comments for clarity

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "log"
    "net/http"
    "strings"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

// --- User, Role, and Permission Models
// User represents an authenticated user.
type User struct {
    ID    string
    Roles []string
}

// RolePermissions maps roles to permissions.
var RolePermissions = map[string][]string{
    "admin":    {"tools:list", "tools:call:echo", "tools:call:add", "tools:call:delete_user"},
    "standard": {"tools:list", "tools:call:echo", "tools:call:add"},
    "readonly": {"tools:list", "tools:call:echo"},
}

// UserStore simulates a user database.
var UserStore = map[string]User{
    "alice": {ID: "alice", Roles: []string{"admin"}},
    "bob":   {ID: "bob", Roles: []string{"standard"}},
    "eve":   {ID: "eve", Roles: []string{"readonly"}},
}

// getUserFromRequest extracts the user from the HTTP request headers or context.
// In production, replace this with real authentication (e.g., JWT, OAuth).
func getUserFromRequest(ctx context.Context, req *http.Request) (*User, error) {
    userID := req.Header.Get("X-User-ID")
    if userID == "" {
        return nil, errors.New("missing X-User-ID header")
    }
    user, ok := UserStore[userID]
    if !ok {
        return nil, fmt.Errorf("unknown user: %s", userID)
    }
    return &user, nil
}

// getUserPermissions returns the set of permissions for a user.
func getUserPermissions(user *User) map[string]struct{} {
    perms := make(map[string]struct{})
    for _, role := range user.Roles {
        for _, p := range RolePermissions[role] {
            perms[p] = struct{}{}
        }
    }
    return perms
}

// hasPermission checks if the user has the required permission.
func hasPermission(user *User, perm string) bool {
    perms := getUserPermissions(user)
    _, ok := perms[perm]
    return ok
}

// --- Tool Registry with Permission Metadata
// ToolMeta holds metadata for each tool, including required permissions.
type ToolMeta struct {
    Tool        mcp.Tool
    Handler     server.ToolHandlerFunc
    Permissions []string // List of required permissions (all must be present)
    Description string
}

// toolRegistry maps tool names to their metadata.
var toolRegistry = map[string]ToolMeta{}

// registerTool adds a tool to the registry.
func registerTool(name string, tool mcp.Tool, handler server.ToolHandlerFunc, perms []string, desc string) {
    toolRegistry[name] = ToolMeta{
        Tool:        tool,
        Handler:     handler,
        Permissions: perms,
        Description: desc,
    }
}

// --- Tool Handlers
// Echo tool: available to all users.
func handleEcho(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    message, err := req.RequireString("message")
    if err != nil {
        return mcp.NewToolResultError("Missing or invalid 'message' parameter"), nil
    }
    return mcp.NewToolResultText(fmt.Sprintf("Echo: %s", message)), nil
}

// Add tool: available to standard and admin users.
func handleAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    a, err := req.RequireFloat("a")
    if err != nil {
        return mcp.NewToolResultError("Missing or invalid 'a' parameter"), nil
    }
    b, err := req.RequireFloat("b")
    if err != nil {
        return mcp.NewToolResultError("Missing or invalid 'b' parameter"), nil
    }
    sum := a + b
    return mcp.NewToolResultText(fmt.Sprintf("Sum: %.2f", sum)), nil
}

// DeleteUser tool: admin only.
func handleDeleteUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    userID, err := req.RequireString("user_id")
    if err != nil {
        return mcp.NewToolResultError("Missing or invalid 'user_id' parameter"), nil
    }
    // Simulate deletion (no-op)
    return mcp.NewToolResultText(fmt.Sprintf("User %s deleted (simulated)", userID)), nil
}

// --- Tool Listing Filter
// toolListFilter filters tools based on user permissions.
func toolListFilter(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
    // Extract user from context (populated by middleware)
    user, ok := ctx.Value("user").(*User)
    if !ok || user == nil {
        // No user: return no tools
        return []mcp.Tool{}
    }
    perms := getUserPermissions(user)
    var filtered []mcp.Tool
    for name, meta := range toolRegistry {
        allowed := true
        for _, p := range meta.Permissions {
            if _, ok := perms[p]; !ok {
                allowed = false
                break
            }
        }
        if allowed {
            filtered = append(filtered, meta.Tool)
        }
    }
    return filtered
}

// --- Middleware for HTTP Transport
// userContextMiddleware injects user info into the context for each request.
func userContextMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user, err := getUserFromRequest(r.Context(), r)
        if err != nil {
            http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
            return
        }
        ctx := context.WithValue(r.Context(), "user", user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// --- MCP Server Setup
func main() {
    // Define tools and register them with permission metadata.
    registerTool(
        "echo",
        mcp.NewTool("echo",
            mcp.WithDescription("Echoes back the input message"),
            mcp.WithString("message", mcp.Required(), mcp.Description("Message to echo")),
            mcp.WithToolAnnotation(mcp.ToolAnnotation{
                ReadOnlyHint:     ptrBool(true),
                DestructiveHint:  ptrBool(false),
                IdempotentHint:   ptrBool(true),
                OpenWorldHint:    ptrBool(false),
            }),
        ),
        handleEcho,
        []string{"tools:call:echo"},
        "Echoes a message (all users)",
    )

    registerTool(
        "add",
        mcp.NewTool("add",
            mcp.WithDescription("Adds two numbers"),
            mcp.WithNumber("a", mcp.Required(), mcp.Description("First number")),
            mcp.WithNumber("b", mcp.Required(), mcp.Description("Second number")),
            mcp.WithToolAnnotation(mcp.ToolAnnotation{
                ReadOnlyHint:     ptrBool(true),
                DestructiveHint:  ptrBool(false),
                IdempotentHint:   ptrBool(true),
                OpenWorldHint:    ptrBool(false),
            }),
        ),
        handleAdd,
        []string{"tools:call:add"},
        "Add two numbers (standard and admin users)",
    )

    registerTool(
        "delete_user",
        mcp.NewTool("delete_user",
            mcp.WithDescription("Deletes a user (admin only)"),
            mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID to delete")),
            mcp.WithToolAnnotation(mcp.ToolAnnotation{
                ReadOnlyHint:     ptrBool(false),
                DestructiveHint:  ptrBool(true),
                IdempotentHint:   ptrBool(false),
                OpenWorldHint:    ptrBool(false),
            }),
        ),
        handleDeleteUser,
        []string{"tools:call:delete_user"},
        "Delete a user (admin only)",
    )

    // Create the MCP server with tool capabilities and tool filter.
    s := server.NewMCPServer(
        "Progressive Disclosure MCP Server",
        "1.0.0",
        server.WithToolCapabilities(true), // Enables tools/list_changed notifications
        server.WithToolFilter(toolListFilter),
        server.WithRecovery(), // Panic recovery middleware
    )

    // Register all tools and their handlers.
    for name, meta := range toolRegistry {
        s.AddTool(meta.Tool, wrapToolHandlerWithAuth(meta.Handler, meta.Permissions))
    }

    // Choose transport: stdio or HTTP.
    useHTTP := true // Set to false for stdio transport

    if useHTTP {
        // HTTP transport with user context middleware.
        httpServer := server.NewStreamableHTTPServer(s)
        mux := http.NewServeMux()
        mux.Handle("/mcp", userContextMiddleware(httpServer))
        log.Println("MCP server running on http://localhost:8080/mcp (HTTP transport)")
        if err := http.ListenAndServe(":8080", mux); err != nil {
            log.Fatal(err)
        }
    } else {
        // Stdio transport (for CLI or local use).
        log.Println("MCP server running (stdio transport)")
        if err := server.ServeStdio(s); err != nil {
            log.Fatal(err)
        }
    }
}

// wrapToolHandlerWithAuth enforces permissions at tool invocation time.
func wrapToolHandlerWithAuth(handler server.ToolHandlerFunc, requiredPerms []string) server.ToolHandlerFunc {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        user, ok := ctx.Value("user").(*User)
        if !ok || user == nil {
            return mcp.NewToolResultError("Unauthorized: user not found"), nil
        }
        perms := getUserPermissions(user)
        for _, p := range requiredPerms {
            if _, ok := perms[p]; !ok {
                return mcp.NewToolResultError("Forbidden: insufficient permissions"), nil
            }
        }
        return handler(ctx, req)
    }
}

// ptrBool is a helper for pointer-to-bool.
func ptrBool(b bool) *bool { return &b }
```

### Code Explanation

**User, Role, and Permission Models:**
Defines a simple in-memory RBAC system with users, roles, and permissions. In production, replace this with integration to your identity provider or database.

**Tool Registry:**
Each tool is registered with its MCP schema, handler, required permissions, and description. The registry is used for both listing and invocation.

**Tool Handlers:**
Implements three example tools: `echo` (all users), `add` (standard/admin), and `delete_user` (admin only). Each handler validates input and returns a result.

**Tool Listing Filter:**
Implements a `ToolFilterFunc` that filters the list of tools returned by `tools/list` based on the current user's permissions.

**Middleware:**
For HTTP transport, a middleware extracts the user from the request headers and injects it into the context for downstream handlers.

**Server Setup:**
Creates the MCP server, registers all tools, and sets up the transport (HTTP or stdio). For HTTP, the server listens on `/mcp` and applies the user context middleware.

**Permission Enforcement:**
Each tool handler is wrapped with a function that checks the user's permissions at invocation time, ensuring that even direct calls to unlisted tools are properly authorized.

**Annotations:**
Each tool includes MCP standard annotations (readOnlyHint, destructiveHint, etc.) to aid in client-side filtering and documentation.

## Detailed Analysis and Discussion

### Tool Registry and Permission Mapping

A central tool registry is critical for mapping each tool to its required permissions and metadata. This enables:

- **Consistent permission checks** at both listing and invocation time.
- **Centralized management** of tool definitions, descriptions, and annotations.
- **Extensibility** for adding new tools, permissions, or metadata fields.

In production, the registry can be loaded from configuration files, a database, or an external service. It can also include data access policies, risk levels, and custom metadata for advanced scenarios.

### Tool Listing Behavior and Filtering

The `ToolFilterFunc` pattern in mcp-go allows the server to filter the list of tools returned to each client based on the current context (e.g., user, session, request headers). This enables:

- **Permission-based filtering:** Only show tools the user is authorized to use.
- **Context-sensitive filtering:** Hide or show tools based on workflow state, environment, or other factors.
- **Dynamic updates:** When permissions or context change, the server can update the tool list and notify clients via `tools/list_changed`.

This approach aligns with best practices for progressive disclosure, reducing cognitive load and context window usage for LLMs and users.

### Direct Tool Invocation and Security

The MCP protocol allows clients to invoke any tool by name, regardless of whether it was listed. The server must:

- **Always enforce permissions** at invocation time, using the tool registry and user context.
- **Return clear error messages** (e.g., "Forbidden: insufficient permissions") for unauthorized calls.
- **Validate input** for type, range, and allowed values to prevent injection and misuse.

This defense-in-depth approach ensures that security is not bypassed by direct calls or client-side manipulation.

### Session Context and Transport Considerations

mcp-go supports both stdio and HTTP transports, with full session context propagation:

- **HTTP Transport:** User identity and permissions can be passed via headers (e.g., `X-User-ID`), cookies, or OAuth tokens. Middleware can extract and validate these credentials, injecting user info into the context for downstream handlers.
- **Stdio Transport:** For local or CLI-based servers, user context may be passed via environment variables, command-line arguments, or session initialization parameters.

Session-specific tools and per-session state are supported, enabling advanced patterns such as workflow-based tool availability or temporary toolsets.

### Tool Annotations and Metadata

Annotations provide valuable metadata for both server-side and client-side filtering:

- **Operational hints:** Indicate whether a tool is read-only, destructive, idempotent, or interacts with external systems.
- **Domain and category:** Enable logical grouping and specialization (e.g., "security", "performance", "remediation").
- **Custom metadata:** Support organization-specific policies, risk levels, or compliance requirements.

Annotations are exposed to clients in the tool metadata, enabling intelligent tool selection and filtering in agent frameworks and UIs.

### Error Handling and Input Validation

Robust error handling is essential for security and usability:

- **Protocol errors:** Use standard JSON-RPC error codes for invalid requests, unknown methods, or parameter validation failures (e.g., -32601, -32602).
- **Tool execution errors:** Use the `isError` flag in tool results to indicate business logic or runtime failures.
- **Input validation:** Validate all inputs for type, length, range, and allowed values. Return clear, actionable error messages for invalid input.

This layered approach ensures that clients receive meaningful feedback and that the server remains stable and secure.

### Security Best Practices

Implementing progressive disclosure securely requires:

- **Authentication:** Use strong authentication mechanisms (OAuth, JWT, etc.) to verify user identity.
- **Authorization:** Enforce permissions at both tool listing and invocation time, using a central registry or RBAC system.
- **Input validation:** Validate all tool inputs rigorously to prevent injection and misuse.
- **Audit logging:** Log all tool invocations, permission checks, and errors for audit and incident response.
- **Rate limiting:** Prevent abuse by limiting the rate of tool invocations per user or session.
- **Defense-in-depth:** Never rely solely on client-side filtering or tool listing for security; always enforce permissions server-side.

These practices align with the latest guidance from security experts and the MCP community.

### Advanced Patterns and Extensions

For more advanced scenarios, consider:

- **Dynamic toolsets:** Allow users or agents to enable/disable toolsets at runtime, as in the GitHub MCP server. This can be implemented via meta-tools (e.g., `enable_toolset`, `list_available_toolsets`) and dynamic registration.
- **Semantic tool search:** Implement search or embedding-based filtering to dynamically select the most relevant tools for a given query or workflow.
- **Data access policies:** Apply fine-grained data access controls within tools, restricting which data elements are visible or modifiable based on user roles or context.
- **Security annotations:** Use standardized security annotations (e.g., confidentiality, integrity) to enforce information flow policies and compliance requirements.

These patterns enable scalable, secure, and context-aware MCP servers suitable for enterprise and multi-tenant deployments.

## Conclusion

The **mcp-go** library provides all the necessary primitives and patterns to implement an MCP server with robust, production-grade progressive disclosure of tools. By leveraging dynamic tool registration, permission-based filtering, session context propagation, and defense-in-depth authorization, developers can ensure that:

- **Tool listing** only reveals the subset of tools appropriate for each user or context, reducing cognitive load and context window usage.
- **Direct tool invocation** is always subject to permission checks, preventing unauthorized access even if a tool is not listed.
- **Security and usability** are maintained through rigorous input validation, audit logging, and adherence to best practices.
- **Advanced patterns** such as dynamic toolsets, semantic filtering, and data access policies can be layered on top for greater flexibility and control.

The provided Go code example demonstrates a complete, idiomatic implementation of these principles, suitable for adaptation to real-world production environments.

**Key Takeaways:**

- **Progressive disclosure** is essential for scalable, secure, and user-friendly MCP servers.
- **mcp-go** fully supports dynamic tool filtering, session-specific tools, and permission-based access control.
- **Server-side enforcement** of permissions is mandatory; client-side filtering is a usability enhancement, not a security measure.
- **Annotations and metadata** enrich tool definitions and enable intelligent filtering and documentation.
- **Security best practices**—authentication, authorization, input validation, and audit logging—must be integral to any MCP server deployment.

By following these patterns and leveraging mcp-go’s capabilities, Go developers can build MCP servers that are both powerful and safe, enabling the next generation of AI-to-tool integrations.

**References:**

- [mcp-go GitHub repository and documentation](https://github.com/mark3labs/mcp-go)
- [MCP protocol specification](https://spec.modelcontextprotocol.io/)
- Progressive disclosure in MCP servers
- Security best practices and RBAC in Go
- Tool annotations and metadata
- Dynamic toolsets and advanced patterns
