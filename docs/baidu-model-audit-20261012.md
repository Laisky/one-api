# Baidu native v1 catalogue — 2026-10-12

The official retirement notice states that `/rpc/2.0/ai_custom/v1/wenxinworkshop/`
was retired on 2026-08-31. All 16 existing native-v1 configurations now state that
endpoint status. No IDs, URL dispatch, credentials, quotas or prices are changed.
They remain historical compatibility records, not working public v1 promises.

Source: https://cloud.baidu.com/doc/qianfan/s/rmse4vxp8

Retiring v1 is not the same as retiring every underlying model: the notice keeps
Embedding-V1 and BGE models available through v2. Migration requires explicit
v2 channel/model configuration and its current prices, never an automatic URL
substitution or applying a v2 tariff to historical v1 records. Some ERNIE models
also have their own independent retirement dates already in their descriptions.

The new public-adaptor test pins every original ID, input price and completion
multiplier and checks the endpoint/model-retirement distinction. Qualification
requires old-code assertion failures and affected package race tests before the
provider commit. No live endpoint invocation or production migration is performed.
Revert this description-only provider commit to roll back.
