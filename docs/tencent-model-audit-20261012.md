# Tencent legacy-adaptor lifecycle audit — 2026-10-12

Official sources:
- https://cloud.tencent.com/announce/detail/2301 (June22 legacy-model retirement)
- https://cloud.tencent.com/announce/detail/2391 (August31 Hy3-preview migration)
- https://cloud.tencent.com/announce/detail/2384 (separate Token Plan routing)
- https://cloud.tencent.com/document/product/1823/130055 (current TokenHub prices)

Existing RETIRING labels now correctly state the June22 retirement as past, not
future. The obsolete hy3-preview replacement advice no longer calls it the new
flagship. TokenHub/Agent Platform/CloudBase retired that preview at 00:00 Beijing
on August31; their pay-as-you-go calls automatically move to Hy3 and its price.
The Token Plan July24 routing announcement is a different subscription product.
All historical IDs and numerical tariffs are retained in this legacy adaptor;
Hy3's current flat CNY 1/0.25/4 input/cache/output defaults are independently
confirmed. Historical preview prices are explicitly not current redirected rates.

This adaptor signs Tencent Cloud API v2023-09-01 requests with X-TC headers and
uses its legacy typed request. It is not the TokenHub OpenAI-compatible API.
Therefore Hy4 preview, new translation models and TokenHub-hosted GLM/DeepSeek/
Kimi models are not silently admitted on this unrelated protocol. TokenHub's
Guangzhou and Singapore tariffs can also differ. Use an explicitly configured
compatible channel for those deployments, with the platform's actual prices.
No legacy auth, URL, channel entitlement or operator override is changed here.

Public regressions pin eight retiring legacy prices, the preview's historical
tiers and the current Hy3 tariff while checking exact platform/date language.
The isolated runner requires old-code assertion failures and full affected
package race tests before publishing. No live deployment validation is claimed.
Revert this provider commit to restore descriptions; no data migration required.
