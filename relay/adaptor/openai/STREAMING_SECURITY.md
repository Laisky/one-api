# Responses streaming accounting boundary

Native and converted Responses streams parse every supported data payload before forwarding it. The maximum single data payload is 4 MiB, inclusive; the SSE `data:` prefix and line ending are not part of this limit. Larger or malformed oversized payloads terminate the stream before that payload is delivered. Ordinary supported wire payloads and event names retain their existing protocol representation.

Read errors, downstream cancellation, failed/short writes and body-close errors do not erase observed usage or deduplicated paid-tool actions. Closing the upstream body also interrupts reads stalled inside an oversized payload. A close error remains an error; it is not merely logged as successful completion.

Missing receipts and unsupported payloads retain explicit uncertainty metadata. Existing controller settlement keeps at least the request's reserved allowance for an estimated charge. An authoritative terminal receipt remains the measured result, even if final client delivery fails. These bounds do not claim to be an aggregate provider spending ceiling or exact tokenization of unknown work. General streaming quota enforcement is tracked separately.
