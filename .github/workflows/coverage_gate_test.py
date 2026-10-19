#!/usr/bin/env python3

import sys
import tempfile
import unittest
from pathlib import Path


sys.path.insert(0, str(Path(__file__).resolve().parent))
import coverage_gate  # noqa: E402


class CoverageGateTest(unittest.TestCase):
    def test_per_package_floor_rejects_aggregate_masking(self) -> None:
        # The aggregate is exactly 90%, but the cold package must still fail.
        actual = {"example/hot": 100.0, "example/cold": 80.0}
        floors = {"example/hot": 90.0, "example/cold": 90.0}

        self.assertEqual(
            coverage_gate.coverage_failures(actual, floors),
            ["example/cold: coverage 80.0% is below 90.0%"],
        )

    def test_discovery_rejects_unfloored_and_stale_packages(self) -> None:
        self.assertEqual(
            coverage_gate.target_failures(
                ["example/current", "example/new"],
                {"example/current": 80.0, "example/removed": 80.0},
            ),
            [
                "package example/new has no coverage floor",
                "coverage floor targets missing package example/removed",
            ],
        )

    def test_missing_total_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "no total statement coverage"):
            coverage_gate.parse_total("example/file.go:1: function 100.0%")

    def test_zero_floor_is_rejected_as_vacuous(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            floor_path = Path(temp_dir) / "coverage-floor.json"
            floor_path.write_text(
                '{"schema_version": 1, "packages": {"example/package": 0.0}}',
                encoding="utf-8",
            )
            with self.assertRaisesRegex(ValueError, "greater than 0"):
                coverage_gate.load_floors(floor_path)


if __name__ == "__main__":
    unittest.main()
