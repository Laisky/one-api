from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def rewrite_function(path: str, signature: str, transform) -> None:
    target = ROOT / path
    source = target.read_text()
    start = source.find(signature)
    if start < 0:
        raise RuntimeError(f"missing function: {signature}")
    next_function = source.find("\nfunc ", start + len(signature))
    if next_function < 0:
        next_function = len(source)
    block = source[start:next_function]
    updated = transform(block)
    target.write_text(source[:start] + updated + source[next_function:])


def normalize_security_block(block: str, return_statement: str) -> str:
    validation = re.compile(
        r'(?:\tif err := c\.validateTransportSecurity\(\); err != nil \{\n\t\treturn (?:nil, )?err\n\t\}\n)+\tclient := c\.httpClient\(\)'
    )
    replacement = (
        '\tif err := c.validateTransportSecurity(); err != nil {\n'
        f'\t\t{return_statement}\n'
        '\t}\n'
        '\tclient := c.httpClient()'
    )
    updated, count = validation.subn(replacement, block)
    if count == 0 and '\tclient := c.httpClient()' in block:
        updated = block.replace('\tclient := c.httpClient()', replacement, 1)
    return updated


rewrite_function(
    'relay/mcp/client.go',
    'func (c *StreamableHTTPClient) doRPCRaw(',
    lambda block: normalize_security_block(block, 'return nil, err'),
)
rewrite_function(
    'relay/mcp/client.go',
    'func (c *StreamableHTTPClient) sendNotification(',
    lambda block: normalize_security_block(block, 'return err'),
)
rewrite_function(
    'relay/mcp/client_latest.go',
    'func (c *StreamableHTTPClient) doModernRPC(',
    lambda block: normalize_security_block(block, 'return err'),
)

# Current protocol calls always send the normalized JSON object used for header derivation.
client_latest = ROOT / 'relay/mcp/client_latest.go'
source = client_latest.read_text().replace('"arguments": arguments,', '"arguments": argumentMap,')
client_latest.write_text(source)

# Tests and old compatibility fixtures use the exported compatibility constant.
for relative in ('controller/mcp_proxy_latest_test.go', 'controller/mcp_proxy_test.go'):
    target = ROOT / relative
    target.write_text(target.read_text().replace('mcpProtocolVersion', 'mcp.LegacyProtocolVersionFallback'))

# Remove any compiler-reported stale model import from the split modern call implementation.
call_file = ROOT / 'controller/mcp_call_latest.go'
source = call_file.read_text().replace('\n\t"github.com/Laisky/one-api/model"', '')
source = source.replace('startedAt := time.Now()', 'startedAt := time.Now().UTC()')
call_file.write_text(source)

# Explicit null is absence for optional tool/result fields; malformed non-null values remain strict.
types_file = ROOT / 'relay/mcp/types.go'
source = types_file.read_text()
if '\t"strings"\n' not in source:
    source = source.replace('import (\n', 'import (\n\t"strings"\n', 1)
if 'func omitNullJSONFields(' not in source:
    source += '''

// omitNullJSONFields removes explicit null values for optional wire fields before strict decoding.
//
// Parameters:
//   - data: The complete JSON object is decoded and re-encoded without selected null fields.
//   - fields: The optional field names treat an explicit null exactly like an absent field.
//
// Return values:
//   - []byte: The normalized JSON object is returned without mutating the caller's bytes.
//   - error: A wrapped JSON decoding or encoding error is returned for malformed input.
func omitNullJSONFields(data []byte, fields ...string) ([]byte, error) {
\tvar object map[string]json.RawMessage
\tif err := json.Unmarshal(data, &object); err != nil {
\t\treturn nil, errors.Wrap(err, "decode optional-null MCP object")
\t}
\tchanged := false
\tfor _, field := range fields {
\t\tif raw, exists := object[field]; exists && strings.TrimSpace(string(raw)) == "null" {
\t\t\tdelete(object, field)
\t\t\tchanged = true
\t\t}
\t}
\tif !changed {
\t\treturn data, nil
\t}
\tnormalized, err := json.Marshal(object)
\tif err != nil {
\t\treturn nil, errors.Wrap(err, "encode optional-null MCP object")
\t}
\treturn normalized, nil
}
'''


def insert_normalizer(source_text: str, signature: str, fields: tuple[str, ...]) -> str:
    start = source_text.find(signature)
    if start < 0:
        raise RuntimeError(f"missing unmarshal function: {signature}")
    next_function = source_text.find('\nfunc ', start + len(signature))
    if next_function < 0:
        next_function = len(source_text)
    block = source_text[start:next_function]
    if 'omitNullJSONFields(data' in block:
        return source_text
    brace = block.find('{')
    nil_guard = re.search(r'\n\tif [^\n]+ == nil \{.*?\n\t\}', block, flags=re.S)
    insertion = brace + 1
    if nil_guard is not None:
        insertion = nil_guard.end()
    arguments = ', '.join(f'"{field}"' for field in fields)
    code = (
        f'\n\tnormalizedData, err := omitNullJSONFields(data, {arguments})\n'
        '\tif err != nil {\n'
        '\t\treturn errors.Wrap(err, "normalize optional-null MCP fields")\n'
        '\t}\n'
        '\tdata = normalizedData'
    )
    block = block[:insertion] + code + block[insertion:]
    return source_text[:start] + block + source_text[next_function:]


source = insert_normalizer(
    source,
    'func (t *ToolDescriptor) UnmarshalJSON(data []byte) error',
    ('title', 'description', 'outputSchema', 'output_schema', 'annotations', 'icons', '_meta'),
)
source = insert_normalizer(
    source,
    'func (c *CallToolResult) UnmarshalJSON(data []byte) error',
    ('structuredContent', 'structured_content', 'isError', 'is_error', 'inputRequests', 'input_requests', 'requestState', 'request_state', '_meta'),
)
types_file.write_text(source)

# Avoid a static-analysis overflow warning while still allocating an efficient metadata map.
protocol_file = ROOT / 'relay/mcp/protocol.go'
protocol_file.write_text(protocol_file.read_text().replace('make(map[string]any, len(params)+1)', 'make(map[string]any, len(params))'))

# Correct the documented result metadata location.
manual = ROOT / 'docs/manuals/mcp_protocol_2026_07_28.md'
manual.write_text(manual.read_text().replace('params._meta["io.modelcontextprotocol/serverInfo"]', 'result._meta["io.modelcontextprotocol/serverInfo"]'))
