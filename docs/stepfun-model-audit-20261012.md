# StepFun audit — 2026-10-12

Primary source: https://platform.stepfun.com/docs/zh/guides/pricing/details

The rate page announces a cache-input change for `step-5-preview` at Beijing
2026-10-14 00:30, equivalent to 2026-10-13 16:30 UTC. The new rate is CNY 0.70
per million cached-input tokens. Accounts that called Step 5 through the PAYG
API before the cutoff retain CNY 0.35. Ordinary input/output stays CNY 7/20.

A global TimeWindow would incorrectly double grandfathered accounts' cache
charges. The catalogue therefore preserves the current/grandfathered 0.35 and
states the exact eligibility rule and required 0.70 override for other accounts.
This is not automatic account classification. The gateway has no authoritative
upstream account-use history with which to select the two tariffs.

The public rate card also confirms the existing Step 3.7 Flash 1.35/0.27/8.1,
Step 3.5 Flash 0.7/0.14/2.1 and Step 1o Turbo Vision 2.5/0.5/8 CNY rates.
Audio entries have distinct token, character, and duration units. New Audio 3
streaming, generation, ASR and voice-cloning interfaces require endpoint/usage
qualification; no text-token-priced surrogate or unlimited-free claim is added.

The linked detailed announcement timed out, but the price page itself contains
the exact cutoff and grandfathering rule. Tests pin that notice and prove that
existing pricing remains unchanged across the cutoff. No schema, account
restriction, deployment or credential change is introduced. Roll back by
reverting this provider commit. Executed validation is recorded in PR #560.
