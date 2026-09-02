#!/usr/bin/env python3
"""Splice the Gherkin tally lines out of the captured verbose log as raw bytes
(never retyped) and print them plus a byte-containment check against the log
file, for task 406's report.

Usage: python3 splice_tally.py <path-to-log>
"""
import sys


def main() -> None:
    path = sys.argv[1] if len(sys.argv) > 1 else "sweep-verbose.log"
    data = open(path, "rb").read()
    lines = data.split(b"\n")

    scenarios_line = None
    steps_line = None
    scenarios_no = None
    steps_no = None
    for i, line in enumerate(lines):
        if scenarios_line is None and b" scenarios (" in line:
            scenarios_line = line
            scenarios_no = i + 1  # 1-indexed line number
        if steps_line is None and b" steps (" in line:
            steps_line = line
            steps_no = i + 1

    assert scenarios_line is not None, "no scenarios tally line found"
    assert steps_line is not None, "no steps tally line found"

    for label, no, line in (
        ("scenarios", scenarios_no, scenarios_line),
        ("steps", steps_no, steps_line),
    ):
        contained = line in data
        print(f"{label} tally, line {no}: {line!r}")
        print(f"  byte-containment check (line in open(path,'rb').read()): {contained}")


if __name__ == "__main__":
    main()
