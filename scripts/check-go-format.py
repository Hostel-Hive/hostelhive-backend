"""Check gofmt without treating Windows CRLF checkout as a formatting error."""
from pathlib import Path
import subprocess

bad = []
for directory in ("cmd", "internal", "tests"):
    for path in Path(directory).rglob("*.go"):
        source = path.read_bytes().replace(b"\r\n", b"\n")
        formatted = subprocess.run(["gofmt"], input=source, capture_output=True, check=True).stdout
        if formatted != source:
            bad.append(str(path))
if bad:
    print("Go files requiring gofmt:\n" + "\n".join(bad))
    raise SystemExit(1)
print("PASS: Go formatting (line endings normalized for comparison only).")
