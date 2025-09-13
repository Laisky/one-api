// Package aws provides the AWS adaptor for OpenAI OSS models on Bedrock.
//
// This package implements AWS Bedrock integration for OpenAI's open-source models
// ([gpt-oss-20b] and [gpt-oss-120b]) through the Converse API, providing:
//
//   - Full OpenAI API compatibility for chat completions
//   - Support for both streaming and non-streaming requests
//   - Reasoning content capabilities similar to DeepSeek-R1
//   - Proper token usage tracking for billing
//
// The adaptor handles the complete request-response lifecycle by converting
// OpenAI-compatible requests to AWS Bedrock Converse API format and back,
// while maintaining support for advanced features like reasoning content
// that are available in OpenAI's OSS models.
//
// Supported Models:
//   - [gpt-oss-20b]: 20B parameter model with reasoning capabilities
//   - [gpt-oss-120b]: 120B parameter model with advanced reasoning capabilities
//
// The implementation follows the established pattern from other AWS adaptors
// in the One API system, ensuring consistent behavior and maintainability.
//
// [gpt-oss-20b]: https://openai.com/index/introducing-gpt-oss/
// [gpt-oss-120b]: https://openai.com/index/introducing-gpt-oss/
package aws
