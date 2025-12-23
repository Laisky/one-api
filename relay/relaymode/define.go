package relaymode

const (
	Unknown = iota
	ChatCompletions
	Completions
	Embeddings
	Moderations
	ImagesGenerations
	Edits
	AudioSpeech
	AudioTranscription
	AudioTranslation
	// Proxy is a special relay mode for proxying requests to custom upstream
	Proxy
	Rerank
	ImagesEdits
	// ResponseAPI is for OpenAI Response API direct requests
	ResponseAPI
	// ClaudeMessages is for Claude Messages API direct requests
	ClaudeMessages
	// Realtime is for OpenAI Realtime API websocket sessions
	Realtime
	// Videos handles OpenAI video generation endpoints (e.g., /v1/videos)
	Videos
)

func String(mode int) string {
	switch mode {
	case ChatCompletions:
		return "chat"
	case Completions:
		return "completion"
	case Embeddings:
		return "embedding"
	case Moderations:
		return "moderation"
	case ImagesGenerations:
		return "image_generation"
	case Edits:
		return "edit"
	case AudioSpeech:
		return "audio_speech"
	case AudioTranscription:
		return "audio_transcription"
	case AudioTranslation:
		return "audio_translation"
	case Proxy:
		return "proxy"
	case Rerank:
		return "rerank"
	case ImagesEdits:
		return "image_edit"
	case ResponseAPI:
		return "response_api"
	case ClaudeMessages:
		return "claude_messages"
	case Realtime:
		return "realtime"
	case Videos:
		return "video"
	default:
		return "unknown"
	}
}
