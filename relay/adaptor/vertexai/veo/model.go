package veo

// CreateVideoRequest is the request body for the Veo API.
//
// https://cloud.google.com/vertex-ai/generative-ai/docs/model-reference/veo-video-generation
type CreateVideoRequest struct {
	Instances  []CreateVideoInstance `json:"instances" binding:"required,min=1"`
	Parameters CreateVideoParameters `json:"parameters" binding:"required"`
}

// CreateVideoInstance is one prompt instance of a Veo request.
type CreateVideoInstance struct {
	Prompt string                    `json:"prompt" binding:"required"`
	Image  *CreateVideoInstanceImage `json:"image,omitempty"` // Optional image to be used as a prompt
}

// CreateVideoInstanceImage is the optional image prompt of a Veo instance.
type CreateVideoInstanceImage struct {
	BytesBase64Encoded string  `json:"bytesBase64Encoded" binding:"required"`
	MimeType           *string `json:"mimeType,omitempty" binding:"omitempty,oneof=image/jpeg image/png"`
}

// CreateVideoParameters carries the Veo generation parameters.
type CreateVideoParameters struct {
	SampleCount      int     `json:"sampleCount" binding:"required,min=1"`
	AspectRatio      *string `json:"aspectRatio,omitempty"`
	NegativePrompt   *string `json:"negativePrompt,omitempty"`
	PersonGeneration *string `json:"personGeneration,omitempty"`
	Seed             *uint32 `json:"seed,omitempty"`
	StorageUri       *string `json:"storageUri,omitempty"`
	// DurationSeconds specifies the duration of the video in seconds, default is 8 seconds.
	DurationSeconds *int  `json:"durationSeconds,omitempty"`
	EnhancePrompt   *bool `json:"enhancePrompt,omitempty"`
	// GenerateAudio specifies whether to generate audio for the video.
	// not support for veo-2.0-generate-001.
	GenerateAudio *bool `json:"generateAudio,omitempty"`
}

// CreateVideoTaskResponse is the predictLongRunning response that names the
// long-running operation created for the video generation job.
type CreateVideoTaskResponse struct {
	// Name is the full operation name, for example
	// projects/P/locations/L/publishers/google/models/M/operations/ID.
	Name string `json:"name"`
}

// PollVideoTaskRequest is the fetchPredictOperation request body.
//
// https://docs.cloud.google.com/vertex-ai/generative-ai/docs/video/generate-videos-from-text
type PollVideoTaskRequest struct {
	OperationName string `json:"operationName" binding:"required"`
}

// PollVideoTaskResponse is the long-running operation returned by
// fetchPredictOperation. Error is set when the provider finished the operation
// unsuccessfully.
type PollVideoTaskResponse struct {
	Name     string                    `json:"name"`
	Done     bool                      `json:"done"`
	Error    *OperationError           `json:"error,omitempty"`
	Response PollVideoTaskResponseData `json:"response"`
}

// OperationError is the google.rpc.Status of a failed long-running operation.
type OperationError struct {
	// Code is a google.rpc.Code value, not an HTTP status.
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PollVideoTaskResponseData is the GenerateVideoResponse of a completed
// operation. Vertex AI reports Videos; GeneratedSamples is the Gemini
// Developer API shape kept for compatibility.
type PollVideoTaskResponseData struct {
	Type                  string            `json:"@type"`
	Videos                []GeneratedVideo  `json:"videos"`
	RaiMediaFilteredCount int               `json:"raiMediaFilteredCount"`
	GeneratedSamples      []GeneratedSample `json:"generatedSamples"`
}

// GeneratedVideo is one Vertex AI Veo output: a Cloud Storage URI when the
// request set storageUri, otherwise the base64-encoded video bytes.
type GeneratedVideo struct {
	GcsUri             string `json:"gcsUri"`
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	MimeType           string `json:"mimeType"`
}

// GeneratedSample is one Gemini Developer API style output sample.
type GeneratedSample struct {
	Video VideoData `json:"video"`
}

// VideoData is the video reference inside a GeneratedSample.
type VideoData struct {
	URI      string `json:"uri"`
	Encoding string `json:"encoding"`
}
