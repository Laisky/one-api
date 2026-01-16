# Requirement: MCP Aggregator Management Layer

## Background

The core function of the one-api project is to aggregate upstream LLM APIs and provide a unified interface for downstream users. The components responsible for aggregating upstream LLM APIs are called channels/adaptors.

However, with the development of the Model Context Protocol (MCP), many remote MCP Servers have emerged. Each MCP provides a series of tools that can be injected into LLM requests as function calling/tools, greatly expanding the capabilities of LLMs.
The problem is that different MCP Servers have their own authentication and billing systems. Users who want to use tools from multiple MCP Servers must register and recharge separately, which is inconvenient.

As an LLM API aggregator, one-api should allow administrators to conveniently aggregate multiple MCP Servers—just like channels/adaptors—and unify all tools provided by MCP servers as built-in tools for downstream users.

## Requirements

- Add a top-level navigation page named "MCPs", similar to "channels", displaying a list of all configured MCP Servers.
- Allow adding/editing/deleting MCP Server configurations, with the following fields:
  - Name
  - Description
  - MCP Server Base URL
  - API Key or Token (optional)
  - MCP Server communication type, defaulting to Streamable HTTP, but designed to support other protocols (e.g., SSE) in a pluggable way
  - Tools whitelist/blacklist configuration, allowing admins to specify which tools are allowed or forbidden
  - Tool fee configuration, allowing admins to set individual fees for each tool (if the MCP Server provides fee info, admins can choose to use the server's fees or override with custom fees)
- In the user request handling flow, allow calling MCP Server tools as built-in tools
- In the billing system, correctly track and charge for MCP Server tool usage
- In the logging system, record and query logs of MCP Server tool invocations
- Add a `/mcp` API endpoint to implement a fully functional Streamable HTTP MCP Server, using https://github.com/mark3labs/mcp-go if needed

## References

- [Claude MessagesAPI Built-in tools usage example](../refs/claude_builtin_tools.md)
- [OpenAI ResponseAPI Built-in tools usage example](../refs/openai_builtin_tools.md)
