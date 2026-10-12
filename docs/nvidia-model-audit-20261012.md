# NVIDIA hosted GLM catalogue — 2026-10-12

Sources:
- https://build.nvidia.com/z-ai/glm-5-3
- https://build.nvidia.com/z-ai/glm-5-3/modelcard
- https://build.nvidia.com/z-ai/glm-5-3-flash
- https://build.nvidia.com/z-ai/glm-5-3-flash/modelcard

The build-page Python examples establish exact dotted IDs z-ai/glm-5.3 and
z-ai/glm-5.3-flash. Both explicitly advertise available Free Endpoints, so zero
represents the trial offer, not unknown monetary pricing. Partner/deployed
services require separately configured channel tariffs. No paid creator rates
are transplanted. The URL's hyphenated slug is not an API model ID.

Both cards specify 1048576 context, tools and low/high/max effort (default max).
A separate output ceiling is not published and remains unspecified. The full
model endpoint uses NVFP4, whereas Flash serves native FP8 and supports images
(up to eight upstream). These are serving-checkpoint properties, not assumptions
from the model creator's weights. NVIDIA recommends clear_thinking=true for chat;
that provider-specific template default is not silently injected here.

The prior converter discarded reasoning_effort for every model. It now preserves
that field only for these two verified native endpoints; legacy normalization
is unchanged for every other model. Behavior tests require exact payload/model/
effort preservation, existing URL construction, free versus unknown pricing,
modalities and independently allocated constructor metadata. Existing model IDs
and all prior prices remain intact. Embedding and safety-specific endpoints are
not blindly advertised as ordinary chat models.

Publish only after actual old-code assertion failures and complete affected
package race tests. No live account or endpoint entitlement test is performed.
Revert this provider commit to remove the new defaults and converter exception.
