// Package aws provides data structures for AWS Bedrock Cohere Command R API integration.
//
// This package defines the request and response models used for communicating with
// AWS Bedrock's Cohere Command R family models. It includes support for:
//
// 1. Standard Chat Completions API:
//   - Request/Response structures for non-streaming chat completions
//   - Message history management with different roles (system, user, assistant)
//   - Cohere Command R's enterprise-grade conversation capabilities
//
// 2. Streaming Chat Completions API:
//   - StreamResponse structures for real-time response streaming
//   - Delta-based message updates for incremental content delivery
//   - Low-latency streaming optimized for enterprise applications
//
// 3. AWS Converse API Integration:
//   - CohereConverse* structures for AWS Bedrock Converse API compatibility
//   - Streaming support with metadata and usage tracking
//   - Inference configuration for model parameters
//   - Enterprise-grade content filtering and safety features
//
// 4. Cohere Command R Specific Features:
//   - Multi-lingual conversation support with 10+ languages
//   - Enterprise-focused safety and content filtering
//   - Optimized for business and professional use cases
//   - Advanced context understanding and coherent responses
//
// Key Features:
//   - Full compatibility with AWS Bedrock Cohere Command R documentation
//   - Support for Cohere's enterprise-grade conversation capabilities
//   - Streaming and non-streaming response modes
//   - Comprehensive usage tracking and metadata
//   - Multi-lingual support for global enterprise applications
//   - Advanced safety and content filtering mechanisms
//
// The structures in this package mirror the AWS Bedrock Cohere Command R API specifications
// while providing Go-idiomatic field names and JSON marshaling tags. Special attention
// is given to Cohere's enterprise features including safety filtering, multi-lingual
// support, and optimized business conversation handling.
//
// Cohere Command R models are designed for:
//   - Enterprise conversational AI applications
//   - Multi-lingual customer support systems
//   - Business process automation with natural language
//   - Professional content generation and analysis
//   - Coherent long-form conversation maintenance
package aws
