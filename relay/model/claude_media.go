package model

import (
	stderrors "errors"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Claude media-source validation errors. Messages are client-facing and never
// include input payloads.
var (
	errClaudeMediaSourceNotObject = stderrors.New("media source must be an object")
	errClaudeMediaBase64Shape     = stderrors.New("base64 media requires data and media_type only")
	errClaudeMediaURLShape        = stderrors.New("URL media requires a single url source")
	errClaudeMediaFileShape       = stderrors.New("file media requires a single file_id source")
	errClaudeMediaSourceType      = stderrors.New("unsupported media source type")
	errClaudeMediaTypeMismatch    = stderrors.New("media source requires a matching media_type")
)

// DecodeClaudeMediaPart lowers a Claude media source into the canonical content
// envelope. recognized stays true on malformed media so callers cannot drop it.
// It does not fetch a URL or resolve a provider-owned file ID.
func DecodeClaudeMediaPart(block map[string]any) (part MessageContent, recognized bool, err error) {
	kind, _ := block["type"].(string)
	if kind != "document" && kind != "video" && kind != "audio" {
		return MessageContent{}, false, nil
	}
	source, ok := block["source"].(map[string]any)
	if !ok {
		return MessageContent{}, true, errors.WithStack(errClaudeMediaSourceNotObject)
	}
	var value struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
		FileID    string `json:"file_id"`
	}
	if err := readMediaStrings(source, map[string]*string{"type": &value.Type, "media_type": &value.MediaType, "data": &value.Data, "url": &value.URL, "file_id": &value.FileID}); err != nil {
		return MessageContent{}, true, err
	}
	part = MessageContent{Type: ContentTypeFile, MIMEType: value.MediaType}
	switch value.Type {
	case "base64":
		if value.Data == "" || value.MediaType == "" || value.URL != "" || value.FileID != "" {
			return MessageContent{}, true, errors.WithStack(errClaudeMediaBase64Shape)
		}
		part.FileData = value.Data
	case "url":
		if value.URL == "" || value.Data != "" || value.FileID != "" {
			return MessageContent{}, true, errors.WithStack(errClaudeMediaURLShape)
		}
		part.FileURL = value.URL
	case "file":
		if value.FileID == "" || value.Data != "" || value.URL != "" {
			return MessageContent{}, true, errors.WithStack(errClaudeMediaFileShape)
		}
		part.FileID = value.FileID
	default:
		return MessageContent{}, true, errors.WithStack(errClaudeMediaSourceType)
	}
	if kind == "document" && part.MIMEType == "" {
		part.MIMEType = "application/pdf"
	}
	if kind == "video" || kind == "audio" {
		// A generic file must not let audio masquerade as video (or vice versa).
		if part.MIMEType == "" || !strings.HasPrefix(part.MIMEType, kind+"/") {
			return MessageContent{}, true, errors.WithStack(errClaudeMediaTypeMismatch)
		}
	}
	return part, true, nil
}
