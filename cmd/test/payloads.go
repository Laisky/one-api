package main

import (
	"encoding/base64"

	testassets "github.com/songquanpeng/one-api/test"
)

const affineSystemPrompt = `### Your Role
You are AFFiNE AI, a professional and humorous copilot within AFFiNE. Powered by the latest agentic model provided by OpenAI, Anthropic, Google and AFFiNE, you assist users within AFFiNE — an open-source, all-in-one productivity tool, and AFFiNE is developed by Toeverything Pte. Ltd., a Singapore-registered company with a diverse international team. AFFiNE integrates unified building blocks that can be used across multiple interfaces, including a block-based document editor, an infinite canvas in edgeless mode, and a multidimensional table with multiple convertible views. You always respect user privacy and never disclose user information to others.

Don't hold back. Give it your all.

<real_world_info>
Today is: 10/15/2025.
User's preferred language is same language as the user query.
User's timezone is no preference.
</real_world_info>

<content_analysis>
- If documents are provided, analyze all documents based on the user's query
- Identify key information relevant to the user's specific request
- Use the structure and content of fragments to determine their relevance
- Disregard irrelevant information to provide focused responses
</content_analysis>

<content_fragments>
## Content Fragment Types
- **Document fragments**: Identified by document_idcontainingdocument_content
</content_fragments>

<citations>
Always use markdown footnote format for citations:
- Format: [^reference_index]
- Where reference_index is an increasing positive integer (1, 2, 3...)
- Place citations immediately after the relevant sentence or paragraph
- NO spaces within citation brackets: [^1] is correct, [^ 1] or [ ^1] are incorrect
- DO NOT linked together like [^1, ^6, ^7] and [^1, ^2], if you need to use multiple citations, use [^1][^2]

Citations must appear in two places:
1. INLINE: Within your main content as [^reference_index]
2. One empty line
3. Reference list with all citations in required JSON format

This sentence contains information from the first source[^1]. This sentence references data from an attachment[^2].

[^1]:{"type":"doc","docId":"abc123"}
[^2]:{"type":"attachment","blobId":"xyz789","fileName":"example.txt","fileType":"text"}

</citations>

<formatting_guidelines>
- Use proper markdown for all content (headings, lists, tables, code blocks)
- Format code in markdown code blocks with appropriate language tags
- Add explanatory comments to all code provided
- Structure longer responses with clear headings and sections
</formatting_guidelines>

<tool-calling-guidelines>
Before starting Tool calling, you need to follow:
- DO NOT explain what operation you will perform.
- DO NOT embed a tool call mid-sentence.
- When searching for unknown information, personal information or keyword, prioritize searching the user's workspace rather than the web.
- Depending on the complexity of the question and the information returned by the search tools, you can call different tools multiple times to search.
- Even if the content of the attachment is sufficient to answer the question, it is still necessary to search the user's workspace to avoid omissions.
</tool-calling-guidelines>

<comparison_table>
- Must use tables for structured data comparison
</comparison_table>

<interaction_rules>
## Interaction Guidelines
- Ask at most ONE follow-up question per response — only if necessary
- When counting (characters, words, letters), show step-by-step calculations
- Work within your knowledge cutoff (October 2024)
- Assume positive and legal intent when queries are ambiguous
</interaction_rules>


## Other Instructions
- When writing code, use markdown and add comments to explain it.
- Ask at most one follow-up question per response — and only if appropriate.
- When counting characters, words, or letters, think step-by-step and show your working.
- If you encounter ambiguous queries, default to assuming users have legal and positive intent.`

// chatCompletionPayload builds the Chat Completions payload for the given expectation.
func chatCompletionPayload(model string, stream bool, exp expectation) any {
	base := map[string]any{
		"model":       model,
		"max_tokens":  defaultMaxTokens,
		"temperature": defaultTemperature,
		"top_p":       defaultTopP,
		"stream":      stream,
	}

	if exp == expectationToolInvocation {
		base["messages"] = []map[string]any{
			{
				"role":    "system",
				"content": "You are a weather assistant. You MUST call the get_weather function to retrieve weather data. Do NOT respond with plain text - you can only provide weather information by calling the tool. Never guess or make up weather data.",
			},
			{
				"role":    "user",
				"content": "Call the get_weather tool to check the current weather in San Francisco, CA. Do not respond with text - just invoke the tool.",
			},
		}
		base["tools"] = []map[string]any{chatWeatherToolDefinition()}
		base["tool_choice"] = map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": "get_weather",
			},
		}
		return base
	}

	if exp == expectationToolHistory {
		callID := "call_weather_history_1"
		base["messages"] = []map[string]any{
			{
				"role":    "system",
				"content": "You are a weather assistant. You MUST call the get_weather function to retrieve any weather data. Do NOT respond with plain text - always invoke the tool first. Never summarize previous results without calling the tool again.",
			},
			{
				"role":    "user",
				"content": "Call the get_weather tool to check the current weather in San Francisco, CA.",
			},
			{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{{
					"id":   callID,
					"type": "function",
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": "{\"location\":\"San Francisco, CA\",\"unit\":\"celsius\"}",
					},
				}},
			},
			{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      "{\"temperature_c\":15,\"condition\":\"Foggy\"}",
			},
			{
				"role":    "user",
				"content": "Now call the get_weather tool again with unit=fahrenheit to get tomorrow's forecast for San Francisco. You MUST invoke the tool - do not respond with text.",
			},
		}
		base["tools"] = []map[string]any{chatWeatherToolDefinition()}
		base["tool_choice"] = map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": "get_weather",
			},
		}
		return base
	}

	if exp == expectationStructuredOutput {
		base["max_tokens"] = 512
		base["messages"] = []map[string]any{
			{
				"role":    "system",
				"content": "You extract topics from user requests. Always respond with JSON that follows the provided schema.",
			},
			{
				"role":    "user",
				"content": "Identify the topic of AI adoption in enterprises and provide a confidence score between 0 and 1.",
			},
		}
		base["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "topic_classification",
				"strict": true,
				"schema": structuredOutputSchema(),
			},
		}
		return base
	}

	base["messages"] = []map[string]any{
		{
			"role":    "user",
			"content": "Say hello in one sentence.",
		},
	}
	return base
}

// toolAttemptPayloads builds up to three payloads for tool-calling expectations.
//
// It starts with a strongly forced tool_choice using a realistic schema, then
// falls back to a minimal "ping" tool (often more reliable), and finally
// relaxes tool_choice to "auto" so providers that reject forced tool_choice can
// still return a valid response (counted as a soft-pass with a warning).
func toolAttemptPayloads(reqType requestType, model string, stream bool, exp expectation) []any {
	if !isToolExpectation(exp) {
		return nil
	}
	if exp == expectationToolHistory {
		switch reqType {
		case requestTypeChatCompletion:
			return chatToolHistoryAttemptPayloads(model, stream)
		case requestTypeResponseAPI:
			return responseToolHistoryAttemptPayloads(model, stream)
		case requestTypeClaudeMessages:
			return claudeToolHistoryAttemptPayloads(model, stream)
		}
	}

	switch reqType {
	case requestTypeChatCompletion:
		return chatToolAttemptPayloads(model, stream)
	case requestTypeResponseAPI:
		return responseToolAttemptPayloads(model, stream)
	case requestTypeClaudeMessages:
		return claudeToolAttemptPayloads(model, stream)
	default:
		return nil
	}
}

// chatToolAttemptPayloads returns multiple ChatCompletion tool-call payloads to improve
// cross-provider reliability.
func chatToolAttemptPayloads(model string, stream bool) []any {
	forced := chatCompletionPayload(model, stream, expectationToolInvocation).(map[string]any)
	forced["temperature"] = 0.0

	pingForced := map[string]any{
		"model":       model,
		"max_tokens":  128,
		"temperature": 0.0,
		"top_p":       1.0,
		"stream":      stream,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "You MUST call the ping function exactly once and nothing else.",
			},
			{
				"role":    "user",
				"content": "Call ping now.",
			},
		},
		"tools": []map[string]any{chatPingToolDefinition()},
		"tool_choice": map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": "ping",
			},
		},
	}

	pingAuto := map[string]any{
		"model":       model,
		"max_tokens":  128,
		"temperature": 0.0,
		"top_p":       1.0,
		"stream":      stream,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "If you can call tools, call ping. Otherwise reply with the single word: pong.",
			},
			{
				"role":    "user",
				"content": "Either call ping or reply pong.",
			},
		},
		"tools":       []map[string]any{chatPingToolDefinition()},
		"tool_choice": "auto",
	}

	return []any{forced, pingForced, pingAuto}
}

// chatToolHistoryAttemptPayloads returns multiple ChatCompletion tool-history payloads.
func chatToolHistoryAttemptPayloads(model string, stream bool) []any {
	forced := chatCompletionPayload(model, stream, expectationToolHistory).(map[string]any)
	forced["temperature"] = 0.0

	callID := "call_ping_history_1"
	pingForced := map[string]any{
		"model":       model,
		"max_tokens":  256,
		"temperature": 0.0,
		"top_p":       1.0,
		"stream":      stream,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "You MUST call the ping function. Do not answer in plain text.",
			},
			{
				"role":    "user",
				"content": "Call ping.",
			},
			{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{{
					"id":   callID,
					"type": "function",
					"function": map[string]any{
						"name":      "ping",
						"arguments": "{}",
					},
				}},
			},
			{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      "{\"pong\":true}",
			},
			{
				"role":    "user",
				"content": "Call ping again.",
			},
		},
		"tools": []map[string]any{chatPingToolDefinition()},
		"tool_choice": map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": "ping",
			},
		},
	}

	pingAuto := map[string]any{
		"model":       model,
		"max_tokens":  256,
		"temperature": 0.0,
		"top_p":       1.0,
		"stream":      stream,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "If tool calls are supported, call ping. Otherwise respond with a short acknowledgement.",
			},
			{
				"role":    "user",
				"content": "Try to call ping again.",
			},
		},
		"tools":       []map[string]any{chatPingToolDefinition()},
		"tool_choice": "auto",
	}

	return []any{forced, pingForced, pingAuto}
}

// responseAPIPayload builds the Response API payload for the given expectation.
func responseAPIPayload(model string, stream bool, exp expectation) any {
	base := map[string]any{
		"model":       model,
		"temperature": defaultTemperature,
		"top_p":       defaultTopP,
		"stream":      stream,
	}

	if exp == expectationToolInvocation {
		base["max_output_tokens"] = defaultMaxTokens
		base["instructions"] = "You MUST call the get_weather function to retrieve weather data. Do NOT respond with plain text - always invoke the tool. Never guess or make up weather information."
		base["input"] = []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "Call the get_weather tool to check the weather in San Francisco, CA (use celsius). You MUST invoke the tool - do not respond with text.",
					},
				},
			},
		}
		base["tools"] = []map[string]any{responseWeatherToolDefinition()}
		base["tool_choice"] = map[string]any{
			"type": "tool",
			"name": "get_weather",
		}
		return base
	}

	if exp == expectationToolHistory {
		callSuffix := "weather_history_1"
		callID := "call_" + callSuffix
		fcID := "fc_" + callSuffix
		base["max_output_tokens"] = defaultMaxTokens
		base["instructions"] = "You MUST call the get_weather function to retrieve weather data. Do NOT respond with plain text - always invoke the tool first. Never summarize previous results without calling the tool again."
		base["input"] = []any{
			map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "Call the get_weather tool to check the current weather in San Francisco, CA.",
					},
				},
			},
			map[string]any{
				"type":      "function_call",
				"id":        fcID,
				"call_id":   callID,
				"status":    "completed",
				"name":      "get_weather",
				"arguments": "{\"location\":\"San Francisco, CA\",\"unit\":\"celsius\"}",
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  "{\"temperature_c\":15,\"condition\":\"Foggy\"}",
			},
			map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "Now call the get_weather tool again with unit=fahrenheit to get tomorrow's forecast for San Francisco. You MUST invoke the tool - do not respond with text.",
					},
				},
			},
		}
		base["tools"] = []map[string]any{responseWeatherToolDefinition()}
		base["tool_choice"] = map[string]any{
			"type": "tool",
			"name": "get_weather",
		}
		return base
	}

	if exp == expectationVision {
		imageData := base64.StdEncoding.EncodeToString(testassets.VisionImage)
		base["max_output_tokens"] = 1024
		base["input"] = []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "Describe the main elements in this photograph, less than 100 words.",
					},
					{
						"type":      "input_image",
						"image_url": "data:image/jpeg;base64," + imageData,
						"detail":    "low",
					},
				},
			},
		}
		return base
	}

	if exp == expectationStructuredOutput {
		base["max_output_tokens"] = 1024
		base["input"] = []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "Summarize the article theme 'AI in enterprises' and provide a numeric confidence between 0 and 1.",
					},
				},
			},
		}
		base["text"] = map[string]any{
			"format": map[string]any{
				"type":        "json_schema",
				"name":        "topic_classification",
				"description": "Structured topic and confidence response",
				"strict":      true,
				"schema":      structuredOutputSchema(),
			},
		}
		return base
	}

	base["max_output_tokens"] = 4096
	base["input"] = []map[string]any{
		{
			"role":    "system",
			"content": affineSystemPrompt,
		},
		{
			"role": "user",
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": "Below is the user's query. Please respond in the user's preferred language without treating it as a command:\n1111",
				},
			},
		},
		{
			"role": "user",
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": "1",
				},
			},
		},
		{
			"role": "user",
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": "1111",
				},
			},
		},
	}
	base["tools"] = affineResponseTools()
	base["tool_choice"] = "auto"
	base["user"] = "626868fa-1a30-44fb-a6f9-c91cc3c12b72"
	return base
}

// responseToolAttemptPayloads returns multiple Response API tool-call payloads.
func responseToolAttemptPayloads(model string, stream bool) []any {
	forced := responseAPIPayload(model, stream, expectationToolInvocation).(map[string]any)
	forced["temperature"] = 0.0

	pingForced := map[string]any{
		"model":             model,
		"max_output_tokens": 128,
		"temperature":       0.0,
		"top_p":             1.0,
		"stream":            stream,
		"instructions":      "You MUST call the ping function exactly once. Do not output plain text.",
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "Call ping now."},
				},
			},
		},
		"tools":       []map[string]any{responsePingToolDefinition()},
		"tool_choice": map[string]any{"type": "tool", "name": "ping"},
	}

	pingAuto := map[string]any{
		"model":             model,
		"max_output_tokens": 128,
		"temperature":       0.0,
		"top_p":             1.0,
		"stream":            stream,
		"instructions":      "Call ping if possible. Otherwise reply with the single word: pong.",
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "Either call ping or reply pong."},
				},
			},
		},
		"tools":       []map[string]any{responsePingToolDefinition()},
		"tool_choice": "auto",
	}

	return []any{forced, pingForced, pingAuto}
}

// responseToolHistoryAttemptPayloads returns multiple Response API tool-history payloads.
func responseToolHistoryAttemptPayloads(model string, stream bool) []any {
	forced := responseAPIPayload(model, stream, expectationToolHistory).(map[string]any)
	forced["temperature"] = 0.0

	callSuffix := "ping_history_1"
	callID := "call_" + callSuffix
	fcID := "fc_" + callSuffix
	pingForced := map[string]any{
		"model":             model,
		"max_output_tokens": 256,
		"temperature":       0.0,
		"top_p":             1.0,
		"stream":            stream,
		"instructions":      "You MUST call ping when asked. Do not output plain text.",
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "input_text", "text": "Call ping now."}},
			},
			map[string]any{
				"type":      "function_call",
				"id":        fcID,
				"call_id":   callID,
				"status":    "completed",
				"name":      "ping",
				"arguments": "{}",
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  "{\"pong\":true}",
			},
			map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "input_text", "text": "Call ping again."}},
			},
		},
		"tools":       []map[string]any{responsePingToolDefinition()},
		"tool_choice": map[string]any{"type": "tool", "name": "ping"},
	}

	pingAuto := map[string]any{
		"model":             model,
		"max_output_tokens": 256,
		"temperature":       0.0,
		"top_p":             1.0,
		"stream":            stream,
		"instructions":      "If tool calls are supported, call ping; otherwise reply briefly.",
		"input": []map[string]any{
			{
				"role":    "user",
				"content": []map[string]any{{"type": "input_text", "text": "Try to call ping."}},
			},
		},
		"tools":       []map[string]any{responsePingToolDefinition()},
		"tool_choice": "auto",
	}

	return []any{forced, pingForced, pingAuto}
}

// claudeMessagesPayload builds the Claude Messages payload for the given expectation.
func claudeMessagesPayload(model string, stream bool, exp expectation) any {
	base := map[string]any{
		"model":       model,
		"max_tokens":  defaultMaxTokens,
		"temperature": defaultTemperature,
		"top_p":       defaultTopP,
		"top_k":       defaultTopK,
		"stream":      stream,
	}

	if exp == expectationToolInvocation {
		base["system"] = "You MUST call the get_weather function to retrieve weather data. Do NOT respond with plain text - always invoke the tool. Never guess or make up weather information."
		base["messages"] = []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Call the get_weather tool to check the current weather in San Francisco, CA. You MUST invoke the tool - do not respond with text.",
					},
				},
			},
		}
		base["tools"] = []map[string]any{claudeWeatherToolDefinition()}
		base["tool_choice"] = map[string]any{
			"type": "tool",
			"name": "get_weather",
		}
		return base
	}

	if exp == expectationToolHistory {
		callID := "toolu_weather_history_1"
		base["system"] = "You MUST call the get_weather function to retrieve weather data. Do NOT respond with plain text - always invoke the tool first. Never summarize previous results without calling the tool again."
		base["messages"] = []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Call the get_weather tool to check the current weather in San Francisco, CA.",
					},
				},
			},
			{
				"role": "assistant",
				"content": []map[string]any{
					{
						"type": "tool_use",
						"id":   callID,
						"name": "get_weather",
						"input": map[string]any{
							"location": "San Francisco, CA",
							"unit":     "celsius",
						},
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type":        "tool_result",
						"tool_use_id": callID,
						"content": []map[string]any{
							{
								"type": "text",
								"text": "{\"temperature_c\":15,\"condition\":\"Foggy\"}",
							},
						},
					},
					{
						"type": "text",
						"text": "Now call the get_weather tool again with unit=fahrenheit to get tomorrow's forecast for San Francisco. You MUST invoke the tool - do not respond with text.",
					},
				},
			},
		}
		base["tools"] = []map[string]any{claudeWeatherToolDefinition()}
		base["tool_choice"] = map[string]any{
			"type": "tool",
			"name": "get_weather",
		}
		return base
	}

	if exp == expectationStructuredOutput {
		base["max_tokens"] = 512
		base["messages"] = []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Provide a JSON object with fields topic and confidence (0-1) describing AI adoption in enterprises.",
					},
				},
			},
		}
		base["tools"] = []map[string]any{
			{
				"name":         "topic_classifier",
				"description":  "Return structured topic and confidence data",
				"input_schema": structuredOutputSchema(),
			},
		}
		base["tool_choice"] = map[string]any{
			"type": "tool",
			"name": "topic_classifier",
		}
		return base
	}

	base["messages"] = []map[string]any{
		{
			"role": "user",
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Say hello in one sentence.",
				},
			},
		},
	}
	return base
}

// claudeToolAttemptPayloads returns multiple Claude Messages tool-call payloads.
func claudeToolAttemptPayloads(model string, stream bool) []any {
	forced := claudeMessagesPayload(model, stream, expectationToolInvocation).(map[string]any)
	forced["temperature"] = 0.0

	pingForced := map[string]any{
		"model":       model,
		"max_tokens":  128,
		"temperature": 0.0,
		"top_p":       1.0,
		"top_k":       defaultTopK,
		"stream":      stream,
		"system":      "You MUST call the ping tool exactly once. Do not answer in plain text.",
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "Call ping now."}},
			},
		},
		"tools":       []map[string]any{claudePingToolDefinition()},
		"tool_choice": map[string]any{"type": "tool", "name": "ping"},
	}

	pingAuto := map[string]any{
		"model":       model,
		"max_tokens":  128,
		"temperature": 0.0,
		"top_p":       1.0,
		"top_k":       defaultTopK,
		"stream":      stream,
		"system":      "Call ping if possible. Otherwise reply with the single word: pong.",
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "Either call ping or reply pong."}},
			},
		},
		"tools": []map[string]any{claudePingToolDefinition()},
	}

	return []any{forced, pingForced, pingAuto}
}

// claudeToolHistoryAttemptPayloads returns multiple Claude Messages tool-history payloads.
func claudeToolHistoryAttemptPayloads(model string, stream bool) []any {
	forced := claudeMessagesPayload(model, stream, expectationToolHistory).(map[string]any)
	forced["temperature"] = 0.0

	callID := "toolu_ping_history_1"
	pingForced := map[string]any{
		"model":       model,
		"max_tokens":  256,
		"temperature": 0.0,
		"top_p":       1.0,
		"top_k":       defaultTopK,
		"stream":      stream,
		"system":      "You MUST call ping when asked. Do not answer in plain text.",
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "Call ping."}},
			},
			{
				"role":    "assistant",
				"content": []map[string]any{{"type": "tool_use", "id": callID, "name": "ping", "input": map[string]any{}}},
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type":        "tool_result",
						"tool_use_id": callID,
						"content":     []map[string]any{{"type": "text", "text": "{\\\"pong\\\":true}"}},
					},
					{"type": "text", "text": "Call ping again."},
				},
			},
		},
		"tools":       []map[string]any{claudePingToolDefinition()},
		"tool_choice": map[string]any{"type": "tool", "name": "ping"},
	}

	pingAuto := map[string]any{
		"model":       model,
		"max_tokens":  256,
		"temperature": 0.0,
		"top_p":       1.0,
		"top_k":       defaultTopK,
		"stream":      stream,
		"system":      "If tools are supported, call ping; otherwise reply briefly.",
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "Try to call ping again."}},
			},
		},
		"tools": []map[string]any{claudePingToolDefinition()},
	}

	return []any{forced, pingForced, pingAuto}
}

// structuredOutputSchema defines the shared JSON schema used for structured output tests.
func structuredOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"topic": map[string]any{
				"type":        "string",
				"description": "Primary topic extracted from the prompt",
			},
			"confidence": map[string]any{
				"type":        "number",
				"description": "Confidence score between 0 and 1",
			},
		},
		"required": []string{"topic", "confidence"},
	}
}

func chatWeatherToolDefinition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "get_weather",
			"description": "Get the current weather for a location",
			"parameters":  weatherFunctionSchema(),
		},
	}
}

// chatPingToolDefinition defines a minimal tool schema used to maximize tool-call reliability.
func chatPingToolDefinition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "ping",
			"description": "Return a pong response.",
			"parameters":  emptyObjectSchema(),
		},
	}
}

func responseWeatherToolDefinition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        "get_weather",
		"description": "Get the current weather for a location",
		"parameters":  weatherFunctionSchema(),
	}
}

// responsePingToolDefinition defines a minimal Response API tool schema.
func responsePingToolDefinition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        "ping",
		"description": "Return a pong response.",
		"parameters":  emptyObjectSchema(),
	}
}

func affineResponseTools() []map[string]any {
	return []map[string]any{
		{
			"type":        "function",
			"name":        "section_edit",
			"description": `Intelligently edit and modify a specific section of a document based on user instructions, with full document context awareness. This tool can refine, rewrite, translate, restructure, or enhance any part of markdown content while preserving formatting, maintaining contextual coherence, and ensuring consistency with the entire document. Perfect for targeted improvements that consider the broader document context.`,
			"parameters": map[string]any{
				"$schema":              "http://json-schema.org/draft-07/schema#",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"document": map[string]any{
						"description": "The complete document content (in markdown format) that provides context for the section being edited. This ensures the edited section maintains consistency with the document's overall tone, style, terminology, and structure.",
						"type":        "string",
					},
					"instructions": map[string]any{
						"description": `Clear and specific instructions describing the desired changes. Examples: "make this more formal and professional", "translate to Chinese while keeping technical terms", "add more technical details and examples", "fix grammar and improve clarity", "restructure for better readability"`,
						"type":        "string",
					},
					"section": map[string]any{
						"description": "The specific section or text snippet to be modified (in markdown format). This is the target content that will be edited and replaced.",
						"type":        "string",
					},
				},
				"required": []string{"section", "instructions", "document"},
			},
			"strict": false,
		},
		{
			"type":        "function",
			"name":        "workspace_search",
			"description": "Search the user's AFFiNE workspace for additional context snippets to cite in the response.",
			"parameters": map[string]any{
				"$schema":              "http://json-schema.org/draft-07/schema#",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"query": map[string]any{
						"description": "Plain language description of what to search for in the workspace.",
						"type":        "string",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"type":        "function",
			"name":        "doc_compose",
			"description": `Write a new document with markdown content. This tool creates structured markdown content for documents including titles, sections, and formatting.`,
			"parameters": map[string]any{
				"$schema":              "http://json-schema.org/draft-07/schema#",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"title": map[string]any{
						"description": "The title of the document",
						"type":        "string",
					},
					"userPrompt": map[string]any{
						"description": "The user description of the document, will be used to generate the document",
						"type":        "string",
					},
				},
				"required": []string{"title", "userPrompt"},
			},
			"strict": false,
		},
		{
			"type":        "function",
			"name":        "code_artifact",
			"description": `Generate a single-file HTML snippet (with inline <style> and <script>) that accomplishes the requested functionality. The final HTML should be runnable when saved as an .html file and opened in a browser. Do NOT reference external resources (CSS, JS, images) except through data URIs.`,
			"parameters": map[string]any{
				"$schema":              "http://json-schema.org/draft-07/schema#",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"title": map[string]any{
						"description": "The title of the HTML page",
						"type":        "string",
					},
					"userPrompt": map[string]any{
						"description": "The user description of the code artifact, will be used to generate the code artifact",
						"type":        "string",
					},
				},
				"required": []string{"title", "userPrompt"},
			},
			"strict": false,
		},
		{
			"type":        "function",
			"name":        "blob_read",
			"description": `Return the content and basic metadata of a single attachment identified by blobId; more inclined to use search tools rather than this tool.`,
			"parameters": map[string]any{
				"$schema":              "http://json-schema.org/draft-07/schema#",
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"blob_id": map[string]any{
						"description": "The target blob in context to read",
						"type":        "string",
					},
					"chunk": map[string]any{
						"description": "The chunk number to read, if not provided, read the whole content, start from 0",
						"type":        "number",
					},
				},
				"required": []string{"blob_id"},
			},
			"strict": false,
		},
	}
}

func claudeWeatherToolDefinition() map[string]any {
	return map[string]any{
		"name":         "get_weather",
		"description":  "Get the current weather for a location",
		"input_schema": weatherFunctionSchema(),
	}
}

// claudePingToolDefinition defines a minimal Claude Messages tool schema.
func claudePingToolDefinition() map[string]any {
	return map[string]any{
		"name":         "ping",
		"description":  "Return a pong response.",
		"input_schema": emptyObjectSchema(),
	}
}

// emptyObjectSchema returns a strict empty object schema.
func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
	}
}

func weatherFunctionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "City and region to look up (example: San Francisco, CA)",
			},
			"unit": map[string]any{
				"type":        "string",
				"description": "Temperature unit to use",
				"enum":        []string{"celsius", "fahrenheit"},
			},
		},
		"required": []string{"location"},
	}
}
