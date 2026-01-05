package channeltype

import (
	"slices"
	"strings"

	"github.com/songquanpeng/one-api/relay/relaymode"
)

// Endpoint represents an API endpoint type that can be enabled/disabled per channel.
// The value corresponds to the relaymode constant for that endpoint.
type Endpoint int

// Endpoint constants matching relaymode values for API endpoints.
// These are used for endpoint configuration and validation.
const (
	EndpointChatCompletions    Endpoint = Endpoint(relaymode.ChatCompletions)
	EndpointCompletions        Endpoint = Endpoint(relaymode.Completions)
	EndpointEmbeddings         Endpoint = Endpoint(relaymode.Embeddings)
	EndpointModerations        Endpoint = Endpoint(relaymode.Moderations)
	EndpointImagesGenerations  Endpoint = Endpoint(relaymode.ImagesGenerations)
	EndpointImagesEdits        Endpoint = Endpoint(relaymode.ImagesEdits)
	EndpointAudioSpeech        Endpoint = Endpoint(relaymode.AudioSpeech)
	EndpointAudioTranscription Endpoint = Endpoint(relaymode.AudioTranscription)
	EndpointAudioTranslation   Endpoint = Endpoint(relaymode.AudioTranslation)
	EndpointRerank             Endpoint = Endpoint(relaymode.Rerank)
	EndpointResponseAPI        Endpoint = Endpoint(relaymode.ResponseAPI)
	EndpointClaudeMessages     Endpoint = Endpoint(relaymode.ClaudeMessages)
	EndpointRealtime           Endpoint = Endpoint(relaymode.Realtime)
	EndpointVideos             Endpoint = Endpoint(relaymode.Videos)
)

// EndpointInfo contains metadata about an endpoint for display purposes.
type EndpointInfo struct {
	ID          Endpoint `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
}

// AllEndpoints returns a list of all available endpoint types with their metadata.
// This is useful for displaying endpoint options in the UI.
func AllEndpoints() []EndpointInfo {
	return []EndpointInfo{
		{ID: EndpointChatCompletions, Name: "chat_completions", Description: "Chat Completions API", Path: "/v1/chat/completions"},
		{ID: EndpointCompletions, Name: "completions", Description: "Completions API (Legacy)", Path: "/v1/completions"},
		{ID: EndpointEmbeddings, Name: "embeddings", Description: "Embeddings API", Path: "/v1/embeddings"},
		{ID: EndpointModerations, Name: "moderations", Description: "Moderations API", Path: "/v1/moderations"},
		{ID: EndpointImagesGenerations, Name: "images_generations", Description: "Image Generation API", Path: "/v1/images/generations"},
		{ID: EndpointImagesEdits, Name: "images_edits", Description: "Image Edits API", Path: "/v1/images/edits"},
		{ID: EndpointAudioSpeech, Name: "audio_speech", Description: "Text-to-Speech API", Path: "/v1/audio/speech"},
		{ID: EndpointAudioTranscription, Name: "audio_transcription", Description: "Audio Transcription API", Path: "/v1/audio/transcriptions"},
		{ID: EndpointAudioTranslation, Name: "audio_translation", Description: "Audio Translation API", Path: "/v1/audio/translations"},
		{ID: EndpointRerank, Name: "rerank", Description: "Rerank API", Path: "/v1/rerank"},
		{ID: EndpointResponseAPI, Name: "response_api", Description: "Response API", Path: "/v1/responses"},
		{ID: EndpointClaudeMessages, Name: "claude_messages", Description: "Claude Messages API", Path: "/v1/messages"},
		{ID: EndpointRealtime, Name: "realtime", Description: "Realtime API (WebSocket)", Path: "/v1/realtime"},
		{ID: EndpointVideos, Name: "videos", Description: "Video Generation API", Path: "/v1/videos"},
	}
}

// endpointNameToID maps endpoint names to their IDs for parsing.
var endpointNameToID = func() map[string]Endpoint {
	m := make(map[string]Endpoint)
	for _, e := range AllEndpoints() {
		m[e.Name] = e.ID
	}
	return m
}()

// endpointIDToName maps endpoint IDs to their names for serialization.
var endpointIDToName = func() map[Endpoint]string {
	m := make(map[Endpoint]string)
	for _, e := range AllEndpoints() {
		m[e.ID] = e.Name
	}
	return m
}()

// EndpointNameToID converts an endpoint name string to its Endpoint ID.
// Returns -1 if the name is not recognized.
func EndpointNameToID(name string) Endpoint {
	if id, ok := endpointNameToID[strings.ToLower(strings.TrimSpace(name))]; ok {
		return id
	}
	return -1
}

// EndpointIDToName converts an Endpoint ID to its string name.
// Returns empty string if the ID is not recognized.
func EndpointIDToName(id Endpoint) string {
	if name, ok := endpointIDToName[id]; ok {
		return name
	}
	return ""
}

// ParseEndpointList parses a list of endpoint name strings into Endpoint IDs.
// Invalid endpoint names are silently ignored.
func ParseEndpointList(names []string) []Endpoint {
	result := make([]Endpoint, 0, len(names))
	for _, name := range names {
		if id := EndpointNameToID(name); id >= 0 {
			result = append(result, id)
		}
	}
	return result
}

// EndpointListToNames converts a list of Endpoint IDs to their string names.
func EndpointListToNames(endpoints []Endpoint) []string {
	result := make([]string, 0, len(endpoints))
	for _, id := range endpoints {
		if name := EndpointIDToName(id); name != "" {
			result = append(result, name)
		}
	}
	return result
}

// DefaultEndpointsForChannelType returns the default supported endpoints for a channel type.
// This provides backward compatibility by defining what each channel type supports out of the box.
func DefaultEndpointsForChannelType(channelType int) []Endpoint {
	// Define common endpoint sets
	openAIFull := []Endpoint{
		EndpointChatCompletions,
		EndpointCompletions,
		EndpointEmbeddings,
		EndpointModerations,
		EndpointImagesGenerations,
		EndpointImagesEdits,
		EndpointAudioSpeech,
		EndpointAudioTranscription,
		EndpointAudioTranslation,
		EndpointResponseAPI,
		EndpointClaudeMessages,
		EndpointRealtime,
		EndpointVideos,
	}

	azureFull := []Endpoint{
		EndpointChatCompletions,
		EndpointCompletions,
		EndpointEmbeddings,
		EndpointImagesGenerations,
		EndpointAudioSpeech,
		EndpointAudioTranscription,
		EndpointResponseAPI,
		EndpointClaudeMessages,
	}

	openAICompatibleBasic := []Endpoint{
		EndpointChatCompletions,
		EndpointCompletions,
		EndpointEmbeddings,
		EndpointImagesGenerations,
		EndpointAudioSpeech,
		EndpointAudioTranscription,
		EndpointResponseAPI,
		EndpointClaudeMessages,
		EndpointVideos,
	}

	claudeCompatible := []Endpoint{
		EndpointChatCompletions,
		EndpointResponseAPI,
		EndpointClaudeMessages,
	}

	chatOnly := []Endpoint{
		EndpointChatCompletions,
		EndpointResponseAPI,
		EndpointClaudeMessages,
	}

	chatAndEmbeddings := []Endpoint{
		EndpointChatCompletions,
		EndpointEmbeddings,
		EndpointResponseAPI,
		EndpointClaudeMessages,
	}

	copilotDefault := []Endpoint{
		EndpointChatCompletions,
		EndpointEmbeddings,
		EndpointResponseAPI,
	}

	switch channelType {
	case OpenAI:
		return openAIFull
	case Azure:
		return azureFull
	case API2D, CloseAI, OpenAISB, OpenAIMax, OhMyGPT, Ails, AIProxy, API2GPT, AIGC2D, FastGPT:
		// OpenAI proxy services - support same as OpenAI
		return openAIFull
	case Anthropic:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case PaLM:
		return chatOnly
	case Gemini, GeminiOpenAICompatible:
		return chatAndEmbeddings
	case Copilot:
		return copilotDefault
	case Zhipu:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointImagesGenerations,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Ali:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointImagesGenerations,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case AliBailian:
		return chatAndEmbeddings
	case Baidu:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointRerank,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case BaiduV2:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointRerank,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Xunfei, XunfeiV2:
		return chatOnly
	case AI360:
		return chatOnly
	case OpenRouter:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case AIProxyLibrary:
		return chatOnly
	case Tencent:
		return chatOnly
	case Moonshot:
		return chatOnly
	case Baichuan:
		return chatOnly
	case Minimax:
		return chatOnly
	case Mistral:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Groq:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointAudioTranscription,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Ollama:
		return chatAndEmbeddings
	case LingYiWanWu:
		return chatOnly
	case StepFun:
		return chatOnly
	case AwsClaude:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Coze:
		return chatOnly
	case Cohere:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointRerank,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case DeepSeek:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Cloudflare:
		return chatAndEmbeddings
	case DeepL:
		return []Endpoint{} // DeepL is translation only, no standard endpoints
	case TogetherAI:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Doubao:
		return chatAndEmbeddings
	case Novita:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case VertextAI:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointImagesGenerations,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Proxy:
		// Proxy mode supports all endpoints - it's a passthrough
		return openAIFull
	case SiliconFlow:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case XAI:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointEmbeddings,
			EndpointImagesGenerations,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Replicate:
		return []Endpoint{
			EndpointChatCompletions,
			EndpointImagesGenerations,
			EndpointResponseAPI,
			EndpointClaudeMessages,
		}
	case Custom, OpenAICompatible:
		return openAICompatibleBasic
	case ClaudeCompatible:
		return claudeCompatible
	case Dummy:
		return chatOnly
	default:
		// Unknown channel type - default to chat only for safety
		return chatOnly
	}
}

// DefaultEndpointNamesForChannelType returns the default endpoint names as strings.
func DefaultEndpointNamesForChannelType(channelType int) []string {
	return EndpointListToNames(DefaultEndpointsForChannelType(channelType))
}

// IsEndpointSupported checks if an endpoint (by relaymode) is in the supported list.
func IsEndpointSupported(relayMode int, supportedEndpoints []Endpoint) bool {
	return slices.Contains(supportedEndpoints, Endpoint(relayMode))
}

// IsEndpointSupportedByName checks if an endpoint name is in the supported list.
func IsEndpointSupportedByName(endpointName string, supportedEndpointNames []string) bool {
	normalizedName := strings.ToLower(strings.TrimSpace(endpointName))
	for _, name := range supportedEndpointNames {
		if strings.ToLower(strings.TrimSpace(name)) == normalizedName {
			return true
		}
	}
	return false
}

// RelayModeToEndpointName converts a relaymode constant to the endpoint name string.
func RelayModeToEndpointName(mode int) string {
	return EndpointIDToName(Endpoint(mode))
}
