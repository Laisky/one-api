# Tool use with Claude

- https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/overview#single-tool-example

Claude is capable of interacting with tools and functions, allowing you to extend Claude's capabilities to perform a wider variety of tasks.

<Tip>
  Learn everything you need to master tool use with Claude as part of the new [courses](https://anthropic.skilljar.com/)! Please
  continue to share your ideas and suggestions using this
  [form](https://forms.gle/BFnYc6iCkWoRzFgk7).
</Tip>

<Tip>
**Guarantee schema conformance with strict tool use**

[Structured Outputs](/docs/en/build-with-claude/structured-outputs) provides guaranteed schema validation for tool inputs. Add `strict: true` to your tool definitions to ensure Claude's tool calls always match your schema exactly, eliminating type mismatches or missing fields.

Perfect for production agents where invalid tool parameters would cause failures. [Learn when to use strict tool use →](/docs/en/build-with-claude/structured-outputs#when-to-use-json-outputs-vs-strict-tool-use)
</Tip>

Here's an example of how to provide tools to Claude using the Messages API:

<CodeGroup>

```bash Shell
curl https://api.anthropic.com/v1/messages \
  -H "content-type: application/json" \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-opus-4-6",
    "max_tokens": 1024,
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather in a given location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city and state, e.g. San Francisco, CA"
            }
          },
          "required": ["location"]
        }
      }
    ],
    "messages": [
      {
        "role": "user",
        "content": "What is the weather like in San Francisco?"
      }
    ]
  }'
```

```python Python
import anthropic

client = anthropic.Anthropic()

response = client.messages.create(
    model="claude-opus-4-6",
    max_tokens=1024,
    tools=[
        {
            "name": "get_weather",
            "description": "Get the current weather in a given location",
            "input_schema": {
                "type": "object",
                "properties": {
                    "location": {
                        "type": "string",
                        "description": "The city and state, e.g. San Francisco, CA",
                    }
                },
                "required": ["location"],
            },
        }
    ],
    messages=[{"role": "user", "content": "What's the weather like in San Francisco?"}],
)
print(response)
```

```typescript TypeScript
import { Anthropic } from '@anthropic-ai/sdk';

const anthropic = new Anthropic({
  apiKey: process.env.ANTHROPIC_API_KEY,
});

async function main() {
  const response = await anthropic.messages.create({
    model: 'claude-opus-4-6',
    max_tokens: 1024,
    tools: [
      {
        name: 'get_weather',
        description: 'Get the current weather in a given location',
        input_schema: {
          type: 'object',
          properties: {
            location: {
              type: 'string',
              description: 'The city and state, e.g. San Francisco, CA',
            },
          },
          required: ['location'],
        },
      },
    ],
    messages: [
      {
        role: 'user',
        content: 'Tell me the weather in San Francisco.',
      },
    ],
  });

  console.log(response);
}

main().catch(console.error);
```

```java Java
import com.anthropic.client.AnthropicClient;
import com.anthropic.client.okhttp.AnthropicOkHttpClient;
import com.anthropic.core.JsonValue;
import com.anthropic.models.messages.Message;
import com.anthropic.models.messages.MessageCreateParams;
import com.anthropic.models.messages.Model;
import com.anthropic.models.messages.Tool;
import com.anthropic.models.messages.Tool.InputSchema;
import java.util.List;
import java.util.Map;

public class GetWeatherExample {

  public static void main(String[] args) {
    AnthropicClient client = AnthropicOkHttpClient.fromEnv();

    InputSchema schema = InputSchema.builder()
      .properties(
        JsonValue.from(
          Map.of(
            "location",
            Map.of(
              "type",
              "string",
              "description",
              "The city and state, e.g. San Francisco, CA"
            )
          )
        )
      )
      .putAdditionalProperty("required", JsonValue.from(List.of("location")))
      .build();

    MessageCreateParams params = MessageCreateParams.builder()
      .model(Model.CLAUDE_OPUS_4_0)
      .maxTokens(1024)
      .addTool(
        Tool.builder()
          .name("get_weather")
          .description("Get the current weather in a given location")
          .inputSchema(schema)
          .build()
      )
      .addUserMessage("What's the weather like in San Francisco?")
      .build();

    Message message = client.messages().create(params);
    System.out.println(message);
  }
}
```

</CodeGroup>

## How tool use works

Claude supports two types of tools:

1. **Client tools**: Tools that execute on your systems, which include:
   - User-defined custom tools that you create and implement
   - Anthropic-defined tools like [computer use](/docs/en/agents-and-tools/tool-use/computer-use-tool) and [text editor](/docs/en/agents-and-tools/tool-use/text-editor-tool) that require client implementation

2. **Server tools**: Tools that execute on Anthropic's servers, like the [web search](/docs/en/agents-and-tools/tool-use/web-search-tool) and [web fetch](/docs/en/agents-and-tools/tool-use/web-fetch-tool) tools. These tools must be specified in the API request but don't require implementation on your part.

<Note>
Anthropic-defined tools use versioned types (e.g., `web_search_20250305`, `text_editor_20250124`) to ensure compatibility across model versions.
</Note>

### Client tools

Integrate client tools with Claude in these steps:

<Steps>
  <Step title="Provide Claude with tools and a user prompt">
    - Define client tools with names, descriptions, and input schemas in your API request.
    - Include a user prompt that might require these tools, e.g., "What's the weather in San Francisco?"
  </Step>
  <Step title="Claude decides to use a tool">
    - Claude assesses if any tools can help with the user's query.
    - If yes, Claude constructs a properly formatted tool use request.
    - For client tools, the API response has a `stop_reason` of `tool_use`, signaling Claude's intent.
  </Step>
  <Step title="Execute the tool and return results">
    - Extract the tool name and input from Claude's request
    - Execute the tool code on your system
    - Return the results in a new `user` message containing a `tool_result` content block
  </Step>
  <Step title="Claude uses tool result to formulate a response">
    - Claude analyzes the tool results to craft its final response to the original user prompt.
  </Step>
</Steps>
Note: Steps 3 and 4 are optional. For some workflows, Claude's tool use request (step 2) might be all you need, without sending results back to Claude.

### Server tools

Server tools follow a different workflow where Anthropic's servers handle tool execution in a loop:

<Steps>
  <Step title="Provide Claude with tools and a user prompt">
    - Server tools, like [web search](/docs/en/agents-and-tools/tool-use/web-search-tool) and [web fetch](/docs/en/agents-and-tools/tool-use/web-fetch-tool), have their own parameters.
    - Include a user prompt that might require these tools, e.g., "Search for the latest news about AI" or "Analyze the content at this URL."
  </Step>
  <Step title="Claude executes the server tool">
    - Claude assesses if a server tool can help with the user's query.
    - If yes, Claude executes the tool, and the results are automatically incorporated into Claude's response.
    - The server runs a sampling loop that may execute multiple tool calls before returning a response.
  </Step>
  <Step title="Claude uses the server tool result to formulate a response">
    - Claude analyzes the server tool results to craft its final response to the original user prompt.
    - In most cases, no additional user interaction is needed for server tool execution.
  </Step>
</Steps>

<Note>
**Handling `pause_turn` with server tools**

The server-side sampling loop has a default limit of 10 iterations. If Claude reaches this limit while executing server tools, the API returns a response with `stop_reason="pause_turn"`. This may include a `server_tool_use` block without a corresponding `server_tool_result`.

When you receive `pause_turn`, continue the conversation by sending the response back to let Claude finish processing. See [handling stop reasons](/docs/en/build-with-claude/handling-stop-reasons#3-implement-retry-logic-for-pause-turn) for implementation details.
</Note>

## Using MCP tools with Claude

If you're building an application that uses the [Model Context Protocol (MCP)](https://modelcontextprotocol.io), you can use tools from MCP servers directly with Claude's Messages API. MCP tool definitions use a schema format that's similar to Claude's tool format. You just need to rename `inputSchema` to `input_schema`.

<Tip>
**Don't want to build your own MCP client?** Use the [MCP connector](/docs/en/agents-and-tools/mcp-connector) to connect directly to remote MCP servers from the Messages API without implementing a client.
</Tip>

### Converting MCP tools to Claude format

When you build an MCP client and call `list_tools()` on an MCP server, you'll receive tool definitions with an `inputSchema` field. To use these tools with Claude, convert them to Claude's format:

<CodeGroup>
```python Python
from mcp import ClientSession

async def get_claude_tools(mcp_session: ClientSession):
"""Convert MCP tools to Claude's tool format."""
mcp_tools = await mcp_session.list_tools()

    claude_tools = []
    for tool in mcp_tools.tools:
        claude_tools.append(
            {
                "name": tool.name,
                "description": tool.description or "",
                "input_schema": tool.inputSchema,  # Rename inputSchema to input_schema
            }
        )

    return claude_tools

````

```typescript TypeScript
import { Client } from "@modelcontextprotocol/sdk/client/index.js";

async function getClaudeTools(mcpClient: Client) {
  // Convert MCP tools to Claude's tool format
  const mcpTools = await mcpClient.listTools();

  return mcpTools.tools.map((tool) => ({
    name: tool.name,
    description: tool.description ?? "",
    input_schema: tool.inputSchema // Rename inputSchema to input_schema
  }));
}
````

</CodeGroup>

Then pass these converted tools to Claude:

<CodeGroup>
```python Python
import anthropic

client = anthropic.Anthropic()
claude_tools = await get_claude_tools(mcp_session)

response = client.messages.create(
model="claude-opus-4-6",
max_tokens=1024,
tools=claude_tools,
messages=[{"role": "user", "content": "What tools do you have available?"}],
)

````

```typescript TypeScript
import Anthropic from "@anthropic-ai/sdk";

const anthropic = new Anthropic();
const claudeTools = await getClaudeTools(mcpClient);

const response = await anthropic.messages.create({
  model: "claude-opus-4-6",
  max_tokens: 1024,
  tools: claudeTools,
  messages: [{ role: "user", content: "What tools do you have available?" }]
});
````

</CodeGroup>

When Claude responds with a `tool_use` block, execute the tool on your MCP server using `call_tool()` and return the result to Claude in a `tool_result` block.

For a complete guide to building MCP clients, see [Build an MCP client](https://modelcontextprotocol.io/docs/develop/build-client).

## Tool use examples

Here are a few code examples demonstrating various tool use patterns and techniques. For brevity's sake, the tools are simple tools, and the tool descriptions are shorter than would be ideal to ensure best performance.

<section title="Single tool example">

<CodeGroup>
    ```bash Shell
    curl https://api.anthropic.com/v1/messages \
         --header "x-api-key: $ANTHROPIC_API_KEY" \
         --header "anthropic-version: 2023-06-01" \
         --header "content-type: application/json" \
         --data \
    '{
        "model": "claude-opus-4-6",
        "max_tokens": 1024,
        "tools": [{
            "name": "get_weather",
            "description": "Get the current weather in a given location",
            "input_schema": {
                "type": "object",
                "properties": {
                    "location": {
                        "type": "string",
                        "description": "The city and state, e.g. San Francisco, CA"
                    },
                    "unit": {
                        "type": "string",
                        "enum": ["celsius", "fahrenheit"],
                        "description": "The unit of temperature, either \"celsius\" or \"fahrenheit\""
                    }
                },
                "required": ["location"]
            }
        }],
        "messages": [{"role": "user", "content": "What is the weather like in San Francisco?"}]
    }'
    ```

    ```python Python
    import anthropic

    client = anthropic.Anthropic()

    response = client.messages.create(
        model="claude-opus-4-6",
        max_tokens=1024,
        tools=[
            {
                "name": "get_weather",
                "description": "Get the current weather in a given location",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA",
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": 'The unit of temperature, either "celsius" or "fahrenheit"',
                        },
                    },
                    "required": ["location"],
                },
            }
        ],
        messages=[
            {"role": "user", "content": "What is the weather like in San Francisco?"}
        ],
    )

    print(response)
    ```

    ```java Java
    import com.anthropic.client.AnthropicClient;
    import com.anthropic.client.okhttp.AnthropicOkHttpClient;
    import com.anthropic.core.JsonValue;
    import com.anthropic.models.messages.Message;
    import com.anthropic.models.messages.MessageCreateParams;
    import com.anthropic.models.messages.Model;
    import com.anthropic.models.messages.Tool;
    import com.anthropic.models.messages.Tool.InputSchema;
    import java.util.List;
    import java.util.Map;

    public class WeatherToolExample {

      public static void main(String[] args) {
        AnthropicClient client = AnthropicOkHttpClient.fromEnv();

        InputSchema schema = InputSchema.builder()
          .properties(
            JsonValue.from(
              Map.of(
                "location",
                Map.of(
                  "type",
                  "string",
                  "description",
                  "The city and state, e.g. San Francisco, CA"
                ),
                "unit",
                Map.of(
                  "type",
                  "string",
                  "enum",
                  List.of("celsius", "fahrenheit"),
                  "description",
                  "The unit of temperature, either \"celsius\" or \"fahrenheit\""
                )
              )
            )
          )
          .putAdditionalProperty("required", JsonValue.from(List.of("location")))
          .build();

        MessageCreateParams params = MessageCreateParams.builder()
          .model(Model.CLAUDE_OPUS_4_0)
          .maxTokens(1024)
          .addTool(
            Tool.builder()
              .name("get_weather")
              .description("Get the current weather in a given location")
              .inputSchema(schema)
              .build()
          )
          .addUserMessage("What is the weather like in San Francisco?")
          .build();

        Message message = client.messages().create(params);
        System.out.println(message);
      }
    }
    ```

</CodeGroup>

Claude will return a response similar to:

```json JSON
{
  "id": "msg_01Aq9w938a90dw8q",
  "model": "claude-opus-4-6",
  "stop_reason": "tool_use",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "I'll check the current weather in San Francisco for you."
    },
    {
      "type": "tool_use",
      "id": "toolu_01A09q90qw90lq917835lq9",
      "name": "get_weather",
      "input": { "location": "San Francisco, CA", "unit": "celsius" }
    }
  ]
}
```

You would then need to execute the `get_weather` function with the provided input, and return the result in a new `user` message:

<CodeGroup>
    ```bash Shell
    curl https://api.anthropic.com/v1/messages \
         --header "x-api-key: $ANTHROPIC_API_KEY" \
         --header "anthropic-version: 2023-06-01" \
         --header "content-type: application/json" \
         --data \
    '{
        "model": "claude-opus-4-6",
        "max_tokens": 1024,
        "tools": [
            {
                "name": "get_weather",
                "description": "Get the current weather in a given location",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA"
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The unit of temperature, either \"celsius\" or \"fahrenheit\""
                        }
                    },
                    "required": ["location"]
                }
            }
        ],
        "messages": [
            {
                "role": "user",
                "content": "What is the weather like in San Francisco?"
            },
            {
                "role": "assistant",
                "content": [
                    {
                        "type": "text",
                        "text": "I'll check the current weather in San Francisco for you."
                    },
                    {
                        "type": "tool_use",
                        "id": "toolu_01A09q90qw90lq917835lq9",
                        "name": "get_weather",
                        "input": {
                            "location": "San Francisco, CA",
                            "unit": "celsius"
                        }
                    }
                ]
            },
            {
                "role": "user",
                "content": [
                    {
                        "type": "tool_result",
                        "tool_use_id": "toolu_01A09q90qw90lq917835lq9",
                        "content": "15 degrees"
                    }
                ]
            }
        ]
    }'
    ```

    ```python Python
    response = client.messages.create(
        model="claude-opus-4-6",
        max_tokens=1024,
        tools=[
            {
                "name": "get_weather",
                "description": "Get the current weather in a given location",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA",
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The unit of temperature, either 'celsius' or 'fahrenheit'",
                        },
                    },
                    "required": ["location"],
                },
            }
        ],
        messages=[
            {"role": "user", "content": "What's the weather like in San Francisco?"},
            {
                "role": "assistant",
                "content": [
                    {
                        "type": "text",
                        "text": "I'll check the current weather in San Francisco for you.",
                    },
                    {
                        "type": "tool_use",
                        "id": "toolu_01A09q90qw90lq917835lq9",
                        "name": "get_weather",
                        "input": {"location": "San Francisco, CA", "unit": "celsius"},
                    },
                ],
            },
            {
                "role": "user",
                "content": [
                    {
                        "type": "tool_result",
                        "tool_use_id": "toolu_01A09q90qw90lq917835lq9",  # from the API response
                        "content": "65 degrees",  # from running your tool
                    }
                ],
            },
        ],
    )

    print(response)
    ```

```java Java
import com.anthropic.client.AnthropicClient;
import com.anthropic.client.okhttp.AnthropicOkHttpClient;
import com.anthropic.core.JsonValue;
import com.anthropic.models.messages.*;
import com.anthropic.models.messages.Tool.InputSchema;
import java.util.List;
import java.util.Map;

public class ToolConversationExample {

  public static void main(String[] args) {
    AnthropicClient client = AnthropicOkHttpClient.fromEnv();

    InputSchema schema = InputSchema.builder()
      .properties(
        JsonValue.from(
          Map.of(
            "location",
            Map.of(
              "type",
              "string",
              "description",
              "The city and state, e.g. San Francisco, CA"
            ),
            "unit",
            Map.of(
              "type",
              "string",
              "enum",
              List.of("celsius", "fahrenheit"),
              "description",
              "The unit of temperature, either \"celsius\" or \"fahrenheit\""
            )
          )
        )
      )
      .putAdditionalProperty("required", JsonValue.from(List.of("location")))
      .build();

    MessageCreateParams params = MessageCreateParams.builder()
      .model(Model.CLAUDE_OPUS_4_0)
      .maxTokens(1024)
      .addTool(
        Tool.builder()
          .name("get_weather")
          .description("Get the current weather in a given location")
          .inputSchema(schema)
          .build()
      )
      .addUserMessage("What is the weather like in San Francisco?")
      .addAssistantMessageOfBlockParams(
        List.of(
          ContentBlockParam.ofText(
            TextBlockParam.builder()
              .text("I'll check the current weather in San Francisco for you.")
              .build()
          ),
          ContentBlockParam.ofToolUse(
            ToolUseBlockParam.builder()
              .id("toolu_01A09q90qw90lq917835lq9")
              .name("get_weather")
              .input(
                JsonValue.from(Map.of("location", "San Francisco, CA", "unit", "celsius"))
              )
              .build()
          )
        )
      )
      .addUserMessageOfBlockParams(
        List.of(
          ContentBlockParam.ofToolResult(
            ToolResultBlockParam.builder()
              .toolUseId("toolu_01A09q90qw90lq917835lq9")
              .content("15 degrees")
              .build()
          )
        )
      )
      .build();

    Message message = client.messages().create(params);
    System.out.println(message);
  }
}
```

</CodeGroup>
This will print Claude's final response, incorporating the weather data:

```json JSON
{
  "id": "msg_01Aq9w938a90dw8q",
  "model": "claude-opus-4-6",
  "stop_reason": "stop_sequence",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "The current weather in San Francisco is 15 degrees Celsius (59 degrees Fahrenheit). It's a cool day in the city by the bay!"
    }
  ]
}
```

</section>
<section title="Parallel tool use">

Claude can call multiple tools in parallel within a single response, which is useful for tasks that require multiple independent operations. When using parallel tools, all `tool_use` blocks are included in a single assistant message, and all corresponding `tool_result` blocks must be provided in the subsequent user message.

<Note>
**Important**: Tool results must be formatted correctly to avoid API errors and ensure Claude continues using parallel tools. See the [implementation guide](/docs/en/agents-and-tools/tool-use/implement-tool-use#parallel-tool-use) for detailed formatting requirements and complete code examples.
</Note>

For comprehensive examples, test scripts, and best practices for implementing parallel tool calls, see the [parallel tool use section](/docs/en/agents-and-tools/tool-use/implement-tool-use#parallel-tool-use) in the implementation guide.

</section>
<section title="Multiple tool example">

You can provide Claude with multiple tools to choose from in a single request. Here's an example with both a `get_weather` and a `get_time` tool, along with a user query that asks for both.

<CodeGroup>
    ```bash Shell
    curl https://api.anthropic.com/v1/messages \
         --header "x-api-key: $ANTHROPIC_API_KEY" \
         --header "anthropic-version: 2023-06-01" \
         --header "content-type: application/json" \
         --data \
    '{
        "model": "claude-opus-4-6",
        "max_tokens": 1024,
        "tools": [{
            "name": "get_weather",
            "description": "Get the current weather in a given location",
            "input_schema": {
                "type": "object",
                "properties": {
                    "location": {
                        "type": "string",
                        "description": "The city and state, e.g. San Francisco, CA"
                    },
                    "unit": {
                        "type": "string",
                        "enum": ["celsius", "fahrenheit"],
                        "description": "The unit of temperature, either 'celsius' or 'fahrenheit'"
                    }
                },
                "required": ["location"]
            }
        },
        {
            "name": "get_time",
            "description": "Get the current time in a given time zone",
            "input_schema": {
                "type": "object",
                "properties": {
                    "timezone": {
                        "type": "string",
                        "description": "The IANA time zone name, e.g. America/Los_Angeles"
                    }
                },
                "required": ["timezone"]
            }
        }],
        "messages": [{
            "role": "user",
            "content": "What is the weather like right now in New York? Also what time is it there?"
        }]
    }'
    ```

    ```python Python
    import anthropic

    client = anthropic.Anthropic()

    response = client.messages.create(
        model="claude-opus-4-6",
        max_tokens=1024,
        tools=[
            {
                "name": "get_weather",
                "description": "Get the current weather in a given location",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA",
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The unit of temperature, either 'celsius' or 'fahrenheit'",
                        },
                    },
                    "required": ["location"],
                },
            },
            {
                "name": "get_time",
                "description": "Get the current time in a given time zone",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "timezone": {
                            "type": "string",
                            "description": "The IANA time zone name, e.g. America/Los_Angeles",
                        }
                    },
                    "required": ["timezone"],
                },
            },
        ],
        messages=[
            {
                "role": "user",
                "content": "What is the weather like right now in New York? Also what time is it there?",
            }
        ],
    )
    print(response)
    ```

    ```java Java
    import com.anthropic.client.AnthropicClient;
    import com.anthropic.client.okhttp.AnthropicOkHttpClient;
    import com.anthropic.core.JsonValue;
    import com.anthropic.models.messages.Message;
    import com.anthropic.models.messages.MessageCreateParams;
    import com.anthropic.models.messages.Model;
    import com.anthropic.models.messages.Tool;
    import com.anthropic.models.messages.Tool.InputSchema;
    import java.util.List;
    import java.util.Map;

    public class MultipleToolsExample {

      public static void main(String[] args) {
        AnthropicClient client = AnthropicOkHttpClient.fromEnv();

        // Weather tool schema
        InputSchema weatherSchema = InputSchema.builder()
          .properties(
            JsonValue.from(
              Map.of(
                "location",
                Map.of(
                  "type",
                  "string",
                  "description",
                  "The city and state, e.g. San Francisco, CA"
                ),
                "unit",
                Map.of(
                  "type",
                  "string",
                  "enum",
                  List.of("celsius", "fahrenheit"),
                  "description",
                  "The unit of temperature, either \"celsius\" or \"fahrenheit\""
                )
              )
            )
          )
          .putAdditionalProperty("required", JsonValue.from(List.of("location")))
          .build();

        // Time tool schema
        InputSchema timeSchema = InputSchema.builder()
          .properties(
            JsonValue.from(
              Map.of(
                "timezone",
                Map.of(
                  "type",
                  "string",
                  "description",
                  "The IANA time zone name, e.g. America/Los_Angeles"
                )
              )
            )
          )
          .putAdditionalProperty("required", JsonValue.from(List.of("timezone")))
          .build();

        MessageCreateParams params = MessageCreateParams.builder()
          .model(Model.CLAUDE_OPUS_4_0)
          .maxTokens(1024)
          .addTool(
            Tool.builder()
              .name("get_weather")
              .description("Get the current weather in a given location")
              .inputSchema(weatherSchema)
              .build()
          )
          .addTool(
            Tool.builder()
              .name("get_time")
              .description("Get the current time in a given time zone")
              .inputSchema(timeSchema)
              .build()
          )
          .addUserMessage(
            "What is the weather like right now in New York? Also what time is it there?"
          )
          .build();

        Message message = client.messages().create(params);
        System.out.println(message);
      }
    }
    ```

</CodeGroup>

In this case, Claude may either:

- Use the tools sequentially (one at a time), calling `get_weather` first, then `get_time` after receiving the weather result
- Use parallel tool calls, outputting multiple `tool_use` blocks in a single response when the operations are independent

When Claude makes parallel tool calls, you must return all tool results in a single `user` message, with each result in its own `tool_result` block.

</section>
<section title="Missing information">

If the user's prompt doesn't include enough information to fill all the required parameters for a tool, Claude Opus is much more likely to recognize that a parameter is missing and ask for it. Claude Sonnet may ask, especially when prompted to think before outputting a tool request. But it may also do its best to infer a reasonable value.

For example, using the `get_weather` tool above, if you ask Claude "What's the weather?" without specifying a location, Claude, particularly Claude Sonnet, may make a guess about tools inputs:

```json JSON
{
  "type": "tool_use",
  "id": "toolu_01A09q90qw90lq917835lq9",
  "name": "get_weather",
  "input": { "location": "New York, NY", "unit": "fahrenheit" }
}
```

This behavior is not guaranteed, especially for more ambiguous prompts and for less intelligent models. If Claude Opus doesn't have enough context to fill in the required parameters, it is far more likely respond with a clarifying question instead of making a tool call.

</section>
<section title="Sequential tools">

Some tasks may require calling multiple tools in sequence, using the output of one tool as the input to another. In such a case, Claude will call one tool at a time. If prompted to call the tools all at once, Claude is likely to guess parameters for tools further downstream if they are dependent on tool results for tools further upstream.

Here's an example of using a `get_location` tool to get the user's location, then passing that location to the `get_weather` tool:

<CodeGroup>
    ```bash Shell
    curl https://api.anthropic.com/v1/messages \
         --header "x-api-key: $ANTHROPIC_API_KEY" \
         --header "anthropic-version: 2023-06-01" \
         --header "content-type: application/json" \
         --data \
    '{
        "model": "claude-opus-4-6",
        "max_tokens": 1024,
        "tools": [
            {
                "name": "get_location",
                "description": "Get the current user location based on their IP address. This tool has no parameters or arguments.",
                "input_schema": {
                    "type": "object",
                    "properties": {}
                }
            },
            {
                "name": "get_weather",
                "description": "Get the current weather in a given location",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA"
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The unit of temperature, either 'celsius' or 'fahrenheit'"
                        }
                    },
                    "required": ["location"]
                }
            }
        ],
        "messages": [{
            "role": "user",
            "content": "What is the weather like where I am?"
        }]
    }'
    ```

    ```python Python
    response = client.messages.create(
        model="claude-opus-4-6",
        max_tokens=1024,
        tools=[
            {
                "name": "get_location",
                "description": "Get the current user location based on their IP address. This tool has no parameters or arguments.",
                "input_schema": {"type": "object", "properties": {}},
            },
            {
                "name": "get_weather",
                "description": "Get the current weather in a given location",
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA",
                        },
                        "unit": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The unit of temperature, either 'celsius' or 'fahrenheit'",
                        },
                    },
                    "required": ["location"],
                },
            },
        ],
        messages=[{"role": "user", "content": "What's the weather like where I am?"}],
    )
    ```

    ```java Java
    import com.anthropic.client.AnthropicClient;
    import com.anthropic.client.okhttp.AnthropicOkHttpClient;
    import com.anthropic.core.JsonValue;
    import com.anthropic.models.messages.Message;
    import com.anthropic.models.messages.MessageCreateParams;
    import com.anthropic.models.messages.Model;
    import com.anthropic.models.messages.Tool;
    import com.anthropic.models.messages.Tool.InputSchema;
    import java.util.List;
    import java.util.Map;

    public class EmptySchemaToolExample {

      public static void main(String[] args) {
        AnthropicClient client = AnthropicOkHttpClient.fromEnv();

        // Empty schema for location tool
        InputSchema locationSchema = InputSchema.builder()
          .properties(JsonValue.from(Map.of()))
          .build();

        // Weather tool schema
        InputSchema weatherSchema = InputSchema.builder()
          .properties(
            JsonValue.from(
              Map.of(
                "location",
                Map.of(
                  "type",
                  "string",
                  "description",
                  "The city and state, e.g. San Francisco, CA"
                ),
                "unit",
                Map.of(
                  "type",
                  "string",
                  "enum",
                  List.of("celsius", "fahrenheit"),
                  "description",
                  "The unit of temperature, either \"celsius\" or \"fahrenheit\""
                )
              )
            )
          )
          .putAdditionalProperty("required", JsonValue.from(List.of("location")))
          .build();

        MessageCreateParams params = MessageCreateParams.builder()
          .model(Model.CLAUDE_OPUS_4_0)
          .maxTokens(1024)
          .addTool(
            Tool.builder()
              .name("get_location")
              .description(
                "Get the current user location based on their IP address. This tool has no parameters or arguments."
              )
              .inputSchema(locationSchema)
              .build()
          )
          .addTool(
            Tool.builder()
              .name("get_weather")
              .description("Get the current weather in a given location")
              .inputSchema(weatherSchema)
              .build()
          )
          .addUserMessage("What is the weather like where I am?")
          .build();

        Message message = client.messages().create(params);
        System.out.println(message);
      }
    }
    ```

</CodeGroup>

In this case, Claude would first call the `get_location` tool to get the user's location. After you return the location in a `tool_result`, Claude would then call `get_weather` with that location to get the final answer.

The full conversation might look like:

| Role      | Content                                                                                                                                                                                                                       |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| User      | What's the weather like where I am?                                                                                                                                                                                           |
| Assistant | I'll find your current location first, then check the weather there. \[Tool use for get_location\]                                                                                                                            |
| User      | \[Tool result for get_location with matching id and result of San Francisco, CA\]                                                                                                                                             |
| Assistant | \[Tool use for get_weather with the following input\]\{ "location": "San Francisco, CA", "unit": "fahrenheit" }                                                                                                               |
| User      | \[Tool result for get_weather with matching id and result of "59°F (15°C), mostly cloudy"\]                                                                                                                                   |
| Assistant | Based on your current location in San Francisco, CA, the weather right now is 59°F (15°C) and mostly cloudy. It's a fairly cool and overcast day in the city. You may want to bring a light jacket if you're heading outside. |

This example demonstrates how Claude can chain together multiple tool calls to answer a question that requires gathering data from different sources. The key steps are:

1. Claude first realizes it needs the user's location to answer the weather question, so it calls the `get_location` tool.
2. The user (i.e. the client code) executes the actual `get_location` function and returns the result "San Francisco, CA" in a `tool_result` block.
3. With the location now known, Claude proceeds to call the `get_weather` tool, passing in "San Francisco, CA" as the `location` parameter (as well as a guessed `unit` parameter, as `unit` is not a required parameter).
4. The user again executes the actual `get_weather` function with the provided arguments and returns the weather data in another `tool_result` block.
5. Finally, Claude incorporates the weather data into a natural language response to the original question.

</section>
<section title="Chain of thought tool use">

By default, Claude Opus is prompted to think before it answers a tool use query to best determine whether a tool is necessary, which tool to use, and the appropriate parameters. Claude Sonnet and Claude Haiku are prompted to try to use tools as much as possible and are more likely to call an unnecessary tool or infer missing parameters. To prompt Sonnet or Haiku to better assess the user query before making tool calls, the following prompt can be used:

Chain of thought prompt

`Answer the user's request using relevant tools (if they are available). Before calling a tool, do some analysis. First, think about which of the provided tools is the relevant tool to answer the user's request. Second, go through each of the required parameters of the relevant tool and determine if the user has directly provided or given enough information to infer a value. When deciding if the parameter can be inferred, carefully consider all the context to see if it supports a specific value. If all of the required parameters are present or can be reasonably inferred, proceed with the tool call. BUT, if one of the values for a required parameter is missing, DO NOT invoke the function (not even with fillers for the missing params) and instead, ask the user to provide the missing parameters. DO NOT ask for more information on optional parameters if it is not provided.
`

</section>

## Pricing

Tool use requests are priced based on:

1. The total number of input tokens sent to the model (including in the `tools` parameter)
2. The number of output tokens generated
3. For server-side tools, additional usage-based pricing (e.g., web search charges per search performed)

Client-side tools are priced the same as any other Claude API request, while server-side tools may incur additional charges based on their specific usage.

The additional tokens from tool use come from:

- The `tools` parameter in API requests (tool names, descriptions, and schemas)
- `tool_use` content blocks in API requests and responses
- `tool_result` content blocks in API requests

When you use `tools`, we also automatically include a special system prompt for the model which enables tool use. The number of tool use tokens required for each model are listed below (excluding the additional tokens listed above). Note that the table assumes at least 1 tool is provided. If no `tools` are provided, then a tool choice of `none` uses 0 additional system prompt tokens.

| Model                                                                      | Tool choice                       | Tool use system prompt token count |
| -------------------------------------------------------------------------- | --------------------------------- | ---------------------------------- |
| Claude Opus 4.6                                                            | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Opus 4.5                                                            | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Opus 4.1                                                            | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Opus 4                                                              | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Sonnet 4.6                                                          | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Sonnet 4.5                                                          | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Sonnet 4                                                            | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Sonnet 3.7 ([deprecated](/docs/en/about-claude/model-deprecations)) | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Haiku 4.5                                                           | `auto`, `none`<hr />`any`, `tool` | 346 tokens<hr />313 tokens         |
| Claude Haiku 3.5                                                           | `auto`, `none`<hr />`any`, `tool` | 264 tokens<hr />340 tokens         |
| Claude Opus 3 ([deprecated](/docs/en/about-claude/model-deprecations))     | `auto`, `none`<hr />`any`, `tool` | 530 tokens<hr />281 tokens         |
| Claude Sonnet 3                                                            | `auto`, `none`<hr />`any`, `tool` | 159 tokens<hr />235 tokens         |
| Claude Haiku 3                                                             | `auto`, `none`<hr />`any`, `tool` | 264 tokens<hr />340 tokens         |

These token counts are added to your normal input and output tokens to calculate the total cost of a request.

Refer to the [models overview table](/docs/en/about-claude/models/overview#latest-models-comparison) for current per-model prices.

When you send a tool use prompt, just like any other API request, the response will output both input and output token counts as part of the reported `usage` metrics.

## Next Steps

Explore the repository of ready-to-implement tool use code examples in the cookbooks:

<CardGroup cols={3}>
  <Card
    title="Calculator Tool"
    icon="calculator"
    href="https://platform.claude.com/cookbook/tool-use-calculator-tool"
  >
    Learn how to integrate a simple calculator tool with Claude for precise numerical computations.
  </Card>

{" "}
<Card
title="Customer Service Agent"
icon="headset"
href="https://platform.claude.com/cookbook/tool-use-customer-service-agent"

> Build a responsive customer service bot that leverages client tools to
> enhance support.
> </Card>

<Card
title="JSON Extractor"
icon="code-brackets"
href="https://platform.claude.com/cookbook/tool-use-extracting-structured-json"

>

    See how Claude and tool use can extract structured data from unstructured text.

  </Card>
</CardGroup>
