"""Unit and pinned-source regression checks for deterministic price conversion."""
import hashlib
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from bs4 import BeautifulSoup
from refresh import Evidence, table_rows, tariff, number, deepinfra, openrouter, novita
from domestic import alibaba, siliconflow, qianfan

class ConversionTests(unittest.TestCase):
    """These tests need no downloaded evidence or provider credentials."""
    def test_free_cache_and_output_are_not_unknown(self):
        self.assertEqual(tariff(1,0,0), {"ratio":1,"completion_ratio":0,"cached_input_ratio":-1})
        self.assertNotIn("cached_input_ratio",tariff(1,2))
        for value in ("NaN","Infinity","-Infinity"):
            with self.assertRaises(ValueError): number(value)
        with self.assertRaises(ValueError): tariff(-1,2)
        with self.assertRaises(ValueError): tariff(0,2)

    def test_table_rowspans_keep_identity_aligned(self):
        soup=BeautifulSoup('<table><tr><th rowspan="2">model</th><td colspan="2">price</td></tr><tr><td>1</td><td>2</td></tr></table>',"html.parser")
        self.assertEqual(table_rows(soup.table),[["model","price","price"],["model","1","2"]])

    def test_source_drift_fails_closed(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);(root/"source").write_bytes(b"new")
            e=Evidence(root);e.lock={"x":{"path":"source","sha256":hashlib.sha256(b"old").hexdigest(),"url":"https://example.com"}}
            with self.assertRaisesRegex(ValueError,"checksum mismatch"): e.read("x")
            self.assertEqual(e.used,{})

class EvidenceTests(unittest.TestCase):
    """Run all dated upstream parser contracts with MODEL_CATALOG_EVIDENCE set."""
    @classmethod
    def setUpClass(cls):
        path=os.getenv("MODEL_CATALOG_EVIDENCE")
        if not path: raise unittest.SkipTest("set MODEL_CATALOG_EVIDENCE to checksum-verified archive directory")
        cls.e=Evidence(Path(path))

    def test_deepinfra_discount_and_cents(self):
        c=deepinfra(self.e)
        self.assertEqual(len(c.models),123)
        p=c.models["Qwen/Qwen3.8-27B"]
        self.assertAlmostEqual(p["ratio"],.15)
        self.assertAlmostEqual(p["ratio"]*p["completion_ratio"],1.875)
        self.assertAlmostEqual(p["cached_input_ratio"],.0375)
        p=c.models["zai-org/GLM-5.3"]
        self.assertAlmostEqual(p["ratio"],.5625)
        self.assertAlmostEqual(p["ratio"]*p["completion_ratio"],2.5)
        self.assertAlmostEqual(c.models["anthropic/claude-sonnet-5"]["ratio"],3)
        self.assertNotIn("input_modalities", c.models["nvidia/llama-nemotron-embed-vl-1b-v2"])
        self.assertIn("thinkingmachines/Inkling", c.excluded)

    def test_openrouter_batch_and_tiers(self):
        c=openrouter(self.e)
        self.assertEqual(len(c.models),351)
        self.assertFalse(any(":batch" in k for k in c.models))
        p=c.models["openai/gpt-6-astra"]
        self.assertAlmostEqual(p["ratio"],10)
        self.assertEqual(p["tiers"][0]["input_token_threshold"],272000)
        self.assertAlmostEqual(p["tiers"][0]["ratio"],20)
        self.assertIn("openrouter/auto",c.excluded)

    def test_novita_integer_decimal_units(self):
        c=novita(self.e)
        self.assertEqual(len(c.models),108)
        self.assertTrue(all(p["ratio"]>=0 for p in c.models.values()))
        self.assertTrue(any("overlap" in reason for reason in c.excluded.values()))

    def test_alibaba_native_currency_and_strict_tiers(self):
        c=alibaba(self.e)
        self.assertEqual(len(c.models),143)
        self.assertAlmostEqual(c.models["qwen3.8-max"]["ratio"],12)
        self.assertAlmostEqual(c.models["qwen3.7-plus"]["ratio"],1.6)
        self.assertAlmostEqual(c.models["qwen3.7-plus-2026-05-26"]["ratio"],2)
        self.assertEqual(c.models["qwen3.5-27b"]["tiers"][0]["input_token_threshold"],128001)
        self.assertIn("qwen-plus", c.excluded)
        self.assertEqual(c.models["deepseek-v4.1-flash"]["time_windows"][0]["ranges"],[{"start":"08:00","end":"22:00"}])
        self.assertEqual(c.models["deepseek-v4.1-flash"]["ratio"],1)
        self.assertNotIn("context_length",c.models["qwen3.8-max"])

    def test_siliconflow_half_open_dayparts(self):
        c=siliconflow(self.e)
        self.assertEqual(len(c.models),36)
        self.assertEqual(c.models["Qwen/Qwen3.5-27B"]["tiers"][0]["input_token_threshold"],128000)
        self.assertEqual(c.models["deepseek-ai/DeepSeek-V4-Flash"]["time_windows"][0]["ranges"],[{"start":"02:00","end":"08:00"}])
        self.assertEqual(c.models["tencent/Hunyuan-MT-7B"]["ratio"],0)

    def test_qianfan_promo_has_end_and_all_token_classes(self):
        c=qianfan(self.e)
        self.assertEqual(len(c.models),13)
        p=c.models["DeepSeek-V4.1-Flash"]
        self.assertEqual(p["ratio"],1)
        w=p["time_windows"][0]
        self.assertEqual(w["date_from"],"2026-09-24")
        self.assertEqual(w["date_to"],"2026-10-08")
        self.assertAlmostEqual(w["overlay"]["ratio"],1.2)
        self.assertAlmostEqual(w["overlay"]["cached_input_ratio"],.024)
        self.assertAlmostEqual(w["overlay"]["ratio"]*w["overlay"]["completion_ratio"],4.8)
        self.assertEqual(c.models["ERNIE-5.1"]["tiers"][0]["input_token_threshold"],32001)

if __name__ == "__main__": unittest.main()
