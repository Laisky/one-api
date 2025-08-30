// Package aws provides data structures for AWS Bedrock DeepSeek-R1 API integration.
//
// This package defines the request and response models used for communicating with
// AWS Bedrock's DeepSeek-R1 language model. It includes support for:
//
// 1. Standard Chat Completions API:
//   - Request/Response structures for non-streaming chat completions
//   - Message history management with different roles (system, user, assistant)
//   - DeepSeek-R1's unique reasoning capabilities with structured reasoning content
//
// 2. Streaming Chat Completions API:
//   - StreamResponse structures for real-time response streaming
//   - Delta-based message updates for incremental content delivery
//   - Support for reasoning content in streaming responses
//
// 3. AWS Converse API Integration:
//   - DeepSeekConverse* structures for AWS Bedrock Converse API compatibility
//   - Streaming support with metadata and usage tracking
//   - Inference configuration for model parameters
//   - Enhanced reasoning content handling in Converse format
//
// 4. DeepSeek-R1 Specific Features:
//   - Reasoning content blocks that capture the model's internal reasoning process
//   - Structured response format supporting both regular text and reasoning text
//   - Advanced stop reason handling including reasoning-specific termination conditions
//
// Key Features:
//   - Full compatibility with AWS Bedrock DeepSeek-R1 documentation
//   - Support for DeepSeek-R1's unique reasoning capabilities
//   - Streaming and non-streaming response modes
//   - Comprehensive usage tracking and metadata
//   - Enhanced error handling for reasoning-specific scenarios
//
// The structures in this package mirror the AWS Bedrock DeepSeek-R1 API specifications
// while providing Go-idiomatic field names and JSON marshaling tags. Special attention
// is given to DeepSeek-R1's reasoning content format, which allows the model to show
// its internal reasoning process alongside the final response.
package aws
