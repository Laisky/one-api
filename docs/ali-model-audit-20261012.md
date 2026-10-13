# Alibaba ali model audit — 2026-10-12

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

Native Ali also corrects MT Flash and Lite output multipliers to exactly 1.95
and 1.60 CNY/M, rather than the rounded 1.9502 and 1.6002 amounts. Their limits are
16384/8192. No IDs are added. The old OCR latest alias is labelled historical;
its unverified alias tariff is not silently rewritten. The current native Ali
request converter uses text-generation and string message content, so a new OCR
image ID would not implement a working image route. The updated OCR record is
retained configuration metadata, not a transport capability claim. MT requests
require translation_options, not currently represented by this gateway's common
request DTO. No new MT or OCR request functionality is asserted.

## Verification and rollout

Regression assertions independently pin current rates, limits, discovery,
unknown-field semantics and unaffected neighbor tariffs. New compatible OCR
models also have image-request preservation and allocation tests. Actual
old-code assertion failures and complete affected package race-test receipts
are attached to the PR. No paid provider request, data migration, deployment
change or administrator-override change is included. Revert this provider
commit to roll back; no new runtime catalogue loader is introduced.
