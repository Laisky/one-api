package model

import (
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Checked content-part shape errors. Messages are client-facing and never
// include input payloads.
var (
	errContentNotTextOrParts      = stderrors.New("content must be text or a list of content parts")
	errTextNotString              = stderrors.New("text must be a string")
	errImageURLEmpty              = stderrors.New("image_url requires a non-empty url")
	errVideoURLEmpty              = stderrors.New("video_url requires a non-empty url")
	errInputAudioIncomplete       = stderrors.New("input_audio requires data and format")
	errFileSourceCount            = stderrors.New("file requires exactly one source")
	errUnsupportedContentPartType = stderrors.New("unsupported content part type")
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
				return nil, errors.WithMessagef(err, "content part %d", i)
			}
		}
		return content, nil
	case []any:
		out := make([]MessageContent, 0, len(content))
		for i, raw := range content {
			object, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.WithStack(fmt.Errorf("content part %d must be an object", i)) //nolint:err113 // client-facing text embeds the part index
			}
			part, recognized, err := decodeMediaPart(object)
			if !recognized {
				// Decode one part at a time: preserve the ordering of text and
				// media, rather than collecting all text before the images.
				parsed := (Message{Content: []any{object}}).ParseContent()
				if len(parsed) != 1 {
					return nil, errors.WithStack(fmt.Errorf("content part %d has an unsupported type or malformed payload", i)) //nolint:err113 // client-facing text embeds the part index
				}
				part = parsed[0]
			}
			if err == nil {
				err = validateMessagePart(part)
			}
			if err != nil {
				return nil, errors.WithMessagef(err, "content part %d", i)
			}
			out = append(out, part)
		}
		return out, nil
	default:
		return nil, errors.WithStack(errContentNotTextOrParts)
	}
}

// validateMessagePart checks shape only; the selected provider validates model
// capabilities, MIME types, resource limits, and reference ownership separately.
func validateMessagePart(part MessageContent) error {
	switch part.Type {
	case ContentTypeText:
		if part.Text == nil {
			return errors.WithStack(errTextNotString)
		}
	case ContentTypeImageURL:
		if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.Url) == "" {
			return errors.WithStack(errImageURLEmpty)
		}
	case ContentTypeVideoURL:
		if part.VideoURL == nil || strings.TrimSpace(part.VideoURL.URL) == "" {
			return errors.WithStack(errVideoURLEmpty)
		}
	case ContentTypeInputAudio:
		if part.InputAudio == nil || part.InputAudio.Data == "" || part.InputAudio.Format == "" {
			return errors.WithStack(errInputAudioIncomplete)
		}
	case ContentTypeFile:
		count := 0
		for _, value := range []string{part.FileID, part.FileData, part.FileURL} {
			if value != "" {
				count++
			}
		}
		if count != 1 {
			return errors.WithStack(errFileSourceCount)
		}
	default:
		return errors.WithStack(errUnsupportedContentPartType)
	}
	return nil
}
