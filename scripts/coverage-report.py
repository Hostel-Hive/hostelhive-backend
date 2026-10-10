"""Standard-library coverage summary; fail CI if database tests were skipped."""
import argparse
import json
from pathlib import Path

PREFIX = "github.com/Hostel-Hive/hostelhive-backend/"
GROUPS = ("internal/modules/identity/", "internal/modules/staff/",
          "internal/modules/student/", "internal/shared/middleware/",
          "internal/platform/firebase/")


def summarize(profile):
    blocks = {}
    lines = profile.splitlines()
    if not lines or lines[0] not in ("mode: atomic", "mode: count", "mode: set"):
        raise ValueError("Invalid coverage profile header")
    for line in lines[1:]:
        location, statements, count = line.split()
        key = (location, int(statements))
        blocks[key] = max(blocks.get(key, 0), int(count))
    totals = {"All application code": [0, 0]}
    totals.update({g.rstrip("/"): [0, 0] for g in GROUPS})
    for (location, size), count in blocks.items():
        targets = ["All application code"]
        targets.extend(g.rstrip("/") for g in GROUPS if location.startswith(PREFIX + g))
        for target in targets:
            totals[target][1] += size
            totals[target][0] += size if count else 0
    if not totals["All application code"][1]:
        raise ValueError("Coverage profile contains no application statements")
    return totals


def verify_integration(events):
    passed = set()
    skipped = []
    failed = []
    for line in events.splitlines():
        event = json.loads(line)
        package = event.get("Package", "")
        if "/tests/integration/" not in package:
            continue
        if event.get("Action") == "skip":
            skipped.append(event.get("Test", package))
        if event.get("Action") == "fail":
            failed.append(event.get("Test", package))
        if event.get("Action") == "pass" and not event.get("Test"):
            passed.add(package.rsplit("/", 1)[-1])
    required = {"accounts", "identity", "provisioning", "staff", "student", "postgres", "inventory"}
    if skipped or failed or required - passed:
        raise ValueError("Integration evidence incomplete: skipped/failed tests or missing package passes")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", required=True)
    parser.add_argument("--events", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--require-integration", action="store_true")
    args = parser.parse_args()
    events = Path(args.events).read_text(encoding="utf-8-sig")
    if args.require_integration:
        verify_integration(events)
    totals = summarize(Path(args.profile).read_text(encoding="utf-8-sig"))
    rows = ["# Backend statement coverage", "", "| Scope | Covered / total | Coverage |",
            "|---|---:|---:|"]
    for group, (covered, total) in totals.items():
        percent = f"{100 * covered / total:.1f}%" if total else "No executable statements"
        rows.append(f"| {group} | {covered} / {total} | {percent} |")
    rows.extend(["", "Database tests: all required packages passed with no skips." if args.require_integration
                 else "Database execution has not been asserted for this report.", "",
                 "Statement coverage is not proof of complete requirements, branch coverage or live provider behavior.",
                 "Firebase and storage tests use fakes; staging verification remains separate."])
    Path(args.output).write_text("\n".join(rows) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
