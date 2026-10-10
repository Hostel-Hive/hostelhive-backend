import unittest
import importlib.util
import json
from pathlib import Path

spec = importlib.util.spec_from_file_location("coverage_report", Path(__file__).with_name("coverage-report.py"))
report = importlib.util.module_from_spec(spec)
spec.loader.exec_module(report)


class CoverageEvidenceTests(unittest.TestCase):
    def test_repeated_instrumented_blocks_are_not_double_counted(self):
        path = report.PREFIX + "internal/modules/staff/service/service.go:1.1,2.2"
        totals = report.summarize("mode: atomic\n" + path + " 2 0\n" + path + " 2 3\n")
        self.assertEqual(totals["internal/modules/staff"], [2, 2])
        self.assertEqual(totals["All application code"], [2, 2])

    def test_skip_and_missing_package_cannot_be_called_pass(self):
        names = ["accounts", "identity", "provisioning", "staff", "student", "postgres", "inventory"]
        events = [dict(Action="pass", Package=report.PREFIX + "tests/integration/" + n) for n in names]
        encode = lambda es: "\n".join(json.dumps(e) for e in es)
        report.verify_integration(encode(events))
        for bad in [events[:-1], events + [dict(Action="skip", Package=events[0]["Package"], Test="TestDB")],
                    events + [dict(Action="fail", Package=events[0]["Package"])]]:
            with self.assertRaises(ValueError):
                report.verify_integration(encode(bad))


if __name__ == "__main__":
    unittest.main()
