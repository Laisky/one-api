# DeepSeek model and pricing audit — 2026-10-12

## Verified defect and correction

The four existing API identifiers, model versions, limits, vision distinctions,
reasoning controls and base token prices remain aligned with the native provider.
The public rate card explicitly excludes Chinese public holidays from weekday
peak pricing. The previous code excluded only Saturday and Sunday, so a weekday
holiday could charge double the advertised input, cache and output amounts.

Sources inspected on 2026-10-12:

- https://api-docs.deepseek.com/quick_start/pricing/
- State Council notice of November 4, 2025, official government republication:
  https://www.bjdch.gov.cn/zwgk/hygq/202601/t20260123_4459144.html
- Independent official republication of the same national notice:
  https://zwfw.gansu.gov.cn/chengguan/zwzx/zxtg/art/2025/art_2529408225b9478487a7bd156ff8c37b.html

Add the seven published 2026 national holiday ranges before each ordinary peak
schedule. Each exception carries all three off-peak prices, and is clipped to
its existing pricing era. The September 10 Flash switch still occurs at 04:00
UTC. Its two explicit bridge windows are unchanged. Pro stays Pro; the canceled
September 14 migration is not reintroduced. Other hosts are not changed.

## Date and unit semantics

The exception end dates are exclusive. Peak intervals occur at 09:00–12:00 and
14:00–18:00 Beijing time, wholly within the same calendar date as UTC. Full-day
UTC exception ranges therefore give exactly the same charge as the published
Beijing holiday dates: hours outside these intervals were already off-peak.
Tests compare the same instant in UTC and UTC+8, including boundary nanoseconds.
Make-up working Saturdays and Sundays remain off-peak, because the provider
explicitly excludes every weekend independently of public holiday scheduling.

This is the published 2026 schedule only, not a recurring lunar-date rule or a
prediction of 2027 holidays. Future national schedules require a fresh source
review. The change retains the existing preceding-era prices; it does not invent
historical launch dates or claim a complete historical price archive. It does
not recalculate already settled customer transactions.

## Regression and rollout

Public tests reproduce holiday overcharges on the old code and verify both
production price resolvers, all four names, unaffected ordinary weekdays,
weekends, exact boundaries and immutable configuration. Constructor tests check
clipped ranges and independent allocations. Existing Flash transition and Pro
continuity tests remain unchanged. Exact negative-control and package race-test
receipts are recorded in the PR qualification artifacts.

No base rate, request conversion, channel override, database schema, credential,
or deployment change is introduced. Revert this provider commit to roll back
holiday handling; no data migration is needed. No paid provider call was made.
