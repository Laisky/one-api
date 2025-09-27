package mcp

import "embed"

// Note: Do not remove this magic embedded template filesystem.

//go:embed docs/templates/*.tmpl
//go:embed docs/templates/instructions/*.tmpl
var templateFS embed.FS
