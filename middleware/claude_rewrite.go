package middleware

import (
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

// claudeCountTokensSuffix is the sub-resource that turns a Messages alias into
// the free token-counting endpoint.
const claudeCountTokensSuffix = "/count_tokens"

// claudeMessagesAliasTarget maps a request path under a Claude Code alias
// prefix to its canonical endpoint. Only two exact shapes are recognized, each
// with an optional single trailing slash:
//
//	<prefix>               -> /v1/messages               (billed generation)
//	<prefix>/count_tokens  -> /v1/messages/count_tokens  (free token counting)
//
// Every other path, including any other sub-resource of the alias, is not an
// alias and must not be rewritten.
//
// Parameters:
//   - path: the incoming request URL path.
//   - prefix: the alias prefix without a trailing slash.
//
// Return values:
//   - string: the canonical target path.
//   - bool: true when path is an exact alias of a known endpoint.
func claudeMessagesAliasTarget(path, prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", false
	}
	switch strings.TrimSuffix(rest, "/") {
	case "":
		return ClaudeMessagesPath, true
	case claudeCountTokensSuffix:
		return ClaudeCountTokensPath, true
	default:
		return "", false
	}
}

// RewriteClaudeMessagesPrefix returns a middleware that rewrites an exact
// Claude Code alias of the Messages endpoints to its canonical path and
// re-dispatches the request to the engine, so the canonical route and its
// middlewares handle it, then aborts the current handler chain.
//
// Only exact aliases are rewritten: <prefix> maps to /v1/messages and
// <prefix>/count_tokens maps to /v1/messages/count_tokens. A prefix match is
// deliberately not enough, because it would turn a free count_tokens request
// (or any other sub-resource) into a billed generation.
//
// Example prefixes observed in the wild:
//   - /v1/v1/messages
//   - /openai/v1/messages
//   - /openai/v1/v1/messages
//   - /api/v1/v1/messages
//
// Parameters:
//   - prefix: the alias prefix; a trailing slash is ignored.
//   - engine: the engine used to re-dispatch the rewritten request.
//
// Return values:
//   - gin.HandlerFunc: the middleware.
func RewriteClaudeMessagesPrefix(prefix string, engine *gin.Engine) gin.HandlerFunc {
	normalized := strings.TrimSuffix(prefix, "/")

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		target, ok := claudeMessagesAliasTarget(path, normalized)
		if !ok {
			c.Next()
			return
		}
		lg := gmw.GetLogger(c)
		lg.Debug("rewriting Claude Messages alias path",
			zap.String("original_path", path),
			zap.String("target_path", target),
		)
		// Rewrite the request path to the canonical endpoint. RawPath would
		// otherwise keep describing the original alias.
		c.Request.URL.Path = target
		c.Request.URL.RawPath = ""
		// Re-dispatch to let the canonical route handle the request.
		engine.HandleContext(c)
		// Stop further processing in the current chain.
		c.Abort()
	}
}
