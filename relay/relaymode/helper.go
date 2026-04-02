package relaymode

import "strings"

func GetByPath(path string) int {
	switch {
	case strings.HasPrefix(path, "/v1/realtime"):
		return Realtime
	case strings.HasPrefix(path, "/v1/oneapi/proxy"):
		return Proxy
	case strings.HasPrefix(path, "/v1/responses"):
		return ResponseAPI
	case strings.HasPrefix(path, "/v1/messages"):
		return ClaudeMessages
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return ChatCompletions
	case strings.HasPrefix(path, "/v1/completions"):
		return Completions
	case strings.HasPrefix(path, "/v1/embeddings"),
		strings.HasSuffix(path, "embeddings"):
		return Embeddings
	case strings.HasPrefix(path, "/v1/rerank"),
		strings.HasSuffix(path, "/v2/rerank"),
		strings.HasSuffix(path, "/rerank"),
		strings.HasSuffix(path, "/rerankers"):
		return Rerank
	case strings.HasPrefix(path, "/v1/moderations"):
		return Moderations
	case strings.HasPrefix(path, "/v1/images/generations"):
		return ImagesGenerations
	case strings.HasPrefix(path, "/v1/edits"):
		return Edits
	case strings.HasPrefix(path, "/v1/audio/speech"):
		return AudioSpeech
	case strings.HasPrefix(path, "/v1/audio/transcriptions"):
		return AudioTranscription
	case strings.HasPrefix(path, "/v1/audio/translations"):
		return AudioTranslation
	case strings.HasPrefix(path, "/v1/images/edits"):
		return ImagesEdits
	case strings.HasPrefix(path, "/v1/videos"):
		return Videos
	case strings.HasPrefix(path, "/api/paas/v4/layout_parsing"):
		return OCR
	default:
		return Unknown
	}
}
