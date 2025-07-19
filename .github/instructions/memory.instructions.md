---
applyTo: "**/*"
---


# Project Memory & Handover Instructions

## Claude Messages API: Universal Conversion Architecture

- **Universal Endpoint:** All Claude Messages API requests (`/v1/messages`) are accepted and routed to the appropriate adapter, supporting both native and conversion-based flows.
- **Adapter Interface:** All adapters implement `ConvertClaudeRequest(c, request)` and `DoResponse(c, resp, meta)` for request/response conversion. Anthropic uses native passthrough; others use OpenAI-compatible or custom conversion.
- **Conversion Patterns:**
  - **Native:** Anthropic sets `ClaudeMessagesNative` and `ClaudeDirectPassthrough` context flags, using native handlers.
  - **OpenAI-Compatible:** Most adapters (DeepSeek, Groq, Mistral, XAI, etc.) use the shared `openai_compatible.ConvertClaudeRequest()` for conversion to/from OpenAI format.
  - **Custom:** Gemini and others convert Claude → OpenAI → Gemini, then transform Gemini responses back to Claude format, using context keys for conversion tracking and response replacement.
- **Context Management:**
  - All conversion state is tracked via context keys in `common/ctxkey/key.go` (e.g., `ClaudeMessagesConversion`, `ConvertedResponse`, `OriginalClaudeRequest`).
  - The controller checks for converted responses and uses them if present, otherwise falls back to native handlers.
- **Data Mapping:**
  - Claude → OpenAI: System, messages, tools, tool_choice, temperature, top_p, stream, stop_sequences, and structured content are mapped to OpenAI equivalents.
  - OpenAI → Claude: Choices, tool_calls, finish_reason, and usage fields are mapped back, with ID and stop reason normalization.
  - Gemini: Custom mapping for candidates, finish reasons, and function calls to Claude content.
- **Feature Parity:**
  - Full support for function calling, streaming, structured/multimodal content, and tool use.
  - Billing, quota, and token counting are handled identically to ChatCompletion, including image token calculation and fallback strategies.
- **Testing:**
  - Comprehensive unit tests for all conversion paths, error handling, and edge cases. See `relay/adaptor/gemini/adaptor_test.go` and `relay/adaptor/openai_compatible/claude_messages_test.go`.
- **Performance:**
  - Minimal context storage, single-pass conversion, and lazy evaluation. Streaming is supported with buffer management.
- **Error Handling:**
  - All errors are wrapped with `github.com/Laisky/errors/v2`. Conversion errors are logged and surfaced with context; malformed content is handled gracefully with fallbacks.
- **Extensibility:**
  - New adapters can be added by implementing the interface and conversion logic. Specialized adapters (e.g., DeepL, Palm, Ollama) are excluded from Claude Messages support.
- **Critical Files:**
  - `relay/controller/claude_messages.go` (controller logic)
  - `relay/adaptor/interface.go` (interface definition)
  - `relay/adaptor/openai_compatible/claude_messages.go` (shared conversion)
  - `relay/adaptor/gemini/adaptor.go` (custom conversion)
  - `common/ctxkey/key.go` (context keys)
  - `docs/arch/api_convert.md` (architecture doc)

## Gemini Adapter: Function Schema Cleaning

- **Problem:** Gemini API rejects OpenAI-style function schemas with unsupported fields (`additionalProperties`, `description`, `strict`).
- **Solution:** Recursive cleaning removes `additionalProperties` everywhere, and `description`/`strict` only at the top level. Cleaned parameters are type-asserted before assignment.
- **Testing:** Unit tests verify recursive removal, top-level-only field removal, and schema compatibility.
- **Subtlety:** Only remove `description`/`strict` at the top; nested objects may require them.

## General Project Practices

- **Error Handling:** Always use `github.com/Laisky/errors/v2` for error wrapping; never return bare errors.
- **Context Keys:** All context keys must be pre-defined in `common/ctxkey/key.go`.
- **Package Management:** Use package managers (npm, pip, etc.), never edit package files by hand.
- **Testing:** All bug fixes/features must be covered by unit tests. No temporary scripts.
- **Time Handling:** Always use UTC for server, DB, and API time.
- **Golang ORM:** Use `gorm.io/gorm` for writes; prefer SQL for reads to minimize DB load.

## Handover Guidance

- **Claude Messages API:** Fully production-ready, with universal conversion and billing parity. See `docs/arch/api_billing.md` and `docs/arch/api_convert.md` for details.
- **Billing Architecture:** Four-layer pricing (channel overrides > adapter defaults > global > fallback).
- **Adaptor Pattern:** All new API formats should follow the Claude Messages pattern: interface method + universal conversion + context marking.
- **Critical Files:**
  - `relay/controller/claude_messages.go`
  - `relay/adaptor/interface.go`
  - `common/ctxkey/key.go`
  - `docs/arch/api_billing.md`
  - `docs/arch/api_convert.md`
- See `.github/memory.prompt` for additional development and architectural context.
