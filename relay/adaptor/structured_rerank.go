package adaptor

// StructuredRerankAdaptor explicitly opts into native text/image rerank inputs.
// Controllers must reject structured input for ordinary RerankAdaptors rather
// than forwarding the text-only accounting placeholders as actual documents.
type StructuredRerankAdaptor interface {
	RerankAdaptor
	SupportsStructuredRerank() bool
}
