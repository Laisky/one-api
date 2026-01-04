package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

// TestStructuredOutputEdgeCases tests edge cases and error handling
func TestStructuredOutputEdgeCases(t *testing.T) {
	t.Run("empty schema handling", func(t *testing.T) {
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "user", Content: "Test"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_object", // No JsonSchema field
			},
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		require.NotNil(t, responseAPI.Text, "Expected text to be set")
		require.NotNil(t, responseAPI.Text.Format, "Expected text format to be set even with empty schema")
		require.Equal(t, "json_object", responseAPI.Text.Format.Type, "Expected type 'json_object'")
	})

	t.Run("nil response format", func(t *testing.T) {
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "user", Content: "Test"},
			},
			ResponseFormat: nil,
		}
		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		require.Nil(t, responseAPI.Text, "Expected text to be nil when no response format")
	})

	t.Run("response without text field", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_no_text",
			Status: "completed",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: "Simple response"},
					},
				},
			},
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		require.Len(t, chatCompletion.Choices, 1, "Expected response conversion to work without text field")
	})

	t.Run("malformed JSON handling", func(t *testing.T) {
		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_malformed",
			Status: "completed",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: `{"incomplete": json`},
					},
				},
			},
		}
		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok && content == `{"incomplete": json`, "Expected malformed JSON to be preserved as-is")
	})
}

// TestAdvancedStructuredOutputScenarios tests advanced real-world scenarios
func TestAdvancedStructuredOutputScenarios(t *testing.T) {
	t.Run("research paper extraction", func(t *testing.T) {
		// Complex schema for research paper extraction
		researchSchema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "The title of the research paper",
				},
				"authors": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Author's full name",
							},
							"affiliation": map[string]any{
								"type":        "string",
								"description": "Author's institutional affiliation",
							},
							"email": map[string]any{
								"type":        "string",
								"format":      "email",
								"description": "Author's email address",
							},
						},
						"required": []string{"name"},
					},
					"minItems": 1,
				},
				"abstract": map[string]any{
					"type":        "string",
					"description": "The paper's abstract",
					"minLength":   50,
				},
				"keywords": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"minItems": 3,
					"maxItems": 10,
				},
				"sections": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title": map[string]any{"type": "string"},
							"content": map[string]any{
								"type":      "string",
								"minLength": 100,
							},
							"subsections": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"title":   map[string]any{"type": "string"},
										"content": map[string]any{"type": "string"},
									},
								},
							},
						},
						"required": []string{"title", "content"},
					},
				},
				"references": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title":   map[string]any{"type": "string"},
							"authors": map[string]any{"type": "array"},
							"journal": map[string]any{"type": "string"},
							"year":    map[string]any{"type": "integer"},
							"doi":     map[string]any{"type": "string"},
							"url":     map[string]any{"type": "string"},
						},
						"required": []string{"title", "authors"},
					},
				},
			},
			"required":             []string{"title", "authors", "abstract", "keywords"},
			"additionalProperties": false,
		}

		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "system", Content: "You are an expert at structured data extraction from academic papers."},
				{Role: "user", Content: "Extract information from this research paper: [complex paper content would go here]"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "research_paper_extraction",
					Description: "Extract structured information from research papers",
					Schema:      researchSchema,
					Strict:      boolPtr(true),
				},
			},
		}

		// Convert to Response API
		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify complex nested schema preservation
		require.NotNil(t, responseAPI.Text, "Expected text to be set for complex schema")
		require.NotNil(t, responseAPI.Text.Format, "Expected text format to be set for complex schema")

		schema := responseAPI.Text.Format.Schema
		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in research schema")

		// Verify authors array structure
		authors, ok := properties["authors"].(map[string]any)
		require.True(t, ok, "Expected authors property")

		items, ok := authors["items"].(map[string]any)
		require.True(t, ok, "Expected items in authors array")

		authorProps, ok := items["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in author items")
		require.Len(t, authorProps, 3, "Expected 3 author properties (name, affiliation, email)")

		// Verify sections nested structure
		sections, ok := properties["sections"].(map[string]any)
		require.True(t, ok, "Expected sections property")

		sectionItems, ok := sections["items"].(map[string]any)
		require.True(t, ok, "Expected items in sections array")

		sectionProps, ok := sectionItems["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in section items")

		// Verify subsections nested array
		subsections, ok := sectionProps["subsections"].(map[string]any)
		require.True(t, ok, "Expected subsections property")
		require.Equal(t, "array", subsections["type"], "Expected subsections type 'array'")

		// Test reverse conversion with complex data
		complexResponseData := `{
			"title": "Quantum Machine Learning Applications",
			"authors": [
				{
					"name": "Dr. Alice Quantum",
					"affiliation": "MIT Quantum Lab",
					"email": "alice@mit.edu"
				}
			],
			"abstract": "This paper explores the intersection of quantum computing and machine learning, demonstrating novel applications in optimization problems.",
			"keywords": ["quantum computing", "machine learning", "optimization", "quantum algorithms"],
			"sections": [
				{
					"title": "Introduction",
					"content": "Quantum computing represents a paradigm shift in computational capabilities, offering exponential speedups for certain classes of problems.",
					"subsections": [
						{
							"title": "Background",
							"content": "Historical context of quantum computing development."
						}
					]
				}
			],
			"references": [
				{
					"title": "Quantum Computing: An Applied Approach",
					"authors": ["Hidary, J."],
					"journal": "Springer",
					"year": 2019
				}
			]
		}`

		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_research",
			Status: "completed",
			Model:  "gpt-4o-2024-08-06",
			Output: []OutputItem{
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: complexResponseData},
					},
				},
			},
			Text: responseAPI.Text,
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok, "Expected content to be string")

		// Verify the complex JSON structure
		var extracted map[string]any
		err := json.Unmarshal([]byte(content), &extracted)
		require.NoError(t, err, "Failed to parse complex JSON")
		require.Equal(t, "Quantum Machine Learning Applications", extracted["title"], "Expected title to match")

		authorsArray, ok := extracted["authors"].([]any)
		require.True(t, ok && len(authorsArray) == 1, "Expected 1 author")

		author, ok := authorsArray[0].(map[string]any)
		require.True(t, ok, "Expected author to be object")
		require.Equal(t, "Dr. Alice Quantum", author["name"], "Expected author name to match")
	})

	t.Run("UI component generation with recursive schema", func(t *testing.T) {
		// Recursive UI component schema
		uiSchema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"description": "The type of the UI component",
					"enum":        []string{"div", "button", "header", "section", "field", "form"},
				},
				"label": map[string]any{
					"type":        "string",
					"description": "The label of the UI component",
				},
				"children": map[string]any{
					"type":        "array",
					"description": "Nested UI components",
					"items":       map[string]any{"$ref": "#"},
				},
				"attributes": map[string]any{
					"type":        "array",
					"description": "Arbitrary attributes for the UI component",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "The name of the attribute",
							},
							"value": map[string]any{
								"type":        "string",
								"description": "The value of the attribute",
							},
						},
						"required":             []string{"name", "value"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"type", "label", "children", "attributes"},
			"additionalProperties": false,
		}

		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-4o-2024-08-06",
			Messages: []model.Message{
				{Role: "system", Content: "You are a UI generator AI."},
				{Role: "user", Content: "Create a user registration form"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "ui_component",
					Description: "Dynamically generated UI component",
					Schema:      uiSchema,
					Strict:      boolPtr(true),
				},
			},
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify recursive schema handling
		schema := responseAPI.Text.Format.Schema
		properties, ok := schema["properties"].(map[string]any)
		require.True(t, ok, "Expected properties in UI schema")

		children, ok := properties["children"].(map[string]any)
		require.True(t, ok, "Expected children property")

		items, ok := children["items"].(map[string]any)
		require.True(t, ok, "Expected items in children")

		// Check for $ref (recursive reference)
		require.Equal(t, "#", items["$ref"], "Expected recursive reference '#'")

		// Test with complex nested UI structure
		uiResponseData := `{
			"type": "form",
			"label": "User Registration",
			"children": [
				{
					"type": "field",
					"label": "Username",
					"children": [],
					"attributes": [
						{"name": "type", "value": "text"},
						{"name": "required", "value": "true"}
					]
				},
				{
					"type": "div",
					"label": "Password Section",
					"children": [
						{
							"type": "field",
							"label": "Password",
							"children": [],
							"attributes": [
								{"name": "type", "value": "password"},
								{"name": "minlength", "value": "8"}
							]
						}
					],
					"attributes": [
						{"name": "class", "value": "password-section"}
					]
				}
			],
			"attributes": [
				{"name": "method", "value": "post"},
				{"name": "action", "value": "/register"}
			]
		}`

		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_ui",
			Status: "completed",
			Output: []OutputItem{
				{
					Type: "message",
					Role: "assistant",
					Content: []OutputContent{
						{Type: "output_text", Text: uiResponseData},
					},
				},
			},
			Text: responseAPI.Text,
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)
		content, ok := chatCompletion.Choices[0].Message.Content.(string)
		require.True(t, ok, "Expected UI content to be string")

		var uiComponent map[string]any
		err := json.Unmarshal([]byte(content), &uiComponent)
		require.NoError(t, err, "Failed to parse UI JSON")
		require.Equal(t, "form", uiComponent["type"], "Expected type 'form'")

		childrenArray, ok := uiComponent["children"].([]any)
		require.True(t, ok && len(childrenArray) == 2, "Expected 2 children")

		// Verify nested structure
		passwordSection, ok := childrenArray[1].(map[string]any)
		require.True(t, ok, "Expected password section to be object")

		nestedChildren, ok := passwordSection["children"].([]any)
		require.True(t, ok && len(nestedChildren) == 1, "Expected 1 nested child")
	})

	t.Run("chain of thought with structured output", func(t *testing.T) {
		// Chain of thought with structured output
		reasoningSchema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"step_number": map[string]any{
								"type":        "integer",
								"description": "The step number in the reasoning process",
							},
							"explanation": map[string]any{
								"type":        "string",
								"description": "Explanation of this step",
							},
							"calculation": map[string]any{
								"type":        "string",
								"description": "Mathematical calculation for this step",
							},
							"result": map[string]any{
								"type":        "string",
								"description": "Result of this step",
							},
						},
						"required": []string{"step_number", "explanation", "result"},
					},
				},
				"final_answer": map[string]any{
					"type":        "string",
					"description": "The final answer to the problem",
				},
				"verification": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"method": map[string]any{
							"type":        "string",
							"description": "Method used to verify the answer",
						},
						"check": map[string]any{
							"type":        "string",
							"description": "Verification calculation",
						},
						"confirmed": map[string]any{
							"type":        "boolean",
							"description": "Whether the answer is confirmed",
						},
					},
					"required": []string{"method", "confirmed"},
				},
			},
			"required": []string{"steps", "final_answer", "verification"},
		}

		chatRequest := &model.GeneralOpenAIRequest{
			Model: "o3-2025-04-16", // Reasoning model
			Messages: []model.Message{
				{Role: "system", Content: "You are a math tutor. Show your reasoning step by step."},
				{Role: "user", Content: "Solve: 3x + 7 = 22"},
			},
			ResponseFormat: &model.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &model.JSONSchema{
					Name:        "math_reasoning",
					Description: "Step-by-step mathematical reasoning",
					Schema:      reasoningSchema,
					Strict:      boolPtr(true),
				},
			},
			ReasoningEffort: stringPtr("high"),
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

		// Verify reasoning effort is normalized to medium for medium-only models
		require.NotNil(t, responseAPI.Reasoning, "Expected reasoning config to be set")
		require.NotNil(t, responseAPI.Reasoning.Effort, "Expected reasoning effort to be set")
		require.Equal(t, "medium", *responseAPI.Reasoning.Effort, "Expected reasoning effort 'medium'")

		// Simulate response with reasoning and structured output
		reasoningResponseData := `{
			"steps": [
				{
					"step_number": 1,
					"explanation": "Start with the equation 3x + 7 = 22",
					"calculation": "3x + 7 = 22",
					"result": "Initial equation established"
				},
				{
					"step_number": 2,
					"explanation": "Subtract 7 from both sides",
					"calculation": "3x + 7 - 7 = 22 - 7",
					"result": "3x = 15"
				},
				{
					"step_number": 3,
					"explanation": "Divide both sides by 3",
					"calculation": "3x / 3 = 15 / 3",
					"result": "x = 5"
				}
			],
			"final_answer": "x = 5",
			"verification": {
				"method": "substitution",
				"check": "3(5) + 7 = 15 + 7 = 22 ✓",
				"confirmed": true
			}
		}`

		responseAPIResp := &ResponseAPIResponse{
			Id:     "resp_reasoning",
			Status: "completed",
			Model:  "o3-2025-04-16",
			Output: []OutputItem{
				{
					Type: "reasoning",
					Summary: []OutputContent{
						{
							Type: "summary_text",
							Text: "I need to solve the linear equation 3x + 7 = 22 step by step, showing each mathematical operation clearly.",
						},
					},
				},
				{
					Type:   "message",
					Role:   "assistant",
					Status: "completed",
					Content: []OutputContent{
						{Type: "output_text", Text: reasoningResponseData},
					},
				},
			},
			Reasoning: responseAPI.Reasoning,
			Text:      responseAPI.Text,
		}

		chatCompletion := ConvertResponseAPIToChatCompletion(responseAPIResp)

		// Verify reasoning is preserved
		require.Len(t, chatCompletion.Choices, 1, "Expected 1 choice")

		choice := chatCompletion.Choices[0]
		require.NotNil(t, choice.Message.Reasoning, "Expected reasoning content to be preserved")

		expectedReasoning := "I need to solve the linear equation 3x + 7 = 22 step by step, showing each mathematical operation clearly."
		require.Equal(t, expectedReasoning, *choice.Message.Reasoning, "Expected reasoning content to match")

		// Verify structured reasoning content
		content, ok := choice.Message.Content.(string)
		require.True(t, ok, "Expected content to be string")

		var reasoning map[string]any
		err := json.Unmarshal([]byte(content), &reasoning)
		require.NoError(t, err, "Failed to parse reasoning JSON")

		steps, ok := reasoning["steps"].([]any)
		require.True(t, ok && len(steps) == 3, "Expected 3 reasoning steps")
		require.Equal(t, "x = 5", reasoning["final_answer"], "Expected final answer 'x = 5'")
	})
}

func TestConvertChatCompletionToResponseAPI_DeepResearchDefaultEffort(t *testing.T) {
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "o4-mini-deep-research",
		Messages: []model.Message{
			{Role: "user", Content: "Conduct a deep research summary."},
		},
	}

	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	require.NotNil(t, responseAPI.Reasoning, "expected reasoning config to be set for deep research model")
	require.NotNil(t, responseAPI.Reasoning.Effort, "expected reasoning effort to be set")
	require.Equal(t, "medium", *responseAPI.Reasoning.Effort, "expected reasoning effort to default to 'medium'")
	require.NotNil(t, chatRequest.ReasoningEffort, "expected source request reasoning effort to be set")
	require.Equal(t, "medium", *chatRequest.ReasoningEffort, "expected source request reasoning effort to be normalized to 'medium'")
}
