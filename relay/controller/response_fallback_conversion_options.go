package controller

import (
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// responseFallbackConversionOptions selects schema fidelity from the trusted mapped native Vertex route.
// Other providers retain their established schema sanitizer independently of native media support.
func responseFallbackConversionOptions(meta *metalib.Meta) openai.ResponseFallbackConversionOptions {
	nativeVertex := meta != nil && meta.APIType == apitype.VertexAI && isNativeGoogleChannel(meta)
	return openai.ResponseFallbackConversionOptions{
		NativeMedia:              isNativeGoogleChannel(meta),
		PreserveJSONSchema:       nativeVertex,
		RequireToolResultPairing: nativeVertex,
	}
}
