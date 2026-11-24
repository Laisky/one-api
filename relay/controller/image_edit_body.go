package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
)

// azureImageEditOmittedFields are the image controls Azure documents for
// neither generations nor edits; Azure edit bodies have always omitted them.
var azureImageEditOmittedFields = []string{"safety_identifier", "complexity"}

// rewriteImageEditBody returns the client's image edit body with its model set
// to model and every root field named in omit removed. Every other field is
// kept as the client sent it: each reference image, the mask, markup, and the
// provider controls the typed request does not model (input_fidelity,
// partial_images, background, output_compression, ...). The typed
// relaymodel.ImageRequest cannot carry those fields, so an edit re-encoded from
// it would reach the provider without its reference images. A JSON object body
// is rewritten as JSON; a multipart body is rewritten part by part under the
// client's own boundary, so the client's Content-Type header, which the
// adaptor forwards, stays valid. The root keys were already validated as
// unambiguous by common.UnmarshalBodyReusable. Parameters: c carries the
// client Content-Type, raw is the cached client body, model the provider
// model, and omit the fields to drop. Returns: the rewritten body and true, or
// false when the body is neither JSON nor multipart (the caller keeps its
// previous handling), or an error for a malformed body.
func rewriteImageEditBody(c *gin.Context, raw []byte, model string, omit []string) ([]byte, bool, error) {
	if common.IsJSONRequestBody(c, raw) {
		body, err := rewriteImageEditJSON(raw, model, omit)
		return body, true, err
	}
	if boundary, ok := multipartBoundary(c); ok {
		body, err := rewriteImageEditMultipart(raw, boundary, model, omit)
		return body, true, err
	}
	return nil, false, nil
}

// imageEditFieldMatches reports whether a client field name is name or one of
// names, compared case-insensitively so that no spelling of a rewritten or
// omitted field survives. Parameters: field is the client name, name the
// rewritten field, and names the omitted fields. Returns: whether it matches.
func imageEditFieldMatches(field, name string, names []string) bool {
	if strings.EqualFold(field, name) {
		return true
	}
	for _, candidate := range names {
		if strings.EqualFold(field, candidate) {
			return true
		}
	}
	return false
}

// rewriteImageEditJSON rewrites a JSON object edit body. Parameters: raw is the
// client body, model the provider model, and omit the fields to drop.
// Returns: the rewritten object, or an error for a body that is not an object.
func rewriteImageEditJSON(raw []byte, model string, omit []string) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, errors.Wrap(err, "decode image edit JSON body")
	}
	if root == nil {
		return nil, errors.New("image edit JSON body must be an object")
	}
	for key := range root {
		if imageEditFieldMatches(key, "model", omit) {
			delete(root, key)
		}
	}
	encodedModel, err := json.Marshal(model)
	if err != nil {
		return nil, errors.Wrap(err, "encode image edit model")
	}
	root["model"] = encodedModel
	body, err := json.Marshal(root)
	if err != nil {
		return nil, errors.Wrap(err, "encode image edit JSON body")
	}
	return body, nil
}

// rewriteImageEditMultipart rewrites a multipart edit body. File parts and
// other fields are copied unchanged with their own part headers; the model
// part is replaced, or prepended when the client sent none. Parameters: raw is
// the client body, boundary its multipart boundary, model the provider model,
// and omit the fields to drop. Returns: the rewritten body under the same
// boundary, or an error for a malformed body.
func rewriteImageEditMultipart(raw []byte, boundary, model string, omit []string) ([]byte, error) {
	reader := multipart.NewReader(bytes.NewReader(raw), boundary)
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, errors.Wrap(err, "reuse image edit multipart boundary")
	}
	if err := writer.WriteField("model", model); err != nil {
		return nil, errors.Wrap(err, "write image edit model part")
	}
	for {
		part, err := reader.NextRawPart()
		// Only a bare io.EOF marks the final boundary; a wrapped one is a
		// truncated body.
		if err == io.EOF { //nolint:errorlint // the wrapped form is a malformed body
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "read image edit multipart part")
		}
		if part.FileName() == "" && imageEditFieldMatches(part.FormName(), "model", omit) {
			if closeErr := part.Close(); closeErr != nil {
				return nil, errors.Wrap(closeErr, "close image edit multipart part")
			}
			continue
		}
		header := make(textproto.MIMEHeader, len(part.Header))
		for key, values := range part.Header {
			header[key] = append([]string(nil), values...)
		}
		target, err := writer.CreatePart(header)
		if err != nil {
			return nil, errors.Wrap(err, "create image edit multipart part")
		}
		if _, err := io.Copy(target, part); err != nil {
			return nil, errors.Wrap(err, "copy image edit multipart part")
		}
		if err := part.Close(); err != nil {
			return nil, errors.Wrap(err, "close image edit multipart part")
		}
	}
	if err := writer.Close(); err != nil {
		return nil, errors.Wrap(err, "finish image edit multipart body")
	}
	return out.Bytes(), nil
}
