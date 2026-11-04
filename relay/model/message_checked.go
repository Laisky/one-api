package model

import (
	"errors"
	"fmt"
	"strings"
)

// Checked content-part shape errors. Messages are client-facing and never
// include input payloads.
var (
	errContentNotTextOrParts      = errors.New("content must be text or a list of content parts")
	errTextNotString              = errors.New("text must be a string")
	errImageURLEmpty              = errors.New("image_url requires a non-empty url")
	errVideoURLEmpty              = errors.New("video_url requires a non-empty url")
	errInputAudioIncomplete       = errors.New("input_audio requires data and format")
	errFileSourceCount            = errors.New("file requires exactly one source")
	errUnsupportedContentPartType = errors.New("unsupported content part type")
)

// ParseContentChecked parses native-provider inputs without silently losing an
// unrecognized or malformed part. ParseContent remains the permissive legacy
// decoder for existing callers. Error messages never include input payloads.
func (m Message) ParseContentChecked() ([]MessageContent, error) {
	switch content := m.Content.(type) {
	case nil, string:
		return m.ParseContent(), nil
	case []MessageContent:
		for i, part := range content {
			if err := validateMessagePart(part); err != nil {
				return nil, fmt.Errorf("content part %d: %w", i, err)
			}
		}
		return content, nil
	case []any:
		out := make([]MessageContent, 0, len(content))
		for i, raw := range content {
			object, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("content part %d must be an object", i) //nolint:err113 // client-facing text embeds the part index
			}
			part, recognized, err := decodeMediaPart(object)
			if !recognized {
				// Decode one part at a time: preserve the ordering of text and
				// media, rather than collecting all text before the images.
				parsed := (Message{Content: []any{object}}).ParseContent()
				if len(parsed) != 1 {
					return nil, fmt.Errorf("content part %d has an unsupported type or malformed payload", i) //nolint:err113 // client-facing text embeds the part index
				}
				part = parsed[0]
			}
			if err == nil {
				err = validateMessagePart(part)
			}
			if err != nil {
				return nil, fmt.Errorf("content part %d: %w", i, err)
			}
			out = append(out, part)
		}
		return out, nil
	default:
		return nil, errContentNotTextOrParts
	}
}

// validateMessagePart checks shape only; the selected provider validates model
// capabilities, MIME types, resource limits, and reference ownership separately.
func validateMessagePart(part MessageContent) error {
	switch part.Type {
	case ContentTypeText:
		if part.Text == nil {
			return errTextNotString
		}
	case ContentTypeImageURL:
		if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.Url) == "" {
			return errImageURLEmpty
		}
	case ContentTypeVideoURL:
		if part.VideoURL == nil || strings.TrimSpace(part.VideoURL.URL) == "" {
			return errVideoURLEmpty
		}
	case ContentTypeInputAudio:
		if part.InputAudio == nil || part.InputAudio.Data == "" || part.InputAudio.Format == "" {
			return errInputAudioIncomplete
		}
	case ContentTypeFile:
		count := 0
		for _, value := range []string{part.FileID, part.FileData, part.FileURL} {
			if value != "" {
				count++
			}
		}
		if count != 1 {
			return errFileSourceCount
		}
	default:
		return errUnsupportedContentPartType
	}
	return nil
}
