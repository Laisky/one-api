package gemini

import (
	stderrors "errors"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/model"
)

// Native Gemini media conversion errors. Messages are client-facing and never
// include URLs, filenames, or payloads.
var (
	errNativeVideoURLMissing          = stderrors.New("video_url requires an object")
	errNativeFileIDUnresolved         = stderrors.New("file_id requires owner-scoped resolution; use inline file_data or a public file_url")
	errNativeInputAudioMissing        = stderrors.New("input_audio requires an object")
	errNativeInputAudioFormat         = stderrors.New("unsupported native Gemini input_audio format")
	errNativeMediaPartUnsupported     = stderrors.New("unsupported native Gemini media part")
	errNativeMediaModalityUnsupported = stderrors.New("media modality is not supported by the selected Gemini model")
)

// convertNativeMediaPart converts a validated media envelope without fetching
// caller-controlled URLs. Model capability checks apply to the effective MIME
// modality, preventing unsupported audio/video from being disguised as a file.
func convertNativeMediaPart(modelName string, content model.MessageContent) (Part, int64, error) {
	source := nativeMediaSource{MIMEType: content.MIMEType, Filename: content.Filename}
	switch content.Type {
	case model.ContentTypeVideoURL:
		if content.VideoURL == nil {
			return Part{}, 0, errors.WithStack(errNativeVideoURLMissing)
		}
		source.URI, source.MIMEType, source.Kind = content.VideoURL.URL, content.VideoURL.MIMEType, "video"
	case model.ContentTypeFile:
		if content.FileID != "" {
			return Part{}, 0, errors.WithStack(errNativeFileIDUnresolved)
		}
		source.URI, source.Data = content.FileURL, content.FileData
	case model.ContentTypeInputAudio:
		if content.InputAudio == nil {
			return Part{}, 0, errors.WithStack(errNativeInputAudioMissing)
		}
		source.Data, source.Kind = content.InputAudio.Data, "audio"
		switch strings.ToLower(content.InputAudio.Format) {
		case "mp3", "mpeg", "mpga":
			source.MIMEType = "audio/mpeg"
		case "wav":
			source.MIMEType = "audio/wav"
		case "mp4", "m4a":
			source.MIMEType = "audio/mp4"
		case "flac", "aac", "ogg", "webm":
			source.MIMEType = "audio/" + strings.ToLower(content.InputAudio.Format)
		default:
			return Part{}, 0, errors.WithStack(errNativeInputAudioFormat)
		}
	default:
		return Part{}, 0, errors.WithStack(errNativeMediaPartUnsupported)
	}
	part, modality, size, err := prepareNativeMediaSource(source)
	if err != nil {
		return Part{}, 0, err
	}
	cfg, known := geminiOpenaiCompatible.ModelRatios[strings.ToLower(modelName)]
	if !known || !slices.Contains(cfg.InputModalities, modality) {
		return Part{}, 0, errors.WithStack(errNativeMediaModalityUnsupported)
	}
	return part, size, nil
}
