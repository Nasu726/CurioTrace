import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCENARIO_ROOT = ROOT / "tests" / "golden" / "scenarios"

ALLOWED_ACTIONS = {
    "start",
    "grant_permission",
    "deny_permission",
    "navigate",
    "scroll_to",
    "wait",
    "select",
    "copy",
    "switch_tab",
    "pause",
    "resume",
    "stop",
    "disconnect_helper",
    "restart_helper",
}

ALLOWED_CAPTURE_MODES = {
    "dom",
    "redacted_visual",
    "fingerprint_only",
    "metadata_only",
    "blocked",
    "failed",
}

ALLOWED_STATES = {"IDLE", "RECORDING", "PAUSED", "FINISHED", "INTERRUPTED"}


class GoldenScenarioManifestTests(unittest.TestCase):
    def scenario_paths(self):
        return sorted(SCENARIO_ROOT.glob("*/scenario.json"))

    def test_every_scenario_manifest_has_basic_valid_shape(self):
        paths = self.scenario_paths()
        self.assertTrue(paths, "no golden scenarios found")
        seen_ids = set()

        for path in paths:
            with self.subTest(path=path):
                data = json.loads(path.read_text(encoding="utf-8"))
                self.assertEqual(data["id"], path.parent.name)
                self.assertRegex(data["id"], r"^[a-z0-9][a-z0-9-]*$")
                self.assertNotIn(data["id"], seen_ids)
                seen_ids.add(data["id"])
                self.assertGreaterEqual(data["version"], 1)
                self.assertTrue(data["description"].strip())
                self.assertTrue(data["steps"])

                for step in data["steps"]:
                    self.assertIn(step["action"], ALLOWED_ACTIONS)
                    if "seconds" in step:
                        self.assertGreaterEqual(step["seconds"], 0)

                expectations = data["expectations"]
                self.assertIn("must_observe", expectations)
                self.assertIn("must_not_observe", expectations)
                self.assertIn("privacy_forbidden_canaries", expectations)

                if "expected_final_state" in expectations:
                    self.assertIn(expectations["expected_final_state"], ALLOWED_STATES)
                for mode in expectations.get("expected_capture_modes", []):
                    self.assertIn(mode, ALLOWED_CAPTURE_MODES)
                for rejection in expectations.get("expected_rejections", []):
                    self.assertTrue(rejection["reason"].strip())
                    if "minimum_count" in rejection:
                        self.assertGreaterEqual(rejection["minimum_count"], 1)

    def test_privacy_canaries_are_unique_across_scenarios(self):
        owners = {}
        for path in self.scenario_paths():
            data = json.loads(path.read_text(encoding="utf-8"))
            for canary in data["expectations"].get("privacy_forbidden_canaries", []):
                self.assertTrue(canary.strip())
                self.assertNotIn(
                    canary,
                    owners,
                    f"privacy canary reused by {owners.get(canary)} and {data['id']}",
                )
                owners[canary] = data["id"]

    def test_permission_denial_is_explicitly_non_recording(self):
        path = SCENARIO_ROOT / "permission-denial" / "scenario.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        actions = [step["action"] for step in data["steps"]]
        self.assertIn("deny_permission", actions)
        self.assertTrue(data["expectations"]["recording_must_not_start"])
        self.assertEqual(data["expectations"]["expected_final_state"], "IDLE")
        self.assertEqual(data["expectations"]["must_observe"], [])

    def test_helper_disconnect_scenario_marks_post_disconnect_content_forbidden(self):
        path = SCENARIO_ROOT / "helper-disconnect" / "scenario.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        actions = [step["action"] for step in data["steps"]]
        self.assertIn("disconnect_helper", actions)
        self.assertIn("restart_helper", actions)
        self.assertEqual(data["expectations"]["expected_final_state"], "INTERRUPTED")
        forbidden = set(data["expectations"]["privacy_forbidden_canaries"])
        self.assertIn("GOLDEN_MUST_NOT_CAPTURE_AFTER_HELPER_DISCONNECT", forbidden)

    def test_synthetic_pdf_fixture_has_consistent_startxref(self):
        pdf_path = SCENARIO_ROOT / "fingerprint-only-pdf" / "fixture" / "fingerprint-only.pdf"
        raw = pdf_path.read_bytes()
        self.assertTrue(raw.startswith(b"%PDF-1.4\n"))
        self.assertIn(b"GOLDEN_PDF_CANARY_9F3A", raw)

        match = re.search(rb"startxref\n(\d+)\n%%EOF", raw)
        self.assertIsNotNone(match)
        xref_offset = int(match.group(1))
        self.assertEqual(raw[xref_offset : xref_offset + 5], b"xref\n")


if __name__ == "__main__":
    unittest.main()
