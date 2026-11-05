package model

import (
	stderrors "errors"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Media envelope validation errors. Messages are client-facing and never
// include input payloads.
var (
	errVideoURLNotObject    = stderrors.New("video_url must be an object")
	errVideoURLFields       = stderrors.New("video_url requires a non-empty string url and a string mime_type")
	errFileNotObject        = stderrors.New("file must be an object")
	errFileMixedShape       = stderrors.New("file cannot mix nested and flat fields")
	errFileFieldsNotStrings = stderrors.New("file fields must be strings")
	errFileSourceChoice     = stderrors.New("file requires exactly one of file_id, file_data, or file_url")
	errMediaFieldType       = stderrors.New("invalid media field type")
)

// ContentTypeVideoURL identifies an OpenAI-compatible video content part.
const ContentTypeVideoURL = "video_url"

// VideoURL carries a provider-readable video URI or an inline data URI.
// MIMEType is required when the URI does not identify the container format.
type VideoURL struct {
	URL      string `json:"url"`
	MIMEType string `json:"mime_type,omitempty"`
}

// decodeMediaPart retains video and file payloads before provider conversion.
// recognized remains true for malformed media, so a checked caller can reject
// the request instead of silently dropping the part. No URI is fetched here.
func decodeMediaPart(part map[string]any) (MessageContent, bool, error) {
	typ, _ := part["type"].(string)
	switch strings.ToLower(typ) {
	case ContentTypeVideoURL, "input_video":
		var video VideoURL
		raw, ok := part["video_url"].(map[string]any)
		if value, exists := part["video_url"].(string); exists {
			raw, ok = map[string]any{"url": value}, true
			if mimeType, exists := part["mime_type"]; exists {
				raw["mime_type"] = mimeType
			}
		}
		if !ok {
			return MessageContent{}, true, errors.WithStack(errVideoURLNotObject)
		}
		if err := readMediaStrings(raw, map[string]*string{"url": &video.URL, "mime_type": &video.MIMEType}); err != nil || strings.TrimSpace(video.URL) == "" {
			return MessageContent{}, true, errors.WithStack(errVideoURLFields)
		}
		return MessageContent{Type: ContentTypeVideoURL, VideoURL: &video}, true, nil
	case ContentTypeFile, "input_file":
		// Chat Completions uses a nested file object; retain the gateway's
		// existing flat file shape and the Responses input_file shape too.
		source := part
		if raw, exists := part["file"]; exists {
			var ok bool
			source, ok = raw.(map[string]any)
			if !ok {
				return MessageContent{}, true, errors.WithStack(errFileNotObject)
			}
			for _, key := range []string{"file_id", "file_data", "file_url", "filename", "mime_type"} {
				if _, exists := part[key]; exists {
					return MessageContent{}, true, errors.WithStack(errFileMixedShape)
				}
			}
		}
		var file struct {
			ID       string `json:"file_id"`
			Data     string `json:"file_data"`
			URL      string `json:"file_url"`
			Filename string `json:"filename"`
			MIMEType string `json:"mime_type"`
		}
		if err := readMediaStrings(source, map[string]*string{"file_id": &file.ID, "file_data": &file.Data, "file_url": &file.URL, "filename": &file.Filename, "mime_type": &file.MIMEType}); err != nil {
			return MessageContent{}, true, errors.WithStack(errFileFieldsNotStrings)
		}
		n := 0
		for _, value := range []string{file.ID, file.Data, file.URL} {
			if value != "" {
				n++
			}
		}
		if n != 1 {
			return MessageContent{}, true, errors.WithStack(errFileSourceChoice)
		}
		return MessageContent{Type: ContentTypeFile, FileID: file.ID, FileData: file.Data,
			FileURL: file.URL, Filename: file.Filename, MIMEType: file.MIMEType}, true, nil
	default:
		return MessageContent{}, false, nil
	}
}

// readMediaStrings validates envelope fields without copying large base64 data.
func readMediaStrings(raw map[string]any, fields map[string]*string) error {
	for key, target := range fields {
		value, exists := raw[key]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return errors.WithStack(errMediaFieldType)
		}
		*target = text
	}
	return nil
}
