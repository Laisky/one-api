#!/usr/bin/env python3
"""Regenerate dated, embedded catalog patches from checksum-pinned public evidence.

No credentials, provider requests, channel settings, or database writes are used.
Pass an evidence root containing the first/second/final archive directories. The
checked-in JSON is the deployable artifact; regeneration never runs at startup.
"""
from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import math
import re
from decimal import Decimal
from pathlib import Path
from typing import Any

from bs4 import BeautifulSoup

HERE = Path(__file__).resolve().parent
DATE = "2026-09-24"
NUMERIC_PRICE_KEYS = ("ratio", "cached_input_ratio", "cache_write_5m_ratio", "cache_write_1h_ratio")


def number(value: Any) -> float:
    """Parse a finite number without guessing a provider's units or placeholders."""
    result = float(Decimal(str(value)))
    if not math.isfinite(result):
        raise ValueError(f"non-finite price: {value!r}")
    return result


def tariff(input_price: float, output_price: float, cached: float | None = None,
           write: float | None = None, write1h: float | None = None) -> dict[str, Any]:
    """Express native-currency/M prices in snapshot fields, including free cache."""
    if input_price < 0 or output_price < 0 or (input_price == 0 and output_price > 0):
        raise ValueError("unrepresentable or unknown input/output price")
    result: dict[str, Any] = {"ratio": input_price,
                             "completion_ratio": output_price / input_price if input_price else 1}
    for key, value in zip(NUMERIC_PRICE_KEYS[1:], (cached, write, write1h)):
        if value is None:
            continue  # Absence is unknown, not a free-cache claim.
        if value < 0:
            raise ValueError("negative provider cache price")
        result[key] = -1 if value == 0 and input_price > 0 else value
    return result


def clock_window(name: str, timezone: str, ranges: list[tuple[str, str]],
                 overlay: dict[str, Any], **bounds: Any) -> dict[str, Any]:
    """Build one pricing window with explicit timezone and date bounds."""
    return {"name": name, "timezone": timezone,
            "ranges": [{"start": a, "end": b} for a, b in ranges], "overlay": overlay, **bounds}


def table_rows(table: Any) -> list[list[str]]:
    """Expand HTML row/column spans before assigning prices to model identities."""
    rows: list[list[str]] = []
    spans: dict[int, tuple[int, str]] = {}
    for tr in table.find_all("tr"):
        row: dict[int, str] = {}
        for col, (remaining, text) in list(spans.items()):
            row[col] = text
            if remaining == 1:
                del spans[col]
            else:
                spans[col] = (remaining - 1, text)
        col = 0
        for cell in tr.find_all(["td", "th"], recursive=False):
            while col in row:
                col += 1
            text = cell.get_text(" ", strip=True)
            width, height = int(cell.get("colspan", 1)), int(cell.get("rowspan", 1))
            for offset in range(width):
                row[col + offset] = text
                if height > 1:
                    spans[col + offset] = (height - 1, text)
            col += width
        rows.append([row.get(i, "") for i in range(max(row, default=-1) + 1)])
    return rows


class Evidence:
    """Read only source bytes matching the reviewed, committed checksum lock."""

    def __init__(self, root: Path):
        self.root = root
        self.lock = json.loads((HERE / "sources.lock.json").read_text())
        self.used: dict[str, dict[str, str]] = {}

    def read(self, name: str) -> bytes:
        record = self.lock[name]
        data = (self.root / record["path"]).read_bytes()
        if hashlib.sha256(data).hexdigest() != record["sha256"]:
            raise ValueError(f"source checksum mismatch: {name}; re-review before updating the lock")
        self.used[name] = {"url": record["url"], "sha256": record["sha256"]}
        return gzip.decompress(data) if data.startswith(b"\x1f\x8b") else data

    def json(self, name: str) -> Any:
        return json.loads(self.read(name))

    def soup(self, name: str) -> BeautifulSoup:
        return BeautifulSoup(self.read(name), "html.parser")


class Catalog:
    """Accumulate explicit model patches and a reviewable exclusion ledger."""

    def __init__(self, provider: str, currency: str, source_names: list[str]):
        self.provider, self.currency, self.source_names = provider, currency, source_names
        self.models: dict[str, dict[str, Any]] = {}
        self.excluded: dict[str, str] = {}
        self.notes: dict[str, Any] = {}

    def add(self, model: str, patch: dict[str, Any]) -> None:
        if not model or model != model.strip() or model in self.models:
            raise ValueError(f"invalid/duplicate {self.provider} model: {model!r}")
        # A dated quote replaces stale future promotions/tiers. Callers attach
        # verified schedules explicitly; ordinary account overrides are untouched.
        patch.setdefault("tiers", None)
        patch.setdefault("time_windows", None)
        patch.setdefault("description", f"{model} on {self.provider}; official catalog snapshot {DATE}. "
                         "Provider access and deployment configuration remain administrator-controlled.")
        self.models[model] = patch

    def skip(self, model: str, reason: str) -> None:
        self.excluded[model] = reason


def openrouter(e: Evidence) -> Catalog:
    """Normalize ordinary OpenRouter token rows, not batch or native media fees."""
    c = Catalog("openrouter", "USD", ["openrouter-models"])
    for item in e.json("openrouter-models")["data"]:
        model, p = item["id"], item["pricing"]
        if ":batch" in model:
            c.skip(model, "Batch-only tariff is not synchronous Chat/Responses pricing")
            continue
        if item["architecture"]["output_modalities"] != ["text"]:
            c.skip(model, "Native output-modality tariff requires its existing media billing contract")
            continue
        if any(number(p.get(k, 0)) != 0 for k in
               ("image", "image_output", "audio", "audio_output", "input_audio_cache", "request")):
            c.skip(model, "Additional media/per-request unit cannot be silently flattened into token prices")
            continue
        if number(p["prompt"]) < 0 or number(p["completion"]) < 0:
            c.skip(model, "Dynamic router reports an unknown negative price sentinel")
            continue
        overrides = p.get("overrides", [])
        allowed = {"min_prompt_tokens", "prompt", "completion", "input_cache_read", "input_cache_write", "input_cache_write_1h"}
        if any(set(t) - allowed for t in overrides):
            c.skip(model, "Non-token/time override requires a dedicated conversion, retained existing metadata")
            continue

        def prices(row: dict[str, Any]) -> dict[str, Any]:
            args = [number(row[k]) * 1_000_000 if k in row else None
                    for k in ("prompt", "completion", "input_cache_read", "input_cache_write", "input_cache_write_1h")]
            return tariff(*args)

        patch = prices(p)
        params = item.get("supported_parameters", [])
        features = []
        for param, feature in (("tools", "tools"), ("response_format", "json_mode"),
                               ("structured_outputs", "structured_outputs"), ("reasoning", "reasoning"),
                               ("reasoning_effort", "reasoning"), ("logprobs", "logprobs")):
            if param in params and feature not in features:
                features.append(feature)
        patch.update(context_length=item["context_length"], input_modalities=item["architecture"]["input_modalities"],
                     output_modalities=["text"], supported_features=features,
                     supported_sampling_parameters=params, hugging_face_id=item.get("hugging_face_id") or "")
        maximum = item.get("top_provider", {}).get("max_completion_tokens")
        if maximum is not None:
            patch["max_output_tokens"] = maximum
        reasoning = item.get("reasoning") or {}
        if reasoning.get("supported_efforts"):
            patch["supported_reasoning_efforts"] = reasoning["supported_efforts"]
            patch["default_reasoning_effort"] = reasoning.get("default_effort") or ""
        tiers = []
        for row in overrides:
            merged = {**p, **row}
            tier = prices(merged)
            tier["input_token_threshold"] = int(row["min_prompt_tokens"])
            tiers.append(tier)
        if tiers:
            patch["tiers"] = sorted(tiers, key=lambda x: x["input_token_threshold"])
        c.add(model, patch)
    return c


def deepinfra(e: Evidence) -> Catalog:
    """Convert cents/token and cache multipliers, applying promotions exactly once."""
    c = Catalog("deepinfra", "USD", ["deepinfra-models"])
    for item in e.json("deepinfra-models"):
        model, p, tags = item["model_name"], item["pricing"], item.get("tags", [])
        if item.get("deprecated") or item.get("private") or item.get("expected"):
            c.skip(model, "Not a currently published live serverless model")
            continue
        kind = item["type"]
        if not ((kind == "text-generation" and p["type"] == "tokens" and "openai" in tags)
                or (kind == "embeddings" and p["type"] == "input_tokens" and "openai" in tags)
                or (kind == "reranker" and model.startswith("Qwen/Qwen3-Reranker-") and p["type"] == "input_tokens")):
            c.skip(model, "Native or non-token task; preserve existing task-specific billing/transport")
            continue
        if "input-audio" in tags or p.get("table"):
            c.skip(model, "Audio/conditional tariff requires preserving existing modality-specific metadata")
            continue
        discount = number(p.get("discount") or 0)
        if not 0 <= discount < 1:
            raise ValueError(f"invalid discount on {model}")
        if p.get("discount_ends_at"):
            c.skip(model, "Dated promotion needs exact timestamp window; not converted to an indefinite tariff")
            continue
        scale = 1 - discount
        input_price = number(p["cents_per_input_token"]) * 10000 * scale
        output_price = number(p.get("cents_per_output_token", 0)) * 10000 * scale
        read_rate = p.get("rate_per_input_token_cached")
        cached = input_price * number(read_rate) if read_rate is not None else None
        # The API reports cache multipliers to eight decimal places. Preserve
        # that precision; do not infer a different price from a rounded UI label.
        patch = tariff(input_price, output_price, cached)
        patch["context_length"] = item["max_tokens"]
        inputs = ["text"]
        if "multimodal" in tags:
            inputs.append("image")
        if "input-video" in tags:
            inputs.append("video")
        if kind != "embeddings" or "multimodal" in tags:
            patch["input_modalities"] = inputs
        if kind == "embeddings":
            patch["embedding"] = {"text_token_ratio": input_price}
            # The openai-tagged embedding API accounts input tokens; a distinct
            # image tariff is not invented from the model's vision capability.
        elif kind == "text-generation":
            patch["output_modalities"] = ["text"]
            patch["supported_features"] = [feature for tag, feature in
                 (("tools", "tools"), ("json", "json_mode"), ("structured-output", "structured_outputs"), ("reasoning", "reasoning")) if tag in tags]
        if discount:
            patch["description"] = (f"{model} on DeepInfra. Observed {discount * 100:g}% promotion on {DATE}; "
                "discount applied once to token/cache prices. No end date published; this is a dated quote, not a permanent-price guarantee.")
            c.notes[model] = {"discount": discount, "discount_ends_at": None,
                             "list_input_usd_per_million": number(p["cents_per_input_token"]) * 10000,
                             "effective_input_usd_per_million": input_price}
        c.add(model, patch)
    return c


def novita(e: Evidence) -> Catalog:
    """Use Novita's decimal USD/M values; never confuse integer billing units."""
    c = Catalog("novita", "USD", ["novita-models"])
    for item in e.json("novita-models")["data"]:
        model = item["id"]
        if item.get("status") != 1 or item.get("model_type") != "chat" or ("endpoints" in item and "chat/completions" not in item["endpoints"]):
            c.skip(model, "Not an active Chat Completions model")
            continue
        if item.get("is_tiered_billing"):
            c.skip(model, "Published adjacent min/max bounds overlap; retain existing tier contract until equality semantics are established")
            continue
        if not item.get("pricing"):
            c.skip(model, "No current structured price quote published; no free placeholder introduced")
            continue
        if item["output_modalities"] != ["text"] or set(item["pricing"]) - {"prompt", "completion", "input_cache_read", "input_cache_write"}:
            c.skip(model, "Non-text or modality-specific tariff")
            continue
        values = []
        for key in ("prompt", "completion", "input_cache_read", "input_cache_write"):
            row = item["pricing"].get(key)
            if row is None:
                values.append(None)
                continue
            value = number(row["price_per_m_decimal"])
            # Integer pricing is in 1/10000 USD per million, independently
            # checked against the public decimal field rather than trusted.
            if abs(value - number(row["price_per_m"]) / 10000) > 1e-8:
                raise ValueError(f"Novita integer/decimal price mismatch: {model}/{key}")
            values.append(value)
        patch = tariff(*values)
        patch.update(context_length=item["context_size"], max_output_tokens=item["max_output_tokens"],
                     input_modalities=item["input_modalities"], output_modalities=item["output_modalities"],
                     supported_features=[feature for tag, feature in
                     (("function-calling", "tools"), ("structured-outputs", "structured_outputs"), ("reasoning", "reasoning"))
                     if tag in item.get("features", [])])
        c.add(model, patch)
    return c


def together(e: Evidence) -> Catalog:
    """Parse the complete serverless chat table separately from image/audio units."""
    c = Catalog("togetherai", "USD", ["together-models"])
    tables = e.soup("together-models").find_all("table")
    rows = table_rows(tables[0])
    if "API model string" not in rows[0] or "Cached input pricing (per 1M tokens)" not in rows[0]:
        raise ValueError("Together chat table layout changed")
    for row in rows[1:]:
        model = row[2]
        val = lambda s: 0.0 if s.lower() == "free" else number(s.removeprefix("$"))
        patch = tariff(val(row[4]), val(row[6]), None if row[5] == "-" else val(row[5]))
        if row[3] != "-":
            patch["context_length"] = int(row[3].replace(",", ""))
        features = []
        if row[8] == "Yes":
            features.append("tools")
        if row[9] == "Yes":
            features += ["json_mode", "structured_outputs"]
        # Blank table cells are unspecified, not evidence a previous capability
        # disappeared. Only set an affirmative capability where documented.
        if features:
            patch["add_supported_features"] = features
        c.add(model, patch)
    c.notes["native_media"] = "Image MP/step and audio tables reviewed separately; existing billing blocks retained."
    return c


def baichuan(e: Evidence) -> Catalog:
    """Parse all independently quoted Baichuan token rows, including dayparts."""
    c = Catalog("baichuan", "CNY", ["baichuan-pricing"])
    soup = e.soup("baichuan-pricing")
    groups: dict[str, list[tuple[list[str], dict[str, Any]]]] = {}
    for row in table_rows(soup.find_all("table")[0])[1:]:
        model_match = re.search(r"Baichuan[\w.-]+", row[0])
        if not model_match:
            continue
        model = model_match[0]
        values = [number(x) * 1000 for x in re.findall(r"([\d.]+)元/千tokens", row[3])]
        if len(values) == 1:
            values *= 2
        if len(values) != 2:
            raise ValueError(f"unrecognized Baichuan token tariff {row}")
        groups.setdefault(model, []).append((row, tariff(*values)))
    for model, rows in groups.items():
        patch = dict(rows[-1][1])
        maximum = re.fullmatch(r"(\d+)k", rows[0][0][1], re.I)
        if maximum:
            patch["context_length"] = int(maximum[1]) * 1000
        if len(rows) > 1:
            windows = []
            for row, price in rows:
                match = re.fullmatch(r"(\d{1,2}:\d\d)\s*~\s*(\d{1,2}:\d\d)", row[2])
                if not match:
                    raise ValueError("invalid Baichuan daypart")
                windows.append(clock_window("baichuan-daypart", "Asia/Shanghai",
                    [(match[1].zfill(5), "00:00" if match[2] == "24:00" else match[2].zfill(5))], price))
            patch["time_windows"] = windows
        c.add(model, patch)
    row = next(r for r in table_rows(soup.find_all("table")[1]) if "Baichuan-Text-Embedding" in r[0])
    price = number(re.search(r"([\d.]+)元/千tokens", row[2])[1]) * 1000
    c.add("Baichuan-Text-Embedding", {**tariff(price, 0), "embedding": {"text_token_ratio": price}})
    c.notes["medical_search"] = "Medical models can trigger separately charged search. Token quote does not include those tool invocations."
    return c


def write_catalog(c: Catalog, e: Evidence, root: Path) -> None:
    """Write a sorted portable snapshot, loader, and explicit review metadata."""
    if not c.models:
        raise ValueError(f"empty generated catalog: {c.provider}")
    directory = root / "relay" / "adaptor" / c.provider
    if not directory.is_dir():
        raise ValueError(f"unknown provider directory: {c.provider}")
    raw = {"version": 1, "currency": c.currency,
           "sources": [e.used[name] for name in c.source_names], "models": c.models}
    (directory / "catalog_20260924.json").write_text(json.dumps(raw, ensure_ascii=False, indent=2, sort_keys=True) + "\n")
    has_model_list = any(re.search(r"(?m)^var ModelList\b", p.read_text()) for p in directory.glob("*.go")
                         if p.name != "zz_catalog_20260924.go")
    import_adaptor = '"github.com/Laisky/one-api/relay/adaptor"\n' if has_model_list else ""
    rebuild = "\n ModelList = adaptor.GetModelListFromPricing(ModelRatios)" if has_model_list else ""
    (directory / "zz_catalog_20260924.go").write_text(f'''// Code generated by scripts/model_catalog/refresh.py; DO NOT EDIT.
package {c.provider}

import (
 _ "embed"
 {import_adaptor}"github.com/Laisky/one-api/relay/adaptor/internal/catalogsnapshot"
)

// auditedCatalog20260924 contains only public defaults, not tenant configuration.
//go:embed catalog_20260924.json
var auditedCatalog20260924 []byte

// init applies the reviewed snapshot after legacy family initializers. It keeps
// omitted historic IDs and does not make network calls or enforce account tiers.
func init() {{
 ModelRatios = catalogsnapshot.Apply(ModelRatios, auditedCatalog20260924){rebuild}
}}
''')
    report = {"provider": c.provider, "currency": c.currency, "date": DATE,
              "included": sorted(c.models), "excluded": c.excluded, "notes": c.notes,
              "sources": raw["sources"]}
    target = root / "docs" / "research" / "model_catalog_20260924"
    target.mkdir(parents=True, exist_ok=True)
    (target / (c.provider + ".json")).write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n")


def main() -> None:
    """Verify evidence and regenerate all converters; fail instead of partial guessing."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-root", type=Path, required=True)
    parser.add_argument("--root", type=Path, default=HERE.parents[1])
    args = parser.parse_args()
    evidence = Evidence(args.evidence_root)
    catalogs = [convert(evidence) for convert in CONVERTERS]
    for catalog in catalogs:
        write_catalog(catalog, evidence, args.root)
        print(f"{catalog.provider}: {len(catalog.models)} included, {len(catalog.excluded)} explicitly excluded")


from domestic import alibaba, alibailian, siliconflow, qianfan

CONVERTERS = [openrouter, deepinfra, novita, together, baichuan, alibaba, alibailian, siliconflow, qianfan]

if __name__ == "__main__":
    main()
