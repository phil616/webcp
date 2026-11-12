#!/usr/bin/env python3
"""Check source formatting, documentation links, version consistency and origins."""

import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parent.parent


def main():
    errors = []
    version = (ROOT / "VERSION").read_text().strip()
    package = json.loads((ROOT / "web/package.json").read_text())
    lock = json.loads((ROOT / "web/package-lock.json").read_text())
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
        errors.append("Invalid VERSION")
    if any(
        value != version
        for value in (
            package["version"],
            lock["version"],
            lock["packages"][""]["version"],
        )
    ):
        errors.append("VERSION and npm package versions differ")
    paths = [
        str(path)
        for base in ("cmd", "internal")
        for path in (ROOT / base).rglob("*.go")
        if "onepanel" not in path.parts
    ]
    unformatted = subprocess.check_output(
        ["gofmt", "-l", *paths], cwd=ROOT, text=True
    ).strip()
    if unformatted:
        errors.append("Unformatted Go source:\n" + unformatted)
    for path in [
        ROOT / "README.md",
        ROOT / "CHANGELOG.md",
        ROOT / "CONTRIBUTING.md",
        *(ROOT / "docs").glob("*.md"),
    ]:
        text = re.sub(r"```.*?```", "", path.read_text(), flags=re.S)
        for target in re.findall(r'!?\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)', text):
            parts = urlsplit(target.strip("<>"))
            if parts.scheme or target.startswith("#"):
                continue
            file = (path.parent / unquote(parts.path)).resolve()
            if not file.exists():
                errors.append(f"{path.relative_to(ROOT)}: missing link target {target}")
    subprocess.run(
        [sys.executable, "scripts/vendor-1panel.py", "--verify"], cwd=ROOT, check=True
    )
    if errors:
        raise SystemExit("\n".join(errors))
    print(
        "Repository formatting, local documentation links, versions and source origins verified"
    )


if __name__ == "__main__":
    main()
