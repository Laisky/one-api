package gemini

import (
	"encoding/base64"
	stderrors "errors"
	"io"
	"mime"
	"net"
	"net/url"
	"path"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Native media source validation errors. Messages are client-facing and never
// include URLs, filenames, or payloads.
var (
	errMediaSourceCount                = stderrors.New("media requires exactly one source")
	errMediaDataURINotBase64           = stderrors.New("media data URI must contain base64 data")
	errMediaDataURIMIMEInvalid         = stderrors.New("media data URI requires a valid MIME type")
	errMediaMIMEConflict               = stderrors.New("media MIME type conflicts with data URI")
	errMediaMIMEUnsupported            = stderrors.New("unsupported or mismatched media MIME type")
	errInlineMediaTooLargeUseReference = stderrors.New("inline media exceeds the relay limit; use a public HTTPS reference")
	errMediaDataInvalidBase64          = stderrors.New("media data must be non-empty valid base64")
	errInlineMediaTooLarge             = stderrors.New("inline media exceeds the relay limit")
	errMediaMIMEInvalid                = stderrors.New("media MIME type is invalid")
	errMediaReferenceNotPublicHTTPS    = stderrors.New("media reference must be a public HTTPS URL without user credentials or fragments")
	errMediaReferencePort              = stderrors.New("media reference must use HTTPS port 443")
	errMediaReferencePrivate           = stderrors.New("private media references are not supported")
	errMediaReferenceProviderFile      = stderrors.New("private provider file references require owner-scoped resolution")
)

// This is a relay memory bound, not Google's model size limit. Larger inputs
// must use public HTTPS references; this converter never downloads remote media.
const nativeInlineMediaLimit int64 = 100_000_000

// nativeMediaSource is a normalized source envelope, not a fetch instruction.
type nativeMediaSource struct {
	URI      string
	Data     string
	MIMEType string
	Filename string
	Kind     string
}

// prepareNativeMediaSource converts one media source to inlineData or fileData.
// It returns the normalized modality and decoded byte count for admission.
// Errors deliberately omit URLs, filenames and payloads. Private provider file
// IDs and gs:// references require a separate, owner-scoped file resolver and
// are not accepted by this public-reference path.
func prepareNativeMediaSource(source nativeMediaSource) (Part, string, int64, error) {
	if (source.URI == "") == (source.Data == "") {
		return Part{}, "", 0, errors.WithStack(errMediaSourceCount)
	}
	mediaType, err := normalizeNativeMIME(source.MIMEType)
	if err != nil {
		return Part{}, "", 0, err
	}
	data := source.Data
	uri := source.URI
	if strings.HasPrefix(data, "data:") {
		uri, data = data, ""
	}
	if strings.HasPrefix(uri, "data:") {
		header, payload, ok := strings.Cut(strings.TrimPrefix(uri, "data:"), ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return Part{}, "", 0, errors.WithStack(errMediaDataURINotBase64)
		}
		declaredType, err := normalizeNativeMIME(strings.TrimSuffix(header, ";base64"))
		if err != nil || declaredType == "" {
			return Part{}, "", 0, errors.WithStack(errMediaDataURIMIMEInvalid)
		}
		if mediaType != "" && mediaType != declaredType {
			return Part{}, "", 0, errors.WithStack(errMediaMIMEConflict)
		}
		mediaType, data, uri = declaredType, payload, ""
	}
	if mediaType == "" {
		name := source.Filename
		if name == "" && uri != "" {
			if u, err := url.Parse(uri); err == nil {
				name = u.Path
			}
		}
		mediaType = nativeMIMEFromFilename(name)
	}
	kind := nativeMIMEKind(mediaType)
	if kind == "" || (source.Kind != "" && source.Kind != kind) {
		return Part{}, "", 0, errors.WithStack(errMediaMIMEUnsupported)
	}
	if uri != "" {
		if err := validatePublicMediaURI(uri); err != nil {
			return Part{}, "", 0, err
		}
		return Part{FileData: &FileData{MimeType: mediaType, FileURI: uri}}, kind, 0, nil
	}
	limit := nativeInlineMediaLimit
	if mediaType == "application/pdf" {
		limit = 50_000_000
	}
	if int64(len(data)) > (limit+2)/3*4 {
		return Part{}, "", 0, errors.WithStack(errInlineMediaTooLargeUseReference)
	}
	decoder := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data))
	n, err := io.Copy(io.Discard, io.LimitReader(decoder, limit+1))
	if err != nil || n == 0 {
		return Part{}, "", 0, errors.WithStack(errMediaDataInvalidBase64)
	}
	if n > limit {
		return Part{}, "", 0, errors.WithStack(errInlineMediaTooLarge)
	}
	return Part{InlineData: &InlineData{MimeType: mediaType, Data: data}}, kind, n, nil
}

// normalizeNativeMIME normalizes an explicit type without accepting parameters.
func normalizeNativeMIME(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	typ, params, err := mime.ParseMediaType(raw)
	if err != nil || len(params) != 0 {
		return "", errors.WithStack(errMediaMIMEInvalid)
	}
	switch typ {
	case "audio/mp3":
		return "audio/mpeg", nil
	case "audio/x-wav":
		return "audio/wav", nil
	}
	return typ, nil
}

// nativeMIMEKind defines containers accepted by the native media bridge.
func nativeMIMEKind(typ string) string {
	switch typ {
	case "video/mp4", "video/mpeg", "video/quicktime", "video/x-msvideo", "video/x-flv", "video/webm", "video/x-ms-wmv", "video/3gpp":
		return "video"
	case "audio/wav", "audio/mpeg", "audio/mp4", "audio/aac", "audio/flac", "audio/ogg", "audio/webm", "audio/aiff", "audio/x-aiff":
		return "audio"
	case "application/pdf":
		return "file"
	case "image/jpeg", "image/png", "image/webp", "image/heic", "image/heif":
		return "image"
	default:
		return ""
	}
}

// nativeMIMEFromFilename uses a deterministic allowlist, not an OS MIME registry.
func nativeMIMEFromFilename(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp4":
		return "video/mp4"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".flv":
		return "video/x-flv"
	case ".webm":
		return "video/webm"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".3gp":
		return "video/3gpp"
	case ".pdf":
		return "application/pdf"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	case ".aif", ".aiff":
		return "audio/aiff"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	default:
		return ""
	}
}

// validatePublicMediaURI accepts only credential-free public HTTPS references.
// The upstream provider, not the gateway, retrieves them. Provider-owned Files
// API references are excluded: possession of a channel credential is not proof
// that a caller owns a private file stored under that upstream account.
func validatePublicMediaURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return errors.WithStack(errMediaReferenceNotPublicHTTPS)
	}
	if port := u.Port(); port != "" && port != "443" {
		return errors.WithStack(errMediaReferencePort)
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || (net.ParseIP(host) == nil && !strings.Contains(host, ".")) {
		return errors.WithStack(errMediaReferencePrivate)
	}
	if ip := net.ParseIP(host); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return errors.WithStack(errMediaReferencePrivate)
	}
	if IsGoogleGenerativeAPIHost(host) {
		return errors.WithStack(errMediaReferenceProviderFile)
	}
	return nil
}
