package validator

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// filterJinaNativeParameters omits warning labels for fields read by Jina's
// adaptor from the reusable body. Other providers and unknown fields are unchanged.
func filterJinaNativeParameters(c *gin.Context, names []string) []string {
	if c == nil {
		return names
	}
	raw, exists := c.Get(ctxkey.Meta)
	if !exists {
		return names
	}
	m, ok := raw.(*meta.Meta)
	if !ok || m == nil || m.ChannelType != channeltype.Jina {
		return names
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		known := false
		switch m.Mode {
		case relaymode.Embeddings:
			switch name {
			case "task", "embedding_type", "normalized", "truncate", "late_chunking", "return_multivector", "return_tokenized_input":
				known = true
			}
		case relaymode.Rerank:
			switch name {
			case "return_documents", "max_doc_length", "return_embeddings":
				known = true
			}
		}
		if !known {
			out = append(out, name)
		}
	}
	return out
}
