package mcp

import "fmt"

// Documentation generation functions (migrated from existing implementation)
func generateChatCompletionsDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Chat Completions API

## Endpoint
POST %s/v1/chat/completions

## Description
Creates a model response for the given chat conversation. This is the main endpoint for conversational AI interactions.

## Authentication
Include your API key in the Authorization header:
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-4",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello!"}
    ],
    "temperature": 0.7,
    "max_tokens": 150
  }'
`+"```"+`

## Parameters
- **model** (required): The model to use (e.g., gpt-4, gpt-3.5-turbo)
- **messages** (required): Array of message objects with role and content
- **temperature**: Controls randomness (0.0 to 2.0, default: 1.0)
- **max_tokens**: Maximum tokens to generate
- **stream**: Set to true for streaming responses

## Response Format
`+"```"+`json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "gpt-4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}
`+"```", baseURL, baseURL)
}

func generateCompletionsDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Completions API

## Endpoint
POST %s/v1/completions

## Description
Creates a completion for the provided prompt and parameters.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "gpt-3.5-turbo-instruct",
    "prompt": "Once upon a time",
    "max_tokens": 50,
    "temperature": 0.7
  }'
`+"```"+`

## Key Parameters
- **model** (required): Model identifier
- **prompt** (required): Text prompt to complete
- **max_tokens**: Maximum tokens to generate
- **temperature**: Sampling temperature (0.0 to 2.0)`, baseURL, baseURL)
}

func generateEmbeddingsDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Embeddings API

## Endpoint
POST %s/v1/embeddings

## Description
Creates an embedding vector representing the input text.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "text-embedding-ada-002",
    "input": "The food was delicious and the waiter..."
  }'
`+"```"+`

## Parameters
- **model** (required): Embedding model (e.g., text-embedding-ada-002)
- **input** (required): Input text to embed`, baseURL, baseURL)
}

func generateImagesDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Image Generation API

## Endpoint
POST %s/v1/images/generations

## Description
Creates images from text prompts.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "dall-e-3",
    "prompt": "A cute baby sea otter",
    "n": 1,
    "size": "1024x1024"
  }'
`+"```"+`

## Parameters
- **model** (required): Image generation model (dall-e-3, dall-e-2)
- **prompt** (required): Text description of desired image
- **n**: Number of images to generate (1-10)
- **size**: Image size (256x256, 512x512, 1024x1024)`, baseURL, baseURL)
}

func generateAudioTranscriptionsDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Audio Transcriptions API

## Endpoint
POST %s/v1/audio/transcriptions

## Description
Transcribes audio into the input language.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/audio/transcriptions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -F file="@audio.mp3" \
  -F model="whisper-1"
`+"```"+`

## Parameters
- **file** (required): Audio file (flac, mp3, mp4, mpeg, mpga, m4a, ogg, wav, webm)
- **model** (required): Model identifier (whisper-1)
- **language**: Language of input audio (ISO-639-1 format)`, baseURL, baseURL)
}

func generateAudioTranslationsDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Audio Translations API

## Endpoint
POST %s/v1/audio/translations

## Description
Translates audio into English.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/audio/translations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -F file="@audio.mp3" \
  -F model="whisper-1"
`+"```"+`

## Parameters
- **file** (required): Audio file to translate
- **model** (required): Model identifier (whisper-1)`, baseURL, baseURL)
}

func generateAudioSpeechDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Audio Speech API

## Endpoint
POST %s/v1/audio/speech

## Description
Generates audio from input text.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/audio/speech \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "tts-1",
    "input": "Hello world",
    "voice": "alloy"
  }'
`+"```"+`

## Parameters
- **model** (required): TTS model (tts-1, tts-1-hd)
- **input** (required): Text to generate audio for
- **voice** (required): Voice option (alloy, echo, fable, onyx, nova, shimmer)`, baseURL, baseURL)
}

func generateModerationsDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Moderations API

## Endpoint
POST %s/v1/moderations

## Description
Classifies if text violates OpenAI's Usage Policies.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/moderations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "input": "I want to kill them."
  }'
`+"```"+`

## Parameters
- **input** (required): Text to classify
- **model**: Moderation model (text-moderation-stable, text-moderation-latest)`, baseURL, baseURL)
}

func generateModelsListDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Models List API

## Endpoint
GET %s/v1/models

## Description
Lists the currently available models and provides basic information about each.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/models \
  -H "Authorization: Bearer YOUR_API_KEY"
`+"```"+`

## Response
Returns a list of model objects containing id, object type, and other metadata.`, baseURL, baseURL)
}

func generateClaudeMessagesDocumentation(baseURL string) string {
	return fmt.Sprintf(`# Claude Messages API

## Endpoint
POST %s/v1/messages

## Description
Creates messages using Claude API format.

## Authentication
Authorization: Bearer YOUR_API_KEY

## Example Request
`+"```"+`bash
curl %s/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "claude-3-opus-20240229",
    "max_tokens": 1000,
    "messages": [
      {"role": "user", "content": "Hello, Claude!"}
    ]
  }'
`+"```"+`

## Parameters
- **model** (required): Claude model identifier
- **messages** (required): Array of message objects
- **max_tokens** (required): Maximum tokens to generate`, baseURL, baseURL)
}
