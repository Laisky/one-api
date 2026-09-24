"""Provider-specific CNY table parsers; native API contracts are kept separate."""
from __future__ import annotations

import re
from typing import Any


def alibaba(e: Any, provider: str = "ali") -> Any:
    """Read Beijing token tariffs, keeping overseas and mode-dependent prices out."""
    from refresh import Catalog, tariff, table_rows, number, clock_window
    c = Catalog(provider, "CNY", ["ali-pricing"])
    soup = e.soup("ali-pricing")
    grouped: dict[str, list[tuple[int, dict[str, Any]]]] = {}
    for table in soup.find_all("table"):
        rows = table_rows(table)
        if not rows:
            continue
        headings = [h.get_text(" ", strip=True) for h in table.find_all_previous(["h2", "h3", "h4", "h5", "h6"])]
        region = next((h for h in headings if any(x in h for x in
                      ("北京", "新加坡", "弗吉尼亚", "法兰克福", "香港", "上海", "东京"))), "")
        if "北京" not in region:
            continue
        headers = rows[0]
        in_cols = [i for i, h in enumerate(headers) if h.startswith("输入单价")]
        out_cols = [i for i, h in enumerate(headers) if h.startswith("输出单价")]
        if len(in_cols) != 1 or not out_cols or "每百万 Token" not in headers[in_cols[0]]:
            continue
        for row in rows[1:]:
            match = re.match(r"[A-Za-z][A-Za-z0-9./_-]+", row[0])
            if not match:
                continue
            model = match[0]
            if model in ("Model", "ID"):
                continue
            if any(x in model.lower() for x in ("omni", "audio", "tts", "asr", "realtime", "deep-research", "qwen-mt-", "doc-", "ocr", "gui-", "farui")):
                c.skip(model, "Specialized/native modality or agent API is not a plain text tariff")
                continue
            if provider == "ali" and any(x in model.lower() for x in ("-vl-", "qvq", "qwen3.5", "qwen3.6", "qwen3.7", "qwen3.8", "kimi")):
                # Price metadata is still valid for basic text on the native
                # generation route; do not claim vision forwarding from this table.
                c.notes[model] = "Native DashScope entry exposes text pricing; multimodal payload support is not inferred."
            price_cells = [row[in_cols[0]], *[row[i] for i in out_cols]]
            if any("已下线" in x or "免费额度用完" in x for x in price_cells):
                c.skip(model, "Retired or quota-only trial row; retain existing compatibility metadata")
                continue
            def amounts(text: str) -> list[float]:
                if text == "限时免费":
                    return [0.0]
                values = [number(x) for x in re.findall(r"([\d.]+)\s*元(?:\s|$|（)", text)]
                promo = re.search(r"限时\s*([\d.]+)\s*折", text)
                return [x * number(promo[1]) / 10 for x in values] if promo else values
            values = [amounts(x) for x in price_cells]
            if not values[0] or any(not v for v in values[1:]):
                c.skip(model, "Tariff unit not representable as standard token billing")
                continue
            if any(v != values[1] for v in values[2:]):
                c.skip(model, "Thinking and non-thinking outputs have different prices; requires request-mode pricing selector")
                continue
            bounds = next((cell for cell in row if "<Token" in cell or "Token≤" in cell), "")
            lower = re.search(r"([\d.]+)([KM]?)<Token", bounds)
            # Alibaba's lower bound is strict (>), whereas one-api uses >=.
            threshold = 0 if not lower or number(lower[1]) == 0 else int(number(lower[1]) * {"":1,"K":1000,"M":1000000}[lower[2]]) + 1
            ins, outs = values[0], values[1]
            if len(ins) != len(outs) or len(ins) not in (1, 2):
                raise ValueError(f"ambiguous Alibaba prices: {model}")
            patch = tariff(ins[0], outs[0])
            if len(ins) == 2:
                if not all("忙时" in x and "闲时" in x for x in price_cells):
                    raise ValueError("unrecognized Alibaba multi-price row")
                patch = tariff(ins[1], outs[1])
                patch["time_windows"] = [clock_window("beijing-peak", "Asia/Shanghai", [("08:00", "22:00")], tariff(ins[0], outs[0]))]
            if "限时" in " ".join(price_cells):
                c.notes[model] = "Observed limited-time quote with no published end date; not an evergreen price guarantee."
            grouped.setdefault(model, []).append((threshold, patch))
    for model, rows in grouped.items():
        if model in c.excluded:
            continue
        by_threshold: dict[int, dict[str, Any]] = {}
        for threshold, price in rows:
            if threshold in by_threshold and price != by_threshold[threshold]:
                c.skip(model, "Conflicting same-tier rows require manual reconciliation")
                break
            by_threshold[threshold] = price
        if model in c.excluded:
            continue
        if 0 not in by_threshold:
            c.skip(model, "No base tier published")
            continue
        patch = dict(by_threshold[0])
        if len(by_threshold) > 1:
            patch["tiers"] = [{**p, "input_token_threshold": threshold} for threshold, p in sorted(by_threshold.items()) if threshold]
        c.add(model, patch)
    c.notes["scope"] = "Beijing CNY list; explicit alias rows only. Token tariff buckets are not model context limits. Unquoted cache rates preserved, never guessed from another provider."
    return c


def alibailian(e: Any) -> Any:
    """Apply the same Beijing quote to the generic OpenAI-compatible channel."""
    return alibaba(e, "alibailian")


def siliconflow(e: Any) -> Any:
    """Normalize visible CNY token rows with half-open tiers and provider dayparts."""
    from refresh import Catalog, tariff, number, clock_window
    c = Catalog("siliconflow", "CNY", ["siliconflow-cn"])
    soup = e.soup("siliconflow-cn")
    for anchor in soup.select("a[title]"):
        model, row = anchor["title"], anchor.parent.parent
        category = row.get("id", "")
        if not category.startswith("pricing-row-text"):
            c.skip(model, "Native image/audio/video unit retained under the existing modality-specific transport and billing contract")
            continue
        cells = row.find_all("div", recursive=False)[1:]
        if len(cells) not in (3, 6):
            raise ValueError(f"SiliconFlow row layout changed: {model}")
        groups = []
        for offset in range(0, len(cells), 3):
            texts = [x.get_text(" ", strip=True) for x in cells[offset:offset+3]]
            def value(text: str) -> float | None:
                if "免费" in text: return 0.0
                match = re.search(r"¥\s*([\d.]+)", text)
                return number(match[1]) if match else None
            ins, outs, cache = map(value, texts)
            if ins is None:
                raise ValueError(f"SiliconFlow input price absent: {model}")
            # Embeddings and reranking publish only an input price.
            patch = tariff(ins, 0 if outs is None else outs, cache)
            if "/bge-" in model and "reranker" not in model:
                patch["embedding"] = {"text_token_ratio": ins}
            groups.append((texts[0], patch))
        patch = dict(groups[0][1])
        if len(groups) > 1:
            if "费用发生时段" in groups[0][0]:
                windows = []
                for text, prices in groups:
                    periods = re.findall(r"(\d+)点[～~](\d+)点", text)
                    if not periods: raise ValueError("missing SiliconFlow daypart")
                    ranges = [(f"{int(a):02}:00", "00:00" if b == "24" else f"{int(b):02}:00") for a,b in periods]
                    windows.append(clock_window("siliconflow-daypart", "Asia/Shanghai", ranges, prices))
                patch["time_windows"] = windows
            else:
                tiers = []
                for text, prices in groups[1:]:
                    match = re.search(r"输入\s*\[\s*(\d+)\s*([kK]?)\s*,", text)
                    if not match: raise ValueError("missing SiliconFlow half-open threshold")
                    threshold = int(match[1]) * (1000 if match[2] else 1)
                    tiers.append({**prices, "input_token_threshold":threshold})
                patch["tiers"] = tiers
        c.add(model, patch)
    c.notes["scope"] = "siliconflow.cn CNY pricing, not siliconflow.com USD. Explicit half-open boundaries and Asia/Shanghai dayparts; no account quota filter."
    return c


def qianfan(e: Any) -> Any:
    """Use current ERNIE rows plus the explicit dated holiday tariff matrix."""
    from refresh import Catalog, tariff, number, table_rows, clock_window
    c = Catalog("baiduv2", "CNY", ["baidu-pricing"])
    soup = e.soup("baidu-pricing")
    tables = soup.find_all("table")
    # The upstream HTML contains malformed/obsolete rowspan cells after the
    # ERNIE section. Parse only the clean current section before DeepSeek;
    # the holiday matrix independently quotes all three relevant model prices.
    table = tables[1]
    clean = type(soup)("<table></table>", "html.parser")
    for tr in table.find_all("tr"):
        if "DeepSeek" in tr.get_text(): break
        clean.table.append(tr.__copy__())
    groups: dict[str, dict[int, dict[str, float]]] = {}
    for row in table_rows(clean.table)[1:]:
        if len(row) != 7 or row[2] != "推理服务" or row[6] != "元/千tokens": continue
        if not row[1].startswith("ERNIE-"): continue
        tier = 32001 if re.search(r"32k<输入", row[3]) else 0
        key = "cache" if "缓存" in row[3] else "output" if row[3].startswith("输出") else "input"
        for model in row[1].split():
            groups.setdefault(model, {}).setdefault(tier, {})[key] = number(row[4])*1000
    for model, tiers in groups.items():
        if any("input" not in p or "output" not in p for p in tiers.values()):
            raise ValueError(f"incomplete ERNIE tariff: {model}")
        quote = lambda p: tariff(p["input"], p["output"], p.get("cache"))
        patch = quote(tiers[0])
        if len(tiers)>1: patch["tiers"] = [{**quote(p),"input_token_threshold":n} for n,p in sorted(tiers.items()) if n]
        c.add(model,patch)
    if "2026年9月24日" not in soup.get_text() or "10月7日" not in soup.get_text():
        raise ValueError("Qianfan holiday bounds changed")
    promo: dict[str, dict[str, list[float]]] = {}
    for row in table_rows(tables[0])[2:]:
        model = "GLM-5.3" if row[0] == "GLM 5.3" else row[0]
        promo.setdefault(model,{})[row[1]] = [number(x)*1000 for x in row[2:6]]
    for model, columns in promo.items():
        def quote(index: int) -> dict[str, Any]:
            return tariff(columns["输入"][index],columns["输出"][index],columns["缓存命中"][index])
        base = quote(1)
        base["time_windows"] = [
            clock_window("qianfan-holiday-peak", "Asia/Shanghai", [("08:00","22:00")], quote(2), date_from="2026-09-24",date_to="2026-10-08"),
            clock_window("qianfan-holiday-offpeak", "Asia/Shanghai", [("22:00","08:00")], quote(3),date_from="2026-09-24",date_to="2026-10-08"),
            clock_window("qianfan-peak", "Asia/Shanghai", [("08:00","22:00")], quote(0)),
        ]
        c.add(model,base)
    c.notes["scope"] = "Canonical v2 API IDs from current rows. Holiday matrix includes all token classes; expires at 2026-10-08 00:00 Asia/Shanghai. Malformed historical rows not silently reinterpreted. Legacy v1 is a separate API."
    return c
