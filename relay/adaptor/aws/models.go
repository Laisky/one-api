package aws

import "github.com/Laisky/one-api/relay/adaptor"

// awsBedrockModelPricing assembles independent host-specific model families.
var awsBedrockModelPricing = adaptor.JoinModelCatalogs(awsClaudeModelPricing, awsFoundationModelPricing)
