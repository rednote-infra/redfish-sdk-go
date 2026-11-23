"""Reject failed, skipped, or empty Go offline test runs."""

import json
import sys
from dataclasses import dataclass
from pathlib import Path


@dataclass
class ResultSummary:
    """Count completed packages and individual test outcomes."""

    passed: int = 0
    failed: int = 0
    skipped: int = 0
    completed: int = 0

    def __str__(self):
        return (
            f"Offline test results: {self.passed} passed, {self.failed} failed, "
            f"{self.skipped} skipped; {self.completed} packages completed"
        )


def validate_results(input_file):
    """Allow packages without tests while rejecting skipped test cases."""
    summary = ResultSummary()
    for line in input_file:
        event = json.loads(line)
        action = event.get("Action")
        if action == "pass":
            if event.get("Test"):
                summary.passed += 1
            else:
                summary.completed += 1
        elif action == "fail":
            summary.failed += 1
        elif action == "skip":
            if event.get("Test"):
                summary.skipped += 1
            else:
                # Go reports packages with no test files as package-level skips.
                summary.completed += 1
    if not summary.passed or not summary.completed or summary.failed or summary.skipped:
        raise ValueError(
            f"{summary}; offline tests must pass without skipped test cases"
        )
    return summary


def main():
    if len(sys.argv) != 2:
        print("usage: testgate.py test-results.jsonl", file=sys.stderr)
        return 2
    try:
        with Path(sys.argv[1]).open() as input_file:
            summary = validate_results(input_file)
    except (OSError, ValueError) as error:
        print(error, file=sys.stderr)
        return 1
    print(summary)
    return 0


if __name__ == "__main__":
    sys.exit(main())
