# Building with extended thinking

> <https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#example-passing-thinking-blocks-with-tool-results>

Extended thinking gives Claude enhanced reasoning capabilities for complex tasks, while providing varying levels of transparency into its step-by-step thought process before it delivers its final answer.

## Supported models

Extended thinking is supported in the following models:

- Claude Opus 4 (`claude-opus-4-20250514`)
- Claude Sonnet 4 (`claude-sonnet-4-20250514`)
- Claude Sonnet 3.7 (`claude-3-7-sonnet-20250219`)

<Note>
  API behavior differs across Claude 3.7 and Claude 4 models, but the API shapes remain exactly the same.

For more information, see [Differences in thinking across model versions](#differences-in-thinking-across-model-versions).
</Note>

## How extended thinking works

When extended thinking is turned on, Claude creates `thinking` content blocks where it outputs its internal reasoning. Claude incorporates insights from this reasoning before crafting a final response.

The API response will include `thinking` content blocks, followed by `text` content blocks.

Here's an example of the default response format:

```json
{
  "content": [
    {
      "type": "thinking",
      "thinking": "Let me analyze this step by step...",
      "signature": "WaUjzkypQ2mUEVM36O2TxuC06KN8xyfbJwyem2dw3URve/op91XWHOEBLLqIOMfFG/UvLEczmEsUjavL...."
    },
    {
      "type": "text",
      "text": "Based on my analysis..."
    }
  ]
}
```

For more information about the response format of extended thinking, see the [Messages API Reference](/en/api/messages).

## How to use extended thinking

Here is an example of using extended thinking in the Messages API:

<CodeGroup>
  ```bash Shell
  curl https://api.anthropic.com/v1/messages \
       --header "x-api-key: $ANTHROPIC_API_KEY" \
       --header "anthropic-version: 2023-06-01" \
       --header "content-type: application/json" \
       --data \
  '{
      "model": "claude-sonnet-4-20250514",
      "max_tokens": 16000,
      "thinking": {
          "type": "enabled",
          "budget_tokens": 10000
      },
      "messages": [
          {
              "role": "user",
              "content": "Are there an infinite number of prime numbers such that n mod 4 == 3?"
          }
      ]
  }'
  ```

```python Python
import anthropic

client = anthropic.Anthropic()

response = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=16000,
    thinking={
        "type": "enabled",
        "budget_tokens": 10000
    },
    messages=[{
        "role": "user",
        "content": "Are there an infinite number of prime numbers such that n mod 4 == 3?"
    }]
)

# The response will contain summarized thinking blocks and text blocks
for block in response.content:
    if block.type == "thinking":
        print(f"\nThinking summary: {block.thinking}")
    elif block.type == "text":
        print(f"\nResponse: {block.text}")
```

```typescript TypeScript
import Anthropic from '@anthropic-ai/sdk';

const client = new Anthropic();

const response = await client.messages.create({
  model: 'claude-sonnet-4-20250514',
  max_tokens: 16000,
  thinking: {
    type: 'enabled',
    budget_tokens: 10000,
  },
  messages: [
    {
      role: 'user',
      content: 'Are there an infinite number of prime numbers such that n mod 4 == 3?',
    },
  ],
});

// The response will contain summarized thinking blocks and text blocks
for (const block of response.content) {
  if (block.type === 'thinking') {
    console.log(`\nThinking summary: ${block.thinking}`);
  } else if (block.type === 'text') {
    console.log(`\nResponse: ${block.text}`);
  }
}
```

```java Java
import com.anthropic.client.AnthropicClient;
import com.anthropic.client.okhttp.AnthropicOkHttpClient;
import com.anthropic.models.beta.messages.*;
import com.anthropic.models.beta.messages.MessageCreateParams;
import com.anthropic.models.messages.*;

public class SimpleThinkingExample {
    public static void main(String[] args) {
        AnthropicClient client = AnthropicOkHttpClient.fromEnv();

        BetaMessage response = client.beta().messages().create(
                MessageCreateParams.builder()
                        .model(Model.CLAUDE_OPUS_4_0)
                        .maxTokens(16000)
                        .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(10000).build())
                        .addUserMessage("Are there an infinite number of prime numbers such that n mod 4 == 3?")
                        .build()
        );

        System.out.println(response);
    }
}
```

</CodeGroup>

To turn on extended thinking, add a `thinking` object, with the `type` parameter set to `enabled` and the `budget_tokens` to a specified token budget for extended thinking.

The `budget_tokens` parameter determines the maximum number of tokens Claude is allowed to use for its internal reasoning process. In Claude 4 models, this limit applies to full thinking tokens, and not to [the summarized output](#summarized-thinking). Larger budgets can improve response quality by enabling more thorough analysis for complex problems, although Claude may not use the entire budget allocated, especially at ranges above 32k.

`budget_tokens` must be set to a value less than `max_tokens`. However, when using [interleaved thinking with tools](#interleaved-thinking), you can exceed this limit as the token limit becomes your entire context window (200k tokens).

### Summarized thinking

With extended thinking enabled, the Messages API for Claude 4 models returns a summary of Claude's full thinking process. Summarized thinking provides the full intelligence benefits of extended thinking, while preventing misuse.

Here are some important considerations for summarized thinking:

- You're charged for the full thinking tokens generated by the original request, not the summary tokens.
- The billed output token count will **not match** the count of tokens you see in the response.
- The first few lines of thinking output are more verbose, providing detailed reasoning that's particularly helpful for prompt engineering purposes.
- As Anthropic seeks to improve the extended thinking feature, summarization behavior is subject to change.
- Summarization preserves the key ideas of Claude's thinking process with minimal added latency, enabling a streamable user experience and easy migration from Claude 3.7 models to Claude 4 models.
- Summarization is processed by a different model than the one you target in your requests. The thinking model does not see the summarized output.

<Note>
  Claude Sonnet 3.7 continues to return full thinking output.

In rare cases where you need access to full thinking output for Claude 4 models, [contact our sales team](mailto:sales@anthropic.com).
</Note>

### Streaming thinking

You can stream extended thinking responses using [server-sent events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent%5Fevents/Using%5Fserver-sent%5Fevents).

When streaming is enabled for extended thinking, you receive thinking content via `thinking_delta` events.

For more documention on streaming via the Messages API, see [Streaming Messages](/en/docs/build-with-claude/streaming).

Here's how to handle streaming with thinking:

<CodeGroup>
  ```bash Shell
  curl https://api.anthropic.com/v1/messages \
       --header "x-api-key: $ANTHROPIC_API_KEY" \
       --header "anthropic-version: 2023-06-01" \
       --header "content-type: application/json" \
       --data \
  '{
      "model": "claude-sonnet-4-20250514",
      "max_tokens": 16000,
      "stream": true,
      "thinking": {
          "type": "enabled",
          "budget_tokens": 10000
      },
      "messages": [
          {
              "role": "user",
              "content": "What is 27 * 453?"
          }
      ]
  }'
  ```

```python Python
import anthropic

client = anthropic.Anthropic()

with client.messages.stream(
    model="claude-opus-4-20250514",
    max_tokens=16000,
    thinking={"type": "enabled", "budget_tokens": 10000},
    messages=[{"role": "user", "content": "What is 27 * 453?"}],
) as stream:
    thinking_started = False
    response_started = False

    for event in stream:
        if event.type == "content_block_start":
            print(f"\nStarting {event.content_block.type} block...")
            # Reset flags for each new block
            thinking_started = False
            response_started = False
        elif event.type == "content_block_delta":
            if event.delta.type == "thinking_delta":
                if not thinking_started:
                    print("Thinking: ", end="", flush=True)
                    thinking_started = True
                print(event.delta.thinking, end="", flush=True)
            elif event.delta.type == "text_delta":
                if not response_started:
                    print("Response: ", end="", flush=True)
                    response_started = True
                print(event.delta.text, end="", flush=True)
        elif event.type == "content_block_stop":
            print("\nBlock complete.")
```

```typescript TypeScript
import Anthropic from '@anthropic-ai/sdk';

const client = new Anthropic();

const stream = await client.messages.stream({
  model: 'claude-opus-4-20250514',
  max_tokens: 16000,
  thinking: {
    type: 'enabled',
    budget_tokens: 10000,
  },
  messages: [
    {
      role: 'user',
      content: 'What is 27 * 453?',
    },
  ],
});

let thinkingStarted = false;
let responseStarted = false;

for await (const event of stream) {
  if (event.type === 'content_block_start') {
    console.log(`\nStarting ${event.content_block.type} block...`);
    // Reset flags for each new block
    thinkingStarted = false;
    responseStarted = false;
  } else if (event.type === 'content_block_delta') {
    if (event.delta.type === 'thinking_delta') {
      if (!thinkingStarted) {
        process.stdout.write('Thinking: ');
        thinkingStarted = true;
      }
      process.stdout.write(event.delta.thinking);
    } else if (event.delta.type === 'text_delta') {
      if (!responseStarted) {
        process.stdout.write('Response: ');
        responseStarted = true;
      }
      process.stdout.write(event.delta.text);
    }
  } else if (event.type === 'content_block_stop') {
    console.log('\nBlock complete.');
  }
}
```

```java Java
import com.anthropic.client.AnthropicClient;
import com.anthropic.client.okhttp.AnthropicOkHttpClient;
import com.anthropic.core.http.StreamResponse;
import com.anthropic.models.beta.messages.MessageCreateParams;
import com.anthropic.models.beta.messages.BetaRawMessageStreamEvent;
import com.anthropic.models.beta.messages.BetaThinkingConfigEnabled;
import com.anthropic.models.messages.Model;

public class SimpleThinkingStreamingExample {
    private static boolean thinkingStarted = false;
    private static boolean responseStarted = false;

    public static void main(String[] args) {
        AnthropicClient client = AnthropicOkHttpClient.fromEnv();

        MessageCreateParams createParams = MessageCreateParams.builder()
                .model(Model.CLAUDE_OPUS_4_0)
                .maxTokens(16000)
                .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(10000).build())
                .addUserMessage("What is 27 * 453?")
                .build();

        try (StreamResponse<BetaRawMessageStreamEvent> streamResponse =
                     client.beta().messages().createStreaming(createParams)) {
            streamResponse.stream()
                    .forEach(event -> {
                        if (event.isContentBlockStart()) {
                            System.out.printf("\nStarting %s block...%n",
                                    event.asContentBlockStart()._type());
                            // Reset flags for each new block
                            thinkingStarted = false;
                            responseStarted = false;
                        } else if (event.isContentBlockDelta()) {
                            var delta = event.asContentBlockDelta().delta();
                            if (delta.isBetaThinking()) {
                                if (!thinkingStarted) {
                                    System.out.print("Thinking: ");
                                    thinkingStarted = true;
                                }
                                System.out.print(delta.asBetaThinking().thinking());
                                System.out.flush();
                            } else if (delta.isBetaText()) {
                                if (!responseStarted) {
                                    System.out.print("Response: ");
                                    responseStarted = true;
                                }
                                System.out.print(delta.asBetaText().text());
                                System.out.flush();
                            }
                        } else if (event.isContentBlockStop()) {
                            System.out.println("\nBlock complete.");
                        }
                    });
        }
    }
}
```

<CodeBlock
filename={
<TryInConsoleButton userPrompt="What is 27 * 453?" thinkingBudgetTokens={16000}>
Try in Console
</TryInConsoleButton>
}
/>
</CodeGroup>

Example streaming output:

```json
event: message_start
data: {"type": "message_start", "message": {"id": "msg_01...", "type": "message", "role": "assistant", "content": [], "model": "claude-sonnet-4-20250514", "stop_reason": null, "stop_sequence": null}}

event: content_block_start
data: {"type": "content_block_start", "index": 0, "content_block": {"type": "thinking", "thinking": ""}}

event: content_block_delta
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "Let me solve this step by step:\n\n1. First break down 27 * 453"}}

event: content_block_delta
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "thinking_delta", "thinking": "\n2. 453 = 400 + 50 + 3"}}

// Additional thinking deltas...

event: content_block_delta
data: {"type": "content_block_delta", "index": 0, "delta": {"type": "signature_delta", "signature": "EqQBCgIYAhIM1gbcDa9GJwZA2b3hGgxBdjrkzLoky3dl1pkiMOYds..."}}

event: content_block_stop
data: {"type": "content_block_stop", "index": 0}

event: content_block_start
data: {"type": "content_block_start", "index": 1, "content_block": {"type": "text", "text": ""}}

event: content_block_delta
data: {"type": "content_block_delta", "index": 1, "delta": {"type": "text_delta", "text": "27 * 453 = 12,231"}}

// Additional text deltas...

event: content_block_stop
data: {"type": "content_block_stop", "index": 1}

event: message_delta
data: {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": null}}

event: message_stop
data: {"type": "message_stop"}
```

<Note>
  When using streaming with thinking enabled, you might notice that text sometimes arrives in larger chunks alternating with smaller, token-by-token delivery. This is expected behavior, especially for thinking content.

The streaming system needs to process content in batches for optimal performance, which can result in this "chunky" delivery pattern, with possible delays between streaming events. We're continuously working to improve this experience, with future updates focused on making thinking content stream more smoothly.
</Note>

## Extended thinking with tool use

Extended thinking can be used alongside [tool use](/en/docs/agents-and-tools/tool-use/overview), allowing Claude to reason through tool selection and results processing.

When using extended thinking with tool use, be aware of the following limitations:

1. **Tool choice limitation**: Tool use with thinking only supports `tool_choice: {"type": "auto"}` (the default) or `tool_choice: {"type": "none"}`. Using `tool_choice: {"type": "any"}` or `tool_choice: {"type": "tool", "name": "..."}` will result in an error because these options force tool use, which is incompatible with extended thinking.

2. **Preserving thinking blocks**: During tool use, you must pass `thinking` blocks back to the API for the last assistant message. Include the complete unmodified block back to the API to maintain reasoning continuity.

<AccordionGroup>
  <Accordion title="Example: Passing thinking blocks with tool results">
    Here's a practical example showing how to preserve thinking blocks when providing tool results:

    <CodeGroup>
      ```python Python
      weather_tool = {
          "name": "get_weather",
          "description": "Get current weather for a location",
          "input_schema": {
              "type": "object",
              "properties": {
                  "location": {"type": "string"}
              },
              "required": ["location"]
          }
      }

      # First request - Claude responds with thinking and tool request
      response = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          tools=[weather_tool],
          messages=[
              {"role": "user", "content": "What's the weather in Paris?"}
          ]
      )
      ```

      ```typescript TypeScript
      const weatherTool = {
        name: "get_weather",
        description: "Get current weather for a location",
        input_schema: {
          type: "object",
          properties: {
            location: { type: "string" }
          },
          required: ["location"]
        }
      };

      // First request - Claude responds with thinking and tool request
      const response = await client.messages.create({
        model: "claude-sonnet-4-20250514",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        tools: [weatherTool],
        messages: [
          { role: "user", content: "What's the weather in Paris?" }
        ]
      });
      ```

      ```java Java
      import java.util.List;
      import java.util.Map;

      import com.anthropic.client.AnthropicClient;
      import com.anthropic.client.okhttp.AnthropicOkHttpClient;
      import com.anthropic.core.JsonValue;
      import com.anthropic.models.beta.messages.BetaMessage;
      import com.anthropic.models.beta.messages.MessageCreateParams;
      import com.anthropic.models.beta.messages.BetaThinkingConfigEnabled;
      import com.anthropic.models.beta.messages.BetaTool;
      import com.anthropic.models.beta.messages.BetaTool.InputSchema;
      import com.anthropic.models.messages.Model;

      public class ThinkingWithToolsExample {
          public static void main(String[] args) {
              AnthropicClient client = AnthropicOkHttpClient.fromEnv();

              InputSchema schema = InputSchema.builder()
                      .properties(JsonValue.from(Map.of(
                              "location", Map.of("type", "string")
                      )))
                      .putAdditionalProperty("required", JsonValue.from(List.of("location")))
                      .build();

              BetaTool weatherTool = BetaTool.builder()
                      .name("get_weather")
                      .description("Get current weather for a location")
                      .inputSchema(schema)
                      .build();

              BetaMessage response = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(10000).build())
                              .addTool(weatherTool)
                              .addUserMessage("What's the weather in Paris?")
                              .build()
              );

              System.out.println(response);
          }
      }
      ```
    </CodeGroup>

    The API response will include thinking, text, and tool\_use blocks:

    ```json
    {
        "content": [
            {
                "type": "thinking",
                "thinking": "The user wants to know the current weather in Paris. I have access to a function `get_weather`...",
                "signature": "BDaL4VrbR2Oj0hO4XpJxT28J5TILnCrrUXoKiiNBZW9P+nr8XSj1zuZzAl4egiCCpQNvfyUuFFJP5CncdYZEQPPmLxYsNrcs...."
            },
            {
                "type": "text",
                "text": "I can help you get the current weather information for Paris. Let me check that for you"
            },
            {
                "type": "tool_use",
                "id": "toolu_01CswdEQBMshySk6Y9DFKrfq",
                "name": "get_weather",
                "input": {
                    "location": "Paris"
                }
            }
        ]
    }
    ```

    Now let's continue the conversation and use the tool

    <CodeGroup>
      ```python Python
      # Extract thinking block and tool use block
      thinking_block = next((block for block in response.content
                            if block.type == 'thinking'), None)
      tool_use_block = next((block for block in response.content
                            if block.type == 'tool_use'), None)

      # Call your actual weather API, here is where your actual API call would go
      # let's pretend this is what we get back
      weather_data = {"temperature": 88}

      # Second request - Include thinking block and tool result
      # No new thinking blocks will be generated in the response
      continuation = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          tools=[weather_tool],
          messages=[
              {"role": "user", "content": "What's the weather in Paris?"},
              # notice that the thinking_block is passed in as well as the tool_use_block
              # if this is not passed in, an error is raised
              {"role": "assistant", "content": [thinking_block, tool_use_block]},
              {"role": "user", "content": [{
                  "type": "tool_result",
                  "tool_use_id": tool_use_block.id,
                  "content": f"Current temperature: {weather_data['temperature']}°F"
              }]}
          ]
      )
      ```

      ```typescript TypeScript
      // Extract thinking block and tool use block
      const thinkingBlock = response.content.find(block =>
        block.type === 'thinking');
      const toolUseBlock = response.content.find(block =>
        block.type === 'tool_use');

      // Call your actual weather API, here is where your actual API call would go
      // let's pretend this is what we get back
      const weatherData = { temperature: 88 };

      // Second request - Include thinking block and tool result
      // No new thinking blocks will be generated in the response
      const continuation = await client.messages.create({
        model: "claude-sonnet-4-20250514",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        tools: [weatherTool],
        messages: [
          { role: "user", content: "What's the weather in Paris?" },
          // notice that the thinkingBlock is passed in as well as the toolUseBlock
          // if this is not passed in, an error is raised
          { role: "assistant", content: [thinkingBlock, toolUseBlock] },
          { role: "user", content: [{
            type: "tool_result",
            tool_use_id: toolUseBlock.id,
            content: `Current temperature: ${weatherData.temperature}°F`
          }]}
        ]
      });
      ```

      ```java Java
      import java.util.List;
      import java.util.Map;
      import java.util.Optional;

      import com.anthropic.client.AnthropicClient;
      import com.anthropic.client.okhttp.AnthropicOkHttpClient;
      import com.anthropic.core.JsonValue;
      import com.anthropic.models.beta.messages.*;
      import com.anthropic.models.beta.messages.BetaTool.InputSchema;
      import com.anthropic.models.messages.Model;

      public class ThinkingToolsResultExample {
          public static void main(String[] args) {
              AnthropicClient client = AnthropicOkHttpClient.fromEnv();

              InputSchema schema = InputSchema.builder()
                      .properties(JsonValue.from(Map.of(
                              "location", Map.of("type", "string")
                      )))
                      .putAdditionalProperty("required", JsonValue.from(List.of("location")))
                      .build();

              BetaTool weatherTool = BetaTool.builder()
                      .name("get_weather")
                      .description("Get current weather for a location")
                      .inputSchema(schema)
                      .build();

              BetaMessage response = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(10000).build())
                              .addTool(weatherTool)
                              .addUserMessage("What's the weather in Paris?")
                              .build()
              );

              // Extract thinking block and tool use block
              Optional<BetaThinkingBlock> thinkingBlockOpt = response.content().stream()
                      .filter(BetaContentBlock::isThinking)
                      .map(BetaContentBlock::asThinking)
                      .findFirst();

              Optional<BetaToolUseBlock> toolUseBlockOpt = response.content().stream()
                      .filter(BetaContentBlock::isToolUse)
                      .map(BetaContentBlock::asToolUse)
                      .findFirst();

              if (thinkingBlockOpt.isPresent() && toolUseBlockOpt.isPresent()) {
                  BetaThinkingBlock thinkingBlock = thinkingBlockOpt.get();
                  BetaToolUseBlock toolUseBlock = toolUseBlockOpt.get();

                  // Call your actual weather API, here is where your actual API call would go
                  // let's pretend this is what we get back
                  Map<String, Object> weatherData = Map.of("temperature", 88);

                  // Second request - Include thinking block and tool result
                  // No new thinking blocks will be generated in the response
                  BetaMessage continuation = client.beta().messages().create(
                          MessageCreateParams.builder()
                                  .model(Model.CLAUDE_OPUS_4_0)
                                  .maxTokens(16000)
                                  .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(10000).build())
                                  .addTool(weatherTool)
                                  .addUserMessage("What's the weather in Paris?")
                                  .addAssistantMessageOfBetaContentBlockParams(
                                          // notice that the thinkingBlock is passed in as well as the toolUseBlock
                                          // if this is not passed in, an error is raised
                                          List.of(
                                                  BetaContentBlockParam.ofThinking(thinkingBlock.toParam()),
                                                  BetaContentBlockParam.ofToolUse(toolUseBlock.toParam())
                                          )
                                  )
                                  .addUserMessageOfBetaContentBlockParams(List.of(
                                          BetaContentBlockParam.ofToolResult(
                                                  BetaToolResultBlockParam.builder()
                                                          .toolUseId(toolUseBlock.id())
                                                          .content(String.format("Current temperature: %d°F", (Integer)weatherData.get("temperature")))
                                                          .build()
                                          )
                                  ))
                                  .build()
                  );

                  System.out.println(continuation);
              }
          }
      }
      ```
    </CodeGroup>

    The API response will now **only** include text

    ```json
    {
        "content": [
            {
                "type": "text",
                "text": "Currently in Paris, the temperature is 88°F (31°C)"
            }
        ]
    }
    ```

  </Accordion>
</AccordionGroup>

### Preserving thinking blocks

During tool use, you must pass `thinking` blocks back to the API, and you must include the complete unmodified block back to the API. This is critical for maintaining the model's reasoning flow and conversation integrity.

<Tip>
  While you can omit `thinking` blocks from prior `assistant` role turns, we suggest always passing back all thinking blocks to the API for any multi-turn conversation. The API will:

- Automatically filter the provided thinking blocks
- Use the relevant thinking blocks necessary to preserve the model's reasoning
- Only bill for the input tokens for the blocks shown to Claude
  </Tip>

When Claude invokes tools, it is pausing its construction of a response to await external information. When tool results are returned, Claude will continue building that existing response. This necessitates preserving thinking blocks during tool use, for a couple of reasons:

1. **Reasoning continuity**: The thinking blocks capture Claude's step-by-step reasoning that led to tool requests. When you post tool results, including the original thinking ensures Claude can continue its reasoning from where it left off.

2. **Context maintenance**: While tool results appear as user messages in the API structure, they're part of a continuous reasoning flow. Preserving thinking blocks maintains this conceptual flow across multiple API calls. For more information on context management, see our [guide on context windows](/en/docs/build-with-claude/context-windows).

**Important**: When providing `thinking` blocks, the entire sequence of consecutive `thinking` blocks must match the outputs generated by the model during the original request; you cannot rearrange or modify the sequence of these blocks.

### Interleaved thinking

Extended thinking with tool use in Claude 4 models supports interleaved thinking, which enables Claude to think between tool calls and make more sophisticated reasoning after receiving tool results.

With interleaved thinking, Claude can:

- Reason about the results of a tool call before deciding what to do next
- Chain multiple tool calls with reasoning steps in between
- Make more nuanced decisions based on intermediate results

To enable interleaved thinking, add [the beta header](/en/api/beta-headers) `interleaved-thinking-2025-05-14` to your API request.

Here are some important considerations for interleaved thinking:

- With interleaved thinking, the `budget_tokens` can exceed the `max_tokens` parameter, as it represents the total budget across all thinking blocks within one assistant turn.
- Interleaved thinking is only supported for [tools used via the Messages API](/en/docs/agents-and-tools/tool-use/overview).
- Interleaved thinking is supported for Claude 4 models only, with the beta header `interleaved-thinking-2025-05-14`.
- Direct calls to Anthropic's API allow you to pass `interleaved-thinking-2025-05-14` in requests to any model, with no effect.
- On 3rd-party platforms (e.g., [Amazon Bedrock](/en/api/claude-on-amazon-bedrock) and [Vertex AI](/en/api/claude-on-vertex-ai)), if you pass `interleaved-thinking-2025-05-14` to any model aside from Claude Opus 4 or Sonnet 4, your request will fail.

<AccordionGroup>
  <Accordion title="Tool use without interleaved thinking">
    <CodeGroup>
      ```python Python
      import anthropic

      client = anthropic.Anthropic()

      # Define tools
      calculator_tool = {
          "name": "calculator",
          "description": "Perform mathematical calculations",
          "input_schema": {
              "type": "object",
              "properties": {
                  "expression": {
                      "type": "string",
                      "description": "Mathematical expression to evaluate"
                  }
              },
              "required": ["expression"]
          }
      }

      database_tool = {
          "name": "database_query",
          "description": "Query product database",
          "input_schema": {
              "type": "object",
              "properties": {
                  "query": {
                      "type": "string",
                      "description": "SQL query to execute"
                  }
              },
              "required": ["query"]
          }
      }

      # First request - Claude thinks once before all tool calls
      response = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          tools=[calculator_tool, database_tool],
          messages=[{
              "role": "user",
              "content": "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
          }]
      )

      # Response includes thinking followed by tool uses
      # Note: Claude thinks once at the beginning, then makes all tool decisions
      print("First response:")
      for block in response.content:
          if block.type == "thinking":
              print(f"Thinking (summarized): {block.thinking}")
          elif block.type == "tool_use":
              print(f"Tool use: {block.name} with input {block.input}")
          elif block.type == "text":
              print(f"Text: {block.text}")

      # You would execute the tools and return results...
      # After getting both tool results back, Claude directly responds without additional thinking
      ```

      ```typescript TypeScript
      import Anthropic from '@anthropic-ai/sdk';

      const client = new Anthropic();

      // Define tools
      const calculatorTool = {
        name: "calculator",
        description: "Perform mathematical calculations",
        input_schema: {
          type: "object",
          properties: {
            expression: {
              type: "string",
              description: "Mathematical expression to evaluate"
            }
          },
          required: ["expression"]
        }
      };

      const databaseTool = {
        name: "database_query",
        description: "Query product database",
        input_schema: {
          type: "object",
          properties: {
            query: {
              type: "string",
              description: "SQL query to execute"
            }
          },
          required: ["query"]
        }
      };

      // First request - Claude thinks once before all tool calls
      const response = await client.messages.create({
        model: "claude-sonnet-4-20250514",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        tools: [calculatorTool, databaseTool],
        messages: [{
          role: "user",
          content: "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
        }]
      });

      // Response includes thinking followed by tool uses
      // Note: Claude thinks once at the beginning, then makes all tool decisions
      console.log("First response:");
      for (const block of response.content) {
        if (block.type === "thinking") {
          console.log(`Thinking (summarized): ${block.thinking}`);
        } else if (block.type === "tool_use") {
          console.log(`Tool use: ${block.name} with input ${JSON.stringify(block.input)}`);
        } else if (block.type === "text") {
          console.log(`Text: ${block.text}`);
        }
      }

      // You would execute the tools and return results...
      // After getting both tool results back, Claude directly responds without additional thinking
      ```

      ```java Java
      import com.anthropic.client.AnthropicClient;
      import com.anthropic.client.okhttp.AnthropicOkHttpClient;
      import com.anthropic.core.JsonValue;
      import com.anthropic.models.beta.messages.*;
      import com.anthropic.models.messages.Model;
      import java.util.List;
      import java.util.Map;

      public class NonInterleavedThinkingExample {
          public static void main(String[] args) {
              AnthropicClient client = AnthropicOkHttpClient.fromEnv();

              // Define calculator tool
              BetaTool.InputSchema calculatorSchema = BetaTool.InputSchema.builder()
                      .properties(JsonValue.from(Map.of(
                              "expression", Map.of(
                                      "type", "string",
                                      "description", "Mathematical expression to evaluate"
                              )
                      )))
                      .putAdditionalProperty("required", JsonValue.from(List.of("expression")))
                      .build();

              BetaTool calculatorTool = BetaTool.builder()
                      .name("calculator")
                      .description("Perform mathematical calculations")
                      .inputSchema(calculatorSchema)
                      .build();

              // Define database tool
              BetaTool.InputSchema databaseSchema = BetaTool.InputSchema.builder()
                      .properties(JsonValue.from(Map.of(
                              "query", Map.of(
                                      "type", "string",
                                      "description", "SQL query to execute"
                              )
                      )))
                      .putAdditionalProperty("required", JsonValue.from(List.of("query")))
                      .build();

              BetaTool databaseTool = BetaTool.builder()
                      .name("database_query")
                      .description("Query product database")
                      .inputSchema(databaseSchema)
                      .build();

              // First request - Claude thinks once before all tool calls
              BetaMessage response = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder()
                                      .budgetTokens(10000)
                                      .build())
                              .addTool(calculatorTool)
                              .addTool(databaseTool)
                              .addUserMessage("What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?")
                              .build()
              );

              // Response includes thinking followed by tool uses
              // Note: Claude thinks once at the beginning, then makes all tool decisions
              System.out.println("First response:");
              for (BetaContentBlock block : response.content()) {
                  if (block.isThinking()) {
                      System.out.println("Thinking (summarized): " + block.asThinking().thinking());
                  } else if (block.isToolUse()) {
                      BetaToolUseBlock toolUse = block.asToolUse();
                      System.out.println("Tool use: " + toolUse.name() + " with input " + toolUse.input());
                  } else if (block.isText()) {
                      System.out.println("Text: " + block.asText().text());
                  }
              }

              // You would execute the tools and return results...
              // After getting both tool results back, Claude directly responds without additional thinking
          }
      }
      ```
    </CodeGroup>

    In this example without interleaved thinking:

    1. Claude thinks once at the beginning to understand the task
    2. Makes all tool use decisions upfront
    3. When tool results are returned, Claude immediately provides a response without additional thinking

  </Accordion>

  <Accordion title="Tool use with interleaved thinking">
    <CodeGroup>
      ```python Python
      import anthropic

      client = anthropic.Anthropic()

      # Same tool definitions as before
      calculator_tool = {
          "name": "calculator",
          "description": "Perform mathematical calculations",
          "input_schema": {
              "type": "object",
              "properties": {
                  "expression": {
                      "type": "string",
                      "description": "Mathematical expression to evaluate"
                  }
              },
              "required": ["expression"]
          }
      }

      database_tool = {
          "name": "database_query",
          "description": "Query product database",
          "input_schema": {
              "type": "object",
              "properties": {
                  "query": {
                      "type": "string",
                      "description": "SQL query to execute"
                  }
              },
              "required": ["query"]
          }
      }

      # First request with interleaved thinking enabled
      response = client.beta.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          tools=[calculator_tool, database_tool],
          betas=["interleaved-thinking-2025-05-14"],
          messages=[{
              "role": "user",
              "content": "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
          }]
      )

      print("Initial response:")
      thinking_blocks = []
      tool_use_blocks = []

      for block in response.content:
          if block.type == "thinking":
              thinking_blocks.append(block)
              print(f"Thinking: {block.thinking}")
          elif block.type == "tool_use":
              tool_use_blocks.append(block)
              print(f"Tool use: {block.name} with input {block.input}")
          elif block.type == "text":
              print(f"Text: {block.text}")

      # First tool result (calculator)
      calculator_result = "7500"  # 150 * 50

      # Continue with first tool result
      response2 = client.beta.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          tools=[calculator_tool, database_tool],
          betas=["interleaved-thinking-2025-05-14"],
          messages=[
              {
                  "role": "user",
                  "content": "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
              },
              {
                  "role": "assistant",
                  "content": [thinking_blocks[0], tool_use_blocks[0]]
              },
              {
                  "role": "user",
                  "content": [{
                      "type": "tool_result",
                      "tool_use_id": tool_use_blocks[0].id,
                      "content": calculator_result
                  }]
              }
          ]
      )

      print("\nAfter calculator result:")
      # With interleaved thinking, Claude can think about the calculator result
      # before deciding to query the database
      for block in response2.content:
          if block.type == "thinking":
              thinking_blocks.append(block)
              print(f"Interleaved thinking: {block.thinking}")
          elif block.type == "tool_use":
              tool_use_blocks.append(block)
              print(f"Tool use: {block.name} with input {block.input}")

      # Second tool result (database)
      database_result = "5200"  # Example average monthly revenue

      # Continue with second tool result
      response3 = client.beta.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          tools=[calculator_tool, database_tool],
          betas=["interleaved-thinking-2025-05-14"],
          messages=[
              {
                  "role": "user",
                  "content": "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
              },
              {
                  "role": "assistant",
                  "content": [thinking_blocks[0], tool_use_blocks[0]]
              },
              {
                  "role": "user",
                  "content": [{
                      "type": "tool_result",
                      "tool_use_id": tool_use_blocks[0].id,
                      "content": calculator_result
                  }]
              },
              {
                  "role": "assistant",
                  "content": thinking_blocks[1:] + tool_use_blocks[1:]
              },
              {
                  "role": "user",
                  "content": [{
                      "type": "tool_result",
                      "tool_use_id": tool_use_blocks[1].id,
                      "content": database_result
                  }]
              }
          ]
      )

      print("\nAfter database result:")
      # With interleaved thinking, Claude can think about both results
      # before formulating the final response
      for block in response3.content:
          if block.type == "thinking":
              print(f"Final thinking: {block.thinking}")
          elif block.type == "text":
              print(f"Final response: {block.text}")
      ```

      ```typescript TypeScript
      import Anthropic from '@anthropic-ai/sdk';

      const client = new Anthropic();

      // Same tool definitions as before
      const calculatorTool = {
        name: "calculator",
        description: "Perform mathematical calculations",
        input_schema: {
          type: "object",
          properties: {
            expression: {
              type: "string",
              description: "Mathematical expression to evaluate"
            }
          },
          required: ["expression"]
        }
      };

      const databaseTool = {
        name: "database_query",
        description: "Query product database",
        input_schema: {
          type: "object",
          properties: {
            query: {
              type: "string",
              description: "SQL query to execute"
            }
          },
          required: ["query"]
        }
      };

      // First request with interleaved thinking enabled
      const response = await client.beta.messages.create({
        // Enable interleaved thinking
        betas: ["interleaved-thinking-2025-05-14"],
        model: "claude-sonnet-4-20250514",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        tools: [calculatorTool, databaseTool],
        messages: [{
          role: "user",
          content: "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
        }]
      });

      console.log("Initial response:");
      const thinkingBlocks = [];
      const toolUseBlocks = [];

      for (const block of response.content) {
        if (block.type === "thinking") {
          thinkingBlocks.push(block);
          console.log(`Thinking: ${block.thinking}`);
        } else if (block.type === "tool_use") {
          toolUseBlocks.push(block);
          console.log(`Tool use: ${block.name} with input ${JSON.stringify(block.input)}`);
        } else if (block.type === "text") {
          console.log(`Text: ${block.text}`);
        }
      }

      // First tool result (calculator)
      const calculatorResult = "7500"; // 150 * 50

      // Continue with first tool result
      const response2 = await client.beta.messages.create({
        betas: ["interleaved-thinking-2025-05-14"],
        model: "claude-sonnet-4-20250514",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        tools: [calculatorTool, databaseTool],
        messages: [
          {
            role: "user",
            content: "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
          },
          {
            role: "assistant",
            content: [thinkingBlocks[0], toolUseBlocks[0]]
          },
          {
            role: "user",
            content: [{
              type: "tool_result",
              tool_use_id: toolUseBlocks[0].id,
              content: calculatorResult
            }]
          }
        ]
      });

      console.log("\nAfter calculator result:");
      // With interleaved thinking, Claude can think about the calculator result
      // before deciding to query the database
      for (const block of response2.content) {
        if (block.type === "thinking") {
          thinkingBlocks.push(block);
          console.log(`Interleaved thinking: ${block.thinking}`);
        } else if (block.type === "tool_use") {
          toolUseBlocks.push(block);
          console.log(`Tool use: ${block.name} with input ${JSON.stringify(block.input)}`);
        }
      }

      // Second tool result (database)
      const databaseResult = "5200"; // Example average monthly revenue

      // Continue with second tool result
      const response3 = await client.beta.messages.create({
        betas: ["interleaved-thinking-2025-05-14"],
        model: "claude-sonnet-4-20250514",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        tools: [calculatorTool, databaseTool],
        messages: [
          {
            role: "user",
            content: "What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?"
          },
          {
            role: "assistant",
            content: [thinkingBlocks[0], toolUseBlocks[0]]
          },
          {
            role: "user",
            content: [{
              type: "tool_result",
              tool_use_id: toolUseBlocks[0].id,
              content: calculatorResult
            }]
          },
          {
            role: "assistant",
            content: thinkingBlocks.slice(1).concat(toolUseBlocks.slice(1))
          },
          {
            role: "user",
            content: [{
              type: "tool_result",
              tool_use_id: toolUseBlocks[1].id,
              content: databaseResult
            }]
          }
        ]
      });

      console.log("\nAfter database result:");
      // With interleaved thinking, Claude can think about both results
      // before formulating the final response
      for (const block of response3.content) {
        if (block.type === "thinking") {
          console.log(`Final thinking: ${block.thinking}`);
        } else if (block.type === "text") {
          console.log(`Final response: ${block.text}`);
        }
      }
      ```

      ```java Java
      import com.anthropic.client.AnthropicClient;
      import com.anthropic.client.okhttp.AnthropicOkHttpClient;
      import com.anthropic.core.JsonValue;
      import com.anthropic.models.beta.messages.*;
      import com.anthropic.models.messages.Model;
      import java.util.*;
      import static java.util.stream.Collectors.toList;

      public class InterleavedThinkingExample {
          public static void main(String[] args) {
              AnthropicClient client = AnthropicOkHttpClient.fromEnv();

              // Define calculator tool
              BetaTool.InputSchema calculatorSchema = BetaTool.InputSchema.builder()
                      .properties(JsonValue.from(Map.of(
                              "expression", Map.of(
                                      "type", "string",
                                      "description", "Mathematical expression to evaluate"
                              )
                      )))
                      .putAdditionalProperty("required", JsonValue.from(List.of("expression")))
                      .build();

              BetaTool calculatorTool = BetaTool.builder()
                      .name("calculator")
                      .description("Perform mathematical calculations")
                      .inputSchema(calculatorSchema)
                      .build();

              // Define database tool
              BetaTool.InputSchema databaseSchema = BetaTool.InputSchema.builder()
                      .properties(JsonValue.from(Map.of(
                              "query", Map.of(
                                      "type", "string",
                                      "description", "SQL query to execute"
                              )
                      )))
                      .putAdditionalProperty("required", JsonValue.from(List.of("query")))
                      .build();

              BetaTool databaseTool = BetaTool.builder()
                      .name("database_query")
                      .description("Query product database")
                      .inputSchema(databaseSchema)
                      .build();

              // First request with interleaved thinking enabled
              BetaMessage response = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder()
                                      .budgetTokens(10000)
                                      .build())
                              .addTool(calculatorTool)
                              .addTool(databaseTool)
                              // Enable interleaved thinking with beta header
                              .putAdditionalHeader("anthropic-beta", "interleaved-thinking-2025-05-14")
                              .addUserMessage("What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?")
                              .build()
              );

              System.out.println("Initial response:");
              List<BetaThinkingBlock> thinkingBlocks = new ArrayList<>();
              List<BetaToolUseBlock> toolUseBlocks = new ArrayList<>();

              for (BetaContentBlock block : response.content()) {
                  if (block.isThinking()) {
                      BetaThinkingBlock thinking = block.asThinking();
                      thinkingBlocks.add(thinking);
                      System.out.println("Thinking: " + thinking.thinking());
                  } else if (block.isToolUse()) {
                      BetaToolUseBlock toolUse = block.asToolUse();
                      toolUseBlocks.add(toolUse);
                      System.out.println("Tool use: " + toolUse.name() + " with input " + toolUse.input());
                  } else if (block.isText()) {
                      System.out.println("Text: " + block.asText().text());
                  }
              }

              // First tool result (calculator)
              String calculatorResult = "7500"; // 150 * 50

              // Continue with first tool result
              BetaMessage response2 = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder()
                                      .budgetTokens(10000)
                                      .build())
                              .addTool(calculatorTool)
                              .addTool(databaseTool)
                              .putAdditionalHeader("anthropic-beta", "interleaved-thinking-2025-05-14")
                              .addUserMessage("What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?")
                              .addAssistantMessageOfBetaContentBlockParams(List.of(
                                      BetaContentBlockParam.ofThinking(thinkingBlocks.get(0).toParam()),
                                      BetaContentBlockParam.ofToolUse(toolUseBlocks.get(0).toParam())
                              ))
                              .addUserMessageOfBetaContentBlockParams(List.of(
                                      BetaContentBlockParam.ofToolResult(
                                              BetaToolResultBlockParam.builder()
                                                      .toolUseId(toolUseBlocks.get(0).id())
                                                      .content(calculatorResult)
                                                      .build()
                                      )
                              ))
                              .build()
              );

              System.out.println("\nAfter calculator result:");
              // With interleaved thinking, Claude can think about the calculator result
              // before deciding to query the database
              for (BetaContentBlock block : response2.content()) {
                  if (block.isThinking()) {
                      BetaThinkingBlock thinking = block.asThinking();
                      thinkingBlocks.add(thinking);
                      System.out.println("Interleaved thinking: " + thinking.thinking());
                  } else if (block.isToolUse()) {
                      BetaToolUseBlock toolUse = block.asToolUse();
                      toolUseBlocks.add(toolUse);
                      System.out.println("Tool use: " + toolUse.name() + " with input " + toolUse.input());
                  }
              }

              // Second tool result (database)
              String databaseResult = "5200"; // Example average monthly revenue

              // Prepare combined content for assistant message
              List<BetaContentBlockParam> combinedContent = new ArrayList<>();
              for (int i = 1; i < thinkingBlocks.size(); i++) {
                  combinedContent.add(BetaContentBlockParam.ofThinking(thinkingBlocks.get(i).toParam()));
              }
              for (int i = 1; i < toolUseBlocks.size(); i++) {
                  combinedContent.add(BetaContentBlockParam.ofToolUse(toolUseBlocks.get(i).toParam()));
              }

              // Continue with second tool result
              BetaMessage response3 = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder()
                                      .budgetTokens(10000)
                                      .build())
                              .addTool(calculatorTool)
                              .addTool(databaseTool)
                              .putAdditionalHeader("anthropic-beta", "interleaved-thinking-2025-05-14")
                              .addUserMessage("What's the total revenue if we sold 150 units of product A at $50 each, and how does this compare to our average monthly revenue from the database?")
                              .addAssistantMessageOfBetaContentBlockParams(List.of(
                                      BetaContentBlockParam.ofThinking(thinkingBlocks.get(0).toParam()),
                                      BetaContentBlockParam.ofToolUse(toolUseBlocks.get(0).toParam())
                              ))
                              .addUserMessageOfBetaContentBlockParams(List.of(
                                      BetaContentBlockParam.ofToolResult(
                                              BetaToolResultBlockParam.builder()
                                                      .toolUseId(toolUseBlocks.get(0).id())
                                                      .content(calculatorResult)
                                                      .build()
                                      )
                              ))
                              .addAssistantMessageOfBetaContentBlockParams(combinedContent)
                              .addUserMessageOfBetaContentBlockParams(List.of(
                                      BetaContentBlockParam.ofToolResult(
                                              BetaToolResultBlockParam.builder()
                                                      .toolUseId(toolUseBlocks.get(1).id())
                                                      .content(databaseResult)
                                                      .build()
                                      )
                              ))
                              .build()
              );

              System.out.println("\nAfter database result:");
              // With interleaved thinking, Claude can think about both results
              // before formulating the final response
              for (BetaContentBlock block : response3.content()) {
                  if (block.isThinking()) {
                      System.out.println("Final thinking: " + block.asThinking().thinking());
                  } else if (block.isText()) {
                      System.out.println("Final response: " + block.asText().text());
                  }
              }
          }
      }
      ```
    </CodeGroup>

    In this example with interleaved thinking:

    1. Claude thinks about the task initially
    2. After receiving the calculator result, Claude can think again about what that result means
    3. Claude then decides how to query the database based on the first result
    4. After receiving the database result, Claude thinks once more about both results before formulating a final response
    5. The thinking budget is distributed across all thinking blocks within the turn

    This pattern allows for more sophisticated reasoning chains where each tool's output informs the next decision.

  </Accordion>
</AccordionGroup>

## Extended thinking with prompt caching

[Prompt caching](/en/docs/build-with-claude/prompt-caching) with thinking has several important considerations:

**Thinking block context removal**

- Thinking blocks from previous turns are removed from context, which can affect cache breakpoints
- When continuing conversations with tool use, thinking blocks are cached and count as input tokens when read from cache
- This creates a tradeoff: while thinking blocks don't consume context window space visually, they still count toward your input token usage when cached
- If thinking becomes disabled, requests will fail if you pass thinking content in the current tool use turn. In other contexts, thinking content passed to the API is simply ignored

**Cache invalidation patterns**

- Changes to thinking parameters (enabled/disabled or budget allocation) invalidate message cache breakpoints
- [Interleaved thinking](#interleaved-thinking) amplifies cache invalidation, as thinking blocks can occur between multiple [tool calls](#extended-thinking-with-tool-use)
- System prompts and tools remain cached despite thinking parameter changes or block removal

<Note>
  While thinking blocks are removed for caching and context calculations, they must be preserved when continuing conversations with [tool use](#extended-thinking-with-tool-use), especially with [interleaved thinking](#interleaved-thinking).
</Note>

### Understanding thinking block caching behavior

When using extended thinking with tool use, thinking blocks exhibit specific caching behavior that affects token counting:

**How it works:**

1. Caching only occurs when you make a subsequent request that includes tool results
2. When the subsequent request is made, the previous conversation history (including thinking blocks) can be cached
3. These cached thinking blocks count as input tokens in your usage metrics when read from the cache
4. When a non-tool-result user block is included, all previous thinking blocks are ignored and stripped from context

**Detailed example flow:**

**Request 1:**

```
User: "What's the weather in Paris?"
```

**Response 1:**

```
[thinking_block_1] + [tool_use block 1]
```

**Request 2:**

```
User: ["What's the weather in Paris?"],
Assistant: [thinking_block_1] + [tool_use block 1],
User: [tool_result_1, cache=True]
```

**Response 2:**

```
[thinking_block_2] + [text block 2]
```

Request 2 writes a cache of the request content (not the response). The cache includes the original user message, the first thinking block, tool use block, and the tool result.

**Request 3:**

```
User: ["What's the weather in Paris?"],
Assistant: [thinking_block_1] + [tool_use block 1],
User: [tool_result_1, cache=True],
Assistant: [thinking_block_2] + [text block 2],
User: [Text response, cache=True]
```

Because a non-tool-result user block was included, all previous thinking blocks are ignored. This request will be processed the same as:

```
User: ["What's the weather in Paris?"],
Assistant: [tool_use block 1],
User: [tool_result_1, cache=True],
Assistant: [text block 2],
User: [Text response, cache=True]
```

**Key points:**

- This caching behavior happens automatically, even without explicit `cache_control` markers
- This behavior is consistent whether using regular thinking or interleaved thinking

<AccordionGroup>
  <Accordion title="System prompt caching (preserved when thinking changes)">
    <CodeGroup>
      ```python Python
      from anthropic import Anthropic
      import requests
      from bs4 import BeautifulSoup

      client = Anthropic()

      def fetch_article_content(url):
          response = requests.get(url)
          soup = BeautifulSoup(response.content, 'html.parser')

          # Remove script and style elements
          for script in soup(["script", "style"]):
              script.decompose()

          # Get text
          text = soup.get_text()

          # Break into lines and remove leading and trailing space on each
          lines = (line.strip() for line in text.splitlines())
          # Break multi-headlines into a line each
          chunks = (phrase.strip() for line in lines for phrase in line.split("  "))
          # Drop blank lines
          text = '\n'.join(chunk for chunk in chunks if chunk)

          return text

      # Fetch the content of the article
      book_url = "https://www.gutenberg.org/cache/epub/1342/pg1342.txt"
      book_content = fetch_article_content(book_url)
      # Use just enough text for caching (first few chapters)
      LARGE_TEXT = book_content[:5000]

      SYSTEM_PROMPT=[
          {
              "type": "text",
              "text": "You are an AI assistant that is tasked with literary analysis. Analyze the following text carefully.",
          },
          {
              "type": "text",
              "text": LARGE_TEXT,
              "cache_control": {"type": "ephemeral"}
          }
      ]

      MESSAGES = [
          {
              "role": "user",
              "content": "Analyze the tone of this passage."
          }
      ]

      # First request - establish cache
      print("First request - establishing cache")
      response1 = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=20000,
          thinking={
              "type": "enabled",
              "budget_tokens": 4000
          },
          system=SYSTEM_PROMPT,
          messages=MESSAGES
      )

      print(f"First response usage: {response1.usage}")

      MESSAGES.append({
          "role": "assistant",
          "content": response1.content
      })
      MESSAGES.append({
          "role": "user",
          "content": "Analyze the characters in this passage."
      })
      # Second request - same thinking parameters (cache hit expected)
      print("\nSecond request - same thinking parameters (cache hit expected)")
      response2 = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=20000,
          thinking={
              "type": "enabled",
              "budget_tokens": 4000
          },
          system=SYSTEM_PROMPT,
          messages=MESSAGES
      )

      print(f"Second response usage: {response2.usage}")

      # Third request - different thinking parameters (cache miss for messages)
      print("\nThird request - different thinking parameters (cache miss for messages)")
      response3 = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=20000,
          thinking={
              "type": "enabled",
              "budget_tokens": 8000  # Changed thinking budget
          },
          system=SYSTEM_PROMPT,  # System prompt remains cached
          messages=MESSAGES  # Messages cache is invalidated
      )

      print(f"Third response usage: {response3.usage}")
      ```

      ```typescript TypeScript
      import Anthropic from '@anthropic-ai/sdk';
      import axios from 'axios';
      import * as cheerio from 'cheerio';

      const client = new Anthropic();

      async function fetchArticleContent(url: string): Promise<string> {
        const response = await axios.get(url);
        const $ = cheerio.load(response.data);

        // Remove script and style elements
        $('script, style').remove();

        // Get text
        let text = $.text();

        // Break into lines and remove leading and trailing space on each
        const lines = text.split('\n').map(line => line.trim());
        // Drop blank lines
        text = lines.filter(line => line.length > 0).join('\n');

        return text;
      }

      // Fetch the content of the article
      const bookUrl = "https://www.gutenberg.org/cache/epub/1342/pg1342.txt";
      const bookContent = await fetchArticleContent(bookUrl);
      // Use just enough text for caching (first few chapters)
      const LARGE_TEXT = bookContent.slice(0, 5000);

      const SYSTEM_PROMPT = [
        {
          type: "text",
          text: "You are an AI assistant that is tasked with literary analysis. Analyze the following text carefully.",
        },
        {
          type: "text",
          text: LARGE_TEXT,
          cache_control: { type: "ephemeral" }
        }
      ];

      const MESSAGES = [
        {
          role: "user",
          content: "Analyze the tone of this passage."
        }
      ];

      // First request - establish cache
      console.log("First request - establishing cache");
      const response1 = await client.messages.create({
        model: "claude-sonnet-4-20250514",
        max_tokens: 20000,
        thinking: {
          type: "enabled",
          budget_tokens: 4000
        },
        system: SYSTEM_PROMPT,
        messages: MESSAGES
      });

      console.log(`First response usage: ${response1.usage}`);

      MESSAGES.push({
        role: "assistant",
        content: response1.content
      });
      MESSAGES.push({
        role: "user",
        content: "Analyze the characters in this passage."
      });

      // Second request - same thinking parameters (cache hit expected)
      console.log("\nSecond request - same thinking parameters (cache hit expected)");
      const response2 = await client.messages.create({
        model: "claude-sonnet-4-20250514",
        max_tokens: 20000,
        thinking: {
          type: "enabled",
          budget_tokens: 4000
        },
        system: SYSTEM_PROMPT,
        messages: MESSAGES
      });

      console.log(`Second response usage: ${response2.usage}`);

      // Third request - different thinking parameters (cache miss for messages)
      console.log("\nThird request - different thinking parameters (cache miss for messages)");
      const response3 = await client.messages.create({
        model: "claude-sonnet-4-20250514",
        max_tokens: 20000,
        thinking: {
          type: "enabled",
          budget_tokens: 8000  // Changed thinking budget
        },
        system: SYSTEM_PROMPT,  // System prompt remains cached
        messages: MESSAGES  // Messages cache is invalidated
      });

      console.log(`Third response usage: ${response3.usage}`);
      ```
    </CodeGroup>

  </Accordion>

  <Accordion title="Messages caching (invalidated when thinking changes)">
    <CodeGroup>
      ```python Python
      from anthropic import Anthropic
      import requests
      from bs4 import BeautifulSoup

      client = Anthropic()

      def fetch_article_content(url):
          response = requests.get(url)
          soup = BeautifulSoup(response.content, 'html.parser')

          # Remove script and style elements
          for script in soup(["script", "style"]):
              script.decompose()

          # Get text
          text = soup.get_text()

          # Break into lines and remove leading and trailing space on each
          lines = (line.strip() for line in text.splitlines())
          # Break multi-headlines into a line each
          chunks = (phrase.strip() for line in lines for phrase in line.split("  "))
          # Drop blank lines
          text = '\n'.join(chunk for chunk in chunks if chunk)

          return text

      # Fetch the content of the article
      book_url = "https://www.gutenberg.org/cache/epub/1342/pg1342.txt"
      book_content = fetch_article_content(book_url)
      # Use just enough text for caching (first few chapters)
      LARGE_TEXT = book_content[:5000]

      # No system prompt - caching in messages instead
      MESSAGES = [
          {
              "role": "user",
              "content": [
                  {
                      "type": "text",
                      "text": LARGE_TEXT,
                      "cache_control": {"type": "ephemeral"},
                  },
                  {
                      "type": "text",
                      "text": "Analyze the tone of this passage."
                  }
              ]
          }
      ]

      # First request - establish cache
      print("First request - establishing cache")
      response1 = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=20000,
          thinking={
              "type": "enabled",
              "budget_tokens": 4000
          },
          messages=MESSAGES
      )

      print(f"First response usage: {response1.usage}")

      MESSAGES.append({
          "role": "assistant",
          "content": response1.content
      })
      MESSAGES.append({
          "role": "user",
          "content": "Analyze the characters in this passage."
      })
      # Second request - same thinking parameters (cache hit expected)
      print("\nSecond request - same thinking parameters (cache hit expected)")
      response2 = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=20000,
          thinking={
              "type": "enabled",
              "budget_tokens": 4000  # Same thinking budget
          },
          messages=MESSAGES
      )

      print(f"Second response usage: {response2.usage}")

      MESSAGES.append({
          "role": "assistant",
          "content": response2.content
      })
      MESSAGES.append({
          "role": "user",
          "content": "Analyze the setting in this passage."
      })

      # Third request - different thinking budget (cache miss expected)
      print("\nThird request - different thinking budget (cache miss expected)")
      response3 = client.messages.create(
          model="claude-sonnet-4-20250514",
          max_tokens=20000,
          thinking={
              "type": "enabled",
              "budget_tokens": 8000  # Different thinking budget breaks cache
          },
          messages=MESSAGES
      )

      print(f"Third response usage: {response3.usage}")
      ```

      ```typescript TypeScript
      import Anthropic from '@anthropic-ai/sdk';
      import axios from 'axios';
      import * as cheerio from 'cheerio';

      const client = new Anthropic();

      async function fetchArticleContent(url: string): Promise<string> {
        const response = await axios.get(url);
        const $ = cheerio.load(response.data);

        // Remove script and style elements
        $('script, style').remove();

        // Get text
        let text = $.text();

        // Clean up text (break into lines, remove whitespace)
        const lines = text.split('\n').map(line => line.trim());
        const chunks = lines.flatMap(line => line.split('  ').map(phrase => phrase.trim()));
        text = chunks.filter(chunk => chunk).join('\n');

        return text;
      }

      async function main() {
        // Fetch the content of the article
        const bookUrl = "https://www.gutenberg.org/cache/epub/1342/pg1342.txt";
        const bookContent = await fetchArticleContent(bookUrl);
        // Use just enough text for caching (first few chapters)
        const LARGE_TEXT = bookContent.substring(0, 5000);

        // No system prompt - caching in messages instead
        let MESSAGES = [
          {
            role: "user",
            content: [
              {
                type: "text",
                text: LARGE_TEXT,
                cache_control: {type: "ephemeral"},
              },
              {
                type: "text",
                text: "Analyze the tone of this passage."
              }
            ]
          }
        ];

        // First request - establish cache
        console.log("First request - establishing cache");
        const response1 = await client.messages.create({
          model: "claude-sonnet-4-20250514",
          max_tokens: 20000,
          thinking: {
            type: "enabled",
            budget_tokens: 4000
          },
          messages: MESSAGES
        });

        console.log(`First response usage: `, response1.usage);

        MESSAGES = [
          ...MESSAGES,
          {
            role: "assistant",
            content: response1.content
          },
          {
            role: "user",
            content: "Analyze the characters in this passage."
          }
        ];

        // Second request - same thinking parameters (cache hit expected)
        console.log("\nSecond request - same thinking parameters (cache hit expected)");
        const response2 = await client.messages.create({
          model: "claude-sonnet-4-20250514",
          max_tokens: 20000,
          thinking: {
            type: "enabled",
            budget_tokens: 4000  // Same thinking budget
          },
          messages: MESSAGES
        });

        console.log(`Second response usage: `, response2.usage);

        MESSAGES = [
          ...MESSAGES,
          {
            role: "assistant",
            content: response2.content
          },
          {
            role: "user",
            content: "Analyze the setting in this passage."
          }
        ];

        // Third request - different thinking budget (cache miss expected)
        console.log("\nThird request - different thinking budget (cache miss expected)");
        const response3 = await client.messages.create({
          model: "claude-sonnet-4-20250514",
          max_tokens: 20000,
          thinking: {
            type: "enabled",
            budget_tokens: 8000  // Different thinking budget breaks cache
          },
          messages: MESSAGES
        });

        console.log(`Third response usage: `, response3.usage);
      }

      main().catch(console.error);
      ```

      ```java Java
      import java.io.IOException;
      import java.io.InputStream;
      import java.util.ArrayList;
      import java.util.List;
      import java.io.BufferedReader;
      import java.io.InputStreamReader;
      import java.net.URL;
      import java.util.Arrays;
      import java.util.regex.Pattern;

      import com.anthropic.client.AnthropicClient;
      import com.anthropic.client.okhttp.AnthropicOkHttpClient;
      import com.anthropic.models.beta.messages.*;
      import com.anthropic.models.beta.messages.MessageCreateParams;
      import com.anthropic.models.messages.Model;

      import static java.util.stream.Collectors.joining;
      import static java.util.stream.Collectors.toList;

      public class ThinkingCacheExample {
          public static void main(String[] args) throws IOException {
              AnthropicClient client = AnthropicOkHttpClient.fromEnv();

              // Fetch the content of the article
              String bookUrl = "https://www.gutenberg.org/cache/epub/1342/pg1342.txt";
              String bookContent = fetchArticleContent(bookUrl);
              // Use just enough text for caching (first few chapters)
              String largeText = bookContent.substring(0, 5000);

              List<BetaTextBlockParam> systemPrompt = List.of(
                      BetaTextBlockParam.builder()
                              .text("You are an AI assistant that is tasked with literary analysis. Analyze the following text carefully.")
                              .build(),
                      BetaTextBlockParam.builder()
                              .text(largeText)
                              .cacheControl(BetaCacheControlEphemeral.builder().build())
                              .build()
              );

              List<BetaMessageParam> messages = new ArrayList<>();
              messages.add(BetaMessageParam.builder()
                      .role(BetaMessageParam.Role.USER)
                      .content("Analyze the tone of this passage.")
                      .build());

              // First request - establish cache
              System.out.println("First request - establishing cache");
              BetaMessage response1 = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(20000)
                              .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(4000).build())
                              .systemOfBetaTextBlockParams(systemPrompt)
                              .messages(messages)
                              .build()
              );

              System.out.println("First response usage: " + response1.usage());

              // Second request - same thinking parameters (cache hit expected)
              System.out.println("\nSecond request - same thinking parameters (cache hit expected)");
              BetaMessage response2 = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(20000)
                              .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(4000).build())
                              .systemOfBetaTextBlockParams(systemPrompt)
                              .addMessage(response1)
                              .addUserMessage("Analyze the characters in this passage.")
                              .messages(messages)
                              .build()
              );

              System.out.println("Second response usage: " + response2.usage());

              // Third request - different thinking budget (cache hit expected because system prompt caching)
              System.out.println("\nThird request - different thinking budget (cache hit expected)");
              BetaMessage response3 = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_OPUS_4_0)
                              .maxTokens(20000)
                              .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(8000).build())
                              .systemOfBetaTextBlockParams(systemPrompt)
                              .addMessage(response1)
                              .addUserMessage("Analyze the characters in this passage.")
                              .addMessage(response2)
                              .addUserMessage("Analyze the setting in this passage.")
                              .build()
              );

              System.out.println("Third response usage: " + response3.usage());
          }

          private static String fetchArticleContent(String url) throws IOException {
              // Fetch HTML content
              String htmlContent = fetchHtml(url);

              // Remove script and style elements
              String noScriptStyle = removeElements(htmlContent, "script", "style");

              // Extract text (simple approach - remove HTML tags)
              String text = removeHtmlTags(noScriptStyle);

              // Clean up text (break into lines, remove whitespace)
              List<String> lines = Arrays.asList(text.split("\n"));
              List<String> trimmedLines = lines.stream()
                      .map(String::trim)
                      .collect(toList());

              // Split on double spaces and flatten
              List<String> chunks = trimmedLines.stream()
                      .flatMap(line -> Arrays.stream(line.split("  "))
                              .map(String::trim))
                      .collect(toList());

              // Filter empty chunks and join with newlines
              return chunks.stream()
                      .filter(chunk -> !chunk.isEmpty())
                      .collect(joining("\n"));
          }

          /**
           * Fetches HTML content from a URL
           */
          private static String fetchHtml(String urlString) throws IOException {
              try (InputStream inputStream = new URL(urlString).openStream()) {
                  StringBuilder content = new StringBuilder();
                  try (BufferedReader reader = new BufferedReader(
                          new InputStreamReader(inputStream))) {
                      String line;
                      while ((line = reader.readLine()) != null) {
                          content.append(line).append("\n");
                      }
                  }
                  return content.toString();
              }
          }

          /**
           * Removes specified HTML elements and their content
           */
          private static String removeElements(String html, String... elementNames) {
              String result = html;
              for (String element : elementNames) {
                  // Pattern to match <element>...</element> and self-closing tags
                  String pattern = "<" + element + "\\s*[^>]*>.*?</" + element + ">|<" + element + "\\s*[^>]*/?>";
                  result = Pattern.compile(pattern, Pattern.DOTALL).matcher(result).replaceAll("");
              }
              return result;
          }

          /**
           * Removes all HTML tags from content
           */
          private static String removeHtmlTags(String html) {
              // Replace <br> and <p> tags with newlines for better text formatting
              String withLineBreaks = html.replaceAll("<br\\s*/?\\s*>|</?p\\s*[^>]*>", "\n");

              // Remove remaining HTML tags
              String noTags = withLineBreaks.replaceAll("<[^>]*>", "");

              // Decode HTML entities (simplified for common entities)
              return decodeHtmlEntities(noTags);
          }

          /**
           * Simple HTML entity decoder for common entities
           */
          private static String decodeHtmlEntities(String text) {
              return text
                      .replaceAll("&nbsp;", " ")
                      .replaceAll("&amp;", "&")
                      .replaceAll("&lt;", "<")
                      .replaceAll("&gt;", ">")
                      .replaceAll("&quot;", "\"")
                      .replaceAll("&#39;", "'")
                      .replaceAll("&hellip;", "...")
                      .replaceAll("&mdash;", "—");
          }

      }
      ```
    </CodeGroup>

    Here is the output of the script (you may see slightly different numbers)

    ```
    First request - establishing cache
    First response usage: { cache_creation_input_tokens: 1370, cache_read_input_tokens: 0, input_tokens: 17, output_tokens: 700 }

    Second request - same thinking parameters (cache hit expected)

    Second response usage: { cache_creation_input_tokens: 0, cache_read_input_tokens: 1370, input_tokens: 303, output_tokens: 874 }

    Third request - different thinking budget (cache miss expected)
    Third response usage: { cache_creation_input_tokens: 1370, cache_read_input_tokens: 0, input_tokens: 747, output_tokens: 619 }
    ```

    This example demonstrates that when caching is set up in the messages array, changing the thinking parameters (budget\_tokens increased from 4000 to 8000) **invalidates the cache**. The third request shows no cache hit with `cache_creation_input_tokens=1370` and `cache_read_input_tokens=0`, proving that message-based caching is invalidated when thinking parameters change.

  </Accordion>
</AccordionGroup>

## Max tokens and context window size with extended thinking

In older Claude models (prior to Claude Sonnet 3.7), if the sum of prompt tokens and `max_tokens` exceeded the model's context window, the system would automatically adjust `max_tokens` to fit within the context limit. This meant you could set a large `max_tokens` value and the system would silently reduce it as needed.

With Claude 3.7 and 4 models, `max_tokens` (which includes your thinking budget when thinking is enabled) is enforced as a strict limit. The system will now return a validation error if prompt tokens + `max_tokens` exceeds the context window size.

<Note>
  You can read through our [guide on context windows](/en/docs/build-with-claude/context-windows) for a more thorough deep dive.
</Note>

### The context window with extended thinking

When calculating context window usage with thinking enabled, there are some considerations to be aware of:

- Thinking blocks from previous turns are stripped and not counted towards your context window
- Current turn thinking counts towards your `max_tokens` limit for that turn

The diagram below demonstrates the specialized token management when extended thinking is enabled:

![Context window diagram with extended thinking](https://mintlify.s3.us-west-1.amazonaws.com/anthropic/images/context-window-thinking.svg)

The effective context window is calculated as:

```
context window =
  (current input tokens - previous thinking tokens) +
  (thinking tokens + encrypted thinking tokens + text output tokens)
```

We recommend using the [token counting API](/en/docs/build-with-claude/token-counting) to get accurate token counts for your specific use case, especially when working with multi-turn conversations that include thinking.

### The context window with extended thinking and tool use

When using extended thinking with tool use, thinking blocks must be explicitly preserved and returned with the tool results.

The effective context window calculation for extended thinking with tool use becomes:

```
context window =
  (current input tokens + previous thinking tokens + tool use tokens) +
  (thinking tokens + encrypted thinking tokens + text output tokens)
```

The diagram below illustrates token management for extended thinking with tool use:

![Context window diagram with extended thinking and tool use](https://mintlify.s3.us-west-1.amazonaws.com/anthropic/images/context-window-thinking-tools.svg)

### Managing tokens with extended thinking

Given the context window and `max_tokens` behavior with extended thinking Claude 3.7 and 4 models, you may need to:

- More actively monitor and manage your token usage
- Adjust `max_tokens` values as your prompt length changes
- Potentially use the [token counting endpoints](/en/docs/build-with-claude/token-counting) more frequently
- Be aware that previous thinking blocks don't accumulate in your context window

This change has been made to provide more predictable and transparent behavior, especially as maximum token limits have increased significantly.

## Thinking encryption

Full thinking content is encrypted and returned in the `signature` field. This field is used to verify that thinking blocks were generated by Claude when passed back to the API.

<Note>
  It is only strictly necessary to send back thinking blocks when using [tools with extended thinking](#extended-thinking-with-tool-use). Otherwise you can omit thinking blocks from previous turns, or let the API strip them for you if you pass them back.

If sending back thinking blocks, we recommend passing everything back as you received it for consistency and to avoid potential issues.
</Note>

Here are some important considerations on thinking encryption:

- When [streaming responses](#streaming-thinking), the signature is added via a `signature_delta` inside a `content_block_delta` event just before the `content_block_stop` event.
- `signature` values are significantly longer in Claude 4 than in previous models.
- The `signature` field is an opaque field and should not be interpreted or parsed - it exists solely for verification purposes.
- `signature` values are compatible across platforms (Anthropic APIs, [Amazon Bedrock](/en/api/claude-on-amazon-bedrock), and [Vertex AI](/en/api/claude-on-vertex-ai)). Values generated on one platform will be compatible with another.

### Thinking redaction

Occasionally Claude's internal reasoning will be flagged by our safety systems. When this occurs, we encrypt some or all of the `thinking` block and return it to you as a `redacted_thinking` block. `redacted_thinking` blocks are decrypted when passed back to the API, allowing Claude to continue its response without losing context.

When building customer-facing applications that use extended thinking:

- Be aware that redacted thinking blocks contain encrypted content that isn't human-readable
- Consider providing a simple explanation like: "Some of Claude's internal reasoning has been automatically encrypted for safety reasons. This doesn't affect the quality of responses."
- If showing thinking blocks to users, you can filter out redacted blocks while preserving normal thinking blocks
- Be transparent that using extended thinking features may occasionally result in some reasoning being encrypted
- Implement appropriate error handling to gracefully manage redacted thinking without breaking your UI

Here's an example showing both normal and redacted thinking blocks:

```json
{
  "content": [
    {
      "type": "thinking",
      "thinking": "Let me analyze this step by step...",
      "signature": "WaUjzkypQ2mUEVM36O2TxuC06KN8xyfbJwyem2dw3URve/op91XWHOEBLLqIOMfFG/UvLEczmEsUjavL...."
    },
    {
      "type": "redacted_thinking",
      "data": "EmwKAhgBEgy3va3pzix/LafPsn4aDFIT2Xlxh0L5L8rLVyIwxtE3rAFBa8cr3qpPkNRj2YfWXGmKDxH4mPnZ5sQ7vB9URj2pLmN3kF8/dW5hR7xJ0aP1oLs9yTcMnKVf2wRpEGjH9XZaBt4UvDcPrQ..."
    },
    {
      "type": "text",
      "text": "Based on my analysis..."
    }
  ]
}
```

<Note>
  Seeing redacted thinking blocks in your output is expected behavior. The model can still use this redacted reasoning to inform its responses while maintaining safety guardrails.

If you need to test redacted thinking handling in your application, you can use this special test string as your prompt: `ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_46C9A13E193C177646C7398A98432ECCCE4C1253D5E2D82641AC0E52CC2876CB`
</Note>

When passing `thinking` and `redacted_thinking` blocks back to the API in a multi-turn conversation, you must include the complete unmodified block back to the API for the last assistant turn. This is critical for maintaining the model's reasoning flow. We suggest always passing back all thinking blocks to the API. For more details, see the [Preserving thinking blocks](#preserving-thinking-blocks) section above.

<AccordionGroup>
  <Accordion title="Example: Working with redacted thinking blocks">
    This example demonstrates how to handle `redacted_thinking` blocks that may appear in responses when Claude's internal reasoning contains content flagged by safety systems:

    <CodeGroup>
      ```python Python
      import anthropic

      client = anthropic.Anthropic()

      # Using a special prompt that triggers redacted thinking (for demonstration purposes only)
      response = client.messages.create(
          model="claude-3-7-sonnet-20250219",
          max_tokens=16000,
          thinking={
              "type": "enabled",
              "budget_tokens": 10000
          },
          messages=[{
              "role": "user",
              "content": "ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_46C9A13E193C177646C7398A98432ECCCE4C1253D5E2D82641AC0E52CC2876CB"
          }]
      )

      # Identify redacted thinking blocks
      has_redacted_thinking = any(
          block.type == "redacted_thinking" for block in response.content
      )

      if has_redacted_thinking:
          print("Response contains redacted thinking blocks")
          # These blocks are still usable in subsequent requests

          # Extract all blocks (both redacted and non-redacted)
          all_thinking_blocks = [
              block for block in response.content
              if block.type in ["thinking", "redacted_thinking"]
          ]

          # When passing to subsequent requests, include all blocks without modification
          # This preserves the integrity of Claude's reasoning

          print(f"Found {len(all_thinking_blocks)} thinking blocks total")
          print(f"These blocks are still billable as output tokens")
      ```

      ```typescript TypeScript
      import Anthropic from '@anthropic-ai/sdk';

      const client = new Anthropic();

      // Using a special prompt that triggers redacted thinking (for demonstration purposes only)
      const response = await client.messages.create({
        model: "claude-3-7-sonnet-20250219",
        max_tokens: 16000,
        thinking: {
          type: "enabled",
          budget_tokens: 10000
        },
        messages: [{
          role: "user",
          content: "ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_46C9A13E193C177646C7398A98432ECCCE4C1253D5E2D82641AC0E52CC2876CB"
        }]
      });

      // Identify redacted thinking blocks
      const hasRedactedThinking = response.content.some(
        block => block.type === "redacted_thinking"
      );

      if (hasRedactedThinking) {
        console.log("Response contains redacted thinking blocks");
        // These blocks are still usable in subsequent requests

        // Extract all blocks (both redacted and non-redacted)
        const allThinkingBlocks = response.content.filter(
          block => block.type === "thinking" || block.type === "redacted_thinking"
        );

        // When passing to subsequent requests, include all blocks without modification
        // This preserves the integrity of Claude's reasoning

        console.log(`Found ${allThinkingBlocks.length} thinking blocks total`);
        console.log(`These blocks are still billable as output tokens`);
      }
      ```

      ```java Java
      import java.util.List;

      import static java.util.stream.Collectors.toList;

      import com.anthropic.client.AnthropicClient;
      import com.anthropic.client.okhttp.AnthropicOkHttpClient;
      import com.anthropic.models.beta.messages.BetaContentBlock;
      import com.anthropic.models.beta.messages.BetaMessage;
      import com.anthropic.models.beta.messages.MessageCreateParams;
      import com.anthropic.models.beta.messages.BetaThinkingConfigEnabled;
      import com.anthropic.models.messages.Model;

      public class RedactedThinkingExample {
          public static void main(String[] args) {
              AnthropicClient client = AnthropicOkHttpClient.fromEnv();

              // Using a special prompt that triggers redacted thinking (for demonstration purposes only)
              BetaMessage response = client.beta().messages().create(
                      MessageCreateParams.builder()
                              .model(Model.CLAUDE_3_7_SONNET_20250219)
                              .maxTokens(16000)
                              .thinking(BetaThinkingConfigEnabled.builder().budgetTokens(10000).build())
                              .addUserMessage("ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_46C9A13E193C177646C7398A98432ECCCE4C1253D5E2D82641AC0E52CC2876CB")
                              .build()
              );

              // Identify redacted thinking blocks
              boolean hasRedactedThinking = response.content().stream()
                      .anyMatch(BetaContentBlock::isRedactedThinking);

              if (hasRedactedThinking) {
                  System.out.println("Response contains redacted thinking blocks");
                  // These blocks are still usable in subsequent requests
                  // Extract all blocks (both redacted and non-redacted)
                  List<BetaContentBlock> allThinkingBlocks = response.content().stream()
                          .filter(block -> block.isThinking() ||
                                  block.isRedactedThinking())
                          .collect(toList());

                  // When passing to subsequent requests, include all blocks without modification
                  // This preserves the integrity of Claude's reasoning
                  System.out.println("Found " + allThinkingBlocks.size() + " thinking blocks total");
                  System.out.println("These blocks are still billable as output tokens");
              }
          }
      }
      ```

      <CodeBlock
        filename={

<TryInConsoleButton
userPrompt="ANTHROPIC_MAGIC_STRING_TRIGGER_REDACTED_THINKING_46C9A13E193C177646C7398A98432ECCCE4C1253D5E2D82641AC0E52CC2876CB"
thinkingBudgetTokens={16000}

>

    Try in Console

  </TryInConsoleButton>
}
      />
    </CodeGroup>
  </Accordion>
</AccordionGroup>

## Differences in thinking across model versions

The Messages API handles thinking differently across Claude Sonnet 3.7 and Claude 4 models, primarily in redaction and summarization behavior.

See the table below for a condensed comparison:

| Feature                  | Claude Sonnet 3.7            | Claude 4 Models                                              |
| ------------------------ | ---------------------------- | ------------------------------------------------------------ |
| **Thinking Output**      | Returns full thinking output | Returns summarized thinking                                  |
| **Interleaved Thinking** | Not supported                | Supported with `interleaved-thinking-2025-05-14` beta header |

## Pricing

Extended thinking uses the standard token pricing scheme:

| Model             | Base Input Tokens | Cache Writes   | Cache Hits    | Output Tokens |
| ----------------- | ----------------- | -------------- | ------------- | ------------- |
| Claude Opus 4     | \$15 / MTok       | \$18.75 / MTok | \$1.50 / MTok | \$75 / MTok   |
| Claude Sonnet 4   | \$3 / MTok        | \$3.75 / MTok  | \$0.30 / MTok | \$15 / MTok   |
| Claude Sonnet 3.7 | \$3 / MTok        | \$3.75 / MTok  | \$0.30 / MTok | \$15 / MTok   |

The thinking process incurs charges for:

- Tokens used during thinking (output tokens)
- Thinking blocks from the last assistant turn included in subsequent requests (input tokens)
- Standard text output tokens

<Note>
  When extended thinking is enabled, a specialized system prompt is automatically included to support this feature.
</Note>

When using summarized thinking:

- **Input tokens**: Tokens in your original request (excludes thinking tokens from previous turns)
- **Output tokens (billed)**: The original thinking tokens that Claude generated internally
- **Output tokens (visible)**: The summarized thinking tokens you see in the response
- **No charge**: Tokens used to generate the summary

<Warning>
  The billed output token count will **not** match the visible token count in the response. You are billed for the full thinking process, not the summary you see.
</Warning>

## Best practices and considerations for extended thinking

### Working with thinking budgets

- **Budget optimization:** The minimum budget is 1,024 tokens. We suggest starting at the minimum and increasing the thinking budget incrementally to find the optimal range for your use case. Higher token counts enable more comprehensive reasoning but with diminishing returns depending on the task. Increasing the budget can improve response quality at the tradeoff of increased latency. For critical tasks, test different settings to find the optimal balance. Note that the thinking budget is a target rather than a strict limit—actual token usage may vary based on the task.
- **Starting points:** Start with larger thinking budgets (16k+ tokens) for complex tasks and adjust based on your needs.
- **Large budgets:** For thinking budgets above 32k, we recommend using [batch processing](/en/docs/build-with-claude/batch-processing) to avoid networking issues. Requests pushing the model to think above 32k tokens causes long running requests that might run up against system timeouts and open connection limits.
- **Token usage tracking:** Monitor thinking token usage to optimize costs and performance.

### Performance considerations

- **Response times:** Be prepared for potentially longer response times due to the additional processing required for the reasoning process. Factor in that generating thinking blocks may increase overall response time.
- **Streaming requirements:** Streaming is required when `max_tokens` is greater than 21,333. When streaming, be prepared to handle both thinking and text content blocks as they arrive.

### Feature compatibility

- Thinking isn't compatible with `temperature` or `top_k` modifications as well as [forced tool use](/en/docs/agents-and-tools/tool-use/implement-tool-use#forcing-tool-use).
- When thinking is enabled, you can set `top_p` to values between 1 and 0.95.
- You cannot pre-fill responses when thinking is enabled.
- Changes to the thinking budget invalidate cached prompt prefixes that include messages. However, cached system prompts and tool definitions will continue to work when thinking parameters change.

### Usage guidelines

- **Task selection:** Use extended thinking for particularly complex tasks that benefit from step-by-step reasoning like math, coding, and analysis.
- **Context handling:** You do not need to remove previous thinking blocks yourself. The Anthropic API automatically ignores thinking blocks from previous turns and they are not included when calculating context usage.
- **Prompt engineering:** Review our [extended thinking prompting tips](/en/docs/build-with-claude/prompt-engineering/extended-thinking-tips) if you want to maximize Claude's thinking capabilities.

## Next steps

<CardGroup>
  <Card title="Try the extended thinking cookbook" icon="book" href="https://github.com/anthropics/anthropic-cookbook/tree/main/extended_thinking">
    Explore practical examples of thinking in our cookbook.
  </Card>

  <Card title="Extended thinking prompting tips" icon="code" href="/en/docs/build-with-claude/prompt-engineering/extended-thinking-tips">
    Learn prompt engineering best practices for extended thinking.
  </Card>
</CardGroup>

## Compatibility with OpenAI ChatCompletion API

### The Asymmetric Compatibility Challenge

When using Claude's extended thinking feature through an OpenAI-compatible API gateway, a fundamental asymmetric compatibility challenge exists:

1. **OpenAI clients** don't know about or handle Claude-specific signature fields - they only work with standard OpenAI message formats
2. **Claude upstream** requires signatures for thinking blocks in historical messages to maintain reasoning continuity
3. **The gateway** must bridge this gap transparently without breaking either side

### Root Cause Analysis

The core issue is **not** about exposing Claude's signature fields to OpenAI clients, but rather about the **asymmetric nature of the problem**:

**The Signature Requirement**: Claude thinking blocks include a `signature` field that is essential for verification and maintaining reasoning continuity across multi-turn conversations. When thinking content appears in conversation history, Claude validates these signatures to ensure the integrity of the reasoning chain.

**The OpenAI Constraint**: OpenAI clients cannot be assumed to support custom headers or Claude-specific fields, as they are designed to work with the standard OpenAI API format.

**The Gateway Challenge**: The gateway must preserve Claude signatures internally while presenting a clean OpenAI interface to clients.

### Implemented Solution: Transparent Signature Caching

The gateway implements a **transparent signature caching solution with intelligent fallback** that bridges the gap between OpenAI clients and Claude upstream requirements.

#### Core Architecture

**Principle**: The gateway acts as an intelligent bridge that:

1. **Captures and stores** signatures from Claude responses automatically
2. **Restores signatures** when converting OpenAI requests back to Claude format
3. **Maintains this process invisibly** to OpenAI clients
4. **Provides robust fallback** when signatures are unavailable

```
OpenAI Client → Gateway → Claude Upstream
     ↑                        ↓
     │    (Standard OpenAI     │ (Claude format with
     │     format without      │  signatures preserved)
     │     signatures)         │
     └─────────────────────────┘
           Gateway handles signature
           caching/restoration transparently
```

#### Signature Caching Implementation

**1. Cache Key Strategy**:
Uses a hierarchical key structure that uniquely identifies each thinking block:

- `token_id`: Isolates signatures per API token (direct context, no user lookup needed)
- `conversation_id`: Generated deterministically from message history hash
- `message_index`: Position in conversation history
- `thinking_block_index`: Handles multiple thinking blocks per message

**2. Conversation ID Generation**:

```go
func generateConversationID(messages []model.Message) string {
    var signature strings.Builder

    for i, msg := range messages {
        if msg.Role == "user" {
            // Use user message content for conversation identity
            content := truncateForHash(msg.StringContent(), 100)
            signature.WriteString(fmt.Sprintf("u%d:%s;", i, content))
        } else if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
            // Include tool calls in conversation identity (but not thinking content)
            signature.WriteString(fmt.Sprintf("a%d:tools:%d;", i, len(msg.ToolCalls)))
        }
    }

    hash := sha256.Sum256([]byte(signature.String()))
    return fmt.Sprintf("conv_%x", hash[:8])
}
```

**Why Hash-based Conversation ID?**

- **Universal Compatibility**: Works with any OpenAI client without custom headers
- **Deterministic**: Same conversation structure always generates same ID
- **Stateless**: No server-side conversation state required
- **Robust**: Handles conversation continuation naturally

#### Response Processing Implementation

**1. Streaming Response Handling**:
The gateway processes Claude streaming responses and extracts signatures transparently:

```go
case "signature_delta":
    if claudeResponse.Delta != nil && claudeResponse.Delta.Signature != nil {
        // Store signature in cache instead of exposing to client
        tokenID := getTokenIDFromRequest(tokenIDInt)
        conversationID := generateConversationID(request.Messages)
        cacheKey := generateSignatureKey(tokenID, conversationID, messageIndex, thinkingIndex)
        GetSignatureCache().Store(cacheKey, *claudeResponse.Delta.Signature)
    }

case "content_block_start":
    if claudeResponse.ContentBlock != nil {
        if claudeResponse.ContentBlock.Signature != nil {
            // Cache signature from content block
            signatureText = *claudeResponse.ContentBlock.Signature
        }
    }
```

**2. Non-Streaming Response Handling**:

```go
case "thinking", "redacted_thinking":
    if v.Thinking != nil {
        reasoningText += *v.Thinking
    }
    // Cache signature for future restoration
    if v.Signature != nil {
        tokenID := getTokenIDFromRequest(tokenIDInt)
        conversationID := generateConversationID(request.Messages)
        cacheKey := generateSignatureKey(tokenID, conversationID, messageIndex, thinkingIndex)
        GetSignatureCache().Store(cacheKey, *v.Signature)
    }
```

**Key Points**:

- Signatures are **never exposed** to OpenAI clients
- Both `thinking` and `redacted_thinking` blocks are supported
- Caching happens transparently during response processing

#### Request Conversion with Signature Restoration

**Primary Path - Signature Restoration**:
When converting OpenAI requests back to Claude format, the gateway attempts to restore cached signatures:

```go
// For assistant messages with thinking content
if reasoningContent != "" {
    var signatureRestored bool
    thinkingContent := Content{
        Type:     "thinking",
        Thinking: &reasoningContent,
    }

    // Try to restore signature from cache
    if tokenID, exists := c.Get("token_id"); exists {
        if tokenIDInt, ok := tokenID.(int); ok {
            tokenIDStr := getTokenIDFromRequest(tokenIDInt)
            conversationID := generateConversationID(textRequest.Messages)

            // Try to restore signature for this thinking block
            messageIndex := len(claudeRequest.Messages)
            cacheKey := generateSignatureKey(tokenIDStr, conversationID, messageIndex, 0)

            if signature := GetSignatureCache().Get(cacheKey); signature != nil {
                thinkingContent.Signature = signature
                signatureRestored = true
            }
        }
    }

    if signatureRestored {
        // Use proper thinking block with signature
        claudeMessage.Content = append([]Content{thinkingContent}, claudeMessage.Content...)
    } else {
        // Use fallback mechanism (see below)
    }
}
```

**Key Implementation Details**:

- **Thinking Block Ordering**: Fixed to prepend thinking blocks using `append([]Content{thinkingContent}, claudeMessage.Content...)`
- **Signature Restoration**: Attempts to restore signatures from cache using deterministic keys
- **Graceful Fallback**: When signatures unavailable, switches to fallback mode

#### Enhanced Fallback Mechanism

**Critical Edge Case Handling**: When cached signatures are not available (due to cache expiration, server restart, or cache failure), the system implements a robust fallback mechanism:

**Fallback Strategy**:

```go
// If signature restoration fails, convert to <think> format and disable thinking
if !signatureRestored {
    // Set fallback mode flag
    useFallbackMode = true

    // Convert thinking content to <think> format and prepend to text content
    thinkingPrefix := fmt.Sprintf("<think>%s</think>\n\n", reasoningContent)

    // Find the first text content and prepend thinking
    for i := range contents {
        if contents[i].Type == "text" {
            contents[i].Text = thinkingPrefix + contents[i].Text
            break
        }
    }

    // If no text content found, create one with thinking prefix
    if len(contents) == 0 {
        contents = append(contents, Content{
            Type: "text",
            Text: thinkingPrefix,
        })
    }
}

// At the end of request conversion:
// If fallback mode was used, disable thinking to avoid Claude validation errors
if useFallbackMode && claudeRequest.Thinking != nil {
    claudeRequest.Thinking = nil
}
```

**Why This Enhanced Fallback Works**:

1. **Preserves Thinking Content**: User's reasoning is not lost
2. **Avoids API Errors**: Claude won't reject requests due to missing signatures
3. **Disables Thinking Parameter**: Prevents Claude validation errors when thinking is enabled but signatures are missing
4. **Maintains Context**: Thinking content is still available to Claude for reasoning via `<think>` tags
5. **Transparent to Users**: OpenAI clients see no difference in behavior
6. **Graceful Degradation**: System continues working even when cache fails
7. **Handles Thinking-Enabled Requests**: Properly manages requests with `thinking` parameter enabled

**Fallback Scenarios**:

- **Cache Expiration**: Signatures expired between response and follow-up request
- **Server Restart**: In-memory cache lost but conversation continues
- **Distributed Deployment**: Request hits different server without cached signature
- **Cache Backend Failure**: Redis or cache backend temporarily unavailable
- **Memory Pressure**: Cache cleanup removed signatures under memory constraints

#### Key Benefits of This Implementation

**1. Transparent Operation**:

- OpenAI clients work unchanged - no API modifications needed
- Maintains full backward compatibility with existing OpenAI clients
- Preserves existing OpenAI API contract

**2. Robust Signature Management**:

- Automatic signature caching during response processing
- Intelligent signature restoration during request conversion
- Graceful fallback when signatures unavailable

**3. Production-Ready Architecture**:

- Hash-based conversation ID generation (no custom headers required)
- TokenID-based isolation (direct context, no user lookups)
- TTL-based cache management with automatic cleanup
- Optional Redis backend support for distributed deployments

**4. Comprehensive Edge Case Handling**:

- Cache expiration scenarios
- Server restart situations
- Distributed deployment support
- Cache backend failures
- Memory pressure situations

**5. Enhanced Fallback Mechanism**:

- Converts thinking content to `<think>` format when signatures missing
- Automatically disables `thinking` parameter to prevent Claude validation errors
- Maintains thinking content availability for Claude reasoning
- Ensures system continues working under all failure conditions

### Summary

The implemented solution successfully resolves Claude thinking compatibility issues through a **transparent signature caching architecture with intelligent fallback**. This approach:

**✅ Solves the Core Problem**: Enables proper multi-turn thinking conversations by preserving Claude signatures internally while presenting a clean OpenAI interface to clients.

**✅ Handles All Edge Cases**: Provides robust fallback mechanism that converts thinking content to `<think>` format and disables the thinking parameter when signatures are unavailable, preventing API failures.

**✅ Maintains Full Compatibility**: Works with any existing OpenAI client without modifications, preserving the standard OpenAI API contract.

**✅ Production Ready**: Handles real-world scenarios including cache expiration, server restarts, distributed deployments, and cache backend failures.

**✅ Transparent Operation**: OpenAI clients remain completely unaware of Claude-specific signature complexity - the gateway handles everything automatically.

The solution elegantly bridges the asymmetric compatibility gap between OpenAI clients (which don't handle signatures) and Claude upstream (which requires signatures for thinking blocks), making the complexity completely invisible while ensuring reliability under all conditions.
