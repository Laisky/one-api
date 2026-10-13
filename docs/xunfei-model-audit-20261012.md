# Spark WebSocket model audit — 2026-10-12

Use an explicit zero input/output tariff for Spark-Lite, which the official
HTTP model table identifies as free. A made-up nominal 0.3 CNY/M fee is not a
provider price. All other existing paid tariffs remain unchanged and are pinned
independently; a free public API does not promise unlimited concurrency or
separately negotiated service terms.

Sources:
- https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html
- https://www.xfyun.cn/doc/spark/Web.html
- https://www.xfyun.cn/doc/spark/X1http.html
- https://xinghuo.xfyun.cn/sparkapi

The WebSocket documentation directs FunctionCall users to HTTP and does not
publish response_format. Stop advertising those HTTP-only features. Advertise
only temperature and max_tokens, which the current converter forwards. Its
legacy N-to-top_k behavior is unchanged and is not advertised as typed TopK
support. The documented WebSocket Pro-128K output ceiling is 131072, distinct
from the HTTP 32768 ceiling. Existing legacy function serialization remains
unchanged for custom endpoints; metadata no longer promises upstream support.

The consumer pricing page is client-rendered in this research environment;
no new paid-price claims or cross-transport tariff equivalence are inferred.
Legacy Max IDs stay configurable with their retirement descriptions rather
than being removed or silently remapped. The specialist kjwx endpoint needs a
separately verified tariff and route before admission; it is not blindly added.

The regression exercises public discovery and pricing and exact transport
metadata. The isolated runner requires old-code assertion failures plus complete
affected package race suites before publishing. No live provider call or
production deployment is performed. Revert this provider commit to roll back;
operator overrides and database schema are unchanged.
