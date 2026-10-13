# Ollama curated-tag audit — 2026-10-12

Sources:
- https://docs.ollama.com/capabilities/thinking
- https://ollama.com/library/nomic-embed-text:latest
- https://ollama.com/library/deepseek-r1:latest
- https://ollama.com/library/qwen3:latest
- https://ollama.com/library/phi3:latest

Nomic's exact tag is F16 and embedding-only, not int4 text generation. Its tag
parameters explicitly set num_ctx=8192, so the shorter library summary is not
used to lower the existing context default. The existing symbolic quota price
is preserved: local models do not have a universal public upstream token tariff.

Qwen3 and DeepSeek-R1 do not acquire GPT-OSS's low/medium/high effort vocabulary
merely because they think. Their native controls are boolean; actual deployment
capabilities are discoverable with /api/show. The named ladder/default is removed
from those two metadata entries only. GPT-OSS retains its documented vocabulary.
This catalogue correction does not claim to implement the native think field in
the gateway conversion; that limitation is stated in the affected descriptions.
DeepSeek-R1's current tag explicitly advertises tools, so tools are not removed.
Phi3's current default is 128K, not the legacy 4K variant; its limit is retained.

Behavior coverage checks metadata, unchanged symbolic rates, neighboring tags,
unknown-model fallback and actual /api/embed request conversion. No cloud-plan
prices, account entitlements or deployment-specific tag remapping are invented.
Qualification requires the old-code assertion failure and full affected package
race tests before publishing. Revert this provider commit to restore metadata.
