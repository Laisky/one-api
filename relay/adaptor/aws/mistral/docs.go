// Package aws provides data structures for AWS Bedrock Mistral Large API integration.
//
// This package defines the request and response models used for communicating with
// AWS Bedrock's Mistral Large language model. It includes support for:
//
// 1. Standard Chat Completions API:
//   - Request/Response structures for non-streaming chat completions
//   - Tool calling capabilities with function definitions
//   - Message history management with different roles (system, user, assistant, tool)
//
// 2. Streaming Chat Completions API:
//   - StreamResponse structures for real-time response streaming
//   - Delta-based message updates for incremental content delivery
//
// 3. AWS Converse API Integration:
//   - MistralConverse* structures for AWS Bedrock Converse API compatibility
//   - Streaming support with metadata and usage tracking
//   - Inference configuration for model parameters
//
// Key Features:
//   - Full compatibility with AWS Bedrock Mistral Large documentation
//   - Support for tool/function calling workflows
//   - Streaming and non-streaming response modes
//   - Comprehensive usage tracking and metadata
//
// The structures in this package mirror the AWS Bedrock API specifications
// while providing Go-idiomatic field names and JSON marshaling tags.
package aws
