"""Offline regression tests for development scripts."""

import io
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import generate_models
import testgate

ROOT = Path(__file__).resolve().parents[1]


class ModelGenerationTests(unittest.TestCase):
    def test_generated_models_match_checked_in_contracts(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "models.go"
            generate_models.generate(ROOT / "scripts/modelgen/schema.json", output)
            self.assertEqual(
                output.read_bytes(), (ROOT / "models/models_gen.go").read_bytes()
            )


class TestGateTests(unittest.TestCase):
    def test_package_without_tests_is_allowed(self):
        events = [
            {"Action": "pass", "Test": "TestClient", "Package": "client"},
            {"Action": "pass", "Package": "client"},
            {"Action": "skip", "Package": "wire"},
        ]
        summary = testgate.validate_results(
            io.StringIO("\n".join(map(json.dumps, events)))
        )
        self.assertEqual(summary.passed, 1)
        self.assertEqual(summary.skipped, 0)
        self.assertEqual(summary.completed, 2)

    def test_cli_rejects_failed_skipped_empty_and_malformed_results(self):
        cases = {
            "skipped": '{"Action":"pass","Test":"TestClient"}\n'
            '{"Action":"skip","Test":"TestSkipped"}\n{"Action":"pass","Package":"client"}',
            "failed": '{"Action":"fail","Test":"TestClient","Package":"client"}',
            "empty": "",
            "malformed": "invalid JSON",
        }
        for name, events in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "results.jsonl"
                path.write_text(events)
                result = subprocess.run(
                    [sys.executable, str(ROOT / "scripts/testgate.py"), str(path)],
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertTrue(result.stderr)


if __name__ == "__main__":
    unittest.main()
