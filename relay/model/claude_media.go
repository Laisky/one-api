package model

import (
	"errors"
	"strings"
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
		return MessageContent{}, true, errors.New("media source must be an object")
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
			return MessageContent{}, true, errors.New("base64 media requires data and media_type only")
		}
		part.FileData = value.Data
	case "url":
		if value.URL == "" || value.Data != "" || value.FileID != "" {
			return MessageContent{}, true, errors.New("URL media requires a single url source")
		}
		part.FileURL = value.URL
	case "file":
		if value.FileID == "" || value.Data != "" || value.URL != "" {
			return MessageContent{}, true, errors.New("file media requires a single file_id source")
		}
		part.FileID = value.FileID
	default:
		return MessageContent{}, true, errors.New("unsupported media source type")
	}
	if kind == "document" && part.MIMEType == "" {
		part.MIMEType = "application/pdf"
	}
	if kind == "video" || kind == "audio" {
		// A generic file must not let audio masquerade as video (or vice versa).
		if part.MIMEType == "" || !strings.HasPrefix(part.MIMEType, kind+"/") {
			return MessageContent{}, true, errors.New("media source requires a matching media_type")
		}
	}
	return part, true, nil
}
