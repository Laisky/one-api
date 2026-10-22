"""Check evidence classification and differential failure detection without Docker."""
import unittest

import qualify_media_runtime as media


class MediaEvidenceTests(unittest.TestCase):
    """MediaEvidenceTests guard against false compatibility and billing claims."""

    def observation(self, duration="2.000001", stderr="", code=0):
        """observation builds one synthetic scalar probe receipt."""
        return {"duration_stdout": duration + "\n", "duration_stderr": stderr,
                "duration_code": code}

    def test_fractional_seconds_round_once_as_decimal_quota(self):
        """test_fractional_seconds checks an independent ceiling at a quota boundary."""
        case = media.manifest_case("fixture", self.observation(), "digest")
        self.assertTrue(case["success"])
        self.assertEqual(case["quota"], 1001)

    def test_combined_probe_diagnostics_cannot_become_success(self):
        """test_combined_probe_diagnostics preserves the actual helper's parse contract."""
        case = media.manifest_case("fixture", self.observation(stderr="invalid packet\n"), "digest")
        self.assertFalse(case["success"])
        self.assertEqual(case["quota"], 0)

    def test_unknown_nonfinite_and_failed_probes_are_not_billable(self):
        """test_unknown_nonfinite checks negative controls instead of fabricated zero duration."""
        for duration in ("N/A", "NaN", "Infinity", "-1", "0", ""):
            with self.subTest(duration=duration):
                self.assertFalse(media.manifest_case(
                    "fixture", self.observation(duration), "digest")["success"])
        self.assertFalse(media.manifest_case(
            "fixture", self.observation(code=1), "digest")["success"])

    def test_decoder_and_probe_regressions_are_both_detected(self):
        """test_decoder_and_probe checks that a probe-only pass cannot hide lost decoding."""
        baseline = {"fixture": {"duration_stdout": "2", "decode_code": 0, "decode_sha256": "a"}}
        candidate = {"fixture": {"duration_stdout": "2", "decode_code": 1, "decode_sha256": "b"}}
        self.assertEqual(set(media.compare_observations(baseline, candidate)["fixture"]),
                         {"decode_code", "decode_sha256"})
        self.assertEqual(media.compare_observations(baseline, baseline), {})

    def test_address_normalization_preserves_real_errors(self):
        """test_address_normalization removes only process-dependent hexadecimal addresses."""
        self.assertEqual(media.normalize_error("[mpeg @ 0x123abc] invalid data\n"),
                         "[mpeg @ 0xADDRESS] invalid data\n")


if __name__ == "__main__":
    unittest.main(verbosity=2)
