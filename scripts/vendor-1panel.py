#!/usr/bin/env python3
"""Verify the pinned 1Panel sources, or reconstruct them from a checkout plus reviewed patches.

  python3 scripts/vendor-1panel.py --verify
  python3 scripts/vendor-1panel.py --upstream /path/to/1Panel

Local transport/bootstrap adapters without upstream equivalents are ordinary
application source files. They are intentionally not synthesized by this script.
"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--verify", action="store_true")
parser.add_argument("--upstream", type=Path)
args = parser.parse_args()
manifest = json.loads((root / "web/panel/manifest.json").read_text())
for entry in manifest["files"]:
    target = root / entry["local"]
    if args.upstream:
        original = subprocess.check_output(
            [
                "git",
                "-C",
                str(args.upstream),
                "show",
                manifest["commit"] + ":" + entry["source"],
            ]
        )
        if hashlib.sha256(original).hexdigest() != entry["originalSha256"]:
            raise SystemExit("Upstream hash mismatch: " + entry["source"])
        # Apply in a temporary directory so a failed patch cannot damage vendored source.
        with tempfile.TemporaryDirectory(prefix="resty-1panel-") as tmp:
            prepared = Path(tmp) / "source"
            prepared.write_bytes(original)
            if entry.get("patch"):
                subprocess.run(
                    ["patch", "--batch", str(prepared), str(root / entry["patch"])],
                    check=True,
                    stdout=subprocess.DEVNULL,
                )
            adapted = prepared.read_bytes()
        if hashlib.sha256(adapted).hexdigest() != entry["adaptedSha256"]:
            raise SystemExit("Patch hash mismatch: " + entry["local"])
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(adapted)
    if hashlib.sha256(target.read_bytes()).hexdigest() != entry["adaptedSha256"]:
        raise SystemExit(
            "Vendored source differs from reviewed manifest: " + entry["local"]
        )
print(
    f"Verified {len(manifest['files'])} pinned 1Panel sources at {manifest['commit']}"
)
