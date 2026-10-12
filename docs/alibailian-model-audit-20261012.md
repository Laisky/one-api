# Alibaba alibailian model audit — 2026-10-12

## Sources and scope

- https://help.aliyun.com/zh/model-studio/model-pricing
- https://help.aliyun.com/zh/model-studio/qwenvl-ocr
- https://help.aliyun.com/zh/model-studio/qwen3-5-ocr
- https://help.aliyun.com/zh/model-studio/qwen-vl-ocr
- https://help.aliyun.com/zh/model-studio/qwen-mt-plus
- https://help.aliyun.com/zh/model-studio/qwen-mt-flash
- https://help.aliyun.com/zh/model-studio/qwen-mt-lite
- https://help.aliyun.com/en/model-studio/machine-translation

These defaults use the Beijing standard CNY tariff, not the separate Singapore
or international USD quote. Comparison of the unambiguous domestic simple-price
rows found 86 existing input/output pairs already matching; this is not an audit
of every token tier, media unit, negotiated offer or regional deployment.

Qwen VL OCR changes from 5.15/5.15 to 0.30/0.50 CNY/M and from the stale 34096/4096
context/output limits to 38192/8192. It does not support tools, structured output
or caching. Qwen MT Plus becomes exactly 1.8/5.4 CNY/M and 16384/8192 context/output.
Unsupported inherited cache discounts and quantization metadata are not retained.
Unknown sampling controls are not advertised as accepted provider parameters.
Account limits do not become gateway request-denial rules or model MaxTokens.

## Transport and compatibility

Add qwen3.5-ocr (CNY 0.50/2.00, context 65536/output 16384) and the documented
qwen-vl-ocr-2025-11-20 snapshot (CNY 0.30/0.50, context 38192/output 8192).
Tests preserve ordinary text/image_url content and the exact model ID through
the existing OpenAI-compatible converter. Input text provides instructions for
the image; no PDF file, native rotation/task option, tool, reasoning or caching
capability is claimed. Remove the unsupported 0.372 MT Plus and 1.03 OCR cache
prices; zero means no advertised special cache rate, not free caching.

The existing MT Plus entry remains configurable, but new MT Flash/Lite IDs are
not admitted as working translations while the common request lacks required
translation_options. Native Ali and this compatible channel intentionally have
different model admissions; they are not blindly synchronized by source URL.

## Verification and rollout

Regression assertions independently pin current rates, limits, discovery,
unknown-field semantics and unaffected neighbor tariffs. New compatible OCR
models also have image-request preservation and allocation tests. Actual
old-code assertion failures and complete affected package race-test receipts
are attached to the PR. No paid provider request, data migration, deployment
change or administrator-override change is included. Revert this provider
commit to roll back; no new runtime catalogue loader is introduced.
