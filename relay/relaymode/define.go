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
)
